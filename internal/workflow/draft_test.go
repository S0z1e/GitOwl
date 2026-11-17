package workflow

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gitowl/internal/ai"
	"gitowl/internal/secrets"
)

// draftText is what the default replies make of one draft: the subject
// line from the first request and the description from the second.
const draftText = "Add greeting\n\nExplain what changed."

// fakeService is a local OpenAI-compatible service recording requests.
type fakeService struct {
	*httptest.Server
	mu       sync.Mutex
	requests []fakeRequest
	reply    func(w http.ResponseWriter, r *http.Request)
}

type fakeRequest struct {
	Auth     string
	Model    string
	Messages []ai.Message
}

func newFakeService(t *testing.T) *fakeService {
	f := &fakeService{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string       `json:"model"`
			Messages []ai.Message `json:"messages"`
		}
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &body)
		f.mu.Lock()
		f.requests = append(f.requests, fakeRequest{Auth: r.Header.Get("Authorization"), Model: body.Model, Messages: body.Messages})
		reply := f.reply
		f.mu.Unlock()
		if reply == nil {
			reply = draftAnswer(body.Messages)
		}
		reply(w, r)
	}))
	t.Cleanup(f.Close)
	return f
}

// draftAnswer replies with a subject line to the first draft request and a
// description to the second.
func draftAnswer(msgs []ai.Message) func(http.ResponseWriter, *http.Request) {
	content := "Add greeting"
	if len(msgs) > 0 && strings.Contains(msgs[0].Content, "description of a Git commit") {
		content = "Explain what changed."
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"` + content + `"}}]}`))
	}
}

func (f *fakeService) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// first returns the first request, which writes the subject line.
func (f *fakeService) first() fakeRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[0]
}

func (f *fakeService) last() fakeRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[len(f.requests)-1]
}

func aiEnv(t *testing.T) (*env, *fakeService, *secrets.Memory, ai.Config) {
	e := newEnv(t)
	f := newFakeService(t)
	s := &secrets.Memory{}
	s.Set(secrets.APIKeyAccount, "sk-test-key")
	e.app.SetAI(AIConfig{Client: &ai.Client{}, Secrets: s})
	return e, f, s, ai.Config{BaseURL: f.URL + "/v1", Model: "test-model", Timeout: 2 * time.Second}
}

func TestDraftSendsOnlyStagedChanges(t *testing.T) {
	e, f, _, cfg := aiEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "hello.go"), "package main\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "hello.go"), "package main\n\nfunc hello() string { return \"staged-change\" }\n")
	e.git(dir, "add", "hello.go")
	write(t, filepath.Join(dir, "hello.go"), "package main\n\nfunc hello() string { return \"UNSTAGED-SECRET\" }\n")
	write(t, filepath.Join(dir, "notes.txt"), "UNTRACKED-SECRET\n")
	r, _ := e.open(dir)
	ctx := context.Background()

	req, err := r.PrepareDraft(ctx, DraftOptions{Language: "auto", InterfaceLanguage: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if f.count() != 0 {
		t.Fatal("preparing a draft contacted the service")
	}
	all := req.Messages[0].Content + req.Messages[1].Content
	if !strings.Contains(all, "staged-change") || strings.Contains(all, "UNSTAGED-SECRET") || strings.Contains(all, "UNTRACKED-SECRET") || strings.Contains(all, "notes.txt") {
		t.Fatalf("request content:\n%s", all)
	}
	// One commit of history: its style and language are the reference.
	if req.Style != StyleFromHistory || len(req.Recent) != 1 || req.LanguageSource != LanguageFromHistory || !strings.Contains(req.Messages[0].Content, "in English") {
		t.Fatalf("default instructions:\n%s", req.Messages[0].Content)
	}

	d, err := r.GenerateDraft(ctx, req, cfg)
	if err != nil || d.Text != draftText || d.Stale {
		t.Fatalf("draft %+v %v", d, err)
	}
	got := f.first()
	if f.count() != 2 || got.Auth != "Bearer sk-test-key" || got.Model != "test-model" || got.Messages[1].Content != req.Messages[1].Content {
		t.Fatalf("sent %+v", got)
	}
	// A draft creates no commit and changes no staging.
	if n := strings.Count(e.git(dir, "log", "--oneline"), "\n"); n != 1 {
		t.Fatalf("%d commits", n)
	}
	if out := e.git(dir, "diff", "--cached", "--name-only"); out != "hello.go\n" {
		t.Fatalf("staging changed: %q", out)
	}
}

func TestDraftCustomPromptAndManualLanguage(t *testing.T) {
	e, _, _, _ := aiEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.git(dir, "add", "a.txt")
	r, _ := e.open(dir)
	req, err := r.PrepareDraft(context.Background(), DraftOptions{Prompt: "Use Conventional Commits: type(scope): subject", Language: "zh-Hans", InterfaceLanguage: "en"})
	if err != nil {
		t.Fatal(err)
	}
	sys := req.Messages[0].Content
	if req.Style != StyleCustom || !strings.Contains(sys, "Use Conventional Commits: type(scope): subject") || !strings.Contains(sys, "take precedence") || strings.Contains(sys, "imperative verb") {
		t.Fatalf("custom prompt:\n%s", sys)
	}
	if req.Language != "zh-Hans" || req.LanguageSource != LanguageChosen || !strings.Contains(sys, "Simplified Chinese") {
		t.Fatalf("language %s %d", req.Language, req.LanguageSource)
	}
}

func TestDraftRefusals(t *testing.T) {
	e, f, _, _ := aiEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	r, _ := e.open(dir)
	ctx := context.Background()
	_, err := r.PrepareDraft(ctx, DraftOptions{})
	wantCode(t, err, CodeNothingStaged)
	if f.count() != 0 {
		t.Fatal("contacted the service")
	}
}

func TestDraftServiceFailures(t *testing.T) {
	e, f, keys, cfg := aiEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.git(dir, "add", "a.txt")
	r, _ := e.open(dir)
	ctx := context.Background()
	req, _ := r.PrepareDraft(ctx, DraftOptions{})

	for _, tc := range []struct {
		reply func(w http.ResponseWriter, r *http.Request)
		code  Code
	}{
		{func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(401)
			w.Write([]byte(`{"error":{"message":"bad key sk-test-key"}}`))
		}, "AIUnauthorized"},
		{func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`not json`)) }, "AIBadResponse"},
		{func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":{"message":"maximum context length exceeded","code":"context_length_exceeded"}}`))
		}, "AITooLarge"},
		{func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(3 * time.Second):
			case <-r.Context().Done():
			}
		}, "AITimeout"},
	} {
		f.mu.Lock()
		f.reply = tc.reply
		f.mu.Unlock()
		c := cfg
		c.Timeout = 200 * time.Millisecond
		_, err := r.GenerateDraft(ctx, req, c)
		wantCode(t, err, tc.code)
		if strings.Contains(err.Error(), "sk-test-key") {
			t.Fatalf("key leaked: %v", err)
		}
	}
	// Cancelling.
	f.mu.Lock()
	f.reply = func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }
	f.mu.Unlock()
	cctx, cancel := context.WithCancel(ctx)
	time.AfterFunc(50*time.Millisecond, cancel)
	_, err := r.GenerateDraft(cctx, req, cfg)
	wantCode(t, err, CodeCancelled)
	// Not configured: a service elsewhere needs a key.
	keys.Delete(secrets.APIKeyAccount)
	before := f.count()
	_, err = r.GenerateDraft(ctx, req, ai.Config{BaseURL: "https://api.example.invalid/v1", Model: "m"})
	wantCode(t, err, "AINotConfigured")
	_, err = r.GenerateDraft(ctx, req, ai.Config{BaseURL: f.URL + "/v1"})
	wantCode(t, err, "AINotConfigured")
	_, err = r.GenerateDraft(ctx, req, ai.Config{BaseURL: "http://example.com/v1", Model: "m"})
	wantCode(t, err, "AIInvalidURL")
	if f.count() != before {
		t.Fatal("an unusable configuration contacted the service")
	}
	// Nothing was committed by any of it.
	if out := e.git(dir, "rev-list", "--all"); out != "" {
		t.Fatal("a commit was created")
	}
}

