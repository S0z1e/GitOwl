package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"gitowl/internal/ai"
)

func TestBudget(t *testing.T) {
	for n, want := range map[int]Budget{
		8192:   {Context: 8192, Output: 1024, Margin: 1024, Input: 6144},
		4096:   {Context: 4096, Output: 1024, Margin: 1024, Input: 2048},
		32768:  {Context: 32768, Output: 1024, Margin: 4096, Input: 27648},
		131072: {Context: 131072, Output: 1024, Margin: 16384, Input: 113664},
		0:      {Context: 8192, Output: 1024, Margin: 1024, Input: 6144},
		100:    {Context: 8192, Output: 1024, Margin: 1024, Input: 6144},
	} {
		if got := NewBudget(n); got != want {
			t.Errorf("%d: %+v", n, got)
		}
	}
}

func TestEstimateTokens(t *testing.T) {
	// 3 to prime the reply; per message 4, the role and the content.
	msgs := []ai.Message{{Role: "user", Content: "abcdefg"}} // "user": 2, content: 3
	if got := EstimateTokens(msgs); got != 3+4+2+3 {
		t.Fatalf("ascii %d", got)
	}
	msgs = []ai.Message{{Role: "user", Content: "修复问题"}} // 4 characters, 2 each
	if got := EstimateTokens(msgs); got != 3+4+2+8 {
		t.Fatalf("cjk %d", got)
	}
	if EstimateTokens(nil) != 3 {
		t.Fatal("empty")
	}
	// The same input gives the same estimate.
	long := []ai.Message{{Role: "system", Content: strings.Repeat("func x() {}\n", 1000)}, {Role: "user", Content: strings.Repeat("中文 mixed ", 500)}}
	if EstimateTokens(long) != EstimateTokens(long) {
		t.Fatal("not reproducible")
	}
}

func TestDraftFitsTheBudget(t *testing.T) {
	e, f, _, cfg := aiEnv(t)
	dir := e.repo()
	history(t, e, dir, "Add parser", "Fix empty lines", "Update docs")
	write(t, filepath.Join(dir, "a.txt"), strings.Repeat("a staged line of text\n", 200)) // ~4.4 KB
	e.git(dir, "add", "a.txt")
	r, _ := e.open(dir)
	ctx := context.Background()

	// The default 8K budget: the request fits and says how much it needs.
	req, d, err := r.Draft(ctx, DraftOptions{}, cfg)
	if err != nil || d.Text == "" {
		t.Fatal(err)
	}
	if req.Budget.Input != 6144 || req.Estimate <= 0 || req.Estimate > req.Budget.Input || req.Estimate != requestEstimate(req, "") {
		t.Fatalf("budget %+v estimate %d", req.Budget, req.Estimate)
	}
	if len(req.Recent) != 3 {
		t.Fatalf("examples %v", req.Recent)
	}

	if req.Shortened() || strings.Contains(f.last().Messages[0].Content, "not shown in full") {
		t.Fatal("a request that fits was marked shortened")
	}

	// A larger diff is shortened to fit 4K and still sent once.
	write(t, filepath.Join(dir, "b.txt"), strings.Repeat("another staged line of text\n", 400))
	e.git(dir, "add", "b.txt")
	req, _, err = r.Draft(ctx, DraftOptions{ContextTokens: 4096}, cfg)
	if err != nil || req.Estimate > 2048 || !req.Shortened() || f.count() != 4 {
		t.Fatalf("4K: %v %d shortened %v calls %d", err, req.Estimate, req.Shortened(), f.count())
	}
	// A larger context takes it whole.
	if req, _, err := r.Draft(ctx, DraftOptions{ContextTokens: 32768}, cfg); err != nil || req.Shortened() || f.count() != 6 {
		t.Fatalf("32K: %v shortened %v", err, req.Shortened())
	}
}

func TestStyleExamplesGiveWayToTheFileList(t *testing.T) {
	e, f, _, cfg := aiEnv(t)
	dir := e.repo()
	subject := strings.Repeat("Refine the parser ", 11) // ~200 characters
	history(t, e, dir, subject+"1", subject+"2", subject+"3")
	r, _ := e.open(dir)
	ctx := context.Background()
	// Add staged files until the examples no longer all fit beside the list.
	sawFewer := false
	for n := 0; n < 400; n += 10 {
		for i := n; i < n+10; i++ {
			write(t, filepath.Join(dir, "pkg", "some_longer_file_name_"+itoa(i)+".go"), "package pkg\n")
		}
		e.git(dir, "add", "-A")
		req, err := r.PrepareDraft(ctx, DraftOptions{ContextTokens: 4096})
		if err != nil {
			// At last the list alone is too long: nothing is sent.
			wantCode(t, err, CodeDraftManifestTooLarge)
			if !sawFewer {
				t.Fatal("examples did not give way before the list")
			}
			if _, _, err := r.Draft(ctx, DraftOptions{ContextTokens: 4096}, cfg); CodeOf(err) != CodeDraftManifestTooLarge || f.count() != 0 {
				t.Fatalf("%v, %d calls", err, f.count())
			}
			return
		}
		if req.Estimate > req.Budget.Input {
			t.Fatalf("estimate %d over %d", req.Estimate, req.Budget.Input)
		}
		if len(req.Recent) < 3 {
			sawFewer = true
		}
		if len(req.Recent) == 0 && req.Style != StyleDefault {
			t.Fatalf("style %d without examples", req.Style)
		}
	}
	t.Fatal("the file list never filled the budget")
}

func TestLongPromptIsNotCut(t *testing.T) {
	e, f, _, cfg := aiEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.git(dir, "add", "a.txt")
	r, _ := e.open(dir)
	prompt := strings.Repeat("Always explain the reason. ", 800) // ~21 KB
	_, _, err := r.Draft(context.Background(), DraftOptions{Prompt: prompt}, cfg)
	wantCode(t, err, CodeDraftPromptTooLong)
	if f.count() != 0 {
		t.Fatal("sent")
	}
	// With room for it, the whole prompt is sent.
	req, _, err := r.Draft(context.Background(), DraftOptions{Prompt: prompt, ContextTokens: 32768}, cfg)
	if err != nil || !strings.Contains(f.last().Messages[0].Content, strings.TrimSpace(prompt)) || req.Style != StyleCustom {
		t.Fatalf("%v", err)
	}
}
