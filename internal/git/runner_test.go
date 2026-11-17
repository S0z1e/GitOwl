package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testRunner(t *testing.T) (*Runner, string) {
	t.Helper()
	r, err := Find()
	if err != nil {
		t.Skip("git not available")
	}
	home := t.TempDir()
	r.Env = []string{"HOME=" + home, "XDG_CONFIG_HOME=" + home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + filepath.Join(home, "none")}
	dir := t.TempDir()
	if _, err := r.Run(context.Background(), dir, nil, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	return r, dir
}

func TestRunLimit(t *testing.T) {
	r, dir := testRunner(t)
	ctx := context.Background()
	big := strings.Repeat("0123456789abcdef\n", 1<<16) // 1 MB
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, dir, nil, "add", "big.txt"); err != nil {
		t.Fatal(err)
	}
	out, truncated, err := r.RunLimit(ctx, dir, 4096, "cat-file", "blob", ":0:big.txt")
	if err != nil || !truncated || len(out) != 4096 || string(out) != big[:4096] {
		t.Fatalf("limited: %d %v %v", len(out), truncated, err)
	}
	out, truncated, err = r.RunLimit(ctx, dir, 2<<20, "cat-file", "blob", ":0:big.txt")
	if err != nil || truncated || string(out) != big {
		t.Fatalf("whole: %d %v %v", len(out), truncated, err)
	}
	// Exactly the limit is not truncated.
	out, truncated, err = r.RunLimit(ctx, dir, len(big), "cat-file", "blob", ":0:big.txt")
	if err != nil || truncated || len(out) != len(big) {
		t.Fatalf("exact: %d %v %v", len(out), truncated, err)
	}
	// A failing command is an *Error with Git's message.
	_, _, err = r.RunLimit(ctx, dir, 100, "cat-file", "blob", ":0:missing.txt")
	var g *Error
	if !errors.As(err, &g) || g.ExitCode == 0 || g.Message() == "" {
		t.Fatalf("failure: %v", err)
	}
	// A cancelled context is reported as such.
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := r.RunLimit(cctx, dir, 100, "cat-file", "blob", ":0:big.txt"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}