func TestDraftLocalServiceWithoutKey(t *testing.T) {
	e, f, keys, cfg := aiEnv(t)
	keys.Delete(secrets.APIKeyAccount)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.git(dir, "add", "a.txt")
	r, _ := e.open(dir)
	ctx := context.Background()
	req, err := r.PrepareDraft(ctx, DraftOptions{})
	if err != nil {
		t.Fatal(err)
	}
	d, err := r.GenerateDraft(ctx, req, cfg)
	if err != nil || d.Text != draftText {
		t.Fatalf("draft %+v %v", d, err)
	}
	if f.count() != 2 || f.last().Auth != "" {
		t.Fatalf("calls %d auth %q", f.count(), f.last().Auth)
	}
	// A local service that wants a key says so, once, and the key can be
	// added.
	f.mu.Lock()
	f.reply = func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-local" {
			w.WriteHeader(401)
			w.Write([]byte(`{"error":{"message":"missing key"}}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"Add a"}}]}`))
	}
	f.mu.Unlock()
	_, err = r.GenerateDraft(ctx, req, cfg)
	wantCode(t, err, "AIUnauthorized")
	if f.count() != 3 {
		t.Fatalf("retried: %d calls", f.count())
	}
	e.app.SaveAPIKey("sk-local")
	if d, err := r.GenerateDraft(ctx, req, cfg); err != nil || d.Text != "Add a\n\nAdd a" || f.last().Auth != "Bearer sk-local" {
		t.Fatalf("with key %+v %v", d, err)
	}
	// A Keychain failure is still reported, and nothing is sent.
	keys.Err = context.DeadlineExceeded
	_, err = r.GenerateDraft(ctx, req, cfg)
	wantCode(t, err, CodeKeychain)
	if f.count() != 5 {
		t.Fatalf("calls %d", f.count())
	}
}

