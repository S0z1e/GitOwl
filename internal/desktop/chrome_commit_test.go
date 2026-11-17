package desktop

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/ai"
	"gitowl/internal/gittest"
	"gitowl/internal/prefs"
	"gitowl/internal/secrets"
	"gitowl/internal/workflow"
)

func chromeStage(h *harness, dir, file, content string) {
	h.T.Helper()
	gittest.Write(h.T, filepath.Join(dir, file), content)
	h.Git(dir, "add", file)
	h.u.Refresh()
	h.tt.Frame()
}

func chromeAIHarness(t *testing.T) (*harness, *service) {
	t.Helper()
	g := gittest.New(t)
	srv := newService(t)
	keys := &secrets.Memory{}
	keys.Set(secrets.APIKeyAccount, "sk-ui-test")
	wf := workflow.New(g.Runner)
	wf.SetAI(workflow.AIConfig{Client: &ai.Client{}, Secrets: keys})
	p := prefs.Default()
	p.Language = "en"
	p.AI = prefs.AI{BaseURL: srv.URL + "/v1", Model: "test-model", Language: prefs.Auto}
	u := New(Options{
		Workflow: wf, Preferred: []string{"en-US"}, Preferences: p,
		AITimeout: 2 * time.Second, Go: func(fn func()) { fn() },
	})
	return &harness{Env: g, u: u, tt: ui.NewTester(chromeWindow(u), 1280, 800)}, srv
}

func TestChromeCommitBoxStaysAtListBottom(t *testing.T) {
	for _, size := range []struct{ w, h int }{{1280, 800}, {720, 800}} {
		h, _ := chromeRepo(t, size.w, size.h)
		h.want("main", "Summary", "Description", "Generate Draft", "Amend previous commit", "Commit", "Nothing staged yet")
		for _, extra := range []string{"Draft Options", "More commit options", "Commit and push"} {
			if _, ok := h.tt.Find(extra); ok {
				t.Fatalf("%dx%d kept %q in %q", size.w, size.h, extra, h.tt.Texts())
			}
		}
	}
}

func TestChromeCommitAllowsTypingBeforeStaging(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	h.typeMessage("Written before staging")
	h.want("Nothing staged yet")
	if h.u.cm.message != "Written before staging" {
		t.Fatalf("message %q: typing is allowed before staging", h.u.cm.message)
	}
	h.click("Commit")
	if h.u.cm.hint != nil || h.u.cm.committing {
		t.Fatalf("a disabled Commit ran: %+v", h.u.cm)
	}
	if out := h.Git(dir, "rev-list", "--count", "HEAD"); strings.TrimSpace(out) != "1" {
		t.Fatalf("a commit was created: %q", out)
	}
	chromeStage(h, dir, "a.txt", "a\n")
	h.u.cm.message = "  \n\t"
	h.u.cm.syncFields()
	h.tt.Frame()
	h.want("Add a message to commit")
	h.click("Commit")
	if h.u.cm.hint != nil {
		t.Fatalf("a disabled Commit ran: %+v", h.u.cm.hint)
	}
	h.u.cm.message = "Add a"
	h.u.cm.syncFields()
	h.tt.Frame()
	h.want("Commits only the staged files, to main")
	h.click("Commit")
	if got := h.Git(dir, "log", "-1", "--format=%s"); got != "Add a\n" {
		t.Fatalf("subject %q", got)
	}
}

func TestChromeSummaryCountDoesNotBlock(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	chromeStage(h, dir, "a.txt", "a\n")
	h.click("Summary")
	h.tt.Type("测试🚀x")
	h.tt.Frame()
	h.want("4 / 72")
	long := strings.Repeat("a", 73)
	h.u.cm.summary = long
	h.u.cm.recompose()
	h.tt.Frame()
	h.want("73 / 72")
	if h.u.commitBlock() != "" {
		t.Fatalf("count blocked Commit: %q", h.u.commitBlock())
	}
	if h.u.cm.summary != long {
		t.Fatalf("summary truncated to %q", h.u.cm.summary)
	}
	h.click("Description")
	h.tt.Type("body")
	h.tt.Frame()
	h.want("73 / 72")
	h.click("Commit")
	if got := strings.TrimRight(h.Git(dir, "log", "-1", "--format=%B"), "\n"); got != long+"\n\nbody" {
		t.Fatalf("committed %q", got)
	}
}

func TestChromeAmendStaysEnabledWhenPushed(t *testing.T) {
	h, _, mine, _ := chromeClones(t, 1280, 800)
	gittest.Write(t, filepath.Join(mine, "a.txt"), "mine\n")
	h.Git(mine, "add", "a.txt")
	h.open(mine)
	h.tt.Frame()
	h.click("Amend previous commit")
	if !h.u.amending() {
		t.Fatal("amend stayed off because the commit was pushed")
	}
	if h.u.cm.summary != "" {
		t.Fatalf("amend loaded the previous message: %q", h.u.cm.summary)
	}
}

func TestChromeAmendStaysEnabledWhenDetached(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	chromeStage(h, dir, "a.txt", "a\n")
	h.Git(dir, "switch", "-q", "--detach", "HEAD")
	h.u.Refresh()
	h.tt.Frame()
	h.click("Amend previous commit")
	if !h.u.amending() {
		t.Fatal("amend stayed off because HEAD is detached")
	}
}

