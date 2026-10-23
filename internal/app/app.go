// Package app merangkai seluruh aplikasi: HTTP server (pengganti Express)
// dan cron job (pengganti node-cron), sejajar dengan src/index.js.
package app

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"

	"cron-job-azure/internal/api"
	"cron-job-azure/internal/attendance"
	"cron-job-azure/internal/config"
	"cron-job-azure/internal/db"
	"cron-job-azure/internal/logger"
)

// App adalah aplikasi utama.
type App struct {
	cfg    *config.Config
	log    *logger.Logger
	pools  *db.Pools
	client *api.Client

	server *http.Server
	cron   *cron.Cron

	jobMu     sync.Mutex // pengganti isJobRunning (anti tumpang tindih)
	stopOnce  sync.Once
	stopCh    chan struct{}
	stopErrCh chan error
}

// exeDir mengembalikan folder tempat binary berada (untuk .env & logs
// saat dijalankan sebagai Windows Service yang CWD-nya bisa system32).
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe)
}

// loadEnv memuat file .env seperti dotenv: dari CWD, fallback ke folder exe.
func loadEnv(dir string) {
	if err := godotenv.Load(); err == nil {
		return
	}
	_ = godotenv.Load(filepath.Join(dir, ".env"))
}

// New menyiapkan konfigurasi, logger, koneksi database, dan API client.
func New() (*App, error) {
	dir := exeDir()
	loadEnv(dir)

	cfg := config.Load(dir)
	log := logger.NewLogger(cfg.LogDir)

	pools, err := db.Open(cfg, log)
	if err != nil {
		return nil, err
	}

	return &App{
		cfg:       cfg,
		log:       log,
		pools:     pools,
		client:    api.New(cfg, log),
		stopCh:    make(chan struct{}),
		stopErrCh: make(chan error, 1),
	}, nil
}

// Run menjalankan HTTP server + cron lalu menunggu sinyal berhenti
// (SIGINT/SIGTERM atau Stop() dari Windows Service).
func (a *App) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.handleIndex) // pengganti app.get("/", ...)

	a.server = &http.Server{
		Addr:              ":" + a.cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		err := a.server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			a.log.Errorf("Server error: %v", err)
			a.stopErrCh <- err
		}
	}()
	a.log.Infof("Server running at http://localhost:%s", a.cfg.Port)

	// Cron job (pengganti cron.schedule).
	a.cron = cron.New()
	spec := a.cfg.CronSchedule
	if _, err := a.cron.AddFunc(spec, a.runJob); err != nil {
		_ = a.server.Close()
		return err
	}
	a.cron.Start()
	a.log.Infof("Cron job scheduled (%s)", spec)

	// Tangani Ctrl+C untuk mode console.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case <-a.stopCh:
	case <-sigCh:
		a.log.Infof("Menerima sinyal berhenti, mematikan aplikasi...")
	case err := <-a.stopErrCh:
		a.Shutdown()
		return err
	}

	a.Shutdown()
	return nil
}

// handleIndex adalah pengganti route GET "/" yang mengembalikan data absen HKV.
func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, ids, err := attendance.GetAttendanceData(r.Context(), a.pools.HKV, a.log)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"resultData": data,
		"resultId":   ids,
	})
}

// runJob adalah isi cron job, sama urutannya dengan src/index.js:
// AttendacneSend() lalu AttendacneSendGD(), dengan guard isJobRunning.
func (a *App) runJob() {
	if !a.jobMu.TryLock() {
		a.log.Warnf("Skip cron: proses sebelumnya masih berjalan")
		return
	}
	defer a.jobMu.Unlock()

	start := time.Now()
	// Batas waktu eksekusi agar job tidak menggantung selamanya.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if _, err := attendance.SendHKV(ctx, a.pools.HKV, a.client, a.cfg.APIBaseURL, a.log); err != nil {
		a.log.Errorf("Cron job error: %v", err)
	} else if _, err := attendance.SendGD(ctx, a.pools.GD, a.client, a.cfg.APIBaseURL, a.log); err != nil {
		a.log.Errorf("Cron job error: %v", err)
	}

	a.log.Infof("Cron job selesai dalam %.3fs", time.Since(start).Seconds())
}

// RunJobOnce menjalankan satu kali job cron (berguna untuk testing manual).
func (a *App) RunJobOnce() {
	a.runJob()
}

// Shutdown menghentikan cron & server dengan rapi (idempoten).
func (a *App) Shutdown() {
	a.stopOnce.Do(func() {
		if a.cron != nil {
			ctx := a.cron.Stop()
			<-ctx.Done() // tunggu job yang sedang berjalan selesai
		}
		if a.server != nil {
			shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = a.server.Shutdown(shCtx)
		}
		if a.pools != nil {
			a.pools.Close()
		}
		a.log.Infof("Aplikasi berhenti.")
		close(a.stopCh)
	})
}
