// Uji koneksi sementara ke database GD: reproduksi error TLS lalu
// membuktikan trustServerCertificate=true memperbaikinya.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/microsoft/go-mssqldb"
)

func try(label, conn string) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	db, err := sql.Open("sqlserver", conn)
	if err != nil {
		fmt.Printf("[%s] open: %v\n", label, err)
		return
	}
	defer db.Close()
	var one int
	err = db.QueryRowContext(ctx, "select 1").Scan(&one)
	if err != nil {
		fmt.Printf("[%s] GAGAL: %v\n", label, err)
		return
	}
	fmt.Printf("[%s] SUKSES: select 1 -> %d\n", label, one)
}

func main() {
	_ = godotenv.Load()
	server := os.Getenv("DB_SERVER_GD")
	port := os.Getenv("DB_PORT_GD")
	if port == "" {
		port = "1433"
	}
	user := os.Getenv("DB_USER_GD")
	pass := os.Getenv("DB_PASSWORD_GD")
	name := os.Getenv("DB_NAME_GD")

	base := fmt.Sprintf("server=%s;port=%s;user id=%s;password=%s;database=%s;connection timeout=8;", server, port, user, pass, name)
	try("lama (trust=false)", base+"encrypt=false;trustServerCertificate=false")
	try("baru (trust=true) ", base+"encrypt=false;trustServerCertificate=true")
}
