package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/renatotecchio/vartija/backend/internal/domain"
	"github.com/renatotecchio/vartija/backend/internal/ports"
)

func TestNewDeviceWithBotToken(t *testing.T) {
	var got map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	n, err := New("xoxb-test", "#vartija", "", "https://vartija.example", "pt")
	if err != nil {
		t.Fatal(err)
	}
	n.api = srv.URL
	err = n.NewDevice(context.Background(), ports.NewDeviceAlert{
		Device: domain.Device{MAC: "da:a1:19:00:00:01", IP: "192.168.1.50", Hostname: "<b>phone", HostnameSource: "dhcp", Vendor: "Apple", RandomMAC: true},
		Subnet: domain.Subnet{CIDR: "192.168.1.0/24", Name: "LAN"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer xoxb-test" || got["channel"] != "#vartija" {
		t.Errorf("auth %q, payload %v", auth, got)
	}
	text, _ := got["text"].(string)
	for _, want := range []string{"da:a1:19:00:00:01", "192.168.1.50", "&lt;b&gt;phone", "LAN (192.168.1.0/24)", "MAC aleatório", "*Fabricante:* Apple", "Revisar no Vartija", "https://vartija.example/?status=new"} {
		if !strings.Contains(text, want) {
			t.Errorf("text misses %q:\n%s", want, text)
		}
	}
}

func TestSlackAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"error":"not_in_channel"}`))
	}))
	defer srv.Close()
	n, _ := New("xoxb-test", "#vartija", "", "", "en")
	n.api = srv.URL
	err := n.Baseline(context.Background(), ports.BaselineSummary{Subnet: domain.Subnet{CIDR: "10.0.0.0/24"}, Devices: 3})
	if err == nil || !strings.Contains(err.Error(), "not_in_channel") {
		t.Fatalf("want not_in_channel, got %v", err)
	}
}

func TestNewNeedsConfig(t *testing.T) {
	if _, err := New("", "", "", "", "en"); err == nil {
		t.Fatal("want an error")
	}
	if _, err := New("x", "#c", "", "", "fi"); err == nil {
		t.Fatal("want an error")
	}
}

func TestEnglishBaseline(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	n, _ := New("xoxb-test", "#vartija", "", "", "en")
	n.api = srv.URL
	if err := n.Baseline(context.Background(), ports.BaselineSummary{Subnet: domain.Subnet{CIDR: "10.0.0.0/24"}, Devices: 3}); err != nil {
		t.Fatal(err)
	}
	if text, _ := got["text"].(string); !strings.Contains(text, "First scan of 10.0.0.0/24:* 3 devices") {
		t.Errorf("text: %q", text)
	}
}
