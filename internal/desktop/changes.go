package desktop

import (
	"context"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"gitowl/internal/workflow"
)

// changeKey identifies a row of the working tree list.
type changeKey struct {
	area workflow.Area
	path string
	// header marks the row naming an area; an empty area is that one row.
	header bool
}

type changeRow struct {
	key    changeKey
	change workflow.Change
	count  int
	// visible is a header row's count of files the filter shows now; it
	// equals count when no filter hides any.
	visible int
}

// diffPane is the diff shown in the right column.
type diffPane struct {
	key     changeKey
	info    workflow.Diff
	state   *editor.DiffState
	err     error
	loading bool
	// hunk is the hunk the hunk bar names; caretHunk the hunk the caret
	// was last seen in, -1 outside them.
	hunk, caretHunk int
	// scrollHunk is the card to bring into view on the next frame, set
	// when Previous/Next moves between changes, then -1 so a plain scroll
	// of the diff is not fought by a scroll-into-view every frame.
	scrollHunk int
	// cards are the design's hunk cards parsed from cardsFor, the patch
	// they were parsed from, so one parse serves a frame.
	cards    []diffHunk
	cardsFor string
}

// workingTree is the state of the working tree review.
type workingTree struct {
	rows     []changeRow
	list     ui.ListState
	cursor   int
	selected *changeKey
	diff     *diffPane
	diffSeq  uint64
	// busy is the file being staged or unstaged; one change runs at a time.
	busy *changeKey
	// bulk is "stage" or "unstage" while Stage All or Unstage All runs.
	bulk string
	// skipped are the changes the last Stage All left alone.
	skipped []workflow.Change
	// listFocused is set while the list showed the keyboard focus in the
	// last frame, to mark its selected row.
	listFocused bool
	// refocus gives the keyboard back to the list after a check box took it.
	refocus bool
	// filter is the case-insensitive literal text the list is filtered by;
	// it hides rows whose name and path do not contain it, without
	// narrowing what Stage All and Unstage All act on.
	filter string
	// stats are the per-file line counts of the current change list, keyed
	// by area and path, read in the background.
	stats map[changeKey]workflow.FileStat
	// stagedAdded and stagedRemoved total the staged side's line counts.
	stagedAdded, stagedRemoved int
	// statsReady reports that stats hold counts for the current snapshot.
	statsReady bool
	// statSeq numbers numstat reads, so a stale one does not overwrite a
	// newer snapshot's counts.
	statSeq uint64
	// chromeList and chromeSel are the custom-shell file list: ui.List
	// owns keyboard movement, Selected is an index into chromeRows.
	chromeList ui.ListState
	chromeSel  int
}

var areas = []workflow.Area{workflow.Staged, workflow.Unstaged, workflow.Untracked}

func areaChanges(s workflow.Snapshot, a workflow.Area) []workflow.Change {
	switch a {
	case workflow.Staged:
		return s.Staged
	case workflow.Unstaged:
		return s.Unstaged
	}
	return s.Untracked
}

// buildRows lays out the three areas, each under a header, applying the
// filter: a header always shows, with the count of files it now reveals and
// the area's total, and only matching file rows follow it. An empty area,
// or one the filter empties, is its header alone.
func (w *workingTree) buildRows(s workflow.Snapshot) []changeRow {
	var rows []changeRow
	for _, a := range areas {
		cs := areaChanges(s, a)
		rows = append(rows, changeRow{
			key:     changeKey{area: a, header: true},
			count:   len(cs),
			visible: w.visibleCount(cs),
		})
		for _, c := range cs {
			if !w.filtered(c.Path) {
				continue
			}
			rows = append(rows, changeRow{key: changeKey{area: a, path: c.Path}, change: c})
		}
	}
	return rows
}

// fileNear returns the first file row from i on, forward or back, else
// the first in the other direction, else -1.
func (w *workingTree) fileNear(i int, forward bool) int {
	for _, fwd := range []bool{forward, !forward} {
		d := 1
		if !fwd {
			d = -1
		}
		for j := i; j >= 0 && j < len(w.rows); j += d {
			if !w.rows[j].key.header {
				return j
			}
		}
	}
	return -1
}

// paths counts the files of the working tree: a file both staged and
// unstaged is one.
func paths(s workflow.Snapshot) int {
	seen := map[string]bool{}
	for _, a := range areas {
		for _, c := range areaChanges(s, a) {
			seen[c.Path] = true
		}
	}
	return len(seen)
}

