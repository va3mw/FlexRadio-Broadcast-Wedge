package vita

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

// Sample is the discovery packet the legacy Python wedge sent, which SmartSDR
// accepts.
func Sample(serial, ip string) []byte {
	hdr := []byte("8T\x00\x8a\x00\x00\x08\x00\x00\x00\x1c-SL\xff\xfff!Hx\x00\x00\x00\x00\x00\x00\x00\x00")
	txt := "discovery_protocol_version=3.0.0.2 model=FLEX-6600 serial=" + serial +
		" version=3.8.2.29415 nickname=Flex\x7f6600 callsign=VA3MW ip=" + ip +
		" port=4992 status=Available inuse_ip= inuse_host= \x00\x00\x00"
	return append(hdr, txt...)
}

func TestParseDiscovery(t *testing.T) {
	r := ParseDiscovery(Sample("0519-0079-6600-2411", "192.168.110.76"))
	if r == nil {
		t.Fatal("sample packet rejected")
	}
	if r.Serial != "0519-0079-6600-2411" || r.IP != "192.168.110.76" || r.Model != "FLEX-6600" ||
		r.Nickname != "Flex 6600" || r.Callsign != "VA3MW" || r.Status != "Available" {
		t.Fatalf("bad parse: %+v", r)
	}
}

func TestBuildDiscovery(t *testing.T) {
	a := Announce{IP: "192.168.110.78", Serial: "1111-2222-8600-3333", Model: "FLEX-8600",
		Version: "4.2.20.41343", Nickname: "Cottage 8600", Callsign: "VA3MW", LicenseID: "00-1C-2D-05-07-AE"}
	b := BuildDiscovery(a, 0x1A, time.Unix(0x6ac6f30c, 0))
	// Same framing as a packet captured from a FLEX-6600 on 4.2.20.
	if got := binary.BigEndian.Uint32(b[0:]); got>>16 != 0x385A || int(got&0xFFFF)*4 != len(b) {
		t.Fatalf("header word %08x for %d bytes", got, len(b))
	}
	if h := string(b[4:20]); h != "\x00\x00\x08\x00\x00\x00\x1c\x2dSL\xff\xff\x6a\xc6\xf3\x0c" {
		t.Fatalf("header %x", b[4:20])
	}
	if len(b)%4 != 0 || b[len(b)-1] != 0 {
		t.Fatalf("payload not NUL-padded to a word: %d bytes", len(b))
	}
	r := ParseDiscovery(b)
	if r == nil {
		t.Fatal("built packet does not parse")
	}
	want := Radio{Serial: a.Serial, Model: a.Model, Nickname: a.Nickname, Callsign: a.Callsign,
		IP: a.IP, Version: a.Version, Status: "Available"}
	if *r != want {
		t.Fatalf("got %+v\nwant %+v", *r, want)
	}
	txt := string(b[28:])
	for _, f := range []string{" max_licensed_version=v4 ", " radio_license_id=00-1C-2D-05-07-AE ",
		" max_slices=4 ", " nickname=Cottage\x7f8600 ", " port=4992 "} {
		if !strings.Contains(txt, f) {
			t.Errorf("missing %q", f)
		}
	}
	if s := string(BuildDiscovery(Announce{Model: "FLEX-6700", Version: "3.8.2.1"}, 0, time.Now())[28:]); !strings.Contains(s, " max_slices=8 ") || !strings.Contains(s, "=v3 ") {
		t.Errorf("6700 on v3: %s", s)
	}
}

func TestParseDiscoveryRejects(t *testing.T) {
	good := Sample("1", "10.0.0.1")
	wrongOUI := append([]byte{}, good...)
	wrongOUI[11] = 0x2E
	wrongClass := append([]byte{}, good...)
	wrongClass[15] = 0x00
	wrongType := append([]byte{}, good...)
	wrongType[0] = 0x18 // IF data with stream id
	for name, b := range map[string][]byte{
		"empty": nil, "short": good[:10], "header only": good[:28],
		"oui": wrongOUI, "class": wrongClass, "type": wrongType,
		"text": []byte("serial=1 ip=10.0.0.1 this is not vita at all"),
	} {
		if ParseDiscovery(b) != nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
