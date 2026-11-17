package desktop

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/ai"
	"gitowl/internal/gittest"
	"gitowl/internal/prefs"
	"gitowl/internal/secrets"
	"gitowl/internal/workflow"
)

// service is a local OpenAI-compatible service for UI tests.
type service struct {
	*httptest.Server
	mu    sync.Mutex
	users []string
	reply func(w http.ResponseWriter, r *http.Request)
}

func newService(t *testing.T) *service {
	s := &service{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []ai.Message `json:"messages"`
		}
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &body)
		s.mu.Lock()
		if len(body.Messages) > 1 {
			s.users = append(s.users, body.Messages[1].Content)
		}
		reply := s.reply
		s.mu.Unlock()
		// The description request always gets the default reply, so a
		// test's reply sets the subject line only.
		if reply == nil || isDescribe(body.Messages) {
			reply = draftAnswer(body.Messages)
		}
		reply(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

// draftText is what the default replies make of one draft: the subject line
// from the first request and the description from the second.
const draftText = "Add greeting\n\nExplain what changed."

// isDescribe reports the second request of a draft, which writes the
// description.
func isDescribe(msgs []ai.Message) bool {
	return len(msgs) > 0 && strings.Contains(msgs[0].Content, "description of a Git commit")
}

// draftAnswer replies with a subject line to the first draft request and a
// description to the second.
func draftAnswer(msgs []ai.Message) func(http.ResponseWriter, *http.Request) {
	content := "Add greeting"
	if isDescribe(msgs) {
		content = "Explain what changed."
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"` + content + `"}}]}`))
	}
}

func (s *service) set(reply func(w http.ResponseWriter, r *http.Request)) {
	s.mu.Lock()
	s.reply = reply
	s.mu.Unlock()
}

func (s *service) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.users)
}

// aiHarness is a harness with a configured AI service.
func newAIHarness(t *testing.T, q *queued) (*harness, *service, *secrets.Memory) {
	g := gittest.New(t)
	srv := newService(t)
	keys := &secrets.Memory{}
	keys.Set(secrets.APIKeyAccount, "sk-ui-test")
	wf := workflow.New(g.Runner)
	wf.SetAI(workflow.AIConfig{Client: &ai.Client{}, Secrets: keys})
	p := prefs.Default()
	p.Language = "en"
	p.AI = prefs.AI{BaseURL: srv.URL + "/v1", Model: "test-model", Language: prefs.Auto}
	opts := Options{Workflow: wf, Preferred: []string{"en-US"}, Preferences: p, AITimeout: 2 * time.Second, Go: func(fn func()) { fn() }}
	if q != nil {
		opts.Go = q.Go
	}
	u := New(opts)
	return &harness{Env: g, u: u, tt: ui.NewTester(window(u), 1280, 800)}, srv, keys
}

func stagedRepo(h *harness) string {
	dir := h.Repo()
	gittest.Write(h.T, filepath.Join(dir, "a.txt"), "staged content\n")
	h.Git(dir, "add", "a.txt")
	gittest.Write(h.T, filepath.Join(dir, "a.txt"), "staged content\nUNSTAGED LINE\n")
	return dir
}

func TestGenerateDraftSendsDirectlyAndCommit(t *testing.T) {
	h, srv, _ := newAIHarness(t, nil)
	dir := stagedRepo(h)
	h.open(dir)
	if srv.calls() != 0 {
		t.Fatal("opening contacted the service")
	}
	// Viewing the request is optional and sends nothing.
	h.u.inspectRequest()
	h.tt.Frame()
	if srv.calls() != 0 || !h.u.ai.inspectOpen {
		t.Fatalf("view request: %d calls, open %v", srv.calls(), h.u.ai.inspectOpen)
	}
	h.want("Request for a draft", "Cancel")
	if !strings.Contains(h.u.ai.inspectText, "staged content") || strings.Contains(h.u.ai.inspectText, "UNSTAGED LINE") {
		t.Fatalf("request:\n%s", h.u.ai.inspectText)
	}
	snapshot(t, h.tt, "draft-request")
	h.click("Cancel")
	if srv.calls() != 0 || h.u.ai.inspectOpen {
		t.Fatal("closing the request sent it")
	}
	// Generate sends its two requests at once: no dialog in between. The reply
	// is a suggestion, not filled by itself (issue 09).
	h.click("Generate Draft")
	if srv.calls() != 2 || h.u.cm.message != draftText || h.u.ai.inspectOpen {
		t.Fatalf("calls %d message %q pending %v", srv.calls(), h.u.cm.message, h.u.ai.pending)
	}
	if out := h.Git(dir, "rev-list", "--all"); out != "" {
		t.Fatal("generating committed")
	}
	h.u.cm.message = "Add greeting\n\nEdited by hand."
	h.tt.Frame()
	h.click("Commit")
	if got := h.Git(dir, "log", "-1", "--format=%B"); got != "Add greeting\n\nEdited by hand.\n\n" {
		t.Fatalf("committed %q", got)
	}
	snapshot(t, h.tt, "draft-committed")
}

func TestDraftKeepsWordsUntilReplaced(t *testing.T) {
	h, srv, _ := newAIHarness(t, nil)
	h.open(stagedRepo(h))
	h.u.cm.message = "My own words"
	h.tt.Frame()
	h.click("Generate Draft")
	if h.u.cm.message != "My own words" || h.u.ai.pending == nil {
		t.Fatalf("draft replaced words without asking: %q", h.u.cm.message)
	}
	h.want("Add greeting", "Replace Message", "Discard")
	snapshot(t, h.tt, "draft-pending")
	h.click("Discard")
	if h.u.cm.message != "My own words" {
		t.Fatal("dismiss changed the message")
	}
	h.click("Generate Draft")
	h.click("Replace Message")
	if h.u.cm.message != draftText {
		t.Fatal(h.u.cm.message)
	}
	// Regenerate suggests again; the reply is accepted explicitly (issue 09
	// never fills the message by itself, even over an AI draft).
	srv.set(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"Add a friendlier greeting"}}]}`))
	})
	h.click("Generate Draft")
	if h.tt.HasText("Replace Message") {
		h.click("Replace Message")
	}
	if h.u.cm.message != "Add a friendlier greeting\n\nExplain what changed." {
		t.Fatal(h.u.cm.message)
	}
}