// partial reports a file with changes both staged and unstaged: each entry
// is one part of it.
func partial(s workflow.Snapshot, ch workflow.Change) bool {
	other := workflow.Unstaged
	switch ch.Area {
	case workflow.Unstaged:
		other = workflow.Staged
	case workflow.Untracked:
		return false
	}
	return inArea(s, other, ch.Path)
}

func (w *workingTree) row(key changeKey) (int, bool) {
	for i, r := range w.rows {
		if r.key == key {
			return i, true
		}
	}
	return -1, false
}

// updateWorkingTree applies a new snapshot to the list, keeping the
// selection when its file is still in the same area.
func (u *UI) updateWorkingTree() {
	w := &u.wt
	w.rows = w.buildRows(u.snap)
	u.loadNumstat()
	if w.selected == nil {
		w.cursor = -1
		return
	}
	i, ok := w.row(*w.selected)
	if !ok {
		w.selected, w.diff, w.cursor = nil, nil, -1
		w.diffSeq++
		return
	}
	w.cursor = i
	u.loadDiff(w.rows[i].change)
}

func (u *UI) resetWorkingTree() {
	u.wt.diffSeq++
	u.wt = workingTree{diffSeq: u.wt.diffSeq, cursor: -1}
}

// selectRow shows the diff of the row at i.
func (u *UI) selectRow(i int) {
	w := &u.wt
	if i < 0 || i >= len(w.rows) {
		return
	}
	if w.rows[i].key.header {
		// Area headers are not files: the arrows move on to the next file
		// in the direction they went, or back when there is none.
		prev := -1
		if w.selected != nil {
			prev, _ = w.row(*w.selected)
		}
		j := w.fileNear(i, i > prev)
		if j < 0 {
			w.cursor = prev
			return
		}
		i = j
		w.list.ScrollIntoView(i)
	}
	r := w.rows[i]
	w.cursor = i
	if w.selected != nil && *w.selected == r.key && w.diff != nil {
		u.showDetail()
		return
	}
	key := r.key
	w.selected = &key
	u.showDetail()
	u.loadDiff(r.change)
}

// loadDiff reads ch's diff in the background. Only the latest request for
// the current repository applies.
func (u *UI) loadDiff(ch workflow.Change) {
	w := &u.wt
	w.diffSeq++
	seq, repo := w.diffSeq, u.repo
	key := changeKey{area: ch.Area, path: ch.Path}
	if w.diff == nil || w.diff.key != key {
		w.diff = &diffPane{key: key, caretHunk: -1, scrollHunk: -1}
	}
	w.diff.loading = true
	u.background(func() func() {
		info, err := repo.FileDiff(context.Background(), ch)
		var state *editor.DiffState
		if err == nil && info.HasText() {
			state, err = diffState(info.Patch, ch.Path)
		}
		if state != nil && info.NoHunks == "" && len(state.Hunks()) == info.HunkCount() {
			// The viewer marks the same hunks as the workflow counts, so
			// its number for a hunk names the one to stage.
			state.HunkActions = true
		}
		return func() {
			if seq != u.wt.diffSeq || repo != u.repo {
				return
			}
			old := u.wt.diff
			pane := &diffPane{key: key, info: info, state: state, err: err, caretHunk: -1, scrollHunk: -1}
			if old != nil && old.key == key {
				// After staging one hunk the bar names the one now in
				// its place.
				pane.hunk = old.hunk
			}
			// Keep the viewer, and its scroll position, when nothing changed.
			if old != nil && old.key == key && old.state != nil && old.info.Patch == info.Patch && err == nil {
				pane.state = old.state
			}
			u.wt.diff = pane
		}
	})
}

// diffState prepares a viewer for a one-file patch.
func diffState(patch, name string) (*editor.DiffState, error) {
	files, err := editor.ParsePatch(patch)
	if err != nil || len(files) == 0 {
		return nil, err
	}
	// Bars, not + and -: MyGo paints the signs inside the text's clip,
	// where code scrolled sideways covers them; the bars are in the
	// gutter, which stays opaque and in place (solid for additions,
	// hatched for deletions, with the line numbers beside them).
	d := &editor.DiffState{Layout: editor.DiffUnified, Indicators: editor.DiffBars}
	d.SetPatch(files[0])
	if lang := editor.LanguageFor(name); lang != nil {
		d.Old.SetLanguage(lang)
		d.New.SetLanguage(lang)
	}
	return d, nil
}

