// Package ai calls an OpenAI-compatible chat completions service to write
// commit message drafts. It sends exactly the messages it is given, once,
// without streaming, and never logs the API key. A service on this computer
// may be used without a key.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultTimeout bounds one request, including reading the reply.
const DefaultTimeout = 60 * time.Second

// maxReply is the most of a reply body GitOwl reads.
const maxReply = 1 << 20

// Kind classifies a failure.
type Kind string

const (
	NotConfigured Kind = "NotConfigured"
	InvalidURL    Kind = "InvalidURL"
	Unauthorized  Kind = "Unauthorized"
	RateLimited   Kind = "RateLimited"
	TooLarge      Kind = "TooLarge"
	Timeout       Kind = "Timeout"
	Cancelled     Kind = "Cancelled"
	Network       Kind = "Network"
	BadResponse   Kind = "BadResponse"
	ServiceError  Kind = "ServiceError"
)

// Error is a failed request. Detail is the service's own message, with the
// API key removed.
type Error struct {
	Kind   Kind
	Status int
	Detail string
}

func (e *Error) Error() string {
	s := "ai: " + string(e.Kind)
	if e.Status != 0 {
		s += fmt.Sprintf(" (HTTP %d)", e.Status)
	}
	if e.Detail != "" {
		s += ": " + e.Detail
	}
	return s
}

// Config names the service and model.
type Config struct {
	// BaseURL is the API base, such as https://api.openai.com/v1; GitOwl
	// posts to BaseURL + "/chat/completions".
	BaseURL string
	Model   string
	// Timeout defaults to DefaultTimeout.
	Timeout time.Duration
}

// Message is one chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Endpoint returns the chat completions URL for base: it trims spaces and
// trailing slashes, accepts a base already ending in /chat/completions, and
// requires https, or http for a service on this computer only, so the API
// key never crosses the network in clear text.
func Endpoint(base string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", &Error{Kind: NotConfigured}
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", &Error{Kind: InvalidURL, Detail: base}
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !loopback(u.Hostname()) {
			return "", &Error{Kind: InvalidURL, Detail: base}
		}
	default:
		return "", &Error{Kind: InvalidURL, Detail: base}
	}
	p := strings.TrimRight(u.Path, "/")
	p = strings.TrimSuffix(p, "/chat/completions")
	u.Path = p + "/chat/completions"
	return u.String(), nil
}

// KeyOptional reports whether base is a valid service URL on this computer,
// for which an API key may be left empty. Such a request is sent without an
// Authorization header.
func KeyOptional(base string) bool {
	endpoint, err := Endpoint(base)
	if err != nil {
		return false
	}
	u, err := url.Parse(endpoint)
	return err == nil && loopback(u.Hostname())
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Client sends requests. The zero value uses a client honoring the
// system's proxy environment.
type Client struct {
	HTTP *http.Client
}

type request struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type reply struct {
	Choices []struct {
		Message struct {
			Content *string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *serviceError `json:"error"`
}

type serviceError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    any    `json:"code"`
}

// Complete sends messages and returns the reply's text, trimmed and without
// a surrounding code fence.
func (c *Client) Complete(ctx context.Context, cfg Config, key string, messages []Message) (string, error) {
	endpoint, err := Endpoint(cfg.BaseURL)
	if err != nil {
		return "", err
	}
	key = strings.TrimSpace(key)
	if strings.TrimSpace(cfg.Model) == "" || key == "" && !KeyOptional(cfg.BaseURL) {
		return "", &Error{Kind: NotConfigured}
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	body, err := json.Marshal(request{Model: strings.TrimSpace(cfg.Model), Messages: messages})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", &Error{Kind: InvalidURL, Detail: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", transportError(ctx, err, key)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReply+1))
	if err != nil {
		return "", transportError(ctx, err, key)
	}
	if len(data) > maxReply {
		return "", &Error{Kind: BadResponse, Status: resp.StatusCode, Detail: "reply over 1 MB"}
	}
	var r reply
	jsonErr := json.Unmarshal(data, &r)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", statusError(resp.StatusCode, r.Error, data, key)
	}
	if jsonErr != nil {
		return "", &Error{Kind: BadResponse, Status: resp.StatusCode, Detail: redact(snippet(data), key)}
	}
	if r.Error != nil {
		return "", statusError(resp.StatusCode, r.Error, data, key)
	}
	if len(r.Choices) == 0 || r.Choices[0].Message.Content == nil {
		return "", &Error{Kind: BadResponse, Status: resp.StatusCode, Detail: "no message in the reply"}
	}
	text := Clean(*r.Choices[0].Message.Content)
	if text == "" {
		return "", &Error{Kind: BadResponse, Status: resp.StatusCode, Detail: "the reply is empty"}
	}
	return text, nil
}

func transportError(ctx context.Context, err error, key string) error {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return &Error{Kind: Timeout}
	case errors.Is(ctx.Err(), context.Canceled):
		return &Error{Kind: Cancelled}
	}
	return &Error{Kind: Network, Detail: redact(err.Error(), key)}
}

func statusError(status int, se *serviceError, body []byte, key string) error {
	detail := ""
	code := ""
	if se != nil {
		detail = se.Message
		code = fmt.Sprint(se.Code) + " " + se.Type
	} else {
		detail = snippet(body)
	}
	detail = redact(detail, key)
	lower := strings.ToLower(detail + " " + code)
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return &Error{Kind: Unauthorized, Status: status, Detail: detail}
	case status == http.StatusRequestEntityTooLarge,
		strings.Contains(lower, "context_length"), strings.Contains(lower, "maximum context"),
		strings.Contains(lower, "too many tokens"), strings.Contains(lower, "too large"):
		return &Error{Kind: TooLarge, Status: status, Detail: detail}
	case status == http.StatusTooManyRequests:
		return &Error{Kind: RateLimited, Status: status, Detail: detail}
	}
	return &Error{Kind: ServiceError, Status: status, Detail: detail}
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

func redact(s, key string) string {
	if k := strings.TrimSpace(key); len(k) >= 4 {
		s = strings.ReplaceAll(s, k, "••••")
	}
	return s
}

// Clean trims a reply and removes a code fence around all of it.
func Clean(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") && strings.HasSuffix(s, "```") && len(s) >= 6 {
		inner := strings.TrimSuffix(strings.TrimPrefix(s, "```"), "```")
		// Drop a language tag on the opening line.
		if i := strings.IndexByte(inner, '\n'); i >= 0 && !strings.ContainsAny(inner[:i], " \t") {
			inner = inner[i+1:]
		}
		s = strings.TrimSpace(inner)
	}
	return s
}
