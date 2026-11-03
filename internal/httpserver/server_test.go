package httpserver

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppRoutes(t *testing.T) {
	app := NewApp("v1.0.0")

	testCases := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{name: "hello", method: http.MethodGet, path: "/", wantStatus: http.StatusOK, wantBody: `{"message":"hello world"}`},
		{name: "health", method: http.MethodGet, path: "/health", wantStatus: http.StatusOK, wantBody: `{"status":"ok"}`},
		{name: "version", method: http.MethodGet, path: "/version", wantStatus: http.StatusOK, wantBody: `{"version":"v1.0.0"}`},
		{name: "echo", method: http.MethodPost, path: "/echo", body: `{"message":"hello"}`, wantStatus: http.StatusOK, wantBody: `{"message":"hello"}`},
		{name: "echo with calculation", method: http.MethodPost, path: "/echo", body: `{"message":"hello","calc":"0 + 2 + 3"}`, wantStatus: http.StatusOK, wantBody: `{"message":"hello","result":"5"}`},
		{name: "echo with empty calculation", method: http.MethodPost, path: "/echo", body: `{"message":"hello","calc":" "}`, wantStatus: http.StatusOK, wantBody: `{"message":"hello","result":"NaN"}`},
		{name: "echo with invalid calculation", method: http.MethodPost, path: "/echo", body: `{"message":"hello","calc":"-1+2"}`, wantStatus: http.StatusOK, wantBody: `{"message":"hello","result":"NaN"}`},
		{name: "echo with fractional calculation", method: http.MethodPost, path: "/echo", body: `{"message":"hello","calc":"1.5+2"}`, wantStatus: http.StatusOK, wantBody: `{"message":"hello","result":"NaN"}`},
		{name: "invalid echo", method: http.MethodPost, path: "/echo", body: `{}`, wantStatus: http.StatusBadRequest, wantBody: `{"message":"invalid request"}`},
		{name: "invalid JSON", method: http.MethodPost, path: "/echo", body: `{"message":`, wantStatus: http.StatusBadRequest, wantBody: `{"message":"invalid request"}`},
		{name: "message too long", method: http.MethodPost, path: "/echo", body: `{"message":"` + strings.Repeat("a", 1025) + `"}`, wantStatus: http.StatusBadRequest, wantBody: `{"message":"invalid request"}`},
		{name: "pprof unavailable", method: http.MethodGet, path: "/debug/pprof/", wantStatus: http.StatusNotFound, wantBody: `{"message":"Not Found"}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequestWithContext(context.Background(), testCase.method, testCase.path, strings.NewReader(testCase.body))
			if testCase.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()

			app.ServeHTTP(response, request)

			if response.Code != testCase.wantStatus {
				t.Errorf("status = %d, want %d", response.Code, testCase.wantStatus)
			}
			if response.Body.String() != testCase.wantBody+"\n" {
				t.Errorf("body = %q, want %q", response.Body.String(), testCase.wantBody+"\n")
			}
		})
	}
}

func TestNewAppLogsVersion(t *testing.T) {
	var output bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	NewApp("v1.0.0")

	if !strings.Contains(output.String(), "version=v1.0.0") {
		t.Errorf("log output = %q, want application version", output.String())
	}
}

func TestCalculatorPage(t *testing.T) {
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/calculator", nil)
	response := httptest.NewRecorder()

	NewApp("v1.0.0").ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	for _, fragment := range []string{`id="expression"`, `Calculate`, `role="status"`} {
		if !strings.Contains(response.Body.String(), fragment) {
			t.Errorf("response does not contain %q", fragment)
		}
	}
}

func TestMetricsHandler(t *testing.T) {
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()

	MetricsHandler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), "go_goroutines") {
		t.Error("metrics response does not contain go_goroutines")
	}
}
