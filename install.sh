#!/usr/bin/env bash
# Suno Camo — развёртывание серверной части одной командой.
#
#   sudo ./install.sh                 # self-signed сертификат (без домена)
#   sudo ./install.sh example.com     # Let's Encrypt на ваш домен
#
# Что делает: ставит shadowsocks-libev + sunocamo-server, выписывает
# сертификат, настраивает systemd, открывает 443/tcp и печатает
# готовые настройки для приложения Shadowsocks.
set -euo pipefail

DOMAIN="${1:-}"
PLUGIN_PORT=443         # наружный порт плагина (имитация HTTPS); ss-server сам уходит на случайный localhost-порт
METHOD="chacha20-ietf-poly1305"
CONF_DIR="/etc/sunocamo"
SS_CONF="/etc/shadowsocks-libev/config.json"
BIN_SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/sunocamo-server"
BIN_DST="/usr/local/bin/sunocamo"

[[ $EUID -eq 0 ]] || { echo "Запустите от root: sudo ./install.sh [домен]"; exit 1; }
[[ -f "$BIN_SRC" ]] || { echo "Не найден $BIN_SRC — положите sunocamo-server рядом со скриптом"; exit 1; }

echo "==> Устанавливаю пакеты"
apt-get update -qq
apt-get install -y -qq shadowsocks-libev openssl curl >/dev/null
if [[ -n "$DOMAIN" ]]; then
    apt-get install -y -qq certbot >/dev/null
fi

echo "==> Устанавливаю бинарник плагина"
install -m 0755 "$BIN_SRC" "$BIN_DST"

echo "==> Сертификат"
mkdir -p "$CONF_DIR"
if [[ -n "$DOMAIN" ]]; then
    certbot certonly --standalone -d "$DOMAIN" --non-interactive --agree-tos \
        --register-unsafely-without-email
    CERT="/etc/letsencrypt/live/$DOMAIN/fullchain.pem"
    KEY="/etc/letsencrypt/live/$DOMAIN/privkey.pem"
else
    openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
        -keyout "$CONF_DIR/key.pem" -out "$CONF_DIR/cert.pem" \
        -subj "/CN=suno.com" 2>/dev/null
    CERT="$CONF_DIR/cert.pem"
    KEY="$CONF_DIR/key.pem"
fi

PASSWORD="$(openssl rand -base64 24 | tr -d '=+/')"

echo "==> Конфиг Shadowsocks ($SS_CONF)"
cat > "$SS_CONF" <<EOF
{
    "server": "0.0.0.0",
    "server_port": $PLUGIN_PORT,
    "password": "$PASSWORD",
    "method": "$METHOD",
    "mode": "tcp_only",
    "plugin": "$BIN_DST",
    "plugin_opts": "server;cert=$CERT;key=$KEY;path=/camo"
}
EOF

echo "==> systemd-юнит для плагина + ssserver"
cat > /etc/systemd/system/sunocamo.service <<EOF
[Unit]
Description=Shadowsocks-libev + Suno Camo plugin
After=network.target

[Service]
ExecStart=/usr/bin/ss-server -c $SS_CONF
Restart=always
RestartSec=3
LimitNOFILE=51200

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now sunocamo.service

echo "==> Файрвол: открываю $PLUGIN_PORT/tcp"
if command -v ufw >/dev/null && ufw status | grep -q "Status: active"; then
    ufw allow "$PLUGIN_PORT/tcp" >/dev/null
fi
if command -v iptables >/dev/null; then
    iptables -C INPUT -p tcp --dport "$PLUGIN_PORT" -j ACCEPT 2>/dev/null || \
    iptables -A INPUT -p tcp --dport "$PLUGIN_PORT" -j ACCEPT
fi

sleep 2
systemctl --no-pager --quiet is-active sunocamo.service \
    && echo "==> Сервис запущен" \
    || { echo "==> СЕРВИС НЕ ПОДНЯЛСЯ, лог:"; journalctl -u sunocamo -n 20 --no-pager; exit 1; }

IP="$(curl -4 -s -m 10 ifconfig.me || echo '<IP-ВАШЕГО-СЕРВЕРА>')"

cat <<EOF

============================================================
 ГОТОВО. Настройки для приложения Shadowsocks на телефоне:
============================================================
 Сервер:            $IP
 Порт:              $PLUGIN_PORT
 Пароль:            $PASSWORD
 Метод шифрования:  $METHOD
 Плагин:            Suno Camo (sunocamo)
 Опции плагина:     $( [[ -n "$DOMAIN" ]] && echo "host=$DOMAIN;path=/camo" || echo "insecure;path=/camo" )

 $( [[ -z "$DOMAIN" ]] && echo "NB: self-signed сертификат — опция 'insecure' обязательна.
 Для полной имитации HTTPS перезапустите: sudo ./install.sh ваш.домен" )
============================================================
EOF
