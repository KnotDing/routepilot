package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"surge-controller/internal/config"
	"testing"
)

type routeService struct {
	Service
	doc     config.Document
	applies int
	err     error
}

func (s *routeService) Configuration() config.Document { return s.doc.Clone() }
func (s *routeService) Apply(_ context.Context, d config.Document) error {
	s.applies++
	if s.err != nil {
		return s.err
	}
	s.doc = d
	return nil
}

type importerTransport func(*http.Request) (*http.Response, error)

func (f importerTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRouteURLPreviewAndAtomicImport(t *testing.T) {
	s := &routeService{doc: config.Default()}
	a := &API{Service: s, RouteImportClient: &http.Client{Transport: importerTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("91.108.56.0/22\n2001:b28:f23d::/48\n")), Header: make(http.Header)}, nil
	})}}
	call := func(path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		data, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/"+path, bytes.NewReader(data)))
		return w
	}
	w := call("bgp/routes/import-url", map[string]any{"url": "https://core.telegram.org/resources/cidr.txt"})
	if w.Code != 200 || s.applies != 0 || len(s.doc.Routes) != 0 {
		t.Fatal(w.Code, w.Body.String(), s.applies)
	}
	body := map[string]any{"prefixes": []string{"91.108.56.0/22", "2001:b28:f23d::/48"}, "next_hop4": "192.168.20.5", "next_hop6": "fd00:20::5", "enabled": true, "local_preference": 100}
	// Validate all entries before a single apply, including the second address family.
	body["next_hop6"] = ""
	w = call("bgp/routes/import", body)
	if w.Code != 422 || s.applies != 0 {
		t.Fatal(w.Code, w.Body.String(), s.applies)
	}
	body["next_hop6"] = "fd00:20::5"
	s.err = errors.New("apply failed")
	w = call("bgp/routes/import", body)
	if w.Code != 409 || len(s.doc.Routes) != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	s.err = nil
	w = call("bgp/routes/import", body)
	if w.Code != 200 || s.applies != 2 || len(s.doc.Routes) != 2 {
		t.Fatal(w.Code, w.Body.String(), s.applies)
	}
	w = call("bgp/routes/import", body)
	if w.Code != 200 || s.applies != 2 || !strings.Contains(w.Body.String(), `"skipped":2`) {
		t.Fatal(w.Code, w.Body.String(), s.applies)
	}
}
