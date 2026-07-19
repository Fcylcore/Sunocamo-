# Suno Camo

**SIP003 plugin for Shadowsocks: WebSocket-over-TLS camouflage.**
Your proxy traffic looks like ordinary HTTPS on port 443 — and an active prober gets a suno.com-style landing page instead of a proxy.

[English](#english) | [Русский](#русский)

---

## English

### Why

DPI systems don't just block proxies — they *probe* them. Suno Camo answers every probe with a believable HTTPS landing page, and only speaks WebSocket proxy protocol on a secret path to clients that already know where to knock.

### How it works

```
Phone                                 VPS
Shadowsocks ──► sunocamo (client) ══wss:443══► sunocamo (server) ──► ss-server ──► internet
```

- **Nothing hardcoded** — the client takes host/port from the Shadowsocks app fields via SIP003 (`SS_REMOTE_HOST` / `SS_REMOTE_PORT`).
- **Anti-probing**: any HTTPS request that is not a WebSocket upgrade to the secret path receives a suno.com-style landing page.
- **TLS for real**: either a self-signed cert (quick start) or a proper Let's Encrypt certificate (full browser-grade handshake with SNI).

### Server — one command

```bash
git clone https://github.com/Fcylcore/Sunocamo-.git && cd Sunocamo-
sudo ./install.sh              # self-signed certificate
sudo ./install.sh my.domain    # Let's Encrypt (domain must point to your VPS)
```

The script installs shadowsocks-libev + the plugin, issues a certificate, creates a systemd unit, opens 443/tcp and prints the phone settings.

### Client (Android)

1. Install the **Suno Camo** plugin APK (from this repo).
2. In the Shadowsocks app: server = VPS IP/domain, port **443**, password & method — from the `install.sh` output.
3. Plugin: **Suno Camo**. Options:
   - with a domain: `host=my.domain;path=/camo`
   - without (self-signed): `insecure;path=/camo`

### Plugin options (`SS_PLUGIN_OPTIONS`, `;`-separated)

| Option     | Side   | Meaning                                          |
|------------|--------|--------------------------------------------------|
| `server`   | server | enable server mode (set by install.sh)           |
| `cert=…`   | server | path to fullchain.pem                            |
| `key=…`    | server | path to privkey.pem                              |
| `host=…`   | client | SNI/Host for TLS (default: server address)       |
| `path=…`   | both   | secret WebSocket path (default `/camo`)          |
| `insecure` | client | skip certificate verification (self-signed only) |

### Build from source

Requires Go 1.23+. Single dependency: `gorilla/websocket`.

```bash
go mod tidy

# Android client (rename to libsunoplugin.so → lib/arm64-v8a/ inside the APK)
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o libsunoplugin.so .

# Linux server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o sunocamo-server .
```

### Honest limitations

- **TCP only** (no UDP — voice calls over the tunnel won't work).
- Camouflage raises the bar for DPI, it doesn't make you invisible. Active probing resistance depends on keeping the WS path secret.
- Built for personal use on your own VPS. Not audited — read the code (it's small on purpose, ~300 lines).

### License

MIT

---

## Русский

SIP003-плагин для Shadowsocks: WebSocket-маскировка поверх TLS. Трафик выглядит как обычное HTTPS-подключение на 443 порту, а пробующий сканер видит лендинг, имитирующий suno.com.

- **Одна команда на сервере**: `sudo ./install.sh` (self-signed) или `sudo ./install.sh мой.домен` (Let's Encrypt). Скрипт ставит shadowsocks-libev, плагин, сертификат, systemd-юнит, открывает 443/tcp и печатает настройки для телефона.
- **Клиент**: APK-плагин Suno Camo + приложение Shadowsocks (сервер, порт 443, пароль из install.sh). Опции плагина: `host=ваш.домен;path=/camo` или `insecure;path=/camo` для self-signed.
- **Принцип**: любой HTTPS-запрос, который не является WebSocket-апгрейдом на секретный путь, получает лендинг. Прокси-протокол начинается только после правильного «стука».

Подробности — в английской части выше. Лицензия: MIT.
