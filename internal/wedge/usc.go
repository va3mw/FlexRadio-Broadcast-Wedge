package wedge

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/logx"
	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/vita"
)

// link is the connection to one RSC. Its fields are guarded by Engine.mu.
type link struct {
	addr    string
	state   string
	err     string
	since   time.Time
	packets uint64
	radios  map[string]*seen
}

// runLink keeps a connection to one RSC, redialling when it drops.
func (e *Engine) runLink(ctx context.Context, l *link, bc *broadcaster) {
	log := e.log.Src("usc")
	lastErr := ""
	for {
		err := e.session(ctx, l, bc)
		if ctx.Err() != nil {
			return
		}
		msg := "connection closed"
		if err != nil {
			msg = err.Error()
		}
		if msg != lastErr {
			log.Warn("RSC %s: %s (retrying every %s)", l.addr, msg, retryDelay)
			lastErr = msg
		}
		e.mu.Lock()
		l.state, l.err = "down", msg
		l.radios = map[string]*seen{}
		e.mu.Unlock()
		e.onChange()
		if !sleep(ctx, retryDelay) {
			return
		}
	}
}

func (e *Engine) session(ctx context.Context, l *link, bc *broadcaster) error {
	log := e.log.Src("usc")
	d := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 15 * time.Second}
	conn, err := d.DialContext(ctx, "tcp4", l.addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	br := bufio.NewReader(conn)
	hello := make([]byte, len(magic))
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.ReadFull(br, hello); err != nil || string(hello) != magic {
		return errors.New("not a Broadcast Wedge RSC")
	}
	e.mu.Lock()
	l.state, l.err, l.since = "connected", "", time.Now()
	e.mu.Unlock()
	log.Info("connected to RSC %s", l.addr)
	e.onChange()

	var hdr [2]byte
	buf := make([]byte, vita.MaxPacket)
	for {
		conn.SetReadDeadline(time.Now().Add(linkIdle))
		if _, err := io.ReadFull(br, hdr[:]); err != nil {
			return err
		}
		n := int(binary.BigEndian.Uint16(hdr[:]))
		if n == 0 { // keepalive
			continue
		}
		if n > len(buf) {
			return fmt.Errorf("oversized frame (%d bytes)", n)
		}
		if _, err := io.ReadFull(br, buf[:n]); err != nil {
			return err
		}
		r := vita.ParseDiscovery(buf[:n])
		if r == nil {
			continue
		}
		e.mu.Lock()
		s, known := l.radios[r.Serial]
		if !known {
			s = &seen{}
			l.radios[r.Serial] = s
		}
		changed := !known || s.radio != *r
		s.radio, s.last = *r, time.Now()
		l.packets++
		e.mu.Unlock()
		if !known {
			log.Info("relaying %s \"%s\" serial %s at %s from RSC %s", r.Model, r.Nickname, r.Serial, r.IP, l.addr)
		}
		if !e.cfg.Get().IsMuted(r.Serial) {
			bc.send(buf[:n])
		}
		if changed {
			e.onChange()
		}
	}
}

// Iface is a local network a USC can rebroadcast on.
type Iface struct {
	IP   string `json:"ip"`
	Name string `json:"name"`
}

// Interfaces lists the broadcast-capable IPv4 interfaces that are up.
func Interfaces() []Iface {
	out := []Iface{}
	ifs, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagBroadcast == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLinkLocalUnicast() {
				out = append(out, Iface{IP: n.IP.String(), Name: i.Name})
			}
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].IP < out[b].IP })
	return out
}

// broadcaster re-emits discovery packets as local UDP broadcasts. It keeps
// one socket per interface, because Windows sends 255.255.255.255 out of only
// one interface unless the socket is bound to a specific address.
type broadcaster struct {
	log   *logx.Source
	iface string // only this local IP; "" = every interface
	dst   *net.UDPAddr
	fixed bool // dst is a test address: send from one unbound socket

	mu      sync.Mutex
	socks   map[string]*net.UDPConn
	checked time.Time
	warned  bool
	closed  bool
}

func newBroadcaster(log *logx.Source, iface string, to *net.UDPAddr) *broadcaster {
	b := &broadcaster{log: log, iface: iface, socks: map[string]*net.UDPConn{},
		dst: &net.UDPAddr{IP: net.IPv4bcast, Port: 4992}}
	if to != nil {
		b.dst, b.fixed = to, true
	}
	return b
}

func (b *broadcaster) send(p []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	if time.Since(b.checked) > 10*time.Second {
		b.refresh()
	}
	for ip, c := range b.socks {
		if _, err := c.WriteToUDP(p, b.dst); err != nil {
			b.log.Warn("broadcast from %s failed: %v", ip, err)
			c.Close()
			delete(b.socks, ip)
		}
	}
}

// refresh opens sockets for interfaces that appeared and closes those that
// went away (Wi-Fi roaming, cable pulled, VPN adapter up or down).
func (b *broadcaster) refresh() {
	b.checked = time.Now()
	want := map[string]bool{}
	if b.fixed {
		want["0.0.0.0"] = true
	} else {
		for _, i := range Interfaces() {
			if b.iface == "" || b.iface == i.IP {
				want[i.IP] = true
			}
		}
	}
	for ip, c := range b.socks {
		if !want[ip] {
			c.Close()
			delete(b.socks, ip)
		}
	}
	for ip := range want {
		if b.socks[ip] != nil {
			continue
		}
		c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(ip)})
		if err != nil {
			b.log.Warn("cannot open broadcast socket on %s: %v", ip, err)
			continue
		}
		b.socks[ip] = c
		b.log.Info("rebroadcasting discovery from %s to %s", ip, b.dst)
	}
	if len(b.socks) == 0 && !b.warned {
		if b.iface != "" {
			b.log.Warn("broadcast interface %s is not up; radios will not appear in SmartSDR", b.iface)
		} else {
			b.log.Warn("no network interface to broadcast on; radios will not appear in SmartSDR")
		}
	}
	b.warned = len(b.socks) == 0
}

func (b *broadcaster) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for ip, c := range b.socks {
		c.Close()
		delete(b.socks, ip)
	}
}