func TestDraftFailuresKeepWordsAndRetry(t *testing.T) {
	h, srv, _ := newAIHarness(t, nil)
	h.open(stagedRepo(h))
	h.u.cm.message = "Keep me"
	srv.set(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"message":"bad key"}}`))
	})
	h.tt.Frame()
	h.click("Generate Draft")
	h.want("The service rejected the request's authentication. Check the API key in AI Settings; a service on this Mac may need one too. Your message is kept.", "Generate Draft")
	if h.u.cm.message != "Keep me" {
		t.Fatal("failure lost the message")
	}
	srv.set(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"Add greeting"}}]}`))
	})
	h.click("Generate Draft")
	if h.u.cm.message != "Keep me" || h.u.ai.pending == nil || h.u.ai.err != nil {
		t.Fatalf("retry %+v message %q", h.u.ai, h.u.cm.message)
	}
}

func TestDraftNeedsStagingAndConfiguration(t *testing.T) {
	h, srv, keys := newAIHarness(t, nil)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.open(dir)
	// Without staged changes the draft action is unavailable and says why.
	h.click("Generate Draft")
	if h.u.ai.generating || h.u.ai.err != nil || h.u.draftBlock() != "draftNeedsStaged" || srv.calls() != 0 {
		t.Fatalf("draft without staged changes: %+v", h.u.ai)
	}
	h.click("Stage a.txt")
	keys.Delete(secrets.APIKeyAccount)
	// A service on this Mac needs no key.
	h.u.inspectRequest()
	h.tt.Frame()
	h.want("Request for a draft")
	h.click("Cancel")
	h.click("Generate Draft")
	if srv.calls() != 2 || h.u.cm.message != draftText {
		t.Fatalf("local service without key: %d calls, message %q pending %v", srv.calls(), h.u.cm.message, h.u.ai.pending)
	}
	// A service elsewhere does: nothing is sent and the settings are offered.
	p := h.u.prefs
	p.AI.BaseURL = "https://api.example.invalid/v1"
	h.u.setPreferences(p)
	h.tt.Frame()
	h.click("Generate Draft")
	h.want("Set the service URL and model in AI Settings, and an API key unless the service runs on this Mac.", "Generate Draft")
	if h.u.cm.message != draftText {
		t.Fatal("failure changed the message")
	}
	if srv.calls() != 2 {
		t.Fatalf("%d calls", srv.calls())
	}
}

