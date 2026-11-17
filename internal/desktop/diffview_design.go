package desktop

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"gitowl/internal/desktop/designicons"
	"gitowl/internal/workflow"
)

// The unified diff drawn from the HTML design
// (reference/measurements/repository-diff.md): each hunk is a card with a
// surface-2 header (.hh) and its lines in a four-column grid (.dl), 44|44|24
// for the old number, the new number and the +/−/space sign, then the code.
// MyGo's editor.DiffView is one continuous scrolled editor with a single
// gutter; it cannot draw the design's discrete, rounded hunk cards or the
// per-hunk header bar, so GitOwl draws the view itself here and reuses
// editor.ParsePatch only for the hunks' structured lines and numbers.

// The four grid columns of a .dl line, in DIPs, from the measurements.
const (
	diffNumWidth  = 44 // an old or a new line-number column
	diffSignWidth = 24 // the +/−/space column
	diffLineH     = 28 // a .ln row's height, the design's 28
	diffCardRad   = 10 // a hunk card's corner radius
)

// diffRange is a half-open [start,end) run of bytes highlighted in a line.
type diffRange struct{ start, end int }

// diffLine is one row of a hunk card.
type diffLine struct {
	oldNum, newNum int    // 0 when the column is blank (its side has no line)
	sign           byte   // ' ', '+' or '-'
	text           string // the line without its leading sign
	words          []diffRange
}

// diffHunk is one card: a human-readable range, the @@ section name and the
// rows to draw.
type diffHunk struct {
	fromLine, toLine int
	section          string
	lines            []diffLine
}

// designHunks parses a one-file patch into the cards the design draws. It
// returns nil when the patch has no hunks (a binary or special change),
// which keeps the hunk entries and their cards off screen.
func designHunks(patch string) []diffHunk {
	files, err := editor.ParsePatch(patch)
	if err != nil || len(files) == 0 {
		return nil
	}
	var out []diffHunk
	for _, h := range files[0].Hunks {
		dh := diffHunk{fromLine: h.NewStart, section: h.Section}
		oldN, newN := h.OldStart, h.NewStart
		last := h.NewStart
		for _, l := range h.Lines {
			if l == "" || l[0] == '\\' {
				continue
			}
			line := diffLine{sign: l[0], text: strings.TrimSuffix(l[1:], "\r")}
			switch l[0] {
			case '+':
				line.newNum = newN
				last = newN
				newN++
			case '-':
				line.oldNum = oldN
				oldN++
			default:
				line.oldNum, line.newNum = oldN, newN
				last = newN
				oldN++
				newN++
			}
			dh.lines = append(dh.lines, line)
		}
		dh.toLine = last
		markWords(dh.lines)
		out = append(out, dh)
	}
	return out
}

// splitRow is one row of the Split view: the old side's line on the left
// and the new side's on the right. Either side is nil where that side has
// no line on this row (an unbalanced add or del), so the two columns stay
// aligned, blank against blank.
type splitRow struct {
	left, right *diffLine
}

// splitRows pairs a hunk's unified lines into the Split view's two columns.
// A context line fills both sides. A run of deletions and the additions
// that follow it pair one-to-one (del i with add i), as the inline
// highlight pairs them; the longer side's extra lines sit opposite a blank,
// so a pure addition or a pure deletion keeps the other side empty and an
// asymmetric change lines its pairs up and pads the rest.
func splitRows(lines []diffLine) []splitRow {
	var out []splitRow
	i := 0
	for i < len(lines) {
		switch lines[i].sign {
		case ' ':
			l := lines[i]
			out = append(out, splitRow{left: &l, right: &l})
			i++
		case '-', '+':
			dels := i
			for i < len(lines) && lines[i].sign == '-' {
				i++
			}
			adds := i
			for i < len(lines) && lines[i].sign == '+' {
				i++
			}
			out = append(out, pairChange(lines[dels:adds], lines[adds:i])...)
		default:
			i++
		}
	}
	return out
}

