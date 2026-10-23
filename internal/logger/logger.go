// Package logger menyediakan logging sederhana dengan format yang sama
// seperti konfigurasi winston pada proyek Node.js:
//
//	" [LOGGER] 2026-10-23 03:15:00.000 PM info : message"
//
// Output dilakukan ke console (semua level, berwarna) dan ke file log
// harian ./logs/app-YYYY-MM-DD.log (level info ke atas, di-rotate/compress
// dan dibersihkan setelah 14 hari) seperti winston-daily-rotate-file.
package logger

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Level menentukan tingkat log, sejajar dengan level winston.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "silly"
	}
}

// ANSI color untuk console (meniru winston colorize).
func (l Level) color() int {
	switch l {
	case LevelDebug:
		return 90
	case LevelInfo:
		return 32
	case LevelWarn:
		return 33
	case LevelError:
		return 31
	default:
		return 37
	}
}

// rotateWriter adalah pengganti winston-daily-rotate-file:
//   - nama file: app-YYYY-MM-DD.log (rotasi saat tanggal berganti)
//   - maxSize 1MB, saat dilewati file di-compress jadi app-YYYY-MM-DD.HHmmss.log.gz
//   - zippedArchive: true, maxFiles: 14d (file lebih tua dari 14 hari dihapus)
type rotateWriter struct {
	mu       sync.Mutex
	dir      string
	prefix   string
	maxSize  int64
	maxAge   time.Duration
	compress bool
	f        *os.File
	size     int64
	date     string
}

func newRotateWriter(dir string) *rotateWriter {
	return &rotateWriter{
		dir:      dir,
		prefix:   "app",
		maxSize:  1 << 20, // 1MB, sama seperti maxSize: "1m"
		maxAge:   14 * 24 * time.Hour,
		compress: true,
	}
}

func (w *rotateWriter) fileName(t time.Time) string {
	return filepath.Join(w.dir, fmt.Sprintf("%s-%s.log", w.prefix, t.Format("2006-01-02")))
}

func (w *rotateWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.ensureOpenLocked(); err != nil {
		return 0, err
	}
	if w.size > 0 && w.size+int64(len(p)) > w.maxSize {
		if err := w.rotateSizeLocked(); err != nil {
			return 0, err
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rotateWriter) ensureOpenLocked() error {
	now := time.Now()
	today := now.Format("2006-01-02")
	if w.f != nil && w.date == today {
		return nil
	}
	// Tanggal berganti: tutup file lama lalu compress.
	if w.f != nil {
		_ = w.f.Close()
		w.f = nil
		if w.compress {
			_ = compressFile(w.pathLocked())
		}
	}
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return err
	}
	path := w.fileName(now)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.f = f
	w.size = st.Size()
	w.date = today
	w.cleanupLocked()
	return nil
}

func (w *rotateWriter) pathLocked() string {
	t, err := time.Parse("2006-01-02", w.date)
	if err != nil {
		t = time.Now()
	}
	return w.fileName(t)
}

func (w *rotateWriter) rotateSizeLocked() error {
	if w.f == nil {
		return nil
	}
	src := w.pathLocked()
	_ = w.f.Close()
	w.f = nil
	rotated := strings.TrimSuffix(src, ".log") + "." + time.Now().Format("150405.000") + ".log"
	if err := os.Rename(src, rotated); err != nil {
		return err
	}
	if w.compress {
		_ = compressFile(rotated)
	}
	// Buka file baru (masih dengan tanggal yang sama).
	w.date = ""
	return w.ensureOpenLocked()
}

func (w *rotateWriter) cleanupLocked() {
	cutoff := time.Now().Add(-w.maxAge)
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, w.prefix+"-") || !strings.HasSuffix(name, ".log") && !strings.HasSuffix(name, ".log.gz") {
			continue
		}
		rest := strings.TrimPrefix(name, w.prefix+"-")
		if len(rest) < len("2006-01-02") {
			continue
		}
		t, err := time.Parse("2006-01-02", rest[:len("2006-01-02")])
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			_ = os.Remove(filepath.Join(w.dir, name))
		}
	}
}

func compressFile(src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(src + ".gz")
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(out)
	if _, err := io.Copy(gz, in); err != nil {
		out.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}

// Logger adalah pengganti logger winston.
type Logger struct {
	mu      sync.Mutex
	console io.Writer
	w       *rotateWriter
}

// New membuat logger yang menulis ke console dan file harian di logDir.
func NewLogger(logDir string) *Logger {
	return &Logger{
		console: os.Stdout,
		w:       newRotateWriter(logDir),
	}
}

func (l *Logger) log(lv Level, msg string) {
	ts := time.Now().Format("2006-01-02 03:04:05.000 PM")

	// Format file (printf winston): " [LOGGER] <ts> <level> : <message>"
	fileLine := fmt.Sprintf(" [LOGGER] %s %s : %s\n", ts, lv.String(), msg)

	// Format console: sama tapi berwarna (seperti winston colorize).
	consoleLine := fmt.Sprintf(
		" \x1b[1m[LOGGER]\x1b[0m \x1b[2m%s\x1b[0m \x1b[%dm%s\x1b[0m : %s\n",
		ts, lv.color(), lv.String(), msg,
	)

	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = io.WriteString(l.console, consoleLine)
	// Transport file winston memakai level "info".
	if lv >= LevelInfo {
		_, _ = l.w.Write([]byte(fileLine))
	}
}

func (l *Logger) Debugf(format string, args ...any) { l.log(LevelDebug, fmt.Sprintf(format, args...)) }
func (l *Logger) Infof(format string, args ...any)  { l.log(LevelInfo, fmt.Sprintf(format, args...)) }
func (l *Logger) Warnf(format string, args ...any)  { l.log(LevelWarn, fmt.Sprintf(format, args...)) }
func (l *Logger) Errorf(format string, args ...any) { l.log(LevelError, fmt.Sprintf(format, args...)) }
