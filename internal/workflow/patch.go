package workflow

import (
	"regexp"
	"strconv"
	"strings"
)

// MaxLineChars is the longest diff line a draft request shows; longer
// lines, as in minified files, are cut.
const MaxLineChars = 240

// filePatch is one file's unified diff, split for shortening.
type filePatch struct {
	// header is the lines before the first hunk: diff --git, index, ---, +++.
	header []string
	hunks  []hunk
	// longLines is set when a line was cut at MaxLineChars.
	longLines bool
}

// hunk is one @@ section.
type hunk struct {
	oldStart, newStart int
	// section is the text after the closing @@, such as a function name.
	section string
	// lines start with ' ', '+', '-' or '\' (no newline at end of file).
	lines []string
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@(.*)$`)

// parsePatch splits a one-file patch. A last line without its newline, as
// when the read stopped at a limit, is dropped.
func parsePatch(patch string) filePatch {
	var p filePatch
	if i := strings.LastIndexByte(patch, '\n'); i >= 0 {
		patch = patch[:i]
	} else {
		patch = ""
	}
	for _, line := range strings.Split(patch, "\n") {
		if m := hunkHeader.FindStringSubmatch(line); m != nil {
			o, _ := strconv.Atoi(m[1])
			n, _ := strconv.Atoi(m[2])
			p.hunks = append(p.hunks, hunk{oldStart: o, newStart: n, section: m[3]})
			continue
		}
		if len(p.hunks) == 0 {
			if line != "" || len(p.header) > 0 {
				p.header = append(p.header, line)
			}
			continue
		}
		if r := []rune(line); len(r) > MaxLineChars {
			line, p.longLines = string(r[:MaxLineChars])+"…", true
		}
		h := &p.hunks[len(p.hunks)-1]
		h.lines = append(h.lines, line)
	}
	return p
}

func isChange(line string) bool {
	return line != "" && (line[0] == '+' || line[0] == '-')
}

// withContext returns h's changes with at most k unchanged lines around
// them, split into hunks as git diff -U<k> would.
func (h hunk) withContext(k int) []hunk {
	n := len(h.lines)
	// Positions among real lines; '\' lines belong to the line before.
	pos := make([]int, n)
	real := 0
	for i, l := range h.lines {
		if strings.HasPrefix(l, `\`) {
			pos[i] = real - 1
			continue
		}
		pos[i] = real
		real++
	}
	var changes []int
	for i, l := range h.lines {
		if isChange(l) {
			changes = append(changes, pos[i])
		}
	}
	keep := make([]bool, n)
	for i := range h.lines {
		for _, c := range changes {
			if d := pos[i] - c; d >= -k && d <= k {
				keep[i] = true
				break
			}
		}
	}
	var out []hunk
	oldNo, newNo := h.oldStart, h.newStart
	var cur *hunk
	for i, l := range h.lines {
		if !keep[i] {
			cur = nil
		} else {
			if cur == nil {
				out = append(out, hunk{oldStart: oldNo, newStart: newNo, section: h.section})
				cur = &out[len(out)-1]
			}
			cur.lines = append(cur.lines, l)
		}
		switch {
		case strings.HasPrefix(l, `\`):
		case strings.HasPrefix(l, "-"):
			oldNo++
		case strings.HasPrefix(l, "+"):
			newNo++
		default:
			oldNo++
			newNo++
		}
	}
	return out
}

// headerLine renders h's @@ line from its lines, as Git counts them.
func (h hunk) headerLine() string {
	oldN, newN := 0, 0
	for _, l := range h.lines {
		switch {
		case strings.HasPrefix(l, `\`):
		case strings.HasPrefix(l, "-"):
			oldN++
		case strings.HasPrefix(l, "+"):
			newN++
		default:
			oldN++
			newN++
		}
	}
	return "@@ -" + hunkRange(h.oldStart, oldN) + " +" + hunkRange(h.newStart, newN) + " @@" + h.section
}

func hunkRange(start, n int) string {
	switch n {
	case 0:
		// An empty side names the line before it.
		return strconv.Itoa(start-1) + ",0"
	case 1:
		return strconv.Itoa(start)
	}
	return strconv.Itoa(start) + "," + strconv.Itoa(n)
}

// render returns the patch's lines with each hunk reduced to k lines of
// context; k < 0 keeps the hunks as read.
func (p filePatch) render(k int) []string {
	lines := append([]string{}, p.header...)
	for _, h := range p.hunks {
		parts := []hunk{h}
		if k >= 0 {
			parts = h.withContext(k)
		}
		for _, part := range parts {
			lines = append(lines, part.headerLine())
			lines = append(lines, part.lines...)
		}
	}
	return lines
}