func TestDraftPreparesAndSendsOnce(t *testing.T) {
	e, f, _, cfg := aiEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "staged-line\n")
	e.git(dir, "add", "a.txt")
	write(t, filepath.Join(dir, "a.txt"), "staged-line\nUNSTAGED\n")
	r, _ := e.open(dir)
	ctx := context.Background()
	opts := DraftOptions{InterfaceLanguage: "en"}

	req, d, err := r.Draft(ctx, opts, cfg)
	if err != nil || d.Text != draftText || d.Stale || d.Fingerprint != req.Fingerprint {
		t.Fatalf("draft %+v %v", d, err)
	}
	sent := f.last().Messages[1].Content
	if f.count() != 2 || !strings.HasPrefix(sent, req.Messages[1].Content) || !strings.Contains(sent, "staged-line") || strings.Contains(sent, "UNSTAGED") {
		t.Fatalf("%d calls, sent:\n%s", f.count(), sent)
	}
	// Viewing the request sends nothing.
	if _, err := r.PrepareDraft(ctx, opts); err != nil || f.count() != 2 {
		t.Fatalf("prepare: %v, %d calls", err, f.count())
	}

	// The staged content changes while the request is read: nothing is sent.
	afterDraftContent = func() {
		write(t, filepath.Join(dir, "b.txt"), "b\n")
		e.git(dir, "add", "b.txt")
	}
	_, _, err = r.Draft(ctx, opts, cfg)
	afterDraftContent = nil
	wantCode(t, err, CodeStateChanged)
	if f.count() != 2 {
		t.Fatal("sent a request read from two staged states")
	}

	// Nothing staged, or a closed handle: nothing is sent.
	e.git(dir, "reset", "-q")
	_, _, err = r.Draft(ctx, opts, cfg)
	wantCode(t, err, CodeNothingStaged)
	e.open(e.repo())
	_, _, err = r.Draft(ctx, opts, cfg)
	wantCode(t, err, CodeRepositoryClosed)
	if f.count() != 2 {
		t.Fatalf("%d calls", f.count())
	}
}

func TestDraftFailuresSendOnce(t *testing.T) {
	e, f, _, cfg := aiEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.git(dir, "add", "a.txt")
	r, _ := e.open(dir)
	cfg.Timeout = 200 * time.Millisecond
	for i, reply := range []func(w http.ResponseWriter, r *http.Request){
		func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) },
		func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`not json`)) },
		func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(time.Second):
			case <-r.Context().Done():
			}
		},
	} {
		f.mu.Lock()
		f.reply = reply
		f.mu.Unlock()
		if _, _, err := r.Draft(context.Background(), DraftOptions{}, cfg); err == nil {
			t.Fatalf("%d: no error", i)
		}
		if f.count() != i+1 {
			t.Fatalf("%d: %d calls, a failure was retried", i, f.count())
		}
	}
}

func TestDraftStaleAndClosed(t *testing.T) {
	e, f, _, cfg := aiEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.git(dir, "add", "a.txt")
	r, _ := e.open(dir)
	ctx := context.Background()
	req, _ := r.PrepareDraft(ctx, DraftOptions{})
	// The staged content changes while the service answers.
	f.mu.Lock()
	f.reply = func(w http.ResponseWriter, _ *http.Request) {
		write(t, filepath.Join(dir, "b.txt"), "b\n")
		e.git(dir, "add", "b.txt")
		w.Write([]byte(`{"choices":[{"message":{"content":"Add a"}}]}`))
	}
	f.mu.Unlock()
	d, err := r.GenerateDraft(ctx, req, cfg)
	if err != nil || !d.Stale || d.Text != "Add a\n\nAdd a" {
		t.Fatalf("stale draft %+v %v", d, err)
	}
	// The repository is switched while the service answers.
	f.mu.Lock()
	f.reply = func(w http.ResponseWriter, _ *http.Request) {
		e.app.Open(context.Background(), e.repo())
		w.Write([]byte(`{"choices":[{"message":{"content":"Add a"}}]}`))
	}
	f.mu.Unlock()
	_, err = r.GenerateDraft(ctx, req, cfg)
	wantCode(t, err, CodeRepositoryClosed)
}

func TestAPIKeyStorage(t *testing.T) {
	e, _, keys, _ := aiEnv(t)
	if ok, err := e.app.HasAPIKey(); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if err := e.app.SaveAPIKey("  sk-new  "); err != nil {
		t.Fatal(err)
	}
	if k, _ := keys.Get(secrets.APIKeyAccount); k != "sk-new" {
		t.Fatalf("saved %q", k)
	}
	e.app.SaveAPIKey("")
	if ok, _ := e.app.HasAPIKey(); ok {
		t.Fatal("empty key did not delete")
	}
	keys.Err = context.DeadlineExceeded
	_, err := e.app.HasAPIKey()
	wantCode(t, err, CodeKeychain)
}
