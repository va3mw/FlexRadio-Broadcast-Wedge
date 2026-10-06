// Package logx is a small leveled logger that keeps a ring buffer for the UI
// log window and mirrors every entry to a daily log file.
package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Level int

const (
	Traffic Level = iota // raw relay traffic
	Debug
	Info
	Warn
	Error
)

func (l Level) String() string {
	switch l {
	case Traffic:
		return "TRAFFIC"
	case Debug:
		return "DEBUG"
	case Info:
		return "INFO"
	case Warn:
		return "WARN"
	default:
		return "ERROR"
	}
}

type Entry struct {
	ID     uint64 `json:"id"`
	Time   string `json:"time"`
	Level  string `json:"level"`
	Source string `json:"source"`
	Msg    string `json:"msg"`
}

const ringSize = 5000

type Logger struct {
	mu        sync.Mutex
	ring      []Entry
	nextID    uint64
	file      *os.File
	dir       string
	day       string
	closed    bool
	traffic   bool // capture Traffic level at all
	listeners []func(Entry)
}

func New(dir string) *Logger {
	l := &Logger{dir: dir, ring: make([]Entry, 0, ringSize)}
	_ = os.MkdirAll(dir, 0o755)
	return l
}

func (l *Logger) Dir() string { return l.dir }

// SetTraffic enables or disables capture of raw protocol lines.
func (l *Logger) SetTraffic(on bool) {
	l.mu.Lock()
	l.traffic = on
	l.mu.Unlock()
}

func (l *Logger) TrafficEnabled() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.traffic
}

// OnEntry registers a callback invoked (outside the lock) for each new entry.
func (l *Logger) OnEntry(f func(Entry)) {
	l.mu.Lock()
	l.listeners = append(l.listeners, f)
	l.mu.Unlock()
}

func (l *Logger) Log(lvl Level, src, format string, args ...any) {
	l.mu.Lock()
	if lvl == Traffic && !l.traffic {
		l.mu.Unlock()
		return
	}
	now := time.Now()
	l.nextID++
	e := Entry{
		ID:     l.nextID,
		Time:   now.Format("15:04:05.000"),
		Level:  lvl.String(),
		Source: src,
		Msg:    fmt.Sprintf(format, args...),
	}
	if len(l.ring) == ringSize {
		copy(l.ring, l.ring[1:])
		l.ring = l.ring[:ringSize-1]
	}
	l.ring = append(l.ring, e)
	l.writeFile(now, e)
	listeners := l.listeners
	l.mu.Unlock()
	for _, f := range listeners {
		f(e)
	}
}

func (l *Logger) writeFile(now time.Time, e Entry) {
	if l.closed {
		return
	}
	day := now.Format("2006-01-02")
	if l.file == nil || day != l.day {
		if l.file != nil {
			l.file.Close()
		}
		f, err := os.OpenFile(filepath.Join(l.dir, "BroadcastWedge-"+day+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			l.file = nil
			return
		}
		l.file, l.day = f, day
	}
	fmt.Fprintf(l.file, "%s %-7s %-10s %s\n", e.Time, e.Level, e.Source, e.Msg)
}

// Since returns entries with ID greater than id.
func (l *Logger) Since(id uint64) []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := []Entry{}
	for _, e := range l.ring {
		if e.ID > id {
			out = append(out, e)
		}
	}
	return out
}

func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
}

// Src returns a logger bound to a source tag.
func (l *Logger) Src(src string) *Source { return &Source{l: l, src: src} }

type Source struct {
	l   *Logger
	src string
}

func (s *Source) Traffic(f string, a ...any) { s.l.Log(Traffic, s.src, f, a...) }
func (s *Source) Debug(f string, a ...any)   { s.l.Log(Debug, s.src, f, a...) }
func (s *Source) Info(f string, a ...any)    { s.l.Log(Info, s.src, f, a...) }
func (s *Source) Warn(f string, a ...any)    { s.l.Log(Warn, s.src, f, a...) }
func (s *Source) Error(f string, a ...any)   { s.l.Log(Error, s.src, f, a...) }
func (s *Source) TrafficOn() bool            { return s.l.TrafficEnabled() }
