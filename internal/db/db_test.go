package db

import (
	"strings"
	"testing"

	"cron-job-azure/internal/config"
)

// TestBuildConnStringTrustServerCertificate memastikan parameter
// trustServerCertificate terkirim ke driver (pengganti options.trustServerCertificate
// pada mssql versi Node.js) — penting untuk sertifikat self-signed.
func TestBuildConnStringTrustServerCertificate(t *testing.T) {
	got := buildConnString(config.DBConfig{
		User:                   "u",
		Password:               "p",
		Server:                 "10.29.56.156",
		Port:                   1433,
		Name:                   "dbdanoneNEW",
		Encrypt:                false,
		TrustServerCertificate: true,
	})
	for _, want := range []string{
		"server=10.29.56.156",
		"port=1433",
		"database=dbdanoneNEW",
		"encrypt=false",
		"trustServerCertificate=true",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("conn string %q tidak mengandung %q", got, want)
		}
	}
}

func TestBuildConnStringDefaultTrustFalse(t *testing.T) {
	got := buildConnString(config.DBConfig{Server: "h", Port: 1433, Encrypt: true})
	if !strings.Contains(got, "trustServerCertificate=false") {
		t.Fatalf("conn string %q harus default trustServerCertificate=false (untuk Azure SQL)", got)
	}
}
