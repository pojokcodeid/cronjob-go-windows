// Package db menyediakan koneksi ke dua database SQL Server (HKV & GD),
// pengganti poolPromise/poolPromiseGD pada src/config/Db.js.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/microsoft/go-mssqldb" // driver SQL Server (pengganti mssql)

	"cron-job-azure/internal/config"
	"cron-job-azure/internal/logger"
)

// Pools berisi kedua connection pool, sejajar dengan export Db.js.
type Pools struct {
	HKV *sql.DB // poolPromise
	GD  *sql.DB // poolPromiseGD
}

func buildConnString(c config.DBConfig) string {
	return fmt.Sprintf(
		"server=%s;port=%d;user id=%s;password=%s;database=%s;encrypt=%t;trustServerCertificate=%t;connection timeout=15",
		c.Server, c.Port, c.User, c.Password, c.Name, c.Encrypt, c.TrustServerCertificate,
	)
}

func open(c config.DBConfig, label string, log *logger.Logger) (*sql.DB, error) {
	pool, err := sql.Open("sqlserver", buildConnString(c))
	if err != nil {
		log.Errorf("Database Connection Failed (%s): %v", label, err)
		return nil, err
	}
	// Opsi pool mssql: max 10, min 0.
	pool.SetMaxOpenConns(c.MaxOpen)
	pool.SetMaxIdleConns(2)
	pool.SetConnMaxIdleTime(c.IdleTimeout)

	// Ping dengan timeout agar tahu apakah koneksi berhasil (seperti .connect()).
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := pool.PingContext(ctx); err != nil {
		// Tetap dipertahankan: database/sql akan mencoba connect ulang
		// pada query berikutnya (lebih tangguh untuk Windows Service).
		log.Errorf("Database Connection Failed (%s): %v", label, err)
		return pool, nil
	}
	log.Infof("Connected to Azure SQL for %s", label)
	return pool, nil
}

// Open membuat kedua connection pool dan melakukan ping awal.
func Open(cfg *config.Config, log *logger.Logger) (*Pools, error) {
	hkv, err := open(cfg.HKV, "HKV", log)
	if err != nil {
		return nil, err
	}
	gd, err := open(cfg.GD, "GD", log)
	if err != nil {
		_ = hkv.Close()
		return nil, err
	}
	return &Pools{HKV: hkv, GD: gd}, nil
}

// Close menutup kedua pool.
func (p *Pools) Close() {
	if p == nil {
		return
	}
	if p.HKV != nil {
		_ = p.HKV.Close()
	}
	if p.GD != nil {
		_ = p.GD.Close()
	}
}
