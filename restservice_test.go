package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoadRESTSecurityConfig(t *testing.T) {
	t.Setenv("REST_TLS_CERT", "")
	t.Setenv("REST_TLS_KEY", "")
	t.Setenv("REST_BEARER_TOKEN", "")
	config, err := loadRESTSecurityConfig()
	if err != nil {
		t.Fatalf("legacy mode config returned an error: %v", err)
	}
	if config.token != "" {
		t.Fatal("legacy mode unexpectedly enabled a token")
	}

	t.Setenv("REST_TLS_CERT", "server.crt")
	if _, err := loadRESTSecurityConfig(); err == nil {
		t.Fatal("partial secure configuration was accepted")
	}

	t.Setenv("REST_TLS_KEY", "server.key")
	t.Setenv("REST_BEARER_TOKEN", strings.Repeat("x", 31))
	if _, err := loadRESTSecurityConfig(); err == nil {
		t.Fatal("short bearer token was accepted")
	}

	t.Setenv("REST_BEARER_TOKEN", strings.Repeat("x", 32))
	config, err = loadRESTSecurityConfig()
	if err != nil {
		t.Fatalf("complete secure configuration returned an error: %v", err)
	}
	if config.certFile != "server.crt" || config.keyFile != "server.key" {
		t.Fatalf("unexpected TLS files in config: %+v", config)
	}
}

func TestBearerTokenAuth(t *testing.T) {
	const token = "test-token-long-enough-for-authentication"
	handler := bearerTokenAuth(token)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	tests := []struct {
		name   string
		header string
		status int
	}{
		{name: "missing token", status: http.StatusUnauthorized},
		{name: "wrong token", header: "Bearer wrong", status: http.StatusUnauthorized},
		{name: "wrong scheme", header: "Basic " + token, status: http.StatusUnauthorized},
		{name: "valid token", header: "Bearer " + token, status: http.StatusNoContent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/sensors", nil)
			if test.header != "" {
				request.Header.Set("Authorization", test.header)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler := securityHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	for header, want := range map[string]string{
		"Cache-Control":             "no-store",
		"X-Content-Type-Options":    "nosniff",
		"Referrer-Policy":           "no-referrer",
		"Strict-Transport-Security": "max-age=31536000",
	} {
		if got := response.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestWiFiSetupPage(t *testing.T) {
	response := httptest.NewRecorder()
	handleWiFiSetupPage(response, httptest.NewRequest(http.MethodGet, "/", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", got)
	}
	for _, want := range []string{"/ssids", "/connect-ssid", "Wi-Fi Setup"} {
		if !strings.Contains(response.Body.String(), want) {
			t.Errorf("setup page does not contain %q", want)
		}
	}
}

func TestDecodeJSONBodyRejectsOversizedAndTrailingData(t *testing.T) {
	for _, body := range []string{
		`{"ok":true} {"extra":true}`,
		`{"value":"` + strings.Repeat("x", maxRESTRequestBody) + `"}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		response := httptest.NewRecorder()
		var value map[string]any
		if err := decodeJSONBody(response, request, &value); err == nil {
			t.Fatal("invalid request body was accepted")
		}
	}
}
