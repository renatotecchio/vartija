# vartija

*[Português](README.pt-BR.md)*

*Vartija* is Finnish for "sentinel". It watches the subnets you choose and alerts on Slack when a new MAC address shows up.

- **Periodic sweeps** of several IPv4 subnets, from /16 to /30, every 5 minutes by default.
- **Baseline.** The first scan of each subnet records every device as *new* and sends **one** summary instead of an alert per device. You mark the ones you know as known, one by one or all at once. From then on, every new MAC raises an alert.
- **Device name** from the network, taken from the first source that answers:
  1. the MikroTik router's DHCP lease;
  2. reverse DNS;
  3. mDNS;
  4. NetBIOS.
- **Vendor** from the MAC prefix, using the IEEE registry embedded in the binary. It helps identify devices without a name.
- **Your own nickname**, shown instead of the hostname everywhere in the UI, plus notes.
- **Random MACs**, the locally administered addresses phones and laptops make up per network, are flagged. They have no vendor.
- **Several IPs per MAC** (secondary addresses, load balancer VIPs). The device keeps its main IP while it still answers on it, and lists the others.
- **History** of events: new device, IP change, marked as known, scan failure.
- **UI in English and Portuguese**, with a light, dark or system theme. Each browser remembers its own choice.
- **Built-in login:** an htpasswd file (bcrypt) and/or one admin from the environment. No SSO needed.

## How MACs are found

| Situation | How |
|---|---|
| The Vartija host has an interface on the subnet | **ARP sweep**: one ARP request per address over a raw socket. It finds everything that is on, even devices that stay quiet. |
| No interface there, but a MikroTik router has one | **The router's ARP table and DHCP leases**, through the API (api-ssl, read-only user). Only lists devices that talked recently. |

ARP doesn't cross routers. So the direct sweep only works on the networks the host sits on, and in Docker the container needs the **host network** and the **NET_RAW** capability.

## Architecture

```
frontend/   React 18 + Vite + TypeScript + Tailwind CSS
backend/    Go + Gin + GORM (PostgreSQL 16), hexagonal:
  internal/domain     entities and rules (Device, Subnet, Event)
  internal/ports      interfaces the core depends on (repositories, Discoverer, HostnameResolver, Notifier, VendorLookup)
  internal/app        use cases (scan, review, subnets, login, scheduler)
  internal/adapters   postgres, arp, mikrotik, hostname, oui, slack, httpapi
```

The frontend is built into the Go binary. One image serves the API and the UI on port 8090.

## Running locally

```sh
cp .env.example .env     # fill in POSTGRES_PASSWORD, VARTIJA_ADMIN_PASSWORD and VARTIJA_SESSION_SECRET
docker compose up -d --build
# http://localhost:8090
```

To work on the frontend with hot reload:
1. Run the backend with compose.
2. Run `cd frontend && npm run dev`. Vite proxies `/api` to port 8090.

Other targets:
- `make test` runs the backend tests and the frontend typecheck.
- `make oui` refreshes the IEEE vendor registry (`backend/internal/adapters/oui/oui.tsv.gz`).

## Configuration

Everything is set through environment variables. [.env.example](.env.example) lists them all, with comments.

| Variable | Default | |
|---|---|---|
| `VARTIJA_DATABASE_URL` | required | `postgres://user:pass@host:5432/vartija?sslmode=...` |
| `VARTIJA_SESSION_SECRET` | required | 32+ random characters; signs the session cookie |
| `VARTIJA_ADMIN_USER` / `VARTIJA_ADMIN_PASSWORD` | `admin` / — | admin from the environment (password with 12+ characters) |
| `VARTIJA_HTPASSWD_FILE` | — | htpasswd file, bcrypt only (`htpasswd -nB user`) |
| `VARTIJA_LISTEN` | `:8090` | |
| `VARTIJA_SCAN_INTERVAL` | `5m` | at least `1m` |
| `VARTIJA_EVENT_RETENTION` | `2160h` (90 days) | |
| `VARTIJA_SESSION_TTL` | `12h` | |
| `VARTIJA_SECURE_COOKIE` | `true` | `false` only for plain HTTP on localhost |
| `VARTIJA_TRUSTED_PROXIES` | — | reverse proxy IPs; the login rate limit is per client IP |
| `VARTIJA_BASE_URL` | — | link in the alerts |
| `VARTIJA_INTERFACES` | all | limits the ARP sweep to these interfaces |
| `VARTIJA_DNS_SERVER` | system resolver | server for reverse DNS |
| `VARTIJA_DNS_STRIP_SUFFIXES` | — | suffixes removed from names (`lan,home.arpa`) |
| `VARTIJA_RESOLVE_MDNS` / `VARTIJA_RESOLVE_NETBIOS` | `true` | |
| `VARTIJA_MIKROTIK_ROUTERS` | — | `name=host[:port],...` (default port 8729, api-ssl) |
| `VARTIJA_MIKROTIK_USER` / `VARTIJA_MIKROTIK_PASSWORD` | — | read-only user |
| `VARTIJA_SLACK_BOT_TOKEN` + `VARTIJA_SLACK_CHANNEL` | — | bot with `chat:write`, invited to the channel |
| `VARTIJA_SLACK_WEBHOOK_URL` | — | instead of the bot |
| `VARTIJA_LANGUAGE` | `en` | language of the Slack messages: `en` or `pt` |

To make an htpasswd line without `htpasswd` installed:

```sh
echo -n 'a-long-password' | docker run --rm -i ghcr.io/renatotecchio/vartija hash-password alice
```

## Security

- **Session:** an `HttpOnly`, `SameSite=Lax` cookie, signed with HMAC-SHA256.
- **CSRF:** every request that changes something needs the `X-Vartija` header.
- **Login rate limit:** 10 wrong passwords within 15 minutes lock that IP out.
- **UI:** strict Content Security Policy.
- **Container:** every capability is dropped except `NET_RAW`, with a read-only filesystem and `no-new-privileges`.

## Release

A `vX.Y.Z` tag publishes `ghcr.io/renatotecchio/vartija:X.Y.Z` and `:latest` (`.github/workflows/release.yml`).

## License

[MIT](LICENSE)
