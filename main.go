// cron-job-azure adalah terjemahan Go dari proyek nodejs-cron-job
// (folder ../cron-job-azure): sinkronisasi data absen dari SQL Server
// ke API eksternal via cron, plus HTTP server untuk pengecekan data.
//
// Binary ini juga bisa dijalankan sebagai Windows Service agar otomatis
// menyala kembali saat server di-restart:
//
//	cron-job-azure.exe install    # daftarkan sebagai service (auto start)
//	cron-job-azure.exe start      # jalankan service
//	cron-job-azure.exe stop       # hentikan service
//	cron-job-azure.exe uninstall  # hapus service
//
// Tanpa argumen, aplikasi berjalan seperti biasa di console.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kardianos/service"

	"cron-job-azure/internal/app"
)

const (
	serviceName        = "cron-job-azure"
	serviceDisplayName = "Cron Job Azure Attendance Sync"
	serviceDescription = "Sinkronisasi data absen SQL Server (HKV & GD) ke API SF7 secara berkala."
)

// program membungkus app.App untuk interface service.Interface.
type program struct {
	app *app.App
}

func (p *program) Start(s service.Service) error {
	go func() {
		if err := p.app.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "aplikasi berhenti dengan error: %v\n", err)
			_ = s.Stop()
		}
	}()
	return nil
}

func (p *program) Stop(s service.Service) error {
	p.app.Shutdown()
	return nil
}

func main() {
	// Working directory & .env mengikuti folder binary (penting untuk service).
	exe, _ := os.Executable()
	wd := filepath.Dir(exe)

	prg := &program{}
	svc, err := service.New(prg, &service.Config{
		Name:             serviceName,
		DisplayName:      serviceDisplayName,
		Description:      serviceDescription,
		WorkingDirectory: wd,
		// Auto start saat Windows server di-restart (+ delayed auto start).
		// Auto start saat Windows server di-restart (+ delayed auto start)
		// dan auto-restart otomatis bila service crash/tiba-tiba mati.
		Option: service.KeyValue{
			"StartType":              "automatic",
			"DelayedAutoStart":       true,
			"OnFailure":              "restart",
			"OnFailureDelayDuration": "10s",
			"OnFailureResetPeriod":   86400,
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "gagal inisialisasi service: %v\n", err)
		os.Exit(1)
	}

	// Perintah kontrol service: install / uninstall / start / stop / restart.
	if len(os.Args) > 1 {
		if err := service.Control(svc, os.Args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "perintah %q gagal: %v\n", os.Args[1], err)
			os.Exit(1)
		}
		fmt.Printf("perintah %q sukses\n", os.Args[1])
		return
	}

	// Jalankan aplikasi (mode console / mode service).
	a, err := app.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gagal menyiapkan aplikasi: %v\n", err)
		os.Exit(1)
	}
	prg.app = a

	if err := svc.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "aplikasi berhenti dengan error: %v\n", err)
		os.Exit(1)
	}
}
