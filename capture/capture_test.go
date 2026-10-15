package capture

import (
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

func TestSelector(t *testing.T) {
	const after = 3 * time.Second
	s := NewSelector(after)
	t0 := time.Unix(0, 0)

	check := func(iface string, at time.Duration, wantAccept, wantSwitched bool) {
		t.Helper()
		accept, switched := s.Accept(iface, t0.Add(at))
		if accept != wantAccept || switched != wantSwitched {
			t.Fatalf("%s @%s: accept=%v switched=%v, ожидалось %v %v",
				iface, at, accept, switched, wantAccept, wantSwitched)
		}
	}

	// Первый пакет делает интерфейс активным, сброс парсера не нужен.
	check("eth", 0, true, false)
	// Копии на другом интерфейсе отбрасываются, пока активный жив.
	check("warp", 100*time.Millisecond, false, false)
	check("eth", 2*time.Second, true, false)
	check("warp", 4*time.Second, false, false) // eth молчал всего 2с
	// Активный молчит дольше 3с — переключаемся.
	check("warp", 5*time.Second+time.Millisecond, true, true)
	check("warp", 6*time.Second, true, false)
	// Старый интерфейс снова ожил, но новый активен — назад не прыгаем.
	check("eth", 6500*time.Millisecond, false, false)
	if s.Active() != "warp" {
		t.Fatalf("активный %q, ожидался warp", s.Active())
	}
}

func buildUDP(t *testing.T, withEthernet bool) []byte {
	t.Helper()
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolUDP,
		SrcIP: []byte{10, 0, 0, 1}, DstIP: []byte{10, 0, 0, 2}}
	udp := &layers.UDP{SrcPort: 5056, DstPort: 50000}
	if err := udp.SetNetworkLayerForChecksum(ip); err != nil {
		t.Fatal(err)
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	ls := []gopacket.SerializableLayer{ip, udp, gopacket.Payload("photon")}
	if withEthernet {
		eth := &layers.Ethernet{SrcMAC: make([]byte, 6), DstMAC: make([]byte, 6), EthernetType: layers.EthernetTypeIPv4}
		ls = append([]gopacket.SerializableLayer{eth}, ls...)
	}
	if err := gopacket.SerializeLayers(buf, opts, ls...); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestUDPPayload(t *testing.T) {
	cases := []struct {
		name     string
		data     []byte
		linkType layers.LinkType
	}{
		{"ethernet", buildUDP(t, true), layers.LinkTypeEthernet},
		{"raw IP (gopacket)", buildUDP(t, false), layers.LinkTypeRaw},
		// Wintun/TAP через Npcap отдают DLT_RAW = 12, которого gopacket не знает.
		{"raw IP (DLT_RAW=12)", buildUDP(t, false), layers.LinkType(12)},
	}
	for _, c := range cases {
		if got := string(udpPayload(c.data, c.linkType)); got != "photon" {
			t.Errorf("%s: получено %q", c.name, got)
		}
	}
}
