// Suno Camo — SIP003-плагин для Shadowsocks.
// WebSocket-маскировка поверх TLS (имитация HTTPS на порту 443).
//
// Клиент (на телефоне, запускает shadowsocks-android):
//   Читает SS_REMOTE_HOST / SS_REMOTE_PORT прямо из полей приложения —
//   никаких зашитых адресов. Слушает SS_LOCAL_HOST:SS_LOCAL_PORT и
//   заворачивает трафик в wss://<поле_сервера>:<поле_порта>/.
//
// Сервер (на VDS, запускает ssserver):
//   Принимает wss на :443, любой другой HTTPS-запрос получает
//   обычную веб-страницу (защита от активного пробинга).
//
// Опции (SS_PLUGIN_OPTIONS, через «;»):
//   Общие:    host=example.com  path=/camo
//   Клиент:   insecure          (пропуск проверки сертификата, для self-signed)
//   Сервер:   server (обязательно!)  cert=/path/cert.pem  key=/path/key.pem
package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const defaultPath = "/camo"

// ---------- SIP003 окружение ----------

type sipEnv struct {
	remoteHost string // куда подключаться (клиент) / куда форвардить (сервер)
	remotePort string
	localHost  string // где слушать
	localPort  string
	options    string
}

func readEnv() (*sipEnv, error) {
	e := &sipEnv{
		remoteHost: os.Getenv("SS_REMOTE_HOST"),
		remotePort: os.Getenv("SS_REMOTE_PORT"),
		localHost:  os.Getenv("SS_LOCAL_HOST"),
		localPort:  os.Getenv("SS_LOCAL_PORT"),
		options:    os.Getenv("SS_PLUGIN_OPTIONS"),
	}
	if e.remoteHost == "" || e.remotePort == "" || e.localPort == "" {
		return nil, fmt.Errorf("SIP003 env incomplete (SS_REMOTE_HOST=%q SS_REMOTE_PORT=%q SS_LOCAL_PORT=%q) — плагин должен запускаться Shadowsocks",
			e.remoteHost, e.remotePort, e.localPort)
	}
	if e.localHost == "" {
		e.localHost = "127.0.0.1"
	}
	return e, nil
}

// ---------- Разбор опций ----------

type options struct {
	host     string // SNI / Host-заголовок (клиент), домен (сервер)
	path     string
	insecure bool
	isServer bool
	certFile string
	keyFile  string
}

func parseOptions(raw string, args []string) *options {
	o := &options{path: defaultPath}
	tokens := strings.Split(raw, ";")
	tokens = append(tokens, args[1:]...)
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		switch {
		case t == "":
		case t == "server":
			o.isServer = true
		case t == "insecure":
			o.insecure = true
		case strings.HasPrefix(t, "host="):
			o.host = strings.TrimPrefix(t, "host=")
		case strings.HasPrefix(t, "path="):
			o.path = strings.TrimPrefix(t, "path=")
		case strings.HasPrefix(t, "cert="):
			o.certFile = strings.TrimPrefix(t, "cert=")
		case strings.HasPrefix(t, "key="):
			o.keyFile = strings.TrimPrefix(t, "key=")
		default:
			log.Printf("sunocamo: неизвестная опция %q (пропускаю)", t)
		}
	}
	if !strings.HasPrefix(o.path, "/") {
		o.path = "/" + o.path
	}
	return o
}

// ---------- Общая склейка потоков ----------

func relay(a, b net.Conn) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		io.Copy(dst, src)
		dst.SetReadDeadline(time.Now()) // будим вторую горутину
		done <- struct{}{}
	}
	go cp(a, b)
	go cp(b, a)
	<-done
	a.Close()
	b.Close()
}

// ---------- Клиентский режим ----------

func runClient(e *sipEnv, o *options) {
	sni := o.host
	if sni == "" {
		sni = e.remoteHost
	}
	dialer := websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
		TLSClientConfig: &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: o.insecure, //nolint:gosec // осознанная опция для self-signed
			MinVersion:         tls.VersionTLS12,
		},
		NetDialContext: (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
	}

	ln, err := net.Listen("tcp", net.JoinHostPort(e.localHost, e.localPort))
	if err != nil {
		log.Fatalf("sunocamo: listen %s:%s: %v", e.localHost, e.localPort, err)
	}
	log.Printf("sunocamo: клиент слушает %s:%s -> wss://%s:%s%s (SNI=%s)",
		e.localHost, e.localPort, e.remoteHost, e.remotePort, o.path, sni)

	for {
		c, err := ln.Accept()
		if err != nil {
			log.Printf("sunocamo: accept: %v", err)
			continue
		}
		go handleClientConn(c, &dialer, e, o, sni)
	}
}

