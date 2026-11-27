// Package slack sends Vartija's alerts to Slack, with a bot token
// (chat.postMessage to a channel) or an incoming webhook.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/renatotecchio/vartija/backend/internal/ports"
)

// Notifier implements ports.Notifier.
type Notifier struct {
	token   string
	channel string
	webhook string
	// BaseURL of the Vartija UI, linked from the messages ("" = no link).
	baseURL string
	text    texts
	http    *http.Client
	api     string
}

// texts are the message strings of one language.
type texts struct {
	newDevice, mac, ip, name, vendor, network, noName, randomMAC, review string
	baseline, open                                                       string
}

var languages = map[string]texts{
	"en": {
		newDevice: ":rotating_light: *New device on the network*",
		mac:       "MAC", ip: "IP", name: "Name", vendor: "Vendor", network: "Network",
		noName:    "no name",
		randomMAC: "_Random MAC: probably a phone or laptop using a private address._",
		review:    "Review in Vartija",
		baseline: ":mag: *First scan of %s:* %d devices found. They don't raise one alert each; " +
			"mark the ones you know as known.",
		open: "Open Vartija",
	},
	"pt": {
		newDevice: ":rotating_light: *Novo dispositivo na rede*",
		mac:       "MAC", ip: "IP", name: "Nome", vendor: "Fabricante", network: "Rede",
		noName:    "sem nome",
		randomMAC: "_MAC aleatório: provavelmente um celular ou notebook com endereço privado._",
		review:    "Revisar no Vartija",
		baseline: ":mag: *Primeira varredura de %s:* %d dispositivos encontrados. " +
			"Eles não geram alerta um a um; marque os que você conhece como conhecidos.",
		open: "Abrir o Vartija",
	},
}

// Languages lists the supported message languages.
func Languages() []string { return []string{"en", "pt"} }

// New returns a notifier. Set token and channel, or webhook. lang is "en"
// or "pt".
func New(token, channel, webhook, baseURL, lang string) (*Notifier, error) {
	if webhook == "" && (token == "" || channel == "") {
		return nil, errors.New("slack: set the bot token and the channel, or a webhook URL")
	}
	t, ok := languages[lang]
	if !ok {
		return nil, fmt.Errorf("slack: unsupported language %q (en, pt)", lang)
	}
	return &Notifier{
		token: token, channel: channel, webhook: webhook,
		baseURL: strings.TrimRight(baseURL, "/"),
		text:    t,
		http:    &http.Client{Timeout: 10 * time.Second},
		api:     "https://slack.com/api/chat.postMessage",
	}, nil
}

// NewDevice implements ports.Notifier.
func (n *Notifier) NewDevice(ctx context.Context, a ports.NewDeviceAlert) error {
	d, t := a.Device, n.text
	name := d.Hostname
	if name == "" {
		name = t.noName
	}
	if d.HostnameSource != "" {
		name = escape(name) + " _(" + d.HostnameSource + ")_"
	} else {
		name = escape(name)
	}
	subnet := a.Subnet.CIDR
	if a.Subnet.Name != "" {
		subnet = a.Subnet.Name + " (" + a.Subnet.CIDR + ")"
	}
	lines := []string{
		fmt.Sprintf("*%s:* `%s`", t.mac, d.MAC),
		fmt.Sprintf("*%s:* %s", t.ip, d.IP),
		fmt.Sprintf("*%s:* %s", t.name, name),
	}
	if d.Vendor != "" {
		lines = append(lines, fmt.Sprintf("*%s:* %s", t.vendor, escape(d.Vendor)))
	}
	lines = append(lines, fmt.Sprintf("*%s:* %s", t.network, escape(subnet)))
	if d.RandomMAC {
		lines = append(lines, t.randomMAC)
	}
	text := t.newDevice + "\n" + strings.Join(lines, "\n")
	if n.baseURL != "" {
		text += fmt.Sprintf("\n<%s/?status=new|%s>", n.baseURL, t.review)
	}
	return n.post(ctx, text)
}

// Baseline implements ports.Notifier.
func (n *Notifier) Baseline(ctx context.Context, s ports.BaselineSummary) error {
	text := fmt.Sprintf(n.text.baseline, escape(s.Subnet.CIDR), s.Devices)
	if n.baseURL != "" {
		text += fmt.Sprintf(" <%s/?status=new|%s>", n.baseURL, n.text.open)
	}
	return n.post(ctx, text)
}

// escape keeps Slack from reading names as markup or links.
func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func (n *Notifier) post(ctx context.Context, text string) error {
	if n.webhook != "" {
		return n.send(ctx, n.webhook, map[string]any{"text": text}, false)
	}
	return n.send(ctx, n.api, map[string]any{"channel": n.channel, "text": text, "unfurl_links": false}, true)
}

func (n *Notifier) send(ctx context.Context, url string, payload map[string]any, bot bool) error {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if bot {
		req.Header.Set("Authorization", "Bearer "+n.token)
	}
	resp, err := n.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("slack: HTTP %d", resp.StatusCode)
	}
	if !bot {
		return nil
	}
	// The Web API answers 200 with ok=false on errors (not_in_channel...)
	var r struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("slack: %w", err)
	}
	if !r.OK {
		return fmt.Errorf("slack: %s", r.Error)
	}
	return nil
}