func TestCancelDraftKeepsWords(t *testing.T) {
	q := &queued{}
	h, srv, _ := newAIHarness(t, q)
	release := make(chan struct{})
	srv.set(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)
	dir := stagedRepo(h)
	h.u.OpenPath(dir)
	q.drain()
	h.u.cm.message = "Mine"
	h.u.generateDraft()
	h.tt.Frame()
	h.want("Writing a draft…", "Cancel")
	done := make(chan struct{})
	go func() { q.run(0); close(done) }()
	time.Sleep(50 * time.Millisecond)
	h.u.cancelDraft()
	<-done
	h.tt.Frame()
	if h.u.ai.generating || h.u.cm.message != "Mine" || h.u.ai.err == nil || h.u.ai.err.code != workflow.CodeCancelled {
		t.Fatalf("after cancel %+v %q", h.u.ai, h.u.cm.message)
	}
	h.want("The operation was cancelled.")
}

func TestDraftForChangedStagingIsMarked(t *testing.T) {
	h, srv, _ := newAIHarness(t, nil)
	dir := stagedRepo(h)
	h.open(dir)
	srv.set(func(w http.ResponseWriter, r *http.Request) {
		gittest.Write(t, filepath.Join(dir, "b.txt"), "b\n")
		h.Git(dir, "add", "b.txt")
		w.Write([]byte(`{"choices":[{"message":{"content":"Add a"}}]}`))
	})
	h.click("Generate Draft")
	if h.u.cm.message != "" || h.u.ai.pending == nil || !h.u.ai.pending.Stale {
		t.Fatalf("stale draft applied: %q %+v", h.u.cm.message, h.u.ai.pending)
	}
	h.want("Use Anyway", "Regenerate")
}

func TestDraftAfterSwitchIsDropped(t *testing.T) {
	q := &queued{}
	h, srv, _ := newAIHarness(t, q)
	a := stagedRepo(h)
	b := h.Repo()
	h.u.OpenPath(a)
	q.drain()
	h.u.generateDraft()
	h.u.OpenPath(b)
	q.runLast() // b opens
	q.drain()   // a's draft arrives late
	if h.u.repo.Root() != b || h.u.cm.message != "" || h.u.ai.pending != nil || h.u.ai.generating {
		t.Fatalf("a's draft reached b: %q %+v", h.u.cm.message, h.u.ai)
	}
	// A→B→A: a draft asked for before leaving a does not reach a's new
	// opening either.
	h.u.OpenPath(a)
	q.drain()
	h.u.generateDraft()
	h.u.OpenPath(b)
	q.runLast()
	h.u.OpenPath(a)
	q.runLast() // a opens again
	q.drain()   // the draft asked for by the earlier opening of a
	if h.u.repo.Root() != a || h.u.cm.message != "" || h.u.ai.pending != nil || h.u.ai.generating {
		t.Fatalf("late draft reached the new opening: %q %+v", h.u.cm.message, h.u.ai)
	}
	if out := h.Git(a, "rev-list", "--all"); out != "" {
		t.Fatal("committed")
	}
	_ = srv
}

func TestWordsTypedWhileGeneratingAreKept(t *testing.T) {
	q := &queued{}
	h, srv, _ := newAIHarness(t, q)
	h.u.OpenPath(stagedRepo(h))
	q.drain()
	h.u.generateDraft()
	h.u.cm.message = "Typed while waiting"
	q.drain()
	h.tt.Frame()
	if h.u.cm.message != "Typed while waiting" || h.u.ai.pending == nil || srv.calls() != 2 {
		t.Fatalf("message %q pending %v calls %d", h.u.cm.message, h.u.ai.pending, srv.calls())
	}
	h.want("Replace Message", "Discard")
}

