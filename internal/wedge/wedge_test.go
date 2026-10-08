package wedge

import (
	"bytes"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/config"
	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/logx"
	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/vita"
)

// discoveryPacket is the packet the legacy Python wedge sent, which SmartSDR
// accepts.
func discoveryPacket(serial, ip string) []byte {
	hdr := []byte("8T\x00\x8a\x00\x00\x08\x00\x00\x00\x1c-SL\xff\xfff!Hx\x00\x00\x00\x00\x00\x00\x00\x00")
	txt := "discovery_protocol_version=3.0.0.2 model=FLEX-6600 serial=" + serial +
		" version=3.8.2.29415 nickname=Flex6600 callsign=VA3MW ip=" + ip +
		" port=4992 status=Available inuse_ip= inuse_host= \x00\x00\x00"
	return append(hdr, txt...)
}

func freePort(t *testing.T, network string) int {
	t.Helper()
	if network == "udp" {
		c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		return c.LocalAddr().(*net.UDPAddr).Port
	}
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

type rig struct {
	t        *testing.T
	rsc, usc *Engine
	rscCfg   *config.Store
	uscCfg   *config.Store
	radio    *net.UDPConn // sends to the RSC as if it were a radio
	sink     *net.UDPConn // stands in for SmartSDR on the user subnet
}

func newRig(t *testing.T) *rig {
	retryDelay = 200 * time.Millisecond
	dir := t.TempDir()
	l := logx.New(filepath.Join(dir, "logs"))
	t.Cleanup(l.Close)
	udpPort, tcpPort := freePort(t, "udp"), freePort(t, "tcp")

	sink, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sink.Close() })
	radio, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: udpPort})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { radio.Close() })

	rscCfg := config.LoadFrom(filepath.Join(dir, "rsc.json"))
	rscCfg.Update(func(c *config.Config) { c.Role, c.ListenPort = config.RoleRSC, tcpPort })
	uscCfg := config.LoadFrom(filepath.Join(dir, "usc.json"))
	uscCfg.Update(func(c *config.Config) {
		c.Role, c.Servers = config.RoleUSC, []string{"127.0.0.1:" + strconv.Itoa(tcpPort)}
	})

	r := &rig{t: t, rscCfg: rscCfg, uscCfg: uscCfg, radio: radio, sink: sink}
	r.rsc = NewWithOptions(l, rscCfg, nil, Options{DiscoveryAddr: "127.0.0.1:" + strconv.Itoa(udpPort)})
	r.usc = NewWithOptions(l, uscCfg, nil, Options{BroadcastTo: sink.LocalAddr().(*net.UDPAddr)})
	r.rsc.Start()
	r.usc.Start()
	t.Cleanup(r.usc.Stop)
	t.Cleanup(r.rsc.Stop)
	r.waitLinked()
	return r
}

func (r *rig) waitLinked() {
	r.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st := r.rsc.Status(); st.RSC != nil && st.RSC.Listening && len(st.RSC.Clients) == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	r.t.Fatal("USC never connected to RSC")
}

// relayed sends pkt as a radio would and returns what the sink receives.
func (r *rig) relayed(pkt []byte) []byte {
	r.t.Helper()
	if _, err := r.radio.Write(pkt); err != nil {
		r.t.Fatal(err)
	}
	buf := make([]byte, 4096)
	r.sink.SetReadDeadline(time.Now().Add(700 * time.Millisecond))
	n, _, err := r.sink.ReadFromUDP(buf)
	if err != nil {
		return nil
	}
	return buf[:n]
}

func TestRelayVerbatim(t *testing.T) {
	r := newRig(t)
	pkt := discoveryPacket("1111-2222-6600-3333", "127.0.0.1")
	if got := r.relayed(pkt); !bytes.Equal(got, pkt) {
		t.Fatalf("relayed packet differs: got %d bytes, want %d", len(got), len(pkt))
	}
	rs, us := r.rsc.Status(), r.usc.Status()
	if len(rs.RSC.Radios) != 1 || !rs.RSC.Radios[0].Exported || rs.RSC.Radios[0].Radio.IP != "127.0.0.1" {
		t.Fatalf("RSC radios: %+v", rs.RSC.Radios)
	}
	if len(us.USC.Links) != 1 || us.USC.Links[0].State != "connected" || len(us.USC.Links[0].Radios) != 1 {
		t.Fatalf("USC links: %+v", us.USC.Links)
	}
}

// A discovery packet whose sender is not the radio it names is another
// wedge's rebroadcast and must not be relayed again.
func TestRebroadcastNotRelayed(t *testing.T) {
	r := newRig(t)
	if got := r.relayed(discoveryPacket("1111-2222-6600-3333", "10.9.9.9")); got != nil {
		t.Fatal("rebroadcast was relayed")
	}
	if n := len(r.rsc.Status().RSC.Radios); n != 0 {
		t.Fatalf("rebroadcast listed as %d local radio(s)", n)
	}
}

