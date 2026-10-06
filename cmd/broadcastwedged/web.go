package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/va3mw/FlexRadio-Broadcast-Wedge/frontend"
	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/service"
)

// webFlag tells the shared page it is talking to this server, not to the
// desktop app's bridge.
const webFlag = "<head>\n<script>window.WEDGE_WEB = true;</script>"

// newWeb serves the UI and the JSON API behind it.
func newWeb(svc *service.Service) http.Handler {
	page, err := fs.ReadFile(frontend.Assets, "dist/index.html")
	if err != nil {
		panic(err)
	}
	page = bytes.Replace(page, []byte("<head>"), []byte(webFlag), 1)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(page)
	})
	mux.HandleFunc("POST /api/{method}", func(w http.ResponseWriter, r *http.Request) {
		if err := sameSite(r); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		var args []json.RawMessage
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&args); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		out, err := call(svc, r.PathValue("method"), args)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})
	return mux
}

// sameSite rejects requests another web site could make a browser send: the
// page has no login, so a hostile page must not be able to change settings
// through a visitor who happens to be on the same LAN.
func sameSite(r *http.Request) error {
	// A JSON content type cannot be sent cross-site without a preflight,
	// which this server never approves.
	if ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); ct != "application/json" {
		return fmt.Errorf("content type must be application/json")
	}
	if !knownHost(r.Host) {
		return fmt.Errorf("open this page by IP address or by this machine's host name")
	}
	if o := r.Header.Get("Origin"); o != "" {
		if u, err := url.Parse(o); err != nil || !strings.EqualFold(u.Host, r.Host) {
			return fmt.Errorf("cross-site request refused")
		}
	}
	return nil
}

// knownHost reports whether the page was opened by an address that is really
// this machine's. A hostile site can point its own name at a LAN address (DNS
// rebinding); its name then arrives here and is refused.
func knownHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" || net.ParseIP(host) != nil {
		return true
	}
	name, _ := os.Hostname()
	name = strings.ToLower(name)
	return name != "" && (host == name || host == name+".local" || strings.HasPrefix(host, name+"."))
}

// call runs one of the methods the page uses; args are its JSON arguments.
func call(svc *service.Service, method string, args []json.RawMessage) (any, error) {
	arg := func(i int, v any) error {
		if i >= len(args) {
			return fmt.Errorf("%s: missing argument", method)
		}
		if err := json.Unmarshal(args[i], v); err != nil {
			return fmt.Errorf("%s: bad argument", method)
		}
		return nil
	}
	switch method {
	case "GetStatus":
		return svc.Status(), nil
	case "GetLogs":
		var since uint64
		if err := arg(0, &since); err != nil {
			return nil, err
		}
		return svc.Logs(since), nil
	case "SaveSettings":
		var s service.Settings
		if err := arg(0, &s); err != nil {
			return nil, err
		}
		return nil, svc.SaveSettings(s)
	case "SetExported":
		var serial string
		var exported bool
		if err := arg(0, &serial); err != nil {
			return nil, err
		}
		if err := arg(1, &exported); err != nil {
			return nil, err
		}
		return nil, svc.SetExported(serial, exported)
	}
	return nil, fmt.Errorf("unknown method %q", method)
}
