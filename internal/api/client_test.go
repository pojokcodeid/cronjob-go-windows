package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cron-job-azure/internal/config"
	"cron-job-azure/internal/logger"
)

func newTestClient(t *testing.T, baseURL string) *Client {
	cfg := &config.Config{
		APIBaseURL: baseURL,
		APITimeout: 5 * time.Second,
		AccName:    "tester",
		APIKey:     "key",
	}
	return New(cfg, logger.NewLogger(t.TempDir()))
}

// TestInitialTokenFetch memastikan token diambil otomatis saat request pertama
// (pengganti request interceptor axios).
func TestInitialTokenFetch(t *testing.T) {
	var tokenHits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery == "" { // endpoint token (RefreshToken.js)
			tokenHits.Add(1)
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse form: %v", err)
			}
			if r.FormValue("accname") != "tester" || r.FormValue("api_key") != "key" {
				t.Errorf("form salah: %+v", r.Form)
			}
			w.Write([]byte(`{"HSTATUS":200,"DATA":{"ACCESS_TOKEN":"tok-1"}}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"HSTATUS":200,"DATA":{"RESULT":[101,102]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	res, status, err := c.PostJSON(context.Background(), srv.URL+"?qlid=CL_AttendanceTemp.setProcessAtt", map[string]any{"data": []int{1}})
	if err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if status != 200 || res.HSTATUS.String() != "200" {
		t.Fatalf("status salah: %d / HSTATUS=%s", status, res.HSTATUS)
	}
	if got := len(res.Data.RESULT); got != 2 {
		t.Fatalf("RESULT = %d item, ingin 2", got)
	}
	if tokenHits.Load() != 1 {
		t.Fatalf("token endpoint dipanggil %d kali, ingin 1", tokenHits.Load())
	}
}

// TestRetryOn401RefreshesToken memastikan401 memicu refresh token lalu retry
// satu kali (pengganti response interceptor axios).
func TestRetryOn401RefreshesToken(t *testing.T) {
	var tokenHits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery == "" {
			n := tokenHits.Add(1)
			// Token pertama (tok-1) dianggap kedaluwarsa.
			w.Write([]byte(`{"HSTATUS":200,"DATA":{"ACCESS_TOKEN":"tok-` +
				strconv.FormatInt(int64(n), 10) + `"}}`))
			return
		}
		// tok-1 selalu ditolak, tok-2 diterima (token lama kedaluwarsa).
		if r.Header.Get("Authorization") == "Bearer tok-2" {
			w.Write([]byte(`{"HSTATUS":200,"DATA":{"RESULT":[7]}}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	// Satu kali panggilan: ambil tok-1 -> kirim ->401 -> refresh (tok-2) -> retry sukses.
	res, _, err := c.PostJSON(context.Background(), srv.URL+"?qlid=x", nil)
	if err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if len(res.Data.RESULT) != 1 {
		t.Fatalf("RESULT = %+v", res.Data.RESULT)
	}
	if tokenHits.Load() != 2 {
		t.Fatalf("token endpoint dipanggil %d kali, ingin 2 (initial + refresh)", tokenHits.Load())
	}
}

// TestHTTPErrorMembawaStatusBody memastikan error response mirip error.response axios.
func TestHTTPErrorMembawaStatusBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery == "" {
			w.Write([]byte(`{"HSTATUS":200,"DATA":{"ACCESS_TOKEN":"t"}}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"msg":"boom"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, _, err := c.PostJSON(context.Background(), srv.URL+"?qlid=x", nil)
	if err == nil {
		t.Fatal("seharusnya error")
	}
	var he *HTTPError
	if !asHTTPError(err, &he) {
		t.Fatalf("error bukan HTTPError: %v", err)
	}
	if he.StatusCode != 500 || !strings.Contains(he.Body, "boom") {
		t.Fatalf("HTTPError = %+v", he)
	}
}

func asHTTPError(err error, target **HTTPError) bool {
	for err != nil {
		if e, ok := err.(*HTTPError); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