func handleClientConn(local net.Conn, dialer *websocket.Dialer, e *sipEnv, o *options, sni string) {
	url := fmt.Sprintf("wss://%s:%s%s", e.remoteHost, e.remotePort, o.path)
	hdr := http.Header{"Host": []string{sni}}
	ws, resp, err := dialer.Dial(url, hdr)
	if err != nil {
		if resp != nil {
			log.Printf("sunocamo: handshake %s: %v (HTTP %s)", url, err, resp.Status)
		} else {
			log.Printf("sunocamo: dial %s: %v", url, err)
		}
		local.Close()
		return
	}
	relay(local, ws.NetConn())
}

// ---------- Серверный режим ----------

// Страница-прикрытие: отдаётся на любой обычный HTTPS-запрос (пробинг).
// Имитация лендинга suno.com — для DPI и сканеров это музыкальный сервис.
const camoPage = `<!DOCTYPE html>
<html lang="en"><head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Suno</title>
<meta name="description" content="Suno is building a future where anyone can make great music.">
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{background:#0e0e10;color:#fff;font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;display:flex;flex-direction:column;align-items:center;justify-content:center;min-height:100vh;text-align:center;padding:2em}
.logo{font-size:3.2em;font-weight:800;letter-spacing:-.02em;margin-bottom:.4em}
.tag{color:#a7a7ad;font-size:1.15em;max-width:34em;line-height:1.5;margin-bottom:2em}
.btn{background:#fff;color:#0e0e10;border-radius:999px;padding:.85em 2.2em;font-weight:600;text-decoration:none;font-size:1em}
.btn2{color:#fff;border:1px solid #3a3a40;border-radius:999px;padding:.85em 2.2em;font-weight:600;text-decoration:none;margin-left:.8em}
.foot{position:fixed;bottom:1.4em;color:#55555c;font-size:.8em}
</style>
</head><body>
<div class="logo">Suno</div>
<p class="tag">Make any song you can imagine. No instrument needed, just imagination.</p>
<div><a class="btn" href="/create">Create</a><a class="btn2" href="/explore">Explore</a></div>
<div class="foot">&copy; 2026 Suno, Inc.</div>
</body></html>`

func runServer(e *sipEnv, o *options) {
	if o.certFile == "" || o.keyFile == "" {
		log.Fatal("sunocamo: серверному режиму нужны cert= и key= в опциях")
	}
	upgrader := websocket.Upgrader{
		ReadBufferSize:  32 * 1024,
		WriteBufferSize: 32 * 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}
	target := net.JoinHostPort(e.remoteHost, e.remotePort)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Валидный WebSocket на секретном пути — это наш клиент.
		if r.URL.Path == o.path &&
			strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			ws, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Printf("sunocamo: upgrade: %v", err)
				return
			}
			backend, err := net.DialTimeout("tcp", target, 10*time.Second)
			if err != nil {
				log.Printf("sunocamo: dial backend %s: %v", target, err)
				ws.Close()
				return
			}
			go relay(backend, ws.NetConn())
			return
		}
		// Всё остальное — маскируемся под обычный nginx.
		w.Header().Set("Server", "nginx")
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, camoPage)
	})

	addr := net.JoinHostPort(e.localHost, e.localPort)
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("sunocamo: сервер слушает wss %s -> %s (path=%s)", addr, target, o.path)
	if err := srv.ListenAndServeTLS(o.certFile, o.keyFile); err != nil {
		log.Fatalf("sunocamo: serve tls: %v", err)
	}
}

// ---------- main ----------

func main() {
	log.SetPrefix("[sunocamo] ")
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)

	e, err := readEnv()
	if err != nil {
		log.Fatal(err)
	}
	o := parseOptions(e.options, os.Args)

	// Аккуратно умираем вместе с Shadowsocks.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() { <-sig; os.Exit(0) }()

	if o.isServer {
		runServer(e, o)
	} else {
		runClient(e, o)
	}
}
