// Package service is the facade the UI talks to, whether that UI is the
// desktop window or the headless service's web page.
package service

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/config"
	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/logx"
	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/wedge"
)

const Version = "2.2.0"

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

var (
	versionRE = regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+$`)
	wordRE    = regexp.MustCompile(`^[^\s=]+$`)
)

// SaveManual adds a hand-entered radio (index -1) or replaces the one at
// index. It is announced from the next second on.
func (s *Service) SaveManual(index int, r config.ManualRadio) error {
	r.IP, r.Serial, r.Model = strings.TrimSpace(r.IP), strings.TrimSpace(r.Serial), strings.TrimSpace(r.Model)
	r.Version, r.LicenseID = strings.TrimPrefix(strings.TrimSpace(r.Version), "v"), strings.TrimSpace(r.LicenseID)
	r.Nickname, r.Callsign = strings.TrimSpace(r.Nickname), strings.ToUpper(strings.TrimSpace(r.Callsign))
	ip := net.ParseIP(r.IP)
	switch {
	case ip == nil || ip.To4() == nil:
		return fmt.Errorf("IP address must look like 192.168.1.50")
	case !wordRE.MatchString(r.Serial):
		return fmt.Errorf("serial number is required, with no spaces (for example 1234-5678-6600-9012)")
	case !wordRE.MatchString(r.Model):
		return fmt.Errorf("model is required, with no spaces (for example FLEX-6600)")
	case !versionRE.MatchString(r.Version):
		return fmt.Errorf("firmware version must be the full four-part number, for example 4.2.20.41343")
	case strings.Contains(r.Nickname, "="), strings.ContainsAny(r.Callsign, " ="):
		return fmt.Errorf("nickname and callsign cannot contain =, and a callsign cannot contain spaces")
	case r.LicenseID != "" && !wordRE.MatchString(r.LicenseID):
		return fmt.Errorf("MAC address cannot contain spaces (for example 00-1C-2D-05-07-AE)")
	}
	r.IP = ip.To4().String()
	cfg := s.cfg.Get()
	if index < -1 || index >= len(cfg.Manual) {
		return fmt.Errorf("no such manual radio")
	}
	for i, m := range cfg.Manual {
		if i != index && m.Serial == r.Serial {
			return fmt.Errorf("a manual radio with serial %s already exists", r.Serial)
		}
	}
	if err := s.cfg.Update(func(c *config.Config) {
		if index < 0 {
			c.Manual = append(c.Manual, r)
		} else {
			c.Manual[index] = r
		}
	}); err != nil {
		return err
	}
	s.log.Log(logx.Info, "app", "manual radio saved: %s \"%s\" serial %s at %s (v%s)", r.Model, r.Nickname, r.Serial, r.IP, r.Version)
	return nil
}

// RemoveManual deletes the hand-entered radio at index.
func (s *Service) RemoveManual(index int) error {
	cfg := s.cfg.Get()
	if index < 0 || index >= len(cfg.Manual) {
		return fmt.Errorf("no such manual radio")
	}
	gone := cfg.Manual[index]
	err := s.cfg.Update(func(c *config.Config) {
		c.Manual = append(c.Manual[:index:index], c.Manual[index+1:]...)
		// Forget its Advertise choice, so adding it again starts switched on.
		muted := []string{}
		for _, m := range c.Muted {
			if m != gone.Serial {
				muted = append(muted, m)
			}
		}
		c.Muted = muted
	})
	s.log.Log(logx.Info, "app", "manual radio removed: serial %s at %s", gone.Serial, gone.IP)
	return err
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
