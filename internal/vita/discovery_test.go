package vita

import "testing"

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
