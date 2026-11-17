package workflow

import (
	"strings"
	"testing"
)

func TestShare(t *testing.T) {
	got := share([]int{10, 1000, 50, 1000}, 600)
	// The small ones get all they need; the large ones split the rest.
	if got[0] != 10 || got[2] != 50 || got[1] != 270 || got[3] != 270 {
		t.Fatalf("%v", got)
	}
	if got := share([]int{5, 5}, 100); got[0] != 5 || got[1] != 5 {
		t.Fatalf("%v", got)
	}
	if got := share([]int{5}, -3); got[0] != 0 {
		t.Fatalf("%v", got)
	}
}

// bigPatch is a patch of one file with hunks separated by unchanged
// lines, each changing size lines.
func bigPatch(name string, hunks, size int) filePatch {
	var b strings.Builder
	b.WriteString("diff --git a/" + name + " b/" + name + "\n--- a/" + name + "\n+++ b/" + name + "\n")
	line := 1
	for h := range hunks {
		b.WriteString("@@ -" + itoa(line) + "," + itoa(size+6) + " +" + itoa(line) + "," + itoa(size+6) + " @@\n")
		for range 3 {
			b.WriteString(" context\n")
		}
		for i := range size {
			b.WriteString("-old " + name + " " + itoa(h) + " " + itoa(i) + "\n")
			b.WriteString("+new " + name + " " + itoa(h) + " " + itoa(i) + "\n")
		}
		for range 3 {
			b.WriteString(" context\n")
		}
		line += size + 100
	}
	return parsePatch(b.String())
}

func TestFitDiffSharesBetweenFilesAndHunks(t *testing.T) {
	notes := []FileNote{{Path: "a.go"}, {Path: "b.go"}, {Path: "small.go"}, {Path: "lock", Treatment: TreatSummary}}
	patches := map[int]filePatch{0: bigPatch("a.go", 5, 200), 1: bigPatch("b.go", 5, 200), 2: bigPatch("small.go", 1, 2)}

	// Plenty of room: as read.
	diff := fitDiff(append([]FileNote{}, notes...), patches, 1<<20)
	if !strings.Contains(diff, " context\n") {
		t.Fatal("context removed with room to spare")
	}
	// Room for everything without context.
	n2 := append([]FileNote{}, notes...)
	for i := range n2 {
		n2[i].Reasons = nil
	}
	costU0 := 0
	for _, p := range patches {
		costU0 += linesCost(p.render(0))
	}
	diff = fitDiff(n2, patches, costU0)
	if strings.Contains(diff, " context\n") || n2[0].Treatment != TreatLessContext || n2[0].Reasons[0] != ReasonContext {
		t.Fatalf("U0: %+v", n2[0])
	}

	// Tight: the small file is whole, both large files keep part of every
	// hunk, and the result fits.
	n3 := append([]FileNote{}, notes...)
	budget := 1500
	diff = fitDiff(n3, patches, budget)
	if est := estimateText(diff); est > budget {
		t.Fatalf("estimate %d over %d", est, budget)
	}
	if !strings.Contains(diff, "+new small.go 0 1\n") || n3[2].Treatment != TreatLessContext {
		t.Fatalf("small file not whole: %+v", n3[2])
	}
	for _, name := range []string{"a.go", "b.go"} {
		for h := range 5 {
			if !strings.Contains(diff, "+new "+name+" "+itoa(h)+" 0\n") {
				t.Fatalf("%s hunk %d left out:\n%s", name, h, diff)
			}
		}
	}
	if n3[0].Treatment != TreatShortened || n3[0].Reasons[0] != ReasonBudget || !strings.Contains(diff, "lines of this hunk omitted") {
		t.Fatalf("a.go %+v", n3[0])
	}
	if n3[3].Treatment != TreatSummary {
		t.Fatal("summary file changed")
	}

	// No room at all: content omitted, never over budget.
	n4 := append([]FileNote{}, notes...)
	if diff := fitDiff(n4, patches, 20); estimateText(diff) > 20 || n4[0].Treatment != TreatOmitted {
		t.Fatalf("%q %+v", diff, n4[0])
	}
}