func TestChromeAmendDisabledWhenUnborn(t *testing.T) {
	h, _ := chromeHarness(t, 1280, 800)
	h.open(h.Repo())
	h.want("Amend previous commit")
	h.wantTip("Amend previous commit", h.u.t("errorAmendUnborn"))
	_ = h.tt.Click("Amend previous commit")
	h.tt.Frame()
	if h.u.amending() {
		t.Fatal("amend turned on with no commits")
	}
}

func TestChromeAmendKeepsTypedMessageAndNeedsStaged(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	h.typeMessage("My new work")
	h.click("Description")
	h.tt.Type("Details here")
	h.tt.Frame()
	h.click("Amend previous commit")
	if !h.u.amending() {
		t.Fatal("amend did not turn on")
	}
	if h.u.cm.summary != "My new work" || h.u.cm.description != "Details here" {
		t.Fatalf("amend replaced the draft: %+v", h.u.cm)
	}
	if _, ok := h.tt.Find("Amend"); ok {
		t.Fatal("button switched to Amend")
	}
	h.want("Commit", "Nothing staged yet")
	h.click("Commit")
	if strings.TrimSpace(h.Git(dir, "log", "-1", "--format=%s")) != "Initial" {
		t.Fatal("message-only amend ran")
	}
	chromeStage(h, dir, "a.txt", "a\n")
	h.want("Commits only the staged files, to main")
	h.click("Commit")
	if got := h.Git(dir, "log", "-1", "--format=%s"); got != "My new work\n" {
		t.Fatalf("subject %q", got)
	}
	if got := h.Git(dir, "show", "HEAD:a.txt"); got != "a\n" {
		t.Fatalf("staged change not folded in: %q", got)
	}
	if n := strings.Count(h.Git(dir, "log", "--oneline"), "\n"); n != 1 {
		t.Fatalf("amend made a new commit: %d commits", n)
	}
	if h.u.amending() {
		t.Fatal("amend mode stayed on")
	}
}

func TestChromeGenerateDraftDisabledWithoutStaged(t *testing.T) {
	h, _ := chromeRepo(t, 1280, 800)
	h.wantTip("Generate Draft", h.u.t("draftNeedsStaged"))
	h.click("Generate Draft")
	if h.u.ai.generating || h.u.ai.pending != nil || h.u.cm.message != "" {
		t.Fatal("Generate Draft ran without staged files")
	}
}

func TestChromeDraftFillsBlankEditor(t *testing.T) {
	h, srv := chromeAIHarness(t)
	dir := stagedRepo(h)
	h.open(dir)
	h.click("Generate Draft")
	if srv.calls() != 2 {
		t.Fatalf("Generate sent %d requests, want 2", srv.calls())
	}
	if h.u.cm.message != draftText {
		t.Fatalf("blank editor was not filled: %q pending %v", h.u.cm.message, h.u.ai.pending)
	}
	if h.u.ai.pending != nil {
		t.Fatal("a card waited over a blank editor")
	}
	h.click("Commit")
	if got := h.Git(dir, "log", "-1", "--format=%s"); got != "Add greeting\n" {
		t.Fatalf("committed subject %q", got)
	}
}

func TestChromeDraftReplacesUneditedPreviousDraft(t *testing.T) {
	h, srv := chromeAIHarness(t)
	dir := stagedRepo(h)
	h.open(dir)
	h.click("Generate Draft")
	if h.u.cm.message != draftText {
		t.Fatalf("first fill %q", h.u.cm.message)
	}
	srv.set(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"Second draft"}}]}`))
	})
	h.click("Generate Draft")
	if srv.calls() != 4 {
		t.Fatalf("Generate sent %d requests, want 4", srv.calls())
	}
	if h.u.cm.message != "Second draft\n\nExplain what changed." || h.u.ai.pending != nil {
		t.Fatalf("unedited draft not replaced: %q pending %v", h.u.cm.message, h.u.ai.pending)
	}
}

func TestChromeDraftCardWhenHandwritten(t *testing.T) {
	h, srv := chromeAIHarness(t)
	dir := stagedRepo(h)
	h.open(dir)
	h.typeMessage("My own words")
	h.click("Generate Draft")
	if srv.calls() != 2 {
		t.Fatalf("Generate sent %d requests", srv.calls())
	}
	if h.u.cm.message != "My own words" {
		t.Fatalf("draft overwrote handwritten text: %q", h.u.cm.message)
	}
	h.want("Replace Message", "Discard")
	h.click("Discard")
	if h.u.ai.pending != nil || h.u.cm.message != "My own words" {
		t.Fatalf("discard: pending %v message %q", h.u.ai.pending, h.u.cm.message)
	}
	h.click("Generate Draft")
	h.click("Replace Message")
	if h.u.cm.message != draftText || h.u.ai.pending != nil {
		t.Fatalf("replace: %q pending %v", h.u.cm.message, h.u.ai.pending)
	}
}

func TestChromeDraftCardWhenStagedMoved(t *testing.T) {
	h, _ := chromeAIHarness(t)
	dir := stagedRepo(h)
	h.open(dir)
	h.typeMessage("My own words")
	h.click("Generate Draft")
	gittest.Write(t, filepath.Join(dir, "b.txt"), "b\n")
	h.Git(dir, "add", "b.txt")
	h.u.Refresh()
	h.tt.Frame()
	h.want("Use Anyway", "Regenerate")
	if h.tt.HasText("Replace Message") {
		t.Fatal("stale card kept Replace Message")
	}
	h.click("Use Anyway")
	if h.u.cm.message != draftText || h.u.ai.pending != nil {
		t.Fatalf("use anyway: %q pending %v", h.u.cm.message, h.u.ai.pending)
	}
}