func TestHiddenRadioNotRelayed(t *testing.T) {
	r := newRig(t)
	const serial = "1111-2222-6600-3333"
	r.rscCfg.Update(func(c *config.Config) { c.Hidden = []string{serial} })
	if got := r.relayed(discoveryPacket(serial, "127.0.0.1")); got != nil {
		t.Fatal("hidden radio was relayed")
	}
	rs := r.rsc.Status().RSC.Radios
	if len(rs) != 1 || rs[0].Exported {
		t.Fatalf("hidden radio should be listed, not exported: %+v", rs)
	}
	other := discoveryPacket("9999-2222-6600-3333", "127.0.0.1")
	if got := r.relayed(other); !bytes.Equal(got, other) {
		t.Fatal("visible radio was not relayed")
	}
	r.rscCfg.Update(func(c *config.Config) { c.Hidden = nil })
	if got := r.relayed(discoveryPacket(serial, "127.0.0.1")); got == nil {
		t.Fatal("radio still hidden after being re-exported")
	}
}

func TestMutedRadioNotAdvertised(t *testing.T) {
	r := newRig(t)
	const serial = "1111-2222-6600-3333"
	pkt := discoveryPacket(serial, "127.0.0.1")
	r.uscCfg.Update(func(c *config.Config) { c.Muted = []string{serial} })
	if got := r.relayed(pkt); got != nil {
		t.Fatal("muted radio was advertised")
	}
	rs := r.usc.Status().USC.Links[0].Radios
	if len(rs) != 1 || rs[0].Advertised {
		t.Fatalf("muted radio should be listed, not advertised: %+v", rs)
	}
	other := discoveryPacket("9999-2222-6600-3333", "127.0.0.1")
	if got := r.relayed(other); !bytes.Equal(got, other) {
		t.Fatal("other radio was not advertised")
	}
	r.uscCfg.Update(func(c *config.Config) { c.Muted = nil })
	if got := r.relayed(pkt); !bytes.Equal(got, pkt) {
		t.Fatal("radio still muted after being turned back on")
	}
}

func TestManualRadioAdvertised(t *testing.T) {
	r := newRig(t)
	m := config.ManualRadio{IP: "10.1.2.3", Serial: "4444-5555-8600-6666", Model: "FLEX-8600",
		Version: "4.2.20.41343", Nickname: "Far Away", Callsign: "VA3MW", LicenseID: "00-1C-2D-00-00-01"}
	r.uscCfg.Update(func(c *config.Config) { c.Manual = []config.ManualRadio{m} })
	read := func() *vita.Radio {
		buf := make([]byte, 4096)
		r.sink.SetReadDeadline(time.Now().Add(2500 * time.Millisecond))
		n, _, err := r.sink.ReadFromUDP(buf)
		if err != nil {
			return nil
		}
		return vita.ParseDiscovery(buf[:n])
	}
	got := read()
	if got == nil || got.Serial != m.Serial || got.IP != m.IP || got.Nickname != m.Nickname || got.Version != m.Version {
		t.Fatalf("manual radio not advertised correctly: %+v", got)
	}
	mv := r.usc.Status().USC.Manual
	if len(mv) != 1 || !mv[0].Advertised || mv[0].Relayed {
		t.Fatalf("status: %+v", mv)
	}
	r.uscCfg.Update(func(c *config.Config) { c.Muted = []string{m.Serial} })
	time.Sleep(1200 * time.Millisecond) // let a packet already on its way arrive
	for read() != nil {
	}
	if mv := r.usc.Status().USC.Manual; mv[0].Advertised {
		t.Fatal("muted manual radio still reported as advertised")
	}
}

func TestUSCReconnects(t *testing.T) {
	r := newRig(t)
	r.rsc.Restart()
	r.waitLinked()
	pkt := discoveryPacket("1111-2222-6600-3333", "127.0.0.1")
	if got := r.relayed(pkt); !bytes.Equal(got, pkt) {
		t.Fatal("no relay after RSC restart")
	}
}

func TestNotAnRSC(t *testing.T) {
	retryDelay = 200 * time.Millisecond
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
			c.Close()
		}
	}()
	dir := t.TempDir()
	l := logx.New(filepath.Join(dir, "logs"))
	defer l.Close()
	cfg := config.LoadFrom(filepath.Join(dir, "usc.json"))
	cfg.Update(func(c *config.Config) { c.Servers = []string{ln.Addr().String()} })
	e := NewWithOptions(l, cfg, nil, Options{BroadcastTo: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9}})
	e.Start()
	defer e.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if lk := e.Status().USC.Links[0]; lk.State == "down" {
			if lk.Err != "not a Broadcast Wedge RSC" {
				t.Fatalf("unexpected error %q", lk.Err)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("link never reported down")
}
