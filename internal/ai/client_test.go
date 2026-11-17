package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEndpoint(t *testing.T) {
	for in, want := range map[string]string{
		"https://api.example.com/v1":                  "https://api.example.com/v1/chat/completions",
		" https://api.example.com/v1/ ":               "https://api.example.com/v1/chat/completions",
		"https://api.example.com/v1/chat/completions": "https://api.example.com/v1/chat/completions",
		"https://api.example.com":                     "https://api.example.com/chat/completions",
		"http://localhost:11434/v1":                   "http://localhost:11434/v1/chat/completions",
		"http://127.0.0.1:8080/v1":                    "http://127.0.0.1:8080/v1/chat/completions",
	} {
		got, err := Endpoint(in)
		if err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	for _, in := range []string{"http://api.example.com/v1", "ftp://x", "api.example.com", "https://user:pw@x.com", "https://x.com/v1?k=1"} {
		var e *Error
		if _, err := Endpoint(in); !errors.As(err, &e) || e.Kind != InvalidURL {
			t.Errorf("%q accepted: %v", in, err)
		}
	}
	var e *Error
	if _, err := Endpoint(""); !errors.As(err, &e) || e.Kind != NotConfigured {
		t.Errorf("empty: %v", err)
	}
}

func kind(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return ""
}

func TestCompleteSendsOneRequestAndReadsReply(t *testing.T) {
	var got request
	var auth, path string
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &got)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"` + "```text\\nAdd parser\\n```" + `"}}]}`))
	}))
	defer srv.Close()
	c := &Client{}
	msgs := []Message{{Role: "system", Content: "S"}, {Role: "user", Content: "U"}}
	text, err := c.Complete(context.Background(), Config{BaseURL: srv.URL + "/v1/", Model: " m "}, "sk-secret", msgs)
	if err != nil || text != "Add parser" {
		t.Fatalf("%q %v", text, err)
	}
	if calls != 1 || path != "/v1/chat/completions" || auth != "Bearer sk-secret" || got.Model != "m" || got.Stream || len(got.Messages) != 2 || got.Messages[1].Content != "U" {
		t.Fatalf("request %d %s %s %+v", calls, path, auth, got)
	}
}

func TestCompleteFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   Kind
	}{
		{"auth", 401, `{"error":{"message":"Incorrect API key provided: sk-secret","type":"invalid_request_error"}}`, Unauthorized},
		{"forbidden", 403, `nope`, Unauthorized},
		{"rate", 429, `{"error":{"message":"slow down"}}`, RateLimited},
		{"size", 400, `{"error":{"message":"This model's maximum context length is 8192 tokens","code":"context_length_exceeded"}}`, TooLarge},
		{"413", 413, ``, TooLarge},
		{"server", 500, `{"error":{"message":"boom"}}`, ServiceError},
		{"html", 200, `<html>proxy login</html>`, BadResponse},
		{"empty choices", 200, `{"choices":[]}`, BadResponse},
		{"null content", 200, `{"choices":[{"message":{"content":null}}]}`, BadResponse},
		{"blank", 200, `{"choices":[{"message":{"content":"  \n"}}]}`, BadResponse},
		{"error in 200", 200, `{"error":{"message":"model not found"}}`, ServiceError},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			w.Write([]byte(tc.body))
		}))
		_, err := (&Client{}).Complete(context.Background(), Config{BaseURL: srv.URL, Model: "m"}, "sk-secret", nil)
		srv.Close()
		if kind(err) != tc.want {
			t.Errorf("%s: %v", tc.name, err)
		}
		if err != nil && strings.Contains(err.Error(), "sk-secret") {
			t.Errorf("%s: key in error %v", tc.name, err)
		}
	}
}

func TestCompleteTimeoutAndCancel(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)
	_, err := (&Client{}).Complete(context.Background(), Config{BaseURL: srv.URL, Model: "m", Timeout: 50 * time.Millisecond}, "k", nil)
	if kind(err) != Timeout {
		t.Fatalf("timeout: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(30 * time.Millisecond); cancel() }()
	_, err = (&Client{}).Complete(ctx, Config{BaseURL: srv.URL, Model: "m"}, "k", nil)
	if kind(err) != Cancelled {
		t.Fatalf("cancel: %v", err)
	}
}

func TestCompleteNeedsConfiguration(t *testing.T) {
	for _, cfg := range []Config{{BaseURL: "https://x.com"}, {Model: "m"}} {
		if _, err := (&Client{}).Complete(context.Background(), cfg, "k", nil); kind(err) != NotConfigured {
			t.Errorf("%+v: %v", cfg, err)
		}
	}
	if _, err := (&Client{}).Complete(context.Background(), Config{BaseURL: "https://x.com", Model: "m"}, " ", nil); kind(err) != NotConfigured {
		t.Errorf("no key: %v", err)
	}
}

func TestKeyOptional(t *testing.T) {
	for in, want := range map[string]bool{
		"http://localhost:11434/v1":  true,
		"http://127.0.0.1:8080/v1":   true,
		"http://127.1.2.3/v1":        true,
		"http://[::1]:8080/v1":       true,
		"https://localhost/v1":       true,
		"https://api.example.com":    false,
		"http://192.168.1.2/v1":      false,
		"http://api.example.com/v1":  false,
		"":                           false,
		"https://localhost.evil.com": false,
	} {
		if got := KeyOptional(in); got != want {
			t.Errorf("%q: %v", in, got)
		}
	}
}

func TestCompleteWithoutKeyOnLoopback(t *testing.T) {
	calls := 0
	auth := "unset"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, has := r.Header["Authorization"]
		auth = r.Header.Get("Authorization")
		if has && auth == "" {
			auth = "<empty header>"
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"Add parser"}}]}`))
	}))
	defer srv.Close()
	// httptest listens on 127.0.0.1, a service on this computer.
	text, err := (&Client{}).Complete(context.Background(), Config{BaseURL: srv.URL + "/v1", Model: "m"}, "  ", nil)
	if err != nil || text != "Add parser" || calls != 1 || auth != "" {
		t.Fatalf("%q %v calls %d auth %q", text, err, calls, auth)
	}
	// A key, when given, is still sent.
	if _, err := (&Client{}).Complete(context.Background(), Config{BaseURL: srv.URL + "/v1", Model: "m"}, "sk-local", nil); err != nil || auth != "Bearer sk-local" {
		t.Fatalf("%v auth %q", err, auth)
	}
	// A remote service still needs a key and is not contacted without one.
	if _, err := (&Client{}).Complete(context.Background(), Config{BaseURL: "https://api.example.invalid/v1", Model: "m"}, "", nil); kind(err) != NotConfigured {
		t.Fatalf("remote without key: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls %d", calls)
	}
}

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"  Fix bug \n":              "Fix bug",
		"```\nFix bug\n\nBody\n```": "Fix bug\n\nBody",
		"```git\nFix bug\n```":      "Fix bug",
		"Use `x` instead":           "Use `x` instead",
	} {
		if got := Clean(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}
