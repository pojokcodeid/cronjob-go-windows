// Package config membaca konfigurasi dari environment variable,
// sama seperti `dotenv` + `process.env` pada proyek Node.js.
package config

import (
	"os"
	"strconv"
	"time"
)

// DBConfig adalah pengganti konfigurasi mssql di src/config/Db.js.
type DBConfig struct {
	User     string
	Password string
	Server   string
	Port     int
	Name     string
	Encrypt  bool

	// TrustServerCertificate setara options.trustServerCertificate pada mssql.
	// Diperlukan true untuk server dengan sertifikat self-signed
	// (mis. "SSL_Self_Signed_Fallback") karena Go x509 menolak sertifikat
	// yang tidak standards compliant.
	TrustServerCertificate bool

	// Pool (mirip opsi pool mssql: max 10, idleTimeoutMillis 30s/300s).
	MaxOpen     int
	IdleTimeout time.Duration
}

// Config berisi seluruh konfigurasi aplikasi.
type Config struct {
	Port         string
	APIBaseURL   string
	APITimeout   time.Duration
	AccName      string
	APIKey       string
	CronSchedule string
	LogDir       string

	HKV DBConfig
	GD  DBConfig
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getBool(key string) bool {
	return os.Getenv(key) == "true" // sama seperti process.env.X === "true"
}

// getBoolOr mengembalikan nilai boolean env; bila env tidak diset pakai def.
func getBoolOr(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v == "true"
}

func getInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func getDurationMs(key string, defMs int) time.Duration {
	return time.Duration(getInt(key, defMs)) * time.Millisecond
}

// Load membaca konfigurasi dari environment.
func Load(logDir string) *Config {
	cfg := &Config{
		Port:         getenv("PORT", "3000"),
		APIBaseURL:   os.Getenv("API_BASE_URL"),
		APITimeout:   getDurationMs("API_TIMEOUT", 5000), // axios: Number(...) || 5000
		AccName:      os.Getenv("ACCNAME"),
		APIKey:       os.Getenv("APIKEY"),
		CronSchedule: getenv("CRON_SCHEDULE", "*/3 * * * *"),
		LogDir:       getenv("LOG_DIR", logDir),
	}

	// Database HKV (pengganti config di Db.js)
	cfg.HKV = DBConfig{
		User:                   os.Getenv("DB_USER"),
		Password:               os.Getenv("DB_PASSWORD"),
		Server:                 os.Getenv("DB_SERVER"),
		Port:                   getInt("DB_PORT", 1433),
		Name:                   os.Getenv("DB_NAME"),
		Encrypt:                getBool("DB_ENCRYPT"),
		TrustServerCertificate: getBoolOr("DB_TRUST_SERVER_CERTIFICATE", false),
		MaxOpen:                10,
		IdleTimeout:            30 * time.Second, // idleTimeoutMillis: 30000
	}

	// Database GD (pengganti configGD di Db.js)
	cfg.GD = DBConfig{
		User:                   os.Getenv("DB_USER_GD"),
		Password:               os.Getenv("DB_PASSWORD_GD"),
		Server:                 os.Getenv("DB_SERVER_GD"),
		Port:                   getInt("DB_PORT_GD", 1433),
		Name:                   os.Getenv("DB_NAME_GD"),
		Encrypt:                getBool("DB_ENCRYPT_GD"),
		TrustServerCertificate: getBoolOr("DB_TRUST_SERVER_CERTIFICATE_GD", false),
		MaxOpen:                10,
		IdleTimeout:            300 * time.Second, // idleTimeoutMillis: 300000
	}

	return cfg
}
