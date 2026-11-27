package oui

import "testing"

func TestEmbeddedRegistry(t *testing.T) {
	db, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if db.Len() < 30000 {
		t.Fatalf("only %d prefixes", db.Len())
	}
	// Espressif (ESP32 boards) and Raspberry Pi
	for mac, want := range map[string]string{
		"a4:cb:8f:00:00:01": "Espressif",
		"e4:5f:01:00:00:01": "Raspberry Pi Trading",
	} {
		if got := db.Vendor(mac); got != want {
			t.Errorf("Vendor(%s) = %q, want %q", mac, got, want)
		}
	}
	if got := db.Vendor("da:a1:19:00:00:01"); got != "" {
		t.Errorf("a random MAC has no vendor, got %q", got)
	}
	if got := db.Vendor("not a mac"); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestLongestPrefixWins(t *testing.T) {
	db := &DB{byLen: map[int]map[string]string{
		6: {"001BC5": "Big Block"},
		7: {},
		9: {"001BC5001": "Small Block"},
	}}
	if got := db.Vendor("00:1b:c5:00:10:00"); got != "Small Block" {
		t.Errorf("got %q", got)
	}
	if got := db.Vendor("00:1b:c5:99:00:00"); got != "Big Block" {
		t.Errorf("got %q", got)
	}
}
