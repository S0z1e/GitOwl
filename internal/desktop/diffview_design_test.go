package desktop

import (
	"strings"
	"testing"
)

// twoHunkPatch is a small unified diff of two separate changes, like the
// design's ai.go: each hunk has context around one replaced line.
const twoHunkPatch = `diff --git a/f.go b/f.go
--- a/f.go
+++ b/f.go
@@ -209,5 +209,5 @@ composerTools
 context a
 context b
-	components.Text("x")
+	controlLabel("x")
 context c
 context d
@@ -224,3 +224,3 @@ composerTools
 context e
-	components.Text("y")
+	controlLabel("y")
 context f
`

// TestDesignHunksSplitsCardsAndNumbersLines checks the parser makes one
// card per hunk, with the design's human range and section, and numbers
// each row's old and new sides, leaving a side blank where it has no line.
func TestDesignHunksSplitsCardsAndNumbersLines(t *testing.T) {
	hunks := designHunks(twoHunkPatch)
	if len(hunks) != 2 {
		t.Fatalf("hunks %d, want 2", len(hunks))
	}
	h := hunks[0]
	if h.section != "composerTools" {
		t.Fatalf("section %q", h.section)
	}
	// The del row has an old number and no new; the add row the reverse.
	var del, add *diffLine
	for i := range h.lines {
		switch h.lines[i].sign {
		case '-':
			del = &h.lines[i]
		case '+':
			add = &h.lines[i]
		}
	}
	if del == nil || add == nil {
		t.Fatalf("no del/add row in %+v", h.lines)
	}
	if del.oldNum == 0 || del.newNum != 0 {
		t.Fatalf("del numbers old=%d new=%d, want old set, new blank", del.oldNum, del.newNum)
	}
	if add.newNum == 0 || add.oldNum != 0 {
		t.Fatalf("add numbers old=%d new=%d, want new set, old blank", add.oldNum, add.newNum)
	}
	// A context row carries both numbers.
	for _, l := range h.lines {
		if l.sign == ' ' && (l.oldNum == 0 || l.newNum == 0) {
			t.Fatalf("context row missing a number: %+v", l)
		}
	}
}

// TestDesignHunksInlineHighlightsTheChangedToken pairs the del and add
// lines and highlights only the token that differs (components.Text →
// controlLabel), not the shared code around it.
func TestDesignHunksInlineHighlightsTheChangedToken(t *testing.T) {
	h := designHunks(twoHunkPatch)[0]
	var del, add diffLine
	for _, l := range h.lines {
		if l.sign == '-' {
			del = l
		}
		if l.sign == '+' {
			add = l
		}
	}
	if len(del.words) != 1 || len(add.words) != 1 {
		t.Fatalf("want one highlight each, got del %v add %v", del.words, add.words)
	}
	got := del.text[del.words[0].start:del.words[0].end]
	if !strings.Contains(got, "components.Text") {
		t.Fatalf("del highlight %q, want the removed token", got)
	}
	got = add.text[add.words[0].start:add.words[0].end]
	if !strings.Contains(got, "controlLabel") {
		t.Fatalf("add highlight %q, want the added token", got)
	}
	// The shared quoted literal is outside the highlight.
	if strings.Contains(del.text[del.words[0].start:del.words[0].end], "\"x\"") {
		t.Fatalf("highlight covers the unchanged literal: %q", got)
	}
}

// TestDesignHunksNoHighlightWhenEqual leaves identical lines unmarked, so
// an unchanged context row never shows an inline highlight.
func TestDesignHunksNoHighlightWhenEqual(t *testing.T) {
	ra, rb := wordDiff("same line", "same line")
	if ra != nil || rb != nil {
		t.Fatalf("equal lines highlighted: %v %v", ra, rb)
	}
}

// TestDesignHunksEmptyForNoHunks returns no cards for a patch without
// hunks, so a binary or special change shows no hunk entry.
func TestDesignHunksEmptyForNoHunks(t *testing.T) {
	if h := designHunks(""); h != nil {
		t.Fatalf("empty patch made %d hunks", len(h))
	}
}
