// Command broadcastwedged is the headless Broadcast Wedge: the same relay as
// the desktop app, run as a service and managed from a web page.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/config"
	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/logx"
	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/service"
)

func main() {
	data := flag.String("data", config.Dir(), "directory for config.json")
	logs := flag.String("logs", "", "directory for log files (default <data>/logs)")
	addr := flag.String("http", ":4997", "address the web page is served on; empty disables it")
	ver := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *ver {
		fmt.Println(service.Version)
		return
	}
	if *logs == "" {
		*logs = filepath.Join(*data, "logs")
	}

	cfg := config.LoadFrom(filepath.Join(*data, "config.json"))
	l := logx.New(*logs)
	// Mirror to stdout so the system journal has the log too.
	l.OnEntry(func(e logx.Entry) {
		if e.Level != "DEBUG" && e.Level != "TRAFFIC" {
			fmt.Printf("%-5s %-6s %s\n", e.Level, e.Source, e.Msg)
		}
	})
	l.Log(logx.Info, "app", "Broadcast Wedge %s starting; settings in %s", service.Version, *data)

	svc := service.New(l, cfg, nil)
	svc.Start()

	var srv *http.Server
	if *addr != "" {
		srv = &http.Server{Addr: *addr, Handler: newWeb(svc), ReadHeaderTimeout: 10 * time.Second}
		go func() {
			l.Log(logx.Info, "web", "status and settings page listening on %s", *addr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				l.Log(logx.Error, "web", "cannot serve the web page on %s: %v", *addr, err)
			}
		}()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	l.Log(logx.Info, "app", "shutting down")
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		srv.Shutdown(ctx)
		cancel()
	}
	svc.Stop()
	l.Close()
}
