package main

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/config"
	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/logx"
	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/wedge"
)

// App is the Wails-bound facade over the relay engine.
type App struct {
	ctx    context.Context
	log    *logx.Logger
	cfg    *config.Store
	engine *wedge.Engine

	logMu  sync.Mutex
	logBuf []logx.Entry
}

func NewApp(l *logx.Logger, cfg *config.Store) *App {
	a := &App{log: l, cfg: cfg}
	a.engine = wedge.New(l, cfg, a.pushStatus)
	l.OnEntry(func(e logx.Entry) {
		a.logMu.Lock()
		a.logBuf = append(a.logBuf, e)
		a.logMu.Unlock()
	})
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go a.pump()
	a.engine.Start()
}

func (a *App) shutdown(context.Context) {
	a.log.Log(logx.Info, "app", "shutting down")
	a.engine.Stop()
	a.log.Close()
}

func (a *App) pushStatus() {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "status", a.GetStatus())
	}
}

// pump forwards log entries to the UI in batches and refreshes the packet
// counters, which change without a status event.
func (a *App) pump() {
	n := 0
	for range time.Tick(150 * time.Millisecond) {
		a.logMu.Lock()
		batch := a.logBuf
		a.logBuf = nil
		a.logMu.Unlock()
		if len(batch) > 0 {
			runtime.EventsEmit(a.ctx, "log", batch)
		}
		if n++; n%10 == 0 {
			a.pushStatus()
		}
	}
}

// ---- bound methods ----

type Status struct {
	Version string        `json:"version"`
	Config  config.Config `json:"config"`
	Wedge   wedge.Status  `json:"wedge"`
}

func (a *App) GetStatus() Status {
	return Status{Version: version, Config: a.cfg.Get(), Wedge: a.engine.Status()}
}

func (a *App) GetLogs(since uint64) []logx.Entry { return a.log.Since(since) }

type Settings struct {
	Role           string   `json:"role"`
	ListenPort     int      `json:"listen_port"`
	Servers        []string `json:"servers"`
	BroadcastIface string   `json:"broadcast_iface"`
}

// SaveSettings validates and stores the settings, then restarts the relay.
func (a *App) SaveSettings(s Settings) error {
	switch s.Role {
	case config.RoleRSC, config.RoleUSC, config.RoleBoth:
	default:
		return fmt.Errorf("unknown role %q", s.Role)
	}
	if s.ListenPort < 1024 || s.ListenPort > 65535 {
		return fmt.Errorf("RSC port must be 1024-65535")
	}
	servers := []string{}
	for _, v := range s.Servers {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if strings.ContainsAny(v, " /") {
			return fmt.Errorf("%q is not an address; use an IP or host name, optionally with :port", v)
		}
		if _, p, err := net.SplitHostPort(v); err == nil {
			var n int
			if _, err := fmt.Sscanf(p, "%d", &n); err != nil || n < 1 || n > 65535 {
				return fmt.Errorf("%q has a bad port", v)
			}
		} else if strings.Contains(v, ":") {
			return fmt.Errorf("%q is not an address; use an IP or host name, optionally with :port", v)
		}
		servers = append(servers, v)
	}
	if s.BroadcastIface != "" && net.ParseIP(s.BroadcastIface) == nil {
		return fmt.Errorf("broadcast interface must be a local IP address")
	}
	if err := a.cfg.Update(func(c *config.Config) {
		c.Role, c.ListenPort, c.Servers, c.BroadcastIface = s.Role, s.ListenPort, servers, s.BroadcastIface
	}); err != nil {
		return err
	}
	a.log.Log(logx.Info, "app", "settings: role %s, RSC port %d, RSCs %v, broadcast interface %q", s.Role, s.ListenPort, servers, s.BroadcastIface)
	a.engine.Restart()
	return nil
}

// SetExported chooses whether an RSC offers a radio to USCs. It takes effect
// on the radio's next discovery packet.
func (a *App) SetExported(serial string, exported bool) error {
	err := a.cfg.Update(func(c *config.Config) {
		hidden := []string{}
		for _, s := range c.Hidden {
			if s != serial {
				hidden = append(hidden, s)
			}
		}
		if !exported {
			hidden = append(hidden, serial)
		}
		c.Hidden = hidden
	})
	if exported {
		a.log.Log(logx.Info, "app", "radio %s is now exported to USCs", serial)
	} else {
		a.log.Log(logx.Info, "app", "radio %s is now hidden from USCs", serial)
	}
	a.pushStatus()
	return err
}

func (a *App) OpenLogFolder() {
	exec.Command("explorer", a.log.Dir()).Start()
}
