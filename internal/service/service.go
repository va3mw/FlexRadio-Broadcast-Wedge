// Package service is the facade the UI talks to, whether that UI is the
// desktop window or the headless service's web page.
package service

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/config"
	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/logx"
	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/wedge"
)

const Version = "2.1.1"

type Service struct {
	log    *logx.Logger
	cfg    *config.Store
	engine *wedge.Engine
}

// New builds the relay; onChange is called whenever Status would differ.
func New(l *logx.Logger, cfg *config.Store, onChange func()) *Service {
	return &Service{log: l, cfg: cfg, engine: wedge.New(l, cfg, onChange)}
}

func (s *Service) Start() { s.engine.Start() }
func (s *Service) Stop()  { s.engine.Stop() }

type Status struct {
	Version string        `json:"version"`
	Config  config.Config `json:"config"`
	Wedge   wedge.Status  `json:"wedge"`
}

func (s *Service) Status() Status {
	return Status{Version: Version, Config: s.cfg.Get(), Wedge: s.engine.Status()}
}

func (s *Service) Logs(since uint64) []logx.Entry { return s.log.Since(since) }

type Settings struct {
	Role           string   `json:"role"`
	ListenPort     int      `json:"listen_port"`
	Servers        []string `json:"servers"`
	BroadcastIface string   `json:"broadcast_iface"`
}

// SaveSettings validates and stores the settings, then restarts the relay.
func (s *Service) SaveSettings(n Settings) error {
	switch n.Role {
	case config.RoleRSC, config.RoleUSC, config.RoleBoth:
	default:
		return fmt.Errorf("unknown role %q", n.Role)
	}
	if n.ListenPort < 1024 || n.ListenPort > 65535 {
		return fmt.Errorf("RSC port must be 1024-65535")
	}
	servers := []string{}
	for _, v := range n.Servers {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		bad := fmt.Errorf("%q is not an address; use an IP or host name, optionally with :port", v)
		if strings.ContainsAny(v, " /") {
			return bad
		}
		if _, p, err := net.SplitHostPort(v); err == nil {
			if port, err := strconv.Atoi(p); err != nil || port < 1 || port > 65535 {
				return fmt.Errorf("%q has a bad port", v)
			}
		} else if strings.Contains(v, ":") {
			return bad
		}
		servers = append(servers, v)
	}
	if n.BroadcastIface != "" && net.ParseIP(n.BroadcastIface) == nil {
		return fmt.Errorf("broadcast interface must be a local IP address")
	}
	if err := s.cfg.Update(func(c *config.Config) {
		c.Role, c.ListenPort, c.Servers, c.BroadcastIface = n.Role, n.ListenPort, servers, n.BroadcastIface
	}); err != nil {
		return err
	}
	s.log.Log(logx.Info, "app", "settings: role %s, RSC port %d, RSCs %v, broadcast interface %q", n.Role, n.ListenPort, servers, n.BroadcastIface)
	s.engine.Restart()
	return nil
}

// SetAdvertised chooses whether a USC rebroadcasts a radio it receives. It
// takes effect on the radio's next discovery packet; SmartSDR then drops a
// silenced radio from its list after a few seconds.
func (s *Service) SetAdvertised(serial string, advertised bool) error {
	err := s.cfg.Update(func(c *config.Config) {
		muted := []string{}
		for _, m := range c.Muted {
			if m != serial {
				muted = append(muted, m)
			}
		}
		if !advertised {
			muted = append(muted, serial)
		}
		c.Muted = muted
	})
	if advertised {
		s.log.Log(logx.Info, "app", "radio %s is now advertised on this subnet", serial)
	} else {
		s.log.Log(logx.Info, "app", "radio %s is no longer advertised on this subnet", serial)
	}
	return err
}

// SetExported chooses whether an RSC offers a radio to USCs. It takes effect
// on the radio's next discovery packet.
func (s *Service) SetExported(serial string, exported bool) error {
	err := s.cfg.Update(func(c *config.Config) {
		hidden := []string{}
		for _, h := range c.Hidden {
			if h != serial {
				hidden = append(hidden, h)
			}
		}
		if !exported {
			hidden = append(hidden, serial)
		}
		c.Hidden = hidden
	})
	if exported {
		s.log.Log(logx.Info, "app", "radio %s is now exported to USCs", serial)
	} else {
		s.log.Log(logx.Info, "app", "radio %s is now hidden from USCs", serial)
	}
	return err
}
