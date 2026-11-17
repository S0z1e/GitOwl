package workflow

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var hunkSection = regexp.MustCompile(`(?m)^(@@ [^@]* @@).*$`)

// hunksOnly keeps a patch's hunks, without the function names Git adds
// after them, which GitOwl does not recompute.
func hunksOnly(patch string) string {
	if i := strings.Index(patch, "\n@@ "); i >= 0 {
		patch = patch[i+1:]
	}
	return hunkSection.ReplaceAllString(patch, "$1")
}

func TestReducedContextMatchesGit(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	var before, after []string
	for i := range 60 {
		before = append(before, "line "+itoa(i))
	}
	after = append(after, before...)
	after[0] = "changed first"
	after[5] = "changed 5"
	after[8] = "changed 8" // 2 unchanged lines from 5: one hunk at -U1
	after[20] = "changed 20"
	after = append(after[:30], after[31:]...)                                  // a deletion
	after = append(after[:40], append([]string{"inserted"}, after[40:]...)...) // an insertion
	after[len(after)-1] = "changed last"
	write(t, filepath.Join(dir, "f.txt"), strings.Join(before, "\n")+"\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "f.txt"), strings.Join(after, "\n")) // no newline at the end
	e.git(dir, "add", "f.txt")

	p := parsePatch(e.git(dir, "diff", "--cached", "-U3", "--no-color", "f.txt"))
	if len(p.header) == 0 || !strings.HasPrefix(p.header[0], "diff --git") {
		t.Fatalf("header %q", p.header)
	}
	for _, k := range []int{3, 1, 0} {
		want := hunksOnly(e.git(dir, "diff", "--cached", "-U"+itoa(k), "--no-color", "f.txt"))
		got := hunksOnly(strings.Join(p.render(k), "\n") + "\n")
		if got != want {
			t.Fatalf("-U%d differs.\ngot:\n%s\nwant:\n%s", k, got, want)
		}
	}
}

func TestParsePatchCutsLongLinesAndPartialEnd(t *testing.T) {
	long := strings.Repeat("x", 1000)
	p := parsePatch("diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1 +1 @@\n-" + long + "\n+short\n+partial line without its newline")
	if !p.longLines || len(p.hunks) != 1 || len(p.hunks[0].lines) != 2 {
		t.Fatalf("%+v", p)
	}
	if l := p.hunks[0].lines[0]; len([]rune(l)) != MaxLineChars+1 || !strings.HasSuffix(l, "…") {
		t.Fatalf("long line %d", len([]rune(l)))
	}
}
