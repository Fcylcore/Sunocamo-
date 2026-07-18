# Suno Camo

SIP003-плагин для Shadowsocks: WebSocket-маскировка поверх TLS.
Трафик выглядит как обычное HTTPS-подключение на 443 порту, а пробующий сканер видит лендинг, имитирующий suno.com.

## Принцип

```
Телефон                          VDS
Shadowsocks ──► sunocamo (клиент) ══wss:443══► sunocamo (сервер) ──► ssserver ──► интернет
```

- **Динамические настройки**: клиент берёт IP и порт из полей приложения
  Shadowsocks (переменные SIP003 `SS_REMOTE_HOST` / `SS_REMOTE_PORT`).
  Ничего не зашито в код.
- **Антипробинг**: любой HTTPS-запрос, который не является WebSocket-апгрейдом
  на секретный путь, получает лендинг в стиле suno.com.

## Сборка

```bash
go mod tidy

# Клиент для Android (переименовать в libsunoplugin.so → lib/arm64-v8a/ в APK)
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o libsunoplugin.so .

# Сервер для VDS
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o sunocamo-server .
```

## Сервер (одна команда)

```bash
sudo ./install.sh              # self-signed сертификат
sudo ./install.sh мой.домен    # Let's Encrypt (нужен домен, смотрящий на VDS)
```

Скрипт ставит shadowsocks-libev, плагин, сертификат, systemd-юнит,
открывает 443/tcp и печатает настройки для телефона.

## Клиент (Android)

1. Установите APK-плагин Suno Camo (бинарник внутри как `lib/arm64-v8a/libsunoplugin.so`).
2. В приложении Shadowsocks: сервер = IP/домен VDS, порт = **443**,
   пароль и метод — из вывода install.sh.
3. Плагин: Suno Camo. Опции:
   - с доменом: `host=ваш.домен;path=/camo`
   - без домена (self-signed): `insecure;path=/camo`

## Опции плагина (SS_PLUGIN_OPTIONS, через `;`)

| Опция      | Режим  | Назначение                                      |
|------------|--------|-------------------------------------------------|
| `server`   | сервер | включить серверный режим (ставит install.sh)    |
| `cert=…`   | сервер | путь к fullchain.pem                            |
| `key=…`    | сервер | путь к privkey.pem                              |
| `host=…`   | клиент | SNI/Host для TLS (по умолчанию — адрес сервера) |
| `path=…`   | оба    | секретный WS-путь (по умолчанию `/camo`)        |
| `insecure` | клиент | не проверять сертификат (self-signed)           |

## Лицензия

MIT