func (u *UI) areaTitle(a workflow.Area) string {
	return u.t([...]string{"areaStaged", "areaUnstaged", "areaUntracked"}[a])
}

// areaCount names an area with its number of files.
func (u *UI) areaCount(a workflow.Area, n int) string {
	return u.t([...]string{"stagedCount", "unstagedCount", "untrackedCount"}[a], "count", n)
}

// filterEmpty reports that the filter hides every file (only headers
// remain).
func (u *UI) filterEmpty() bool {
	for _, r := range u.wt.rows {
		if !r.key.header {
			return false
		}
	}
	return true
}

// setFilter applies new filter text and rebuilds the visible rows, keeping
// the selection when its file is still shown.
func (u *UI) setFilter(text string) {
	w := &u.wt
	if w.filter == text {
		return
	}
	w.filter = text
	w.rows = w.buildRows(u.snap)
	if w.selected == nil {
		w.cursor = -1
		return
	}
	if i, ok := w.row(*w.selected); ok {
		w.cursor = i
	} else {
		// The selected file is filtered out: the list shows no cursor, but
		// the diff on the right stays until another file is chosen.
		w.cursor = -1
	}
}

// indexBusy reports that the index cannot be changed now: a staging
// change, or another Git change, is in flight.
func (u *UI) indexBusy() bool { return u.writing() }

// stageAllBlock returns the message key explaining why Stage All cannot
// run now, "busy" while another operation runs, or "".
func (u *UI) stageAllBlock() string {
	if u.repo == nil || u.writing() {
		return "busy"
	}
	for _, ch := range append(append([]workflow.Change{}, u.snap.Unstaged...), u.snap.Untracked...) {
		if ch.Stageable() {
			return ""
		}
	}
	return "errorNothingToStage"
}

// unstageAllBlock is stageAllBlock for Unstage All.
func (u *UI) unstageAllBlock() string {
	if u.repo == nil || u.writing() {
		return "busy"
	}
	for _, ch := range u.snap.Unstaged {
		if ch.Conflicted {
			return "errorUnstageAllConflicts"
		}
	}
	if len(u.snap.Staged) == 0 {
		return "emptyStaged"
	}
	return ""
}

// stageAllTarget counts the whole-worktree files Stage All would stage: the
// stageable unstaged and untracked changes, whatever the filter hides.
func (u *UI) stageAllTarget() int {
	n := 0
	for _, a := range []workflow.Area{workflow.Unstaged, workflow.Untracked} {
		for _, ch := range areaChanges(u.snap, a) {
			if ch.Stageable() {
				n++
			}
		}
	}
	return n
}

// unstageAllTarget counts the whole-worktree files Unstage All would
// unstage: every staged change.
func (u *UI) unstageAllTarget() int { return len(u.snap.Staged) }

// bulkStage runs Stage All, or Unstage All, on the whole worktree. The
// selection follows its file to its new area.
func (u *UI) bulkStage(unstage bool) {
	block := u.stageAllBlock()
	if unstage {
		block = u.unstageAllBlock()
	}
	if block != "" {
		return
	}
	u.wt.bulk, u.wt.skipped = "stage", nil
	if unstage {
		u.wt.bulk = "unstage"
	}
	repo := u.repo
	u.background(func() func() {
		var snap workflow.Snapshot
		var res workflow.BulkResult
		var err error
		if unstage {
			snap, err = repo.UnstageAll(context.Background())
		} else {
			snap, res, err = repo.StageAll(context.Background())
		}
		return func() {
			if repo != u.repo {
				return
			}
			u.wt.bulk = ""
			u.report(err)
			u.wt.skipped = res.Skipped
			if snap.Root == "" {
				return
			}
			if sel := u.wt.selected; sel != nil && !inArea(snap, sel.area, sel.path) {
				u.wt.selected = u.followFile(snap, workflow.Change{Path: sel.path, Area: sel.area})
			}
			u.setSnapshot(snap)
		}
	})
}

func inArea(s workflow.Snapshot, a workflow.Area, path string) bool {
	for _, c := range areaChanges(s, a) {
		if c.Path == path {
			return true
		}
	}
	return false
}

// hintKey names the long description of an area's bulk action.
func hintKey(area workflow.Area) string {
	if area == workflow.Staged {
		return "unstageAllHint"
	}
	return "stageAllHint"
}

