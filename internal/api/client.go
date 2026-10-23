// Package api adalah pengganti axios instance pada src/utils/AxiosConfig.js
// dan RefreshToken.js: request selalu menyertakan Bearer token, dan bila
// mendapat 401 akan refresh token lalu retry satu kali.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"

	"cron-job-azure/internal/config"
	"cron-job-azure/internal/logger"
)

// Result adalah struktur response JSON API sepusat (HSTATUS, DATA.RESULT, DATA.ACCESS_TOKEN).
type Result struct {
	HSTATUS json.Number `json:"HSTATUS"`
	Data    struct {
		RESULT      []any  `json:"RESULT"`
		AccessToken string `json:"ACCESS_TOKEN"`
	} `json:"DATA"`
}

// HTTPError membawa status code & body response (untuk log mirip error.response axios).
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

// Client adalah pengganti axios.create(...) + interceptor.
type Client struct {
	baseURL  string
	accName  string
	apiKey   string
	log      *logger.Logger
	http     *http.Client
	refreshM sync.Mutex // menjamin hanya satu refresh berjalan (mirip refreshPromise)
	token    string     // accessToken
	mu       sync.RWMutex
}

// New membuat client baru (sejajar dengan pembuatan api instance).
func New(cfg *config.Config, log *logger.Logger) *Client {
	jar, _ := cookiejar.New(nil) // withCredentials: true
	return &Client{
		baseURL: cfg.APIBaseURL,
		accName: cfg.AccName,
		apiKey:  cfg.APIKey,
		log:     log,
		http: &http.Client{
			Timeout: cfg.APITimeout,
			Jar:     jar,
		},
	}
}

func (c *Client) getToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}

func (c *Client) setToken(t string) {
	c.mu.Lock()
	c.token = t
	c.mu.Unlock()
}

// refreshToken adalah pengganti RefreshToken.js: POST form accname+api_key
// ke API_BASE_URL lalu ambil DATA.ACCESS_TOKEN.
func (c *Client) refreshToken(ctx context.Context) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("accname", c.accName)
	_ = mw.WriteField("api_key", c.apiKey)
	_ = mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("token endpoint HTTP %d: %s", resp.StatusCode, snippet(body))
	}

	var parsed Result
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("parse token response: %w", err)
	}
	if parsed.Data.AccessToken == "" {
		return fmt.Errorf("access token kosong: %s", snippet(body))
	}
	c.setToken(parsed.Data.AccessToken)
	return nil
}

// ensureToken mengambil token bila belum ada (request interceptor).
func (c *Client) ensureToken(ctx context.Context) {
	if c.getToken() != "" {
		return
	}
	c.refreshM.Lock()
	defer c.refreshM.Unlock()
	if c.getToken() != "" { // sudah di-refresh goroutine lain
		return
	}
	if err := c.refreshToken(ctx); err != nil {
		c.log.Errorf("Initial Token Fetch Failed: %v", err)
	}
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 2000 {
		s = s[:2000] + "..."
	}
	return s
}

func encodeBody(body any) (*bytes.Reader, string, error) {
	if body == nil {
		return bytes.NewReader(nil), "", nil
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, "", err
	}
	return bytes.NewReader(buf), "application/json", nil
}

func (c *Client) send(ctx context.Context, method, url string, body *bytes.Reader, contentType, auth string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, 0, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	return respBody, resp.StatusCode, nil
}

// PostJSON melakukan POST JSON dengan Bearer token; bila 401 maka refresh
// token dan retry satu kali (sama seperti interceptor response axios).
func (c *Client) PostJSON(ctx context.Context, url string, body any) (*Result, int, error) {
	// Request interceptor: pastikan token ada.
	c.ensureToken(ctx)

	reqBody, contentType, err := encodeBody(body)
	if err != nil {
		return nil, 0, err
	}

	auth := ""
	if tok := c.getToken(); tok != "" {
		auth = "Bearer " + tok
	}

	respBody, status, err := c.send(ctx, http.MethodPost, url, reqBody, contentType, auth)
	if err != nil {
		return nil, 0, err
	}

	// Response interceptor: retry sekali untuk 401.
	if status == http.StatusUnauthorized {
		// Cek apakah token sudah berubah (refresh dilakukan request lain).
		if auth != "" && c.getToken() != "" && auth != "Bearer "+c.getToken() {
			reqBody, contentType, _ = encodeBody(body)
			respBody, status, err = c.send(ctx, http.MethodPost, url, reqBody, contentType, "Bearer "+c.getToken())
			if err != nil {
				return nil, status, err
			}
		} else {
			c.log.Infof("Refreshing access token...")
			if err := c.refreshToken(ctx); err != nil {
				c.log.Errorf("Token Refresh Failed in Interceptor: %v", err)
				return nil, status, err
			}
			reqBody, contentType, _ = encodeBody(body)
			respBody, status, err = c.send(ctx, http.MethodPost, url, reqBody, contentType, "Bearer "+c.getToken())
			if err != nil {
				return nil, status, err
			}
		}
	}

	// Axios melempar error untuk status di luar 2xx.
	if status < 200 || status >= 300 {
		return nil, status, &HTTPError{StatusCode: status, Body: snippet(respBody)}
	}

	var parsed Result
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, status, fmt.Errorf("parse response: %w (body: %s)", err, snippet(respBody))
	}
	return &parsed, status, nil
}
