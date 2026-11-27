package domain

import (
	"net/netip"
	"testing"
)

func TestNormalizeMAC(t *testing.T) {
	cases := map[string]string{
		"AA:BB:CC:DD:EE:FF":  "aa:bb:cc:dd:ee:ff",
		"aa-bb-cc-dd-ee-ff":  "aa:bb:cc:dd:ee:ff",
		"aabb.ccdd.eeff":     "aa:bb:cc:dd:ee:ff",
		" 0A:1b:2C:3d:4E:5f": "0a:1b:2c:3d:4e:5f",
	}
	for in, want := range cases {
		got, err := NormalizeMAC(in)
		if err != nil || got != want {
			t.Errorf("NormalizeMAC(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "zz:zz", "00:00:00:00:00:00:00:e0:00:00:00:00:00:00:00:00:00:00:00:00"} {
		if _, err := NormalizeMAC(bad); err == nil {
			t.Errorf("NormalizeMAC(%q) should fail", bad)
		}
	}
}

func TestIsRandomMAC(t *testing.T) {
	if IsRandomMAC("a4:cb:8f:00:00:01") {
		t.Error("a4:... is a vendor MAC")
	}
	if !IsRandomMAC("da:a1:19:00:00:01") {
		t.Error("da:... has the locally administered bit")
	}
}

func TestNormalizeCIDR(t *testing.T) {
	p, err := NormalizeCIDR("192.168.1.7/24")
	if err != nil || p.String() != "192.168.1.0/24" {
		t.Fatalf("got %v, %v", p, err)
	}
	for _, bad := range []string{"10.0.0.0/8", "fd00::/64", "x", "10.0.0.1/32"} {
		if _, err := NormalizeCIDR(bad); err == nil {
			t.Errorf("NormalizeCIDR(%q) should fail", bad)
		}
	}
}

func TestHosts(t *testing.T) {
	h := Hosts(netip.MustParsePrefix("10.0.0.0/30"))
	if len(h) != 2 || h[0].String() != "10.0.0.1" || h[1].String() != "10.0.0.2" {
		t.Fatalf("got %v", h)
	}
	if n := len(Hosts(netip.MustParsePrefix("192.168.1.0/24"))); n != 254 {
		t.Fatalf("/24 has %d hosts", n)
	}
}

func TestDisplayName(t *testing.T) {
	d := Device{MAC: "aa:bb:cc:dd:ee:ff"}
	if d.DisplayName() != d.MAC {
		t.Error("falls back to the MAC")
	}
	d.Hostname = "printer"
	if d.DisplayName() != "printer" {
		t.Error("hostname over MAC")
	}
	d.Nickname = "Impressora da sala"
	if d.DisplayName() != "Impressora da sala" {
		t.Error("nickname wins")
	}
}

func TestSortIPsAndPickIP(t *testing.T) {
	got := SortIPs([]string{"192.168.1.10", "x", "192.168.1.9", "192.168.1.10"})
	if len(got) != 2 || got[0] != "192.168.1.9" || got[1] != "192.168.1.10" {
		t.Fatalf("SortIPs: %v", got)
	}
	if m, e := PickIP("192.168.1.10", got); m != "192.168.1.10" || len(e) != 1 || e[0] != "192.168.1.9" {
		t.Fatalf("keeps the current IP: %q %v", m, e)
	}
	if m, _ := PickIP("192.168.1.50", got); m != "192.168.1.9" {
		t.Fatalf("lowest when the current one is gone: %q", m)
	}
	if m, e := PickIP("192.168.1.50", nil); m != "192.168.1.50" || e != nil {
		t.Fatalf("no IPs keeps the current: %q %v", m, e)
	}
}
