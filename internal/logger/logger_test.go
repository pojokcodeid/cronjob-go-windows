package logger

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// readGzip mengembalikan isi file .gz dalam bentuk string.
func readGzip(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("buka %s: %v", path, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader %s: %v", path, err)
	}
	defer gz.Close()

	data, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("baca %s: %v", path, err)
	}
	return string(data)
}

// assertTidakDobel memastikan tidak ada file .log yang isinya sudah ada di
// dalam .log.gz bernama sama, dan tidak ada file sementara (.tmp) tertinggal.
func assertTidakDobel(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("baca %s: %v", dir, err)
	}

	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		names[e.Name()] = true
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("file sementara tertinggal: %s", e.Name())
		}
	}
	for name := range names {
		if !strings.HasSuffix(name, ".log.gz") {
			continue
		}
		if base := strings.TrimSuffix(name, ".gz"); names[base] {
			t.Errorf("file dobel: %s masih ada padahal isinya sudah di dalam %s", base, name)
		}
	}
}

// File yang sudah di-zip harus hilang; isi .gz harus sama dengan file aslinya.
func TestCompressFileHapusFileAsli(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "app-2026-10-24.log")
	want := strings.Repeat(" [LOGGER] 2026-10-24 03:15:00.000 PM info : pesan log\n", 50)
	if err := os.WriteFile(src, []byte(want), 0o644); err != nil {
		t.Fatalf("tulis %s: %v", src, err)
	}

	if err := compressFile(src); err != nil {
		t.Fatalf("compressFile: %v", err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("%s seharusnya dihapus setelah di-zip, tapi masih ada", src)
	}
	if got := readGzip(t, src+".gz"); got != want {
		t.Errorf("isi .gz tidak sama dengan file asli (got %d byte, want %d byte)", len(got), len(want))
	}
	assertTidakDobel(t, dir)
}

// Rotasi karena ukuran (maxSize 1MB) tidak boleh menyisakan .log asli.
func TestWriteMelebihiMaxSize_TidakTinggalkanLogAsli(t *testing.T) {
	dir := t.TempDir()
	w := newRotateWriter(dir)
	defer func() {
		if w.f != nil {
			_ = w.f.Close()
		}
	}()

	// ~300KB per tulisan; write ke-4 melewati maxSize 1MB dan memicu rotasi.
	line := strings.Repeat("x", 300*1024)
	for i := 0; i < 5; i++ {
		if _, err := w.Write([]byte(line)); err != nil {
			t.Fatalf("write #%d: %v", i+1, err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("baca %s: %v", dir, err)
	}
	adaGz := false
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".log.gz") {
			adaGz = true
		}
	}
	if !adaGz {
		t.Fatalf("seharusnya ada hasil kompresi .log.gz, dapat: %v", names(entries))
	}
	assertTidakDobel(t, dir)
}

// Rotasi tanggal (file kemarin dikompres saat hari berganti) tidak boleh
// menyisakan .log aslinya, dan file hari ini harus tetap ditulis normal.
func TestTanggalBerganti_TidakTinggalkanLogAsli(t *testing.T) {
	dir := t.TempDir()
	w := newRotateWriter(dir)
	defer func() {
		if w.f != nil {
			_ = w.f.Close()
		}
	}()

	// "Bahkan aplikasi berjalan semalam": file kemarin masih terbuka oleh writer.
	kemarin := time.Now().AddDate(0, 0, -1)
	src := w.fileName(kemarin)
	isi := "isi log kemarin yang harus ikut ter-zip\n"
	if err := os.WriteFile(src, []byte(isi), 0o644); err != nil {
		t.Fatalf("tulis %s: %v", src, err)
	}
	f, err := os.OpenFile(src, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("buka %s: %v", src, err)
	}
	w.f = f
	w.date = kemarin.Format("2006-01-02")
	w.size = int64(len(isi))

	// Tulisan pertama hari ini mendeteksi pergantian tanggal.
	if _, err := w.Write([]byte("baris pertama hari ini\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("%s seharusnya dihapus setelah di-zip, tapi masih ada", src)
	}
	if got := readGzip(t, src+".gz"); !strings.Contains(got, "isi log kemarin") {
		t.Errorf("isi %s.gz tidak berisi log kemarin", src)
	}
	if _, err := os.Stat(w.fileName(time.Now())); err != nil {
		t.Errorf("file log hari ini tidak terbuka: %v", err)
	}
	assertTidakDobel(t, dir)
}

func names(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}
