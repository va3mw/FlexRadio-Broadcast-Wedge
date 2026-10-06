// Package vita decodes FlexRadio VITA-49 discovery packets.
package vita

import (
	"encoding/binary"
	"strings"
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