// headerCount is a header's count text. Without a filter it is the area's
// total; while a filter hides files it reads "visible / total", so a
// filtered view does not pass a reduced count off as the whole area.
func (u *UI) headerCount(r changeRow) string {
	if u.wt.filter == "" || r.visible == r.count {
		return strconv.Itoa(r.count)
	}
	return strconv.Itoa(r.visible) + " / " + strconv.Itoa(r.count)
}

// toggleSelected stages or unstages the selected file.
func (u *UI) toggleSelected() {
	w := &u.wt
	key := w.selected
	if key == nil || u.indexBusy() {
		return
	}
	if i, ok := w.row(*key); ok && canStage(w.rows[i].change) {
		u.toggleStaged(w.rows[i].change)
	}
}

func kindKey(ch workflow.Change) string {
	if ch.Letter() == "!" {
		return "kindX"
	}
	return "kind" + ch.Letter()
}

// canStage reports whether ch can be staged or unstaged as a whole file.
func canStage(ch workflow.Change) bool { return ch.Stageable() }

// toggleStaged stages an unstaged or untracked file, or unstages a staged
// one. The selection follows the file to its new area.
func (u *UI) toggleStaged(ch workflow.Change) {
	if u.indexBusy() || u.repo == nil {
		return
	}
	key := changeKey{area: ch.Area, path: ch.Path}
	u.wt.busy = &key
	repo := u.repo
	u.background(func() func() {
		var snap workflow.Snapshot
		var err error
		if ch.Area == workflow.Staged {
			snap, err = repo.Unstage(context.Background(), ch)
		} else {
			snap, err = repo.Stage(context.Background(), ch)
		}
		return func() {
			if repo != u.repo {
				return
			}
			u.wt.busy = nil
			u.report(err)
			if snap.Root == "" {
				return
			}
			follow := err == nil && u.wt.selected != nil && *u.wt.selected == key
			at := u.wt.cursor
			if follow {
				u.wt.selected = u.followFile(snap, ch)
			}
			u.setSnapshot(snap)
			if follow && u.wt.selected == nil {
				// The file left the list: the next file takes its place.
				if i := u.wt.fileNear(min(at, len(u.wt.rows)-1), true); i >= 0 {
					u.selectRow(i)
				}
			}
		}
	})
}

// followFile returns where ch went after staging or unstaging, preferring
// the side with the same content.
func (u *UI) followFile(snap workflow.Snapshot, ch workflow.Change) *changeKey {
	order := []workflow.Area{workflow.Staged}
	if ch.Area == workflow.Staged {
		order = []workflow.Area{workflow.Unstaged, workflow.Untracked}
	}
	for _, a := range order {
		for _, c := range areaChanges(snap, a) {
			if c.Path == ch.Path {
				return &changeKey{area: a, path: c.Path}
			}
		}
	}
	return nil
}

// diffBody shows a diff pane's lines, or why it has none.
func (u *UI) diffBody(c *ui.Context, p *diffPane) {
	tk := kitDesignTokens(c.Theme().Dark)
	switch {
	case p.err != nil:
		kitPlaceholder(c, u.t("diffFailed"), p.err.Error())
	case p.state != nil:
		editor.DiffView(c, p.state).Grow(1).FillWidth()
	case p.loading:
		ui.Column(c).Fill().Center().Children(func() { kitSpinner(c, tk, 16) })
	case p.info.Binary:
		kitPlaceholder(c, u.t("binaryTitle"), u.t("binaryBody"))
	case p.info.TooLarge:
		kitPlaceholder(c, u.t("tooLargeTitle"), u.t("tooLargeBody"))
	case p.info.Submodule:
		kitPlaceholder(c, u.t("submoduleTitle"), u.t("submoduleBody"))
	case p.info.Conflicted:
		kitPlaceholder(c, u.t("conflictTitle"), u.t("conflictBody"))
	case p.info.NoTextChange:
		kitPlaceholder(c, u.t("noTextTitle"), u.t("noTextBody"))
	default:
		kitPlaceholder(c, "", u.t("noDiff"))
	}
}

// changeItems are the actions on a working tree change, in its context
// menu and the diff's … menu.
func (u *UI) changeItems(m *ui.Menu, ch workflow.Change) {
	if u.opts.Launch != nil {
		u.fileItems(m, ch.Path, missingInWorktree(ch))
	}
	if ch.Area == workflow.Unstaged {
		if u.opts.Launch != nil {
			m.Separator()
		}
		if m.Item(u.t("discardAction")).Disabled(u.writing()).Chosen() {
			u.planDiscard(ch)
		}
	}
}
