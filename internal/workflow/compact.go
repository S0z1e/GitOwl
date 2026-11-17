package workflow

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

// Read limits for a draft request's diff, so a large staged change is
// never held in memory whole.
const (
	// maxContentFiles is the most ordinary files whose diff is read.
	maxContentFiles = 200
	minFileRead     = 32 << 10
	minTotalRead    = 256 << 10
	maxTotalRead    = 16 << 20
)

// readPatches reads the staged diff of each ordinary file in notes, each
// up to a share of a total that is four times the input budget in bytes.
func (r *Repository) readPatches(ctx context.Context, notes []FileNote, input int) (map[int]filePatch, error) {
	var files []int
	for i, n := range notes {
		if n.Treatment != TreatFull {
			continue
		}
		if len(files) == maxContentFiles {
			notes[i].Treatment, notes[i].Reasons = TreatOmitted, []string{ReasonManyFiles}
			continue
		}
		files = append(files, i)
	}
	patches := map[int]filePatch{}
	if len(files) == 0 {
		return patches, nil
	}
	total := min(max(4*input*asciiBytesPerToken, minTotalRead), maxTotalRead)
	each := max(total/len(files), minFileRead)
	for _, i := range files {
		n := &notes[i]
		args := append([]string{"--literal-pathspecs", "diff", "--cached", "-U3"}, diffOptions...)
		args = append(args, "--", n.Path)
		if n.OrigPath != "" {
			args = append(args, n.OrigPath)
		}
		out, truncated, err := r.app.git.RunLimit(ctx, r.root, each, args...)
		if err != nil {
			return nil, gitFailure(err)
		}
		p := parsePatch(string(out))
		if truncated {
			n.Reasons = append(n.Reasons, ReasonReadLimit)
		}
		if p.longLines {
			n.Reasons = append(n.Reasons, ReasonLongLines)
		}
		patches[i] = p
	}
	return patches, nil
}

// lineCost estimates a line with its newline. Summing lines never counts
// less than estimating them joined.
func lineCost(l string) int { return estimateText(l + "\n") }

func linesCost(ls []string) int {
	n := 0
	for _, l := range ls {
		n += lineCost(l)
	}
	return n
}

// share divides budget between items costing costs: each gets its cost, or
// an equal part of what the cheaper ones leave, whichever is smaller.
func share(costs []int, budget int) []int {
	order := make([]int, len(costs))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return costs[order[a]] < costs[order[b]] })
	out := make([]int, len(costs))
	left := max(budget, 0)
	for k, i := range order {
		out[i] = min(costs[i], left/(len(order)-k))
		left -= out[i]
	}
	return out
}

func omittedLine(n int, what string) string {
	return "… " + strconv.Itoa(n) + " " + what + " omitted"
}

// fitDiff renders the patches of notes in at most budget tokens and
// records each file's treatment. Everything fits as read when it can;
// otherwise every file loses context lines, one then all; then the
// remaining budget is shared between files and, within a file, between
// its hunks, each keeping its first lines.
func fitDiff(notes []FileNote, patches map[int]filePatch, budget int) string {
	var files []int
	for i := range notes {
		if _, ok := patches[i]; ok {
			files = append(files, i)
		}
	}
	sort.Ints(files)
	render := func(k int) ([][]string, int) {
		out := make([][]string, len(files))
		total := 0
		for j, i := range files {
			out[j] = patches[i].render(k)
			total += linesCost(out[j])
		}
		return out, total
	}
	join := func(parts [][]string) string {
		var b strings.Builder
		for _, p := range parts {
			for _, l := range p {
				b.WriteString(l)
				b.WriteByte('\n')
			}
		}
		return b.String()
	}
	if parts, total := render(-1); total <= budget {
		return join(parts)
	}
	for _, i := range files {
		notes[i].Treatment = TreatLessContext
		notes[i].Reasons = append([]string{ReasonContext}, notes[i].Reasons...)
	}
	for _, k := range []int{1, 0} {
		if parts, total := render(k); total <= budget {
			return join(parts)
		}
	}
	// Share what is left of the budget between files, then hunks.
	costs := make([]int, len(files))
	for j, i := range files {
		costs[j] = linesCost(patches[i].render(0))
	}
	shares := share(costs, budget)
	parts := make([][]string, len(files))
	for j, i := range files {
		if shares[j] >= costs[j] {
			parts[j] = patches[i].render(0)
			continue
		}
		parts[j] = shorten(patches[i], shares[j])
		n := &notes[i]
		n.Reasons = append([]string{ReasonBudget}, n.Reasons[1:]...)
		n.Treatment = TreatShortened
		if parts[j] == nil {
			n.Treatment = TreatOmitted
		}
	}
	return join(parts)
}

// shorten renders p without context in at most budget tokens, or returns
// nil when not even its header and one change fit.
func shorten(p filePatch, budget int) []string {
	var hunks []hunk
	for _, h := range p.hunks {
		hunks = append(hunks, h.withContext(0)...)
	}
	header := linesCost(p.header)
	marker := lineCost(omittedLine(9999, "changed lines"))
	room := budget - header - marker
	if len(hunks) == 0 || room <= 0 {
		return nil
	}
	costs := make([]int, len(hunks))
	mins := make([]int, len(hunks))
	for i, h := range hunks {
		head := lineCost(h.headerLine())
		costs[i] = head + linesCost(h.lines)
		mins[i] = min(costs[i], head+lineCost(h.lines[0])+marker)
	}
	// When not every hunk can show its start, show hunks spread evenly
	// over the file rather than only the first ones.
	picked := evenly(len(hunks), len(hunks))
	for k := len(hunks); k > 0; k-- {
		picked = evenly(len(hunks), k)
		need := 0
		for _, i := range picked {
			need += mins[i]
		}
		if need <= room {
			break
		}
	}
	pickedCosts := make([]int, len(picked))
	for j, i := range picked {
		pickedCosts[j] = costs[i]
	}
	shares := share(pickedCosts, room)
	out := append([]string{}, p.header...)
	kept := 0
	dropped := len(hunks) - len(picked)
	for j, i := range picked {
		h := hunks[i]
		if shares[j] >= costs[i] {
			out = append(append(out, h.headerLine()), h.lines...)
			kept++
			continue
		}
		head := h.headerLine()
		left := shares[j] - lineCost(head) - marker
		n := 0
		for n < len(h.lines) && lineCost(h.lines[n]) <= left {
			left -= lineCost(h.lines[n])
			n++
		}
		if n == 0 {
			dropped++
			continue
		}
		out = append(append(append(out, head), h.lines[:n]...), omittedLine(len(h.lines)-n, "lines of this hunk"))
		kept++
	}
	if kept == 0 {
		return nil
	}
	if dropped > 0 {
		out = append(out, omittedLine(dropped, "more hunks"))
	}
	return out
}

// evenly returns k of the indexes 0…n-1, spread from the first to the
// last.
func evenly(n, k int) []int {
	if k <= 0 {
		return nil
	}
	out := make([]int, k)
	for j := range out {
		out[j] = j * n / k
	}
	return out
}
