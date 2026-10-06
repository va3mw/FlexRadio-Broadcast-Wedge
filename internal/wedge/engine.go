// Package wedge relays FlexRadio discovery broadcasts between subnets.
//
// The Radio Subnet Client (RSC) hears the radios' UDP 4992 discovery
// broadcasts and streams them, unmodified, over TCP to every connected User
// Subnet Client (USC). The USC rebroadcasts them on its own subnet, where
// SmartSDR sees the radio in its chooser and connects straight to the radio's
// IP over the routed link (VPN).
package wedge

import (
	"context"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/config"
	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/logx"
	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/vita"
)

// magic opens every RSC->USC stream so a USC can tell it reached an RSC.
const magic = "FRBW1\n"

const (
	radioTimeout = 15 * time.Second // radios broadcast about once a second
	keepalive    = 5 * time.Second  // RSC sends an empty frame when idle
	linkIdle     = 20 * time.Second // USC drops a link that goes silent
)

// retryDelay is how long a USC waits before redialling an RSC.
var retryDelay = 5 * time.Second

// Options override the well-known ports; tests use them, the app does not.
type Options struct {
	DiscoveryAddr string       // UDP address the RSC listens on
	BroadcastTo   *net.UDPAddr // if set, the USC sends here instead of broadcasting
}

type seen struct {
	radio   vita.Radio
	last    time.Time
	packets uint64
}

type Engine struct {
	log      *logx.Logger
	cfg      *config.Store
	opt      Options
	onChange func()

	startMu sync.Mutex // serialises Start/Stop
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	mu       sync.Mutex
	role     string
	port     int
	udpUp    bool
	udpErr   string
	tcpUp    bool
	tcpErr   string
	tcpAddr  net.Addr
	local    map[string]*seen // radios heard on this subnet, by serial
	echoes   map[string]bool  // rebroadcasts already reported
	clients  map[*client]struct{}
	links    []*link
	bcast    *broadcaster
	bcastSel string
}

func New(l *logx.Logger, cfg *config.Store, onChange func()) *Engine {
	return NewWithOptions(l, cfg, onChange, Options{})
}

func NewWithOptions(l *logx.Logger, cfg *config.Store, onChange func(), opt Options) *Engine {
	if opt.DiscoveryAddr == "" {
		opt.DiscoveryAddr = ":4992"
	}
	if onChange == nil {
		onChange = func() {}
	}
	return &Engine{log: l, cfg: cfg, opt: opt, onChange: onChange}
}

// Start runs the roles selected in the config until Stop.
func (e *Engine) Start() {
	e.startMu.Lock()
	defer e.startMu.Unlock()
	if e.cancel != nil {
		return
	}
	cfg := e.cfg.Get()
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel

	e.mu.Lock()
	e.role, e.port = cfg.Role, cfg.ListenPort
	e.udpUp, e.udpErr, e.tcpUp, e.tcpErr, e.tcpAddr = false, "", false, "", nil
	e.local = map[string]*seen{}
	e.echoes = map[string]bool{}
	e.clients = map[*client]struct{}{}
	e.links = nil
	e.bcast, e.bcastSel = nil, cfg.BroadcastIface
	if cfg.IsUSC() {
		e.bcast = newBroadcaster(e.log.Src("usc"), cfg.BroadcastIface, e.opt.BroadcastTo)
		used := map[string]bool{}
		for _, s := range cfg.Servers {
			addr := withPort(s)
			if used[addr] {
				continue
			}
			used[addr] = true
			e.links = append(e.links, &link{addr: addr, state: "connecting", radios: map[string]*seen{}})
		}
	}
	links, bc := e.links, e.bcast
	e.mu.Unlock()

	e.log.Log(logx.Info, "wedge", "starting as %s", roleName(cfg.Role))
	if cfg.IsRSC() {
		e.wg.Go(func() { e.listenRadios(ctx) })
		e.wg.Go(func() { e.serve(ctx, cfg.ListenPort) })
	}
	if cfg.IsUSC() {
		if len(links) == 0 {
			e.log.Log(logx.Warn, "usc", "no RSC address configured; nothing to relay")
		}
		for _, l := range links {
			e.wg.Go(func() { e.runLink(ctx, l, bc) })
		}
	}
	e.wg.Go(func() { e.expire(ctx) })
	e.onChange()
}

// Stop ends all listeners and links and waits for them.
func (e *Engine) Stop() {
	e.startMu.Lock()
	defer e.startMu.Unlock()
	if e.cancel == nil {
		return
	}
	e.cancel()
	e.wg.Wait()
	e.cancel = nil
	e.mu.Lock()
	if e.bcast != nil {
		e.bcast.close()
	}
	e.mu.Unlock()
}

// Restart applies a changed role, port, server list or interface.
func (e *Engine) Restart() {
	e.Stop()
	e.Start()
}