func TestAISettingsSaveSeparately(t *testing.T) {
	var saved prefs.Preferences
	keys := &secrets.Memory{}
	g := gittest.New(t)
	wf := workflow.New(g.Runner)
	wf.SetAI(workflow.AIConfig{Secrets: keys})
	u := New(Options{Workflow: wf, Preferred: []string{"en-US"}, Go: func(fn func()) { fn() },
		SavePreferences: func(p prefs.Preferences) error { saved = p; return nil }})
	tt := ui.NewTester(window(u), 1280, 800)
	u.openAISettings()
	tt.Frame()
	tt.Advance(time.Second) // let the dialog finish its entrance before measuring
	tt.Frame()
	for _, s := range []string{"Service URL", "Model", "API key", "Commit message prompt", "Draft language", "No key saved."} {
		if !tt.HasText(s) {
			t.Fatalf("missing %q in %q", s, tt.Texts())
		}
	}
	// The service URL field is a wide text box stacked below its title
	// and description, as the design's rows place wide controls (issue 18).
	title, ok1 := tt.Find("Service URL")
	desc, ok2 := tt.Find(u.t("aiBaseURLHint"))
	if !ok1 || !ok2 {
		t.Fatalf("service URL item not found in %q", tt.Texts())
	}
	tt.ClickAt(title.X+40, desc.Y+desc.H+18)
	tt.Type("https://api.example.com/v1")
	tt.Frame()
	if saved.AI.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("base URL not saved: %+v", saved.AI)
	}
	u.form.key = "sk-from-settings"
	tt.Frame()
	if err := tt.Click("Save Key"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if k, _ := keys.Get(secrets.APIKeyAccount); k != "sk-from-settings" || u.form.key != "" {
		t.Fatalf("key %q form %q", k, u.form.key)
	}
	data, _ := json.Marshal(saved)
	if strings.Contains(string(data), "sk-from-settings") {
		t.Fatal("key in preferences")
	}
	tt.Advance(time.Second)
	tt.Frame()
	if !tt.HasText("A key is saved in the Keychain.") {
		t.Fatalf("texts %q", tt.Texts())
	}
	snapshot(t, tt, "ai-settings")
}

func TestDraftLanguageSwitchIsSeparateFromInterface(t *testing.T) {
	h, srv, _ := newAIHarness(t, nil)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "h.txt"), "h\n")
	h.Commit(dir, "修复登录问题")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Git(dir, "add", "a.txt")
	h.open(dir)
	h.u.inspectRequest()
	h.tt.Frame()
	h.want("Request for a draft")
	if !strings.Contains(h.u.ai.inspectText, "简体中文") {
		t.Fatalf("request missing Chinese:\n%s", h.u.ai.inspectText)
	}
	h.click("Cancel")
	p := h.u.prefs
	p.AI.Language = "en"
	h.u.setPreferences(p)
	if h.u.prefs.AI.Language != "en" || h.u.prefs.Language != "en" {
		t.Fatalf("prefs %+v", h.u.prefs)
	}
	h.u.inspectRequest()
	h.tt.Frame()
	h.want("Request for a draft")
	if !strings.Contains(h.u.ai.inspectText, "English") {
		t.Fatalf("request missing English:\n%s", h.u.ai.inspectText)
	}
	snapshot(t, h.tt, "draft-request-language")
	h.click("Cancel")
	h.click("Generate Draft")
	srv.mu.Lock()
	last := srv.users[len(srv.users)-1]
	srv.mu.Unlock()
	if !strings.Contains(last, "修复登录问题") {
		t.Fatalf("history not sent:\n%s", last)
	}
	// The interface stays English.
	h.want("Commit", "Staged")
}

