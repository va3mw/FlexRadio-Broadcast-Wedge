// Package vita decodes FlexRadio VITA-49 discovery packets.
package vita

import (
	"encoding/binary"
	"strconv"
	"strings"
	"time"
)

const (
	flexOUI        = 0x001C2D
	discoveryClass = 0xFFFF
)

// MaxPacket is larger than any discovery datagram a radio sends.
const MaxPacket = 2048

// Radio is the subset of a discovery packet the wedge displays and filters on.
type Radio struct {
	Serial   string `json:"serial"`
	Model    string `json:"model"`
	Nickname string `json:"nickname"`
	Callsign string `json:"callsign"`
	IP       string `json:"ip"`
	Version  string `json:"version"`
	Status   string `json:"status"`
}

// ParseDiscovery returns the radio described by a discovery datagram, or nil
// if b is not a FlexRadio discovery packet.
func ParseDiscovery(b []byte) *Radio {
	if len(b) < 16 {
		return nil
	}
	hdr := binary.BigEndian.Uint32(b[0:4])
	// Discovery is ExtDataWithStream (type 3) with a class id.
	if hdr>>28 != 3 || hdr&(1<<27) == 0 {
		return nil
	}
	if binary.BigEndian.Uint32(b[8:12])&0xFFFFFF != flexOUI || binary.BigEndian.Uint16(b[14:16]) != discoveryClass {
		return nil
	}
	off := 16
	if (hdr>>22)&3 != 0 { // integer timestamp
		off += 4
	}
	if (hdr>>20)&3 != 0 { // fractional timestamp
		off += 8
	}
	end := int(hdr&0xFFFF) * 4
	if end > len(b) || end <= off {
		end = len(b)
	}
	if off >= end {
		return nil
	}
	kv := parseKV(strings.TrimRight(string(b[off:end]), "\x00 "))
	if kv["serial"] == "" || kv["ip"] == "" {
		return nil
	}
	return &Radio{
		Serial:   kv["serial"],
		Model:    kv["model"],
		Nickname: unescape(kv["nickname"]),
		Callsign: kv["callsign"],
		IP:       kv["ip"],
		Version:  kv["version"],
		Status:   kv["status"],
	}
}

// Announce describes a radio well enough to write its discovery packet.
type Announce struct {
	IP, Serial, Model, Version, Nickname, Callsign, LicenseID string
}

// BuildDiscovery writes the discovery packet a radio with these details
// would broadcast when idle. seq is the VITA packet counter.
func BuildDiscovery(a Announce, seq uint8, now time.Time) []byte {
	n := strconv.Itoa(sliceCount(a.Model))
	major, _, _ := strings.Cut(a.Version, ".")
	txt := "discovery_protocol_version=3.1.0.4" +
		" model=" + a.Model +
		" serial=" + a.Serial +
		" version=" + a.Version +
		" nickname=" + escape(a.Nickname) +
		" callsign=" + escape(a.Callsign) +
		" ip=" + a.IP +
		" port=4992 status=Available inuse_ip= inuse_host=" +
		" max_licensed_version=v" + major +
		" radio_license_id=" + a.LicenseID +
		" fpc_mac= wan_connected=0 licensed_clients=2 available_clients=2" +
		" max_panadapters=" + n + " available_panadapters=" + n +
		" max_slices=" + n + " available_slices=" + n +
		" gui_client_ips= gui_client_hosts= gui_client_programs= gui_client_stations= gui_client_handles=" +
		" min_software_version=2.1.20.0 external_port_link=1 license_is_unknown=0 is_system_model=0"
	size := 28 + len(txt) + 1
	size += (4 - size%4) % 4
	b := make([]byte, size)
	// ExtDataWithStream, class id present, UTC integer and fractional time.
	binary.BigEndian.PutUint32(b[0:], 0x38500000|uint32(seq&0xF)<<16|uint32(size/4))
	binary.BigEndian.PutUint32(b[4:], 0x00000800)
	binary.BigEndian.PutUint32(b[8:], flexOUI)
	binary.BigEndian.PutUint32(b[12:], 0x534C0000|discoveryClass)
	binary.BigEndian.PutUint32(b[16:], uint32(now.Unix()))
	copy(b[28:], txt)
	return b
}

// sliceCount is how many slices and panadapters a model has.
func sliceCount(model string) int {
	switch {
	case strings.Contains(model, "6700"):
		return 8
	case strings.Contains(model, "6400"), strings.Contains(model, "8400"),
		strings.Contains(model, "6300"), strings.Contains(model, "510"):
		return 2
	}
	return 4
}

// escape encodes spaces the way the radio does in names.
func escape(s string) string { return strings.ReplaceAll(s, " ", "\x7f") }

// parseKV splits "a=1 b=2 c" into a map. Bare words map to "".
func parseKV(s string) map[string]string {
	m := map[string]string{}
	for _, f := range strings.Fields(s) {
		k, v, _ := strings.Cut(f, "=")
		m[k] = v
	}
	return m
}

// unescape reverses the radio's encoding of spaces (0x7F) in names.
func unescape(s string) string { return strings.ReplaceAll(s, "\x7f", " ") }
