package routingapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// httpClient issues JSON requests against one router origin. TLS verification
// can be disabled per configuration for self-signed router certificates; the
// choice is reported in the UI instead of being silently ignored.
type httpClient struct {
	base   *url.URL
	client *http.Client
}

func newHTTPClient(baseURL string, verifyTLS bool) (*httpClient, error) {
	u, e := url.Parse(baseURL)
	if e != nil || u.Host == "" || u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("routing API base URL must be an HTTP(S) URL")
	}
	transport := &http.Transport{}
	if !verifyTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &httpClient{base: u, client: &http.Client{Timeout: 10 * time.Second, Transport: transport}}, nil
}

func (c *httpClient) do(ctx context.Context, method, path string, headers map[string]string, body []byte) (int, []byte, error) {
	u := *c.base
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	u.RawQuery = ""
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, e := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if e != nil {
		return 0, nil, e
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		if k != "" {
			req.Header.Set(k, v)
		}
	}
	resp, e := c.client.Do(req)
	if e != nil {
		return 0, nil, e
	}
	defer resp.Body.Close()
	data, e := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, data, e
}

func decodeJSON(status int, data []byte, out any) error {
	if status < 200 || status >= 300 {
		summary := strings.TrimSpace(string(data))
		if len(summary) > 200 {
			summary = summary[:200]
		}
		return errors.New("router API returned HTTP " + itoa(status) + summary)
	}
	if len(data) == 0 || out == nil {
		return nil
	}
	if e := json.Unmarshal(data, out); e != nil {
		return errors.New("router API response is not valid JSON")
	}
	return nil
}

// openWrtSection maps a route name onto a uci-safe section identifier. Hashing
// keeps names within the [A-Za-z0-9_] alphabet uci accepts.
func openWrtSection(name string) string {
	sum := sha256.Sum256([]byte(name))
	return "rp_" + hex.EncodeToString(sum[:8])
}