// expire forgets radios that stopped broadcasting.
func (e *Engine) expire(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		removed := false
		e.mu.Lock()
		drop := func(m map[string]*seen, where string) {
			for k, s := range m {
				if time.Since(s.last) > radioTimeout {
					delete(m, k)
					removed = true
					e.log.Log(logx.Info, "wedge", "lost %s \"%s\" serial %s (%s)", s.radio.Model, s.radio.Nickname, k, where)
				}
			}
		}
		drop(e.local, "local subnet")
		for _, l := range e.links {
			drop(l.radios, "via "+l.addr)
		}
		e.mu.Unlock()
		if removed {
			e.onChange()
		}
	}
}

// ---- status for the UI ----

type LocalRadio struct {
	Radio    vita.Radio `json:"radio"`
	Exported bool       `json:"exported"`
}

type ClientView struct {
	Addr    string `json:"addr"`
	Since   string `json:"since"`
	Sent    uint64 `json:"sent"`
	Dropped uint64 `json:"dropped"`
}

type RSCStatus struct {
	Listening bool         `json:"listening"` // UDP 4992 open
	ListenErr string       `json:"listen_err"`
	Serving   bool         `json:"serving"` // TCP port open
	ServeErr  string       `json:"serve_err"`
	Port      int          `json:"port"`
	Radios    []LocalRadio `json:"radios"`
	Clients   []ClientView `json:"clients"`
}

// RemoteRadio is a radio a USC receives from an RSC.
type RemoteRadio struct {
	Radio      vita.Radio `json:"radio"`
	Advertised bool       `json:"advertised"` // rebroadcast on the local subnet
}

type LinkView struct {
	Addr    string        `json:"addr"`
	State   string        `json:"state"` // connecting, connected, down
	Err     string        `json:"err"`
	Since   string        `json:"since"`
	Packets uint64        `json:"packets"`
	Radios  []RemoteRadio `json:"radios"`
}

type USCStatus struct {
	Links      []LinkView `json:"links"`
	Iface      string     `json:"iface"`
	Interfaces []Iface    `json:"interfaces"`
}

type Status struct {
	Role string     `json:"role"`
	RSC  *RSCStatus `json:"rsc"`
	USC  *USCStatus `json:"usc"`
}

func (e *Engine) Status() Status {
	cfg := e.cfg.Get()
	e.mu.Lock()
	defer e.mu.Unlock()
	st := Status{Role: e.role}
	if e.role == config.RoleRSC || e.role == config.RoleBoth {
		r := &RSCStatus{Listening: e.udpUp, ListenErr: e.udpErr, Serving: e.tcpUp, ServeErr: e.tcpErr, Port: e.port,
			Radios: []LocalRadio{}, Clients: []ClientView{}}
		for _, s := range e.local {
			r.Radios = append(r.Radios, LocalRadio{Radio: s.radio, Exported: !cfg.IsHidden(s.radio.Serial)})
		}
		sort.Slice(r.Radios, func(i, j int) bool { return r.Radios[i].Radio.Serial < r.Radios[j].Radio.Serial })
		for c := range e.clients {
			r.Clients = append(r.Clients, ClientView{Addr: c.addr, Since: c.since.Format("15:04:05"), Sent: c.sent, Dropped: c.dropped})
		}
		sort.Slice(r.Clients, func(i, j int) bool { return r.Clients[i].Addr < r.Clients[j].Addr })
		st.RSC = r
	}
	if e.role == config.RoleUSC || e.role == config.RoleBoth {
		u := &USCStatus{Links: []LinkView{}, Iface: e.bcastSel, Interfaces: Interfaces()}
		for _, l := range e.links {
			v := LinkView{Addr: l.addr, State: l.state, Err: l.err, Packets: l.packets, Radios: []RemoteRadio{}}
			if l.state == "connected" {
				v.Since = l.since.Format("15:04:05")
			}
			for _, s := range l.radios {
				v.Radios = append(v.Radios, RemoteRadio{Radio: s.radio, Advertised: !cfg.IsMuted(s.radio.Serial)})
			}
			sort.Slice(v.Radios, func(i, j int) bool { return v.Radios[i].Radio.Serial < v.Radios[j].Radio.Serial })
			u.Links = append(u.Links, v)
		}
		st.USC = u
	}
	return st
}

// ServerAddr is the RSC's bound TCP address, once it is serving.
func (e *Engine) ServerAddr() net.Addr {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.tcpAddr
}

func roleName(r string) string {
	switch r {
	case config.RoleRSC:
		return "Radio Subnet Client (RSC)"
	case config.RoleBoth:
		return "RSC + USC"
	default:
		return "User Subnet Client (USC)"
	}
}

// withPort appends the default RSC port to a bare host.
func withPort(s string) string {
	if _, _, err := net.SplitHostPort(s); err == nil {
		return s
	}
	return net.JoinHostPort(s, strconv.Itoa(config.DefaultPort))
}

// sleep waits for d; it returns false if ctx ended first.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
