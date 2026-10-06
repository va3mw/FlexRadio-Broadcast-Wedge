package wedge

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/logx"
	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/vita"
)

// client is one connected USC.
type client struct {
	addr    string
	since   time.Time
	out     chan []byte
	sent    uint64
	dropped uint64
}

// listenRadios receives the radios' discovery broadcasts.
func (e *Engine) listenRadios(ctx context.Context) {
	log := e.log.Src("rsc")
	// SmartSDR also listens on UDP 4992, so the port must be opened shared.
	lc := net.ListenConfig{Control: reuseAddr}
	warned := false
	for {
		pc, err := lc.ListenPacket(ctx, "udp4", e.opt.DiscoveryAddr)
		if err != nil {
			if !warned {
				log.Warn("cannot listen for radio discovery on UDP %s: %v (retrying)", e.opt.DiscoveryAddr, err)
				warned = true
			}
			e.setUDP(false, err.Error())
			if !sleep(ctx, 10*time.Second) {
				return
			}
			continue
		}
		warned = false
		stop := context.AfterFunc(ctx, func() { pc.Close() })
		e.setUDP(true, "")
		log.Info("listening for radio discovery on UDP %s (shared)", e.opt.DiscoveryAddr)
		buf := make([]byte, vita.MaxPacket)
		for {
			n, src, err := pc.ReadFrom(buf)
			if err != nil {
				break
			}
			e.onRadioPacket(buf[:n], src.(*net.UDPAddr).IP)
		}
		stop()
		pc.Close()
		if ctx.Err() != nil {
			return
		}
		e.setUDP(false, "discovery socket closed")
		if !sleep(ctx, 2*time.Second) {
			return
		}
	}
}

func (e *Engine) setUDP(up bool, msg string) {
	e.mu.Lock()
	e.udpUp, e.udpErr = up, msg
	e.mu.Unlock()
	e.onChange()
}

func (e *Engine) onRadioPacket(pkt []byte, src net.IP) {
	r := vita.ParseDiscovery(pkt)
	if r == nil {
		return
	}
	// A radio broadcasts from its own address. Anything else on this subnet
	// announcing a radio is a wedge rebroadcasting one from another site;
	// relaying that again would loop packets between sites forever.
	if !src.Equal(net.ParseIP(r.IP)) {
		key := r.Serial + "@" + src.String()
		e.mu.Lock()
		first := !e.echoes[key]
		e.echoes[key] = true
		e.mu.Unlock()
		if first {
			e.log.Log(logx.Debug, "rsc", "not relaying %s \"%s\" at %s: announced by %s, not by the radio itself", r.Model, r.Nickname, r.IP, src)
		}
		return
	}
	hidden := e.cfg.Get().IsHidden(r.Serial)
	e.mu.Lock()
	s, known := e.local[r.Serial]
	if !known {
		s = &seen{}
		e.local[r.Serial] = s
	}
	changed := !known || s.radio != *r
	s.radio, s.last = *r, time.Now()
	s.packets++
	if !hidden && len(e.clients) > 0 {
		frame := append([]byte(nil), pkt...)
		for c := range e.clients {
			select {
			case c.out <- frame:
				c.sent++
			default: // slow USC: discovery repeats every second, so just skip
				c.dropped++
			}
		}
	}
	e.mu.Unlock()
	if !known {
		e.log.Log(logx.Info, "rsc", "found %s \"%s\" serial %s at %s (v%s)", r.Model, r.Nickname, r.Serial, r.IP, r.Version)
	}
	if changed {
		e.onChange()
	}
}

// serve accepts USC connections.
func (e *Engine) serve(ctx context.Context, port int) {
	log := e.log.Src("rsc")
	var lc net.ListenConfig
	warned := false
	for {
		ln, err := lc.Listen(ctx, "tcp4", fmt.Sprintf(":%d", port))
		if err != nil {
			if !warned {
				log.Error("cannot listen for USCs on TCP %d: %v (retrying)", port, err)
				warned = true
			}
			e.setTCP(false, err.Error(), nil)
			if !sleep(ctx, 10*time.Second) {
				return
			}
			continue
		}
		warned = false
		stop := context.AfterFunc(ctx, func() { ln.Close() })
		e.setTCP(true, "", ln.Addr())
		log.Info("waiting for USCs on TCP %s", ln.Addr())
		for {
			conn, err := ln.Accept()
			if err != nil {
				break
			}
			e.wg.Go(func() { e.handleClient(ctx, conn) })
		}
		stop()
		ln.Close()
		if ctx.Err() != nil {
			return
		}
		e.setTCP(false, "listener closed", nil)
		if !sleep(ctx, 2*time.Second) {
			return
		}
	}
}

func (e *Engine) setTCP(up bool, msg string, addr net.Addr) {
	e.mu.Lock()
	e.tcpUp, e.tcpErr, e.tcpAddr = up, msg, addr
	e.mu.Unlock()
	e.onChange()
}

func (e *Engine) handleClient(ctx context.Context, conn net.Conn) {
	log := e.log.Src("rsc")
	c := &client{addr: conn.RemoteAddr().String(), since: time.Now(), out: make(chan []byte, 64)}
	e.mu.Lock()
	e.clients[c] = struct{}{}
	e.mu.Unlock()
	log.Info("USC %s connected", c.addr)
	e.onChange()

	stop := context.AfterFunc(ctx, func() { conn.Close() })
	// A USC never sends anything; reading is how a closed link is noticed.
	gone := make(chan struct{})
	go func() {
		io.Copy(io.Discard, conn)
		close(gone)
	}()

	err := writeAll(conn, []byte(magic))
	tick := time.NewTicker(keepalive)
loop:
	for err == nil {
		select {
		case <-ctx.Done():
			break loop
		case <-gone:
			break loop
		case p := <-c.out:
			err = writeFrame(conn, p)
		case <-tick.C:
			err = writeFrame(conn, nil)
		}
	}
	tick.Stop()
	stop()
	conn.Close()
	<-gone

	e.mu.Lock()
	delete(e.clients, c)
	e.mu.Unlock()
	if err != nil && ctx.Err() == nil {
		log.Info("USC %s disconnected: %v", c.addr, err)
	} else {
		log.Info("USC %s disconnected", c.addr)
	}
	e.onChange()
}

// writeFrame sends one length-prefixed packet; an empty frame is a keepalive.
func writeFrame(conn net.Conn, p []byte) error {
	b := make([]byte, 2+len(p))
	binary.BigEndian.PutUint16(b, uint16(len(p)))
	copy(b[2:], p)
	return writeAll(conn, b)
}

func writeAll(conn net.Conn, b []byte) error {
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := conn.Write(b)
	return err
}