func TestContextSizeSetting(t *testing.T) {
	var saved []prefs.Preferences
	g := gittest.New(t)
	wf := workflow.New(g.Runner)
	wf.SetAI(workflow.AIConfig{Secrets: &secrets.Memory{}})
	u := New(Options{Workflow: wf, Preferred: []string{"en-US"}, Go: func(fn func()) { fn() },
		SavePreferences: func(p prefs.Preferences) error { saved = append(saved, p); return nil }})
	tt := ui.NewTester(window(u), 1280, 800)
	u.openAISettings()
	tt.Frame()
	// The default is 8K, with its input limit explained.
	for _, s := range []string{"Model context", "8K (8,192 tokens)"} {
		if !tt.HasText(s) {
			t.Fatalf("missing %q in %q", s, tt.Texts())
		}
	}
	if !strings.Contains(strings.Join(tt.Texts(), "\n"), "about 6,144 tokens") {
		t.Fatalf("input limit not explained: %q", tt.Texts())
	}
	// Model context sits lower on the stacked AI page (issue 18); let the
	// dialog settle, then scroll its select into the dialog before use.
	tt.Advance(time.Second)
	tt.Frame()
	mc, _ := tt.Find("8K (8,192 tokens)")
	for i := 0; i < 20 && (mc.H == 0 || mc.Y < 170 || mc.Y+mc.H > 640); i++ {
		tt.Scroll(640, 400, 0, 120)
		tt.Frame()
		mc, _ = tt.Find("8K (8,192 tokens)")
	}
	// A preset saves its number.
	if err := tt.Click("8K (8,192 tokens)"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("32K (32,768 tokens)"); err != nil {
		t.Fatalf("%v %q", err, tt.Texts())
	}
	tt.Frame()
	if u.prefs.AI.ContextTokens != 32768 || saved[len(saved)-1].AI.ContextTokens != 32768 {
		t.Fatalf("preset not saved: %d", u.prefs.AI.ContextTokens)
	}
	// Custom: a valid number saves, an invalid one is explained and not saved.
	tt.Click("32K (32,768 tokens)")
	tt.Frame()
	if err := tt.Click("Custom…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !u.form.contextCustom || u.form.contextText != "32768" {
		t.Fatalf("custom field %+v", u.form)
	}
	if err := tt.Click("Context size in tokens"); err != nil {
		t.Fatalf("custom field not shown: %v %q", err, tt.Texts())
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("12000")
	tt.Frame()
	if u.prefs.AI.ContextTokens != 12000 {
		t.Fatalf("custom not saved: %d (%q)", u.prefs.AI.ContextTokens, u.form.contextText)
	}
	n := len(saved)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("2k")
	tt.Frame()
	if u.prefs.AI.ContextTokens != 12000 || len(saved) != n || !tt.HasText("Enter a whole number of tokens from 4,096 to 1,048,576. The previous size is kept until then.") {
		t.Fatalf("invalid value: %d saves %d texts %q", u.prefs.AI.ContextTokens, len(saved)-n, tt.Texts())
	}
	snapshot(t, tt, "ai-settings-context")

	// A saved size outside the presets shows as Custom with its number.
	p := prefs.Default()
	p.AI.ContextTokens = 20000
	u2 := New(Options{Workflow: wf, Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }, Preferences: p})
	tt2 := ui.NewTester(window(u2), 1280, 800)
	u2.openAISettings()
	tt2.Frame()
	tt2.Frame()
	if !tt2.HasText("Custom…") || u2.form.contextText != "20000" {
		t.Fatalf("non-preset size: %q %q", u2.form.contextText, tt2.Texts())
	}
}

func TestDraftShowsEstimateAndShortening(t *testing.T) {
	h, srv, _ := newAIHarness(t, nil)
	dir := stagedRepo(h)
	h.open(dir)
	h.click("Generate Draft")
	req := h.u.ai.last
	if req == nil || req.Budget.Input != 6144 {
		t.Fatalf("last request %+v", req)
	}
	snapshot(t, h.tt, "draft-estimate")
	// The estimate is on the request preview dialog (the footer now carries
	// the state hint, with the estimate as its tooltip).
	h.u.inspectRequest()
	h.tt.Frame()
	if h.u.ai.inspect == nil {
		t.Fatal("inspect did not load the request")
	}
	h.click("Cancel")
	// A staged change too large for 4K is shortened, sent once and says so.
	gittest.Write(t, filepath.Join(dir, "big.txt"), strings.Repeat("a long staged line of text\n", 600))
	gittest.Write(t, filepath.Join(dir, "package-lock.json"), "{}\n")
	h.Git(dir, "add", "big.txt", "package-lock.json")
	p := h.u.prefs
	p.AI.ContextTokens = 4096
	h.u.setPreferences(p)
	h.u.Refresh()
	h.tt.Frame()
	h.u.cm.message = "Mine"
	h.u.generateDraft()
	h.tt.Frame()
	if srv.calls() != 4 || h.u.cm.message != "Mine" || h.u.ai.err != nil || h.u.ai.pending == nil {
		t.Fatalf("shortened: %d calls %q %+v", srv.calls(), h.u.cm.message, h.u.ai.err)
	}
	if got := h.u.shortenedText(h.u.ai.last); !strings.Contains(got, "statistics only") {
		t.Fatalf("shortened %q", got)
	}
	h.u.inspectRequest()
	h.tt.Frame()
	h.want("package-lock.json", "big.txt")
	snapshot(t, h.tt, "draft-request-shortened")
	if srv.calls() != 4 {
		t.Fatal("viewing sent")
	}
}

// TestManualCommitWhenTheServiceFails keeps the manual path whole: a
// failed draft leaves the message and Commit as they were, and the commit
// has only the staged changes.
func TestManualCommitWhenTheServiceFails(t *testing.T) {
	h, srv, _ := newAIHarness(t, nil)
	dir := stagedRepo(h)
	h.open(dir)
	srv.Close()
	h.typeMessage("Write it by hand")
	h.click("Generate Draft")
	if h.u.ai.err == nil || h.u.cm.message != "Write it by hand" {
		t.Fatalf("draft error %+v, message %q", h.u.ai.err, h.u.cm.message)
	}
	// Panel B signals a ready commit with the ⌘↵ hint on the enabled
	// split button, not a status line; Retry still offers the draft again.
	h.want("Generate Draft", "Commit")
	if h.u.commitBlock() != "" {
		t.Fatalf("commit blocked after a failed draft: %q", h.u.commitBlock())
	}
	// Commit waits while another Git change runs, then works.
	h.u.wt.bulk = "stage"
	h.tt.Frame()
	h.click("Commit")
	if out := h.Git(dir, "rev-list", "--all"); out != "" {
		t.Fatal("committed while busy")
	}
	h.u.wt.bulk = ""
	h.tt.Frame()
	h.click("Commit")
	if got := h.Git(dir, "show", "HEAD:a.txt"); got != "staged content\n" {
		t.Fatalf("committed %q, want only the staged content", got)
	}
	if got := h.Git(dir, "diff", "--name-only"); got != "a.txt\n" {
		t.Fatalf("unstaged change lost: %q", got)
	}
}

// TestComposerReachableInSmallWindows keeps the list, the draft action,
// the message and Commit on screen together in narrow windows.
func TestComposerReachableInSmallWindows(t *testing.T) {
	for _, size := range [][2]int{{600, 600}, {560, 480}} {
		t.Run(itoa(size[0]), func(t *testing.T) {
			h := newHarness(t, size[0], size[1])
			dir := h.Repo()
			for i := range 12 {
				gittest.Write(t, filepath.Join(dir, fmt.Sprintf("f%02d.txt", i)), "x\n")
			}
			h.open(dir)
			for _, s := range []string{"Stage f00.txt", "Generate Draft", "Summary", "Commit", "Nothing staged yet"} {
				if !h.tt.HasText(s) {
					t.Fatalf("missing %q in %q", s, h.tt.Texts())
				}
			}
			// The list scrolls above the editor to its last file.
			i, _ := h.u.wt.row(changeKey{area: workflow.Untracked, path: "f11.txt"})
			h.u.wt.list.ScrollIntoView(i)
			h.tt.Frame()
			h.click("Stage f11.txt")
			if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "f11.txt\n" {
				t.Fatalf("index %q", got)
			}
			msg, _ := h.tt.Find("Summary")
			if r, _ := h.tt.Find("f10.txt"); r.Y+r.H > msg.Y {
				t.Fatalf("list row %v under the editor %v", r, msg)
			}
			snapshot(t, h.tt, "composer-"+itoa(size[0]))
		})
	}
}
