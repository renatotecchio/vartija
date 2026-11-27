# vartija

*Vartija* é "sentinela" em finlandês. É um software que vigia as sub-redes que você escolher e avisa no Slack quando um MAC address novo aparece.

- **Varredura periódica** de várias sub-redes IPv4, de /16 a /30, a cada 5 min por padrão.
- **Linha de base:** a primeira varredura de cada sub-rede registra todo mundo como *novo* e manda **um** resumo, não um alerta por dispositivo. Você marca os que conhece (um a um ou todos de uma vez). Depois disso, todo MAC novo gera alerta.
- **Nome do dispositivo** vindo da rede, na primeira fonte que responder:
  1. lease DHCP do roteador MikroTik;
  2. DNS reverso;
  3. mDNS;
  4. NetBIOS.
- **Apelido seu**, que aparece no lugar do hostname em toda a interface, e notas.
- **MAC aleatório:** endereços administrados localmente, o MAC privado que celulares e notebooks inventam, são marcados como tal.
- **Histórico** de eventos: novo dispositivo, troca de IP, marcado como conhecido, falhas de varredura.
- **Login próprio:** um arquivo htpasswd (bcrypt) e/ou um admin por variável de ambiente. Não depende de SSO.

## Como os MACs são descobertos

| Situação | Como |
|---|---|
| O host do Vartija tem interface na sub-rede | **Varredura ARP**: um pedido ARP para cada endereço, pelo socket raw. Acha tudo o que está ligado, mesmo calado. |
| Não tem interface, mas um MikroTik tem | **Tabela ARP e leases DHCP do roteador**, pela API (api-ssl, usuário só de leitura). Só lista quem conversou recentemente. |

ARP não atravessa roteador. Por isso a varredura direta só funciona nas redes onde o host está, e no Docker o container precisa da **rede do host** e da capability **NET_RAW**.

## Arquitetura

```
frontend/   React 18 + Vite + TypeScript + Tailwind CSS
backend/    Go + Gin + GORM (PostgreSQL 16), hexagonal:
  internal/domain     entidades e regras (Device, Subnet, Event)
  internal/ports      interfaces que o núcleo usa (repositórios, Discoverer, HostnameResolver, Notifier)
  internal/app        casos de uso (varredura, revisão, sub-redes, login, agendador)
  internal/adapters   postgres, arp, mikrotik, hostname, slack, httpapi
```

O frontend é compilado e embutido no binário Go. Uma imagem só serve a API e a interface na porta 8090.

## Rodando localmente

```sh
cp .env.example .env     # preencha POSTGRES_PASSWORD, VARTIJA_ADMIN_PASSWORD e VARTIJA_SESSION_SECRET
docker compose up -d --build
# http://localhost:8090
```

Para desenvolver o frontend com hot reload, rode o backend pelo compose e depois `cd frontend && npm run dev`. O Vite faz proxy de `/api` para a porta 8090.

`make test` roda os testes do backend e o typecheck do frontend.

## Configuração

Tudo por variável de ambiente. O [.env.example](.env.example) traz todas comentadas.

| Variável | Padrão | |
|---|---|---|
| `VARTIJA_DATABASE_URL` | obrigatória | `postgres://user:pass@host:5432/vartija?sslmode=...` |
| `VARTIJA_SESSION_SECRET` | obrigatória | 32+ caracteres aleatórios; assina o cookie de sessão |
| `VARTIJA_ADMIN_USER` / `VARTIJA_ADMIN_PASSWORD` | `admin` / — | admin por variável (senha com 12+ caracteres) |
| `VARTIJA_HTPASSWD_FILE` | — | arquivo htpasswd, só bcrypt (`htpasswd -nB usuario`) |
| `VARTIJA_LISTEN` | `:8090` | |
| `VARTIJA_SCAN_INTERVAL` | `5m` | mínimo `1m` |
| `VARTIJA_EVENT_RETENTION` | `2160h` (90 dias) | |
| `VARTIJA_SESSION_TTL` | `12h` | |
| `VARTIJA_SECURE_COOKIE` | `true` | `false` só em HTTP local |
| `VARTIJA_TRUSTED_PROXIES` | — | IPs do proxy reverso; o limite de tentativas de login é por IP do cliente |
| `VARTIJA_BASE_URL` | — | link nos alertas |
| `VARTIJA_INTERFACES` | todas | restringe a varredura ARP a essas interfaces |
| `VARTIJA_DNS_SERVER` | resolvedor do sistema | servidor para o DNS reverso |
| `VARTIJA_DNS_STRIP_SUFFIXES` | — | sufixos tirados dos nomes (`lan,home.arpa`) |
| `VARTIJA_RESOLVE_MDNS` / `VARTIJA_RESOLVE_NETBIOS` | `true` | |
| `VARTIJA_MIKROTIK_ROUTERS` | — | `nome=host[:porta],...` (porta padrão 8729, api-ssl) |
| `VARTIJA_MIKROTIK_USER` / `VARTIJA_MIKROTIK_PASSWORD` | — | usuário só de leitura |
| `VARTIJA_SLACK_BOT_TOKEN` + `VARTIJA_SLACK_CHANNEL` | — | bot com `chat:write`, convidado para o canal |
| `VARTIJA_SLACK_WEBHOOK_URL` | — | alternativa ao bot |

Para gerar uma linha do htpasswd sem o `htpasswd` instalado:

```sh
echo -n 'senha-longa-aqui' | docker run --rm -i ghcr.io/renatotecchio/vartija hash-password renato
```

## Segurança

- Sessão em cookie `HttpOnly` e `SameSite=Lax`, assinado com HMAC-SHA256.
- Toda requisição que altera algo exige o header `X-Vartija`, como proteção contra CSRF.
- Depois de 10 senhas erradas em 15 min, o IP é bloqueado.
- A interface tem CSP restritiva.
- O container roda com todas as capabilities removidas, menos `NET_RAW`, com o sistema de arquivos só de leitura e `no-new-privileges`.

## Release

Uma tag `vX.Y.Z` publica `ghcr.io/renatotecchio/vartija:X.Y.Z` e `:latest` (`.github/workflows/release.yml`).

## Licença

[MIT](LICENSE)