// pairChange lines a run of deletions up against the additions that replace
// them, one per row, padding the shorter side with a blank cell.
func pairChange(dels, adds []diffLine) []splitRow {
	n := max(len(dels), len(adds))
	out := make([]splitRow, n)
	for k := 0; k < n; k++ {
		if k < len(dels) {
			d := dels[k]
			out[k].left = &d
		}
		if k < len(adds) {
			a := adds[k]
			out[k].right = &a
		}
	}
	return out
}

// numText is a line number for a .dl column, blank for the zero that marks
// a side without a line on this row.
func numText(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// markWords fills each changed line's words with the byte ranges a word-
// level diff finds, pairing the deletions and additions of each change run
// one-to-one (del i with add i), as the design pairs one removed line with
// one added line for its inline highlight. Runs of unequal length, and the
// lines past the shorter side, keep no inline highlight.
func markWords(lines []diffLine) {
	i := 0
	for i < len(lines) {
		if lines[i].sign != '-' {
			i++
			continue
		}
		dels := i
		for i < len(lines) && lines[i].sign == '-' {
			i++
		}
		adds := i
		for i < len(lines) && lines[i].sign == '+' {
			i++
		}
		nDel, nAdd := adds-dels, i-adds
		n := min(nDel, nAdd)
		for k := 0; k < n; k++ {
			del, add := &lines[dels+k], &lines[adds+k]
			del.words, add.words = wordDiff(del.text, add.text)
		}
	}
}

// wordDiff returns the byte ranges of a and b that differ, by their common
// prefix and suffix over whole words, so a changed token is one highlight
// rather than a run of single characters. Equal lines highlight nothing.
func wordDiff(a, b string) (ra, rb []diffRange) {
	aw, bw := splitWords(a), splitWords(b)
	p := 0
	for p < len(aw) && p < len(bw) && aw[p].text == bw[p].text {
		p++
	}
	s := 0
	for s < len(aw)-p && s < len(bw)-p &&
		aw[len(aw)-1-s].text == bw[len(bw)-1-s].text {
		s++
	}
	if span := wordSpan(aw, p, s); span.end > span.start {
		ra = []diffRange{span}
	}
	if span := wordSpan(bw, p, s); span.end > span.start {
		rb = []diffRange{span}
	}
	return ra, rb
}

// word is a token of a line with the byte offset it starts at.
type word struct {
	text string
	at   int
}

// splitWords breaks a line into words and the runs of non-word bytes
// between them, each a token, so the diff aligns on identifiers and
// punctuation alike.
func splitWords(s string) []word {
	var out []word
	isWord := func(b byte) bool {
		return b == '_' || b >= '0' && b <= '9' ||
			b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= 0x80
	}
	i := 0
	for i < len(s) {
		j, w := i+1, isWord(s[i])
		for j < len(s) && isWord(s[j]) == w {
			j++
		}
		out = append(out, word{text: s[i:j], at: i})
		i = j
	}
	return out
}

// wordSpan is the byte range from the p-th word to the s-th from the end,
// the middle a diff leaves after stripping the common ends.
func wordSpan(ws []word, p, s int) diffRange {
	if p > len(ws)-s {
		return diffRange{}
	}
	start := 0
	if p < len(ws) {
		start = ws[p].at
	} else if len(ws) > 0 {
		last := ws[len(ws)-1]
		start = last.at + len(last.text)
	}
	end := start
	if to := len(ws) - s; to > p && to <= len(ws) {
		w := ws[to-1]
		end = w.at + len(w.text)
	}
	return diffRange{start, end}
}

// diffChanges returns the pane's hunk cards for the current snapshot, and
// caches them on the pane so one parse serves the header count, the
// navigation and the body of a frame.
func (u *UI) diffChanges(p *diffPane) []diffHunk {
	if p.state == nil {
		return nil
	}
	if p.cards == nil || p.cardsFor != p.info.Patch {
		p.cards = designHunks(p.info.Patch)
		p.cardsFor = p.info.Patch
	}
	return p.cards
}

// splitPath returns a file's directory prefix (with a trailing slash) and
// its name, as the design's muted "path/" before the bold file name.
func splitPath(p string) (string, string) {
	i := strings.LastIndexByte(p, '/')
	if i < 0 {
		return "", p
	}
	return p[:i+1], p[i+1:]
}

// diffSplitActive reports the view the user chose, clamped to Unified in a
// narrow window (the middle and detail columns folded), where there is no
// room for two code columns. The user's choice is kept and returns when the
// window widens again.
func (u *UI) diffSplitActive() bool { return u.diffSplit && !u.lay.narrow }

// diffBodyDesign draws the scrolling list of hunk cards for a text diff,
// or the same explanations as diffBody for a change with no text. It serves
// both the Changes diff and History's read-only diff; the hunk actions show
// only where the pane allows them (diffCardAction), so History stays
// read-only. The body scrolls vertically and the cards fill the width, so
// each card's header and its Stage hunk button stay on screen; when Wrap
// lines is off, the card's code lines scroll sideways within the card.
func (u *UI) diffBodyDesign(c *ui.Context, p *diffPane) {
	t := c.Theme()
	cards := u.diffChanges(p)
	if p.state == nil || len(cards) == 0 {
		u.diffBody(c, p)
		return
	}
	// Selectable makes the code of every card one selection, so a drag can
	// copy across lines; the line numbers and signs are left out of it.
	ui.Scroll(c).Grow(1).FillWidth().Selectable().Children(func() {
		ui.Column(c).FillWidth().Padding(t.Space(2), t.Space(2.5)).Gap(t.Space(2)).Children(func() {
			for i, h := range cards {
				u.diffCard(c, p, i, h)
			}
		})
	})
}

// diffCard draws one hunk as the design's rounded card: a surface-2 header
// (.hh) naming the range and section with its Stage/Unstage hunk button,
// then the hunk's lines, unified or split by the current view. Staging is
// bound to the hunk's index i, the same in either view, so a Stage hunk
// click always stages the source hunk it sits on. The header fills the
// card's width and stays put; when Wrap lines is off the lines scroll
// sideways inside the card, below the header.
func (u *UI) diffCard(c *ui.Context, p *diffPane, i int, h diffHunk) {
	t := c.Theme()
	card := ui.Column(c).Key("diffCard"+strconv.Itoa(i)).FillWidth().Shrink(0).
		Border(1, t.Border).Radius(diffCardRad).Clip()
	if i == p.scrollHunk {
		card.ScrollIntoView()
		p.scrollHunk = -1
	}
	card.Children(func() {
		ui.Row(c).FillWidth().Shrink(0).Padding(t.Space(.75), t.Space(1), t.Space(.75), t.Space(1.5)).
			Gap(t.Space(1)).AlignItems(ui.Center).Background(kitDesignTokens(t.Dark).hd).Children(func() {
			caption(c, u.hunkTitle(h)).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
			ui.Spacer(c)
			u.diffCardAction(c, p, i)
		})
		lines := func() {
			if u.diffSplitActive() {
				for _, r := range splitRows(h.lines) {
					u.diffSplitRow(c, r)
				}
				return
			}
			for _, l := range h.lines {
				u.diffRow(c, l)
			}
		}
		if u.diffWrap {
			ui.Column(c).FillWidth().Children(lines)
			return
		}
		// Wrap off: the lines scroll sideways within the card, below the
		// pinned header; the column grows to its widest line. The column
		// paints the tints, each to the visible edge or its widest line,
		// whichever is further, so every changed line is tinted across the
		// card and not only to the end of its own code.
		ui.ScrollHorizontal(c).FillWidth().Shrink(0).Children(func() {
			col := ui.Column(c).Shrink(0)
			if !u.diffSplitActive() {
				col.Draw(func(p *ui.Painter, r ui.Rect) {
					right := max(r.X+r.W, p.Visible().X+p.Visible().W)
					tk := kitDesignTokens(t.Dark)
					for i, l := range h.lines {
						diffTint(p, r.X, r.Y+float32(i)*diffLineH, diffLineH, right, l.sign, tk)
					}
				})
			}
			col.Children(lines)
		})
	})
}

// hunkTitle is the design's human-readable range and section ("Lines a–b ·
// section"), or the range alone when the hunk has no section.
func (u *UI) hunkTitle(h diffHunk) string {
	if h.section != "" {
		return u.t("diffHunkRangeSection", "from", h.fromLine, "to", h.toLine, "section", h.section)
	}
	return u.t("diffHunkRange", "from", h.fromLine, "to", h.toLine)
}

// diffCardAction draws a card header's Stage hunk / Unstage hunk button. It
// stages or unstages hunk i through applyHunk, which reads the file again
// and refuses a stale hunk, so nothing is lost when the file has changed.
func (u *UI) diffCardAction(c *ui.Context, p *diffPane, i int) {
	if p.state == nil || !p.state.HunkActions {
		return
	}
	label, icon := u.t("stageHunk"), designicons.Plus()
	if p.key.area == workflow.Staged {
		label, icon = u.t("unstageHunk"), designicons.Minus()
	}
	hint := u.t("hunkActionHint", "index", i+1, "count", len(p.cards))
	tk := kitDesignTokens(c.Theme().Dark)
	// Only a click is refused while a Git change runs. The button stays lit,
	// so each stage does not flash the card header.
	busy := u.indexBusy()
	b := kitPillButtonIcon(c, icon, label, tk, false).Shrink(0)
	b.Label(label + " · " + hint).Description(hint)
	if b.Clicked() && !busy {
		p.hunk = i
		u.applyHunk(i)
	}
}

// diffNum draws a line-number column (44, right aligned, muted), blank for
// a side without a line on this row.
func (u *UI) diffNum(c *ui.Context, n int) {
	t := c.Theme()
	codeText(c, numText(n)).TextColor(t.TextMuted).SingleLine().
		Width(diffNumWidth).Shrink(0).Padding(0, t.Space(1), 0, 0).TextAlign(ui.End).Unselectable()
}

// diffSign draws the +/−/space column (24), colored for an add or a del.
func (u *UI) diffSign(c *ui.Context, sign byte) {
	t := c.Theme()
	tk := kitDesignTokens(t.Dark)
	color := t.TextMuted
	switch sign {
	case '+':
		color = tk.add
	case '-':
		color = tk.del
	}
	codeText(c, signText(sign)).TextColor(color).SingleLine().Width(diffSignWidth).Shrink(0).Center().Unselectable()
}

// rowBackground tints a row by its sign, from the design-system add/del.
func rowBackground(row ui.Element, tk kitTokens, sign byte) {
	switch sign {
	case '+':
		row.Background(tk.addbg)
	case '-':
		row.Background(tk.delbg)
	}
}

// diffTint paints a changed line's tint over its sign and code, as the
// design's .ln b block: from the sign column to right, less a 6 DIP margin,
// with 8 DIP corners. The line numbers stay untinted.
func diffTint(p *ui.Painter, x0, y, h, right float32, sign byte, tk kitTokens) {
	var color ui.Color
	switch sign {
	case '+':
		color = tk.addbg
	case '-':
		color = tk.delbg
	default:
		return
	}
	x := x0 + 2*diffNumWidth
	if right-6 <= x {
		return
	}
	p.Fill(ui.Rect{X: x, Y: y, W: right - 6 - x, H: h}, color, 8)
}

// diffRow draws one unified .dl line: the old and new number columns, the
// sign column and the code, with the line's tint behind the sign and code.
func (u *UI) diffRow(c *ui.Context, l diffLine) {
	tk := kitDesignTokens(c.Theme().Dark)
	row := ui.Row(c).Shrink(0).MinHeight(diffLineH).AlignItems(ui.Stretch)
	if u.diffWrap {
		// A wrapped row fills the card, so its tint runs to the card's edge.
		// Without wrap the card paints the tints (diffCard), since a row is
		// only as wide as its own code.
		row.FillWidth()
		row.Draw(func(p *ui.Painter, r ui.Rect) {
			diffTint(p, r.X, r.Y, r.H, r.X+r.W, l.sign, tk)
		})
	}
	row.Children(func() {
		u.diffNum(c, l.oldNum)
		u.diffNum(c, l.newNum)
		u.diffSign(c, l.sign)
		u.diffCode(c, l)
	})
}

// diffSplitRow draws one Split row: the old side's line number, sign and
// code on the left, the new side's on the right, each tinted by its own
// kind. A blank cell (nil) holds a side with no line on this row, keeping
// the two columns aligned.
func (u *UI) diffSplitRow(c *ui.Context, r splitRow) {
	t := c.Theme()
	row := ui.Row(c).Shrink(0).MinHeight(diffLineH).AlignItems(ui.Stretch)
	if u.diffWrap {
		row.FillWidth()
	}
	row.Children(func() {
		u.diffSplitSide(c, r.left, false)
		ui.Box(c).Width(1).Shrink(0).Background(t.Border)
		u.diffSplitSide(c, r.right, true)
	})
}

// diffSplitSide draws one side of a Split row: a single line number column,
// the sign and the code, with the side's background tint. A nil line is an
// empty cell for a side without a line here. newSide picks which line
// number the cell shows.
func (u *UI) diffSplitSide(c *ui.Context, l *diffLine, newSide bool) {
	t := c.Theme()
	side := ui.Row(c).MinHeight(diffLineH).AlignItems(ui.Stretch)
	if u.diffWrap {
		side.Grow(1).MinWidth(0)
	}
	if l != nil {
		rowBackground(side, kitDesignTokens(t.Dark), l.sign)
	}
	side.Children(func() {
		if l == nil {
			u.diffNum(c, 0)
			u.diffSign(c, ' ')
			if u.diffWrap {
				ui.Box(c).Grow(1).MinWidth(0)
			}
			return
		}
		n := l.oldNum
		if newSide {
			n = l.newNum
		}
		u.diffNum(c, n)
		u.diffSign(c, l.sign)
		u.diffCode(c, *l)
	})
}

// diffCode draws the code column, wrapping long lines when Wrap lines is on
// or keeping them on one line (the body scrolls sideways) when it is off,
// and highlighting the changed token's background without bolding it, as
// the design's .ha/.hd.
func (u *UI) diffCode(c *ui.Context, l diffLine) {
	t := c.Theme()
	tk := kitDesignTokens(t.Dark)
	hl := tk.addwd
	if l.sign == '-' {
		hl = tk.delwd
	}
	if len(l.words) == 0 {
		code := codeText(c, l.text).TextColor(t.Text)
		if u.diffWrap {
			code.Wrap().Grow(1).MinWidth(0)
		} else {
			code.SingleLine().Shrink(0)
		}
		return
	}
	// The line is one rich text paragraph, not a row of texts: a copy then
	// keeps the line whole, where each text would start a new line.
	spans := make([]ui.Span, 0, 2*len(l.words)+1)
	at := 0
	for _, w := range l.words {
		if w.start > at {
			spans = append(spans, codeSpan(c, l.text[at:w.start], t.Text, ui.Color{}))
		}
		spans = append(spans, codeSpan(c, l.text[w.start:w.end], t.Text, hl))
		at = w.end
	}
	if at < len(l.text) {
		spans = append(spans, codeSpan(c, l.text[at:], t.Text, ui.Color{}))
	}
	run := ui.RichText(c, spans...)
	if u.diffWrap {
		run.Grow(1).MinWidth(0).Wrap()
	} else {
		run.SingleLine().Shrink(0)
	}
}

// codeSpan is one piece of a code line, in the code face of codeText, with
// the changed-word highlight as its background.
func codeSpan(c *ui.Context, text string, color, bg ui.Color) ui.Span {
	return ui.Span{Text: text, Font: "monospace", Size: c.Theme().Rem(12.5 / 13), Color: color, Background: bg}
}

// signText is the sign column's character: a true minus sign for a deletion,
// as the design uses U+2212, a plus for an addition, a space otherwise.
func signText(b byte) string {
	switch b {
	case '+':
		return "+"
	case '-':
		return "\u2212"
	}
	return ""
}
