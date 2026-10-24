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

const (
	// connectTimeout detik untuk TCP+TLS+login. Jalur HKV ke Azure SQL melewati
	// Private Link (danidpsql002.privatelink... -> endpoint privat 10.29.56.134)
	// via VPN/ExpressRoute, sehingga handshake bisa lebih lambat. Nilai 15s
	// sebelumnya terlalu ketat dan memicu "i/o timeout" yang tidak menentu.
	connectTimeout = 30

	// Ping ditolerir terhadap blip sesaat di jalur private endpoint:
	// coba beberapa kali sebelum menyatakan koneksi gagal.
	pingAttempts = 3
	pingDelay    = 2 * time.Second

	// connMaxLifetime memaksa recycle koneksi di pool. Firewall/NAT pada jalur
	// private endpoint sering memutus koneksi idle; tanpa batas ini database/sql
	// bisa memakai koneksi yang sudah mati sehingga memicu i/o timeout intermittente
	// pada Windows Service yang berjalan lama.
	connMaxLifetime = 5 * time.Minute
)

func buildConnString(c config.DBConfig) string {
	return fmt.Sprintf(
		"server=%s;port=%d;user id=%s;password=%s;database=%s;encrypt=%t;trustServerCertificate=%t;connection timeout=%d",
		c.Server, c.Port, c.User, c.Password, c.Name, c.Encrypt, c.TrustServerCertificate, connectTimeout,
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
	pool.SetConnMaxLifetime(connMaxLifetime)

	// Ping dengan retry agar tahu apakah koneksi berhasil (seperti .connect()).
	if err := pingWithRetry(pool, label, log); err != nil {
		// Tetap dipertahankan: database/sql akan mencoba connect ulang
		// pada query berikutnya (lebih tangguh untuk Windows Service).
		log.Errorf("Database Connection Failed (%s): %v", label, err)
		return pool, nil
	}
	log.Infof("Connected to Azure SQL for %s", label)
	return pool, nil
}

// pingWithRetry melakukan ping dengan beberapa percobaan untuk menolerir
// kegagalan transien (i/o timeout sesaat) pada jalur private endpoint.
func pingWithRetry(pool *sql.DB, label string, log *logger.Logger) error {
	var err error
	for attempt := 1; attempt <= pingAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(),
			time.Duration(connectTimeout)*time.Second)
		err = pool.PingContext(ctx)
		cancel()
		if err == nil {
			return nil
		}
		if attempt < pingAttempts {
			log.Warnf("Ping %s gagal (percobaan %d/%d): %v; mengulang dalam %s",
				label, attempt, pingAttempts, err, pingDelay)
			time.Sleep(pingDelay)
		}
	}
	return err
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
