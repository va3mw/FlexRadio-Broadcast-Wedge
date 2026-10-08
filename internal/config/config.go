// Package config holds persisted user settings.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Roles. A PC that is on a radio subnet and also wants to see radios from
// another site runs "both".
const (
	RoleRSC  = "rsc"  // Radio Subnet Client: hears radios, serves them to USCs
	RoleUSC  = "usc"  // User Subnet Client: dials RSCs, rebroadcasts locally
	RoleBoth = "both" // RSC and USC in one process
)

// DefaultPort is the TCP port an RSC listens on for USCs.
const DefaultPort = 4996

// ManualRadio is a radio the user describes by hand, for a USC that has no
// RSC at the radio's site to learn it from.
type ManualRadio struct {
	IP        string `json:"ip"`
	Serial    string `json:"serial"`
	Model     string `json:"model"`
	Version   string `json:"version"` // radio firmware, e.g. 4.2.20.41343
	Nickname  string `json:"nickname"`
	Callsign  string `json:"callsign"`
	LicenseID string `json:"license_id"` // radio_license_id, e.g. 00-1C-2D-05-07-AE
}

type Config struct {
	Role string `json:"role"`

	// RSC
	ListenPort int      `json:"listen_port"` // TCP port USCs dial
	Hidden     []string `json:"hidden"`      // radio serials NOT exported to USCs

	// USC
	Servers        []string      `json:"servers"`         // RSC addresses, host or host:port
	BroadcastIface string        `json:"broadcast_iface"` // local IPv4 to rebroadcast from; "" = all interfaces
	Muted          []string      `json:"muted"`           // radio serials received but NOT advertised locally
	Manual         []ManualRadio `json:"manual"`          // radios announced from typed-in details
}

func Default() *Config {
	c := &Config{Role: RoleUSC, ListenPort: DefaultPort}
	c.normalize()
	return c
}

func (c *Config) normalize() {
	switch c.Role {
	case RoleRSC, RoleUSC, RoleBoth:
	default:
		c.Role = RoleUSC
	}
	if c.ListenPort < 1 || c.ListenPort > 65535 {
		c.ListenPort = DefaultPort
	}
	if c.Hidden == nil {
		c.Hidden = []string{}
	}
	if c.Muted == nil {
		c.Muted = []string{}
	}
	if c.Manual == nil {
		c.Manual = []ManualRadio{}
	}
	servers := []string{}
	for _, s := range c.Servers {
		if s = strings.TrimSpace(s); s != "" {
			servers = append(servers, s)
		}
	}
	c.Servers = servers
}

func (c Config) IsRSC() bool { return c.Role == RoleRSC || c.Role == RoleBoth }
func (c Config) IsUSC() bool { return c.Role == RoleUSC || c.Role == RoleBoth }

// IsHidden reports whether the radio with this serial is withheld from USCs.
func (c Config) IsHidden(serial string) bool {
	for _, s := range c.Hidden {
		if s == serial {
			return true
		}
	}
	return false
}

// IsMuted reports whether a USC keeps the radio with this serial to itself
// instead of advertising it on the local subnet.
func (c Config) IsMuted(serial string) bool {
	for _, s := range c.Muted {
		if s == serial {
			return true
		}
	}
	return false
}

type Store struct {
	mu   sync.Mutex
	path string
	cfg  *Config
}

func Dir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "BroadcastWedge")
}

func Load() *Store { return LoadFrom(filepath.Join(Dir(), "config.json")) }

func LoadFrom(path string) *Store {
	s := &Store{path: path, cfg: Default()}
	if b, err := os.ReadFile(s.path); err == nil {
		c := Default()
		if json.Unmarshal(b, c) == nil {
			c.normalize()
			s.cfg = c
		}
	}
	return s
}

// Get returns a copy of the config.
func (s *Store) Get() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := *s.cfg
	c.Hidden = append([]string{}, s.cfg.Hidden...)
	c.Servers = append([]string{}, s.cfg.Servers...)
	c.Muted = append([]string{}, s.cfg.Muted...)
	c.Manual = append([]ManualRadio{}, s.cfg.Manual...)
	return c
}

// Update applies f to the config and saves it.
func (s *Store) Update(f func(c *Config)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(s.cfg)
	s.cfg.normalize()
	b, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}
