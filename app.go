package main

import (
	"context"
	"os/exec"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/config"
	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/logx"
	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/service"
)

// App is the Wails-bound facade over the relay service.
type App struct {
	ctx context.Context
	log *logx.Logger
	svc *service.Service

	logMu  sync.Mutex
	logBuf []logx.Entry
}

func NewApp(l *logx.Logger, cfg *config.Store) *App {
	a := &App{log: l}
	a.svc = service.New(l, cfg, a.pushStatus)
	l.OnEntry(func(e logx.Entry) {
		a.logMu.Lock()
		a.logBuf = append(a.logBuf, e)
		a.logMu.Unlock()
	})
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go a.pump()
	a.svc.Start()
}

func (a *App) shutdown(context.Context) {
	a.log.Log(logx.Info, "app", "shutting down")
	a.svc.Stop()
	a.log.Close()
}

func (a *App) pushStatus() {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "status", a.svc.Status())
	}
}

// pump forwards log entries to the UI in batches and refreshes the packet
// counters, which change without a status event.
func (a *App) pump() {
	n := 0
	for range time.Tick(150 * time.Millisecond) {
		a.logMu.Lock()
		batch := a.logBuf
		a.logBuf = nil
		a.logMu.Unlock()
		if len(batch) > 0 {
			runtime.EventsEmit(a.ctx, "log", batch)
		}
		if n++; n%10 == 0 {
			a.pushStatus()
		}
	}
}

// ---- bound methods ----

func (a *App) GetStatus() service.Status { return a.svc.Status() }

func (a *App) GetLogs(since uint64) []logx.Entry { return a.svc.Logs(since) }

func (a *App) SaveSettings(s service.Settings) error { return a.svc.SaveSettings(s) }

func (a *App) SetExported(serial string, exported bool) error {
	err := a.svc.SetExported(serial, exported)
	a.pushStatus()
	return err
}

func (a *App) SetAdvertised(serial string, advertised bool) error {
	err := a.svc.SetAdvertised(serial, advertised)
	a.pushStatus()
	return err
}

func (a *App) OpenLogFolder() {
	exec.Command("explorer", a.log.Dir()).Start()
}
