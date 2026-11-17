package desktop

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

// historyPage is how many commits a read of history returns.
const historyPage = 200

// historyAllLimit is large enough that All commits can read the full list
// in one Graph call, with no pagination UI.
const historyAllLimit = 1_000_000

// fileColumnWidth is the detail's file column, the design's 220 (history.md
// §7).
const fileColumnWidth = 220

// history is the state of the commit history review.
type history struct {
	commits []workflow.CommitInfo
	more    bool
	loading bool
	loaded  bool
	// awaitBranches says a read was asked for before the branch list was
	// read; it runs when the list arrives.
	awaitBranches bool
	err           error
	// seq numbers history reads; only the latest applies.
	seq uint64
	// head is the HEAD the list was read at.
	head, branch string

	// The toolbar: shownRef the one branch the graph draws ("" for All
	// branches), order the sort, branchFilter the chooser's filter text and pop
	// which of its popovers is open. field is the criterion the search text
	// matches (message, hash or author) and text the search itself.
	shownRef     string
	remotes      bool
	order        workflow.HistoryOrder
	branchFilter string
	field        workflow.HistoryField
	text         string
	pop          historyPopovers
	// query is what the list shows, fixed to a start commit in rq so its
	// pages stay consistent; cancel stops its read in flight.
	query  workflow.HistoryQuery
	rq     workflow.ResolvedQuery
	cancel context.CancelFunc

	list     ui.ListState
	cursor   int
	selected string

	// The Graph the list draws when not searching: its rows (one per
	// commit), the open lanes to continue the next page with, and each
	// commit's class (Unpushed/Incoming/Shared). graphing says the list is
	// a Graph read, not a search. rg fixes the scope's tips so pages stay
	// consistent.
	graphing bool
	rg       workflow.ResolvedGraph
	rows     []workflow.GraphRow
	lanes    workflow.LaneState
	classes  map[string]workflow.GraphClass
	// sources is the ref each commit came from, and groups the palette group
	// of every row's lines (see graph_groups.go).
	sources map[string]string
	groups  []graphRowGroups
	// searching is true when the list shows search results rather than a
	// continuous Graph page; it picks the load-more read (SearchGraph vs
	// Graph) and the lane handling, as a search does not carry lanes across
	// pages. The list graphs in both cases, so graphing stays true.
	searching bool

	detail        *workflow.CommitDetail
	detailErr     error
	detailLoading bool
	detailSeq     uint64
	// detailOpen shows the panel under the All commits list. detailTab is
	// 0 Commit, 1 Changes, 2 Files. scrollTo is a list index to bring into
	// view after selecting a parent, or -1.
	detailOpen bool
	// listHeight is the list's height while the detail is open, set by
	// dragging the grip above the panel; 0 is the design's 312 DIP. gripping
	// marks a drag in progress, and panelH the detail panel's height last
	// frame, which decides whether a release closes the panel.
	listHeight float32
	gripping   bool
	panelH     float32
	// bodyH is the height of the list and detail area last frame, the start
	// of a slide (chromeHistoryPanels).
	bodyH     float32
	detailTab int
	scrollTo  int
	// commitList virtualises the commit rows, so only the visible ones are drawn.
	commitList ui.ListState

	// restoreTop is the first visible row to scroll to once the list is
	// read, after returning to this worktree.
	restoreTop int

	files      ui.ListState
	fileCursor int
	diff       *diffPane
	diffSeq    uint64
}

func newHistory() history {
	return history{cursor: -1, fileCursor: -1, scrollTo: -1, remotes: true}
}

// historyPopovers says which toolbar popover is open.
type historyPopovers struct {
	branches bool
	sort     bool
	field    bool
}

// historyChanged reads the first page of history again, keeping the
// selected commit when it is still in it.
func (u *UI) historyChanged() {
	repo := u.repo
	if repo == nil {
		return
	}
	h := &u.hist
	if !u.br.loaded {
		// The graph's branches are not read yet, so a list read now would
		// show HEAD alone. Wait for them; their arrival reads the list.
		h.awaitBranches = true
		u.loadBranches()
		return
	}
	h.awaitBranches = false
	h.seq++
	seq := h.seq
	h.loading = true
	head, branch := u.snap.Head, u.snap.Branch
	q := u.query()
	if !h.query.Equal(q) {
		// A new search or view: its first results replace the list.
		h.query = q
		h.commits, h.more, h.loaded = nil, false, false
	}
	if h.cancel != nil {
		h.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	searching := q.Searching()
	graphing := true // the list draws a graph whether browsing or searching.
	primary := u.primaryScope()
	u.background(func() func() {
		read := u.readHistoryPage(ctx, repo, q, primary, !searching)
		return func() {
			if repo != u.repo || seq != u.hist.seq {
				return
			}
			h := &u.hist
			h.loading, h.loaded, h.err, h.cancel = false, true, read.err, nil
			if read.err != nil {
				h.commits, h.more, h.graphing, h.searching = nil, false, false, false
				h.rows, h.classes = nil, nil
				h.groups, h.sources = nil, nil
				u.clearCommit()
				return
			}
			h.rq, h.rg, h.graphing, h.searching = read.rq, read.rg, graphing, searching
			h.rows, h.lanes, h.classes = read.rows, read.lanes, read.classes
			h.commits, h.more, h.head, h.branch = read.commits, read.more, head, branch
			h.sources = read.sources
			h.groups = u.graphGroups(h.commits, h.rows, h.sources)
			h.cursor = -1
			for i, c := range read.commits {
				if c.Hash == h.selected {
					h.cursor = i
				}
			}
			switch {
			case h.cursor < 0:
				u.clearCommit()
			case h.detail == nil && !h.detailLoading:
				// A selection restored from an earlier visit.
				u.openCommit(h.cursor)
			}
			if h.restoreTop > 0 {
				h.list.ScrollTo(min(h.restoreTop, len(read.commits)-1), ui.Start)
			}
			h.restoreTop = 0
		}
	})
}

// loadMoreHistory appends the next page.
func (u *UI) loadMoreHistory() {
	repo := u.repo
	h := &u.hist
	if repo == nil || h.loading || !h.more {
		return
	}
	h.seq++
	seq, skip, rq := h.seq, len(h.commits), h.rq
	graphing, searching, rg, lanes := h.graphing, h.searching, h.rg, h.lanes
	// loaded is a snapshot of the results so far, read on the main goroutine
	// so the background relayout never races the UI's slice.
	loaded := append([]workflow.CommitInfo(nil), h.commits...)
	h.loading = true
	u.background(func() func() {
		if graphing && !searching {
			g, err := repo.Graph(context.Background(), rg, skip, historyPage, lanes)
			return func() {
				if repo != u.repo || seq != u.hist.seq {
					return
				}
				h := &u.hist
				h.loading, h.err = false, err
				if err != nil {
					return
				}
				h.commits = append(h.commits, graphCommits(g)...)
				h.rows = append(h.rows, g.Rows...)
				for hash, cl := range graphClasses(g) {
					h.classes[hash] = cl
				}
				h.lanes, h.more = g.Lanes, g.More
				h.addSources(graphSources(g))
				h.groups = u.graphGroups(h.commits, h.rows, h.sources)
			}
		}
		// A search does not carry lanes across pages (its results are not a
		// continuous history), so the next page is read and the whole set is
		// laid out together over the scope topology, keeping lanes and elided
		// edges continuous across the boundary: a result's nearest visible
		// ancestor may sit on the next page, so only a whole-run relayout
		// joins them instead of restarting lanes mid-list (issue 14 #31).
		g, err := repo.SearchGraph(context.Background(), rq, rg, skip, historyPage)
		var rows []workflow.GraphRow
		if err == nil {
			rows, err = repo.SearchGraphRows(context.Background(), rq,
				append(loaded, searchCommits(g)...))
		}
		return func() {
			if repo != u.repo || seq != u.hist.seq {
				return
			}
			h := &u.hist
			h.loading, h.err = false, err
			if err != nil {
				return
			}
			h.commits = append(h.commits, searchCommits(g)...)
			for hash, cl := range searchClasses(g) {
				h.classes[hash] = cl
			}
			h.rows = rows
			h.more = g.More
			h.addSources(searchSources(g))
			h.groups = u.graphGroups(h.commits, h.rows, h.sources)
		}
	})
}

// followHead reloads history when the checked-out commit or branch moved,
// as after a commit, a branch switch or work outside GitOwl.
func (u *UI) followHead() {
	h := &u.hist
	if !h.loaded && !h.loading {
		u.historyChanged()
		return
	}
	if u.snap.Head != h.head || u.snap.Branch != h.branch {
		u.historyChanged()
	}
}

// scopeRefsChanged reads history again when it draws a Graph, which a fetch,
// pull or branch change may have moved: the branches it shows, and the
// upstream a fetch advanced, both change the picture.
func (u *UI) scopeRefsChanged() {
	h := &u.hist
	if !h.loaded {
		return
	}
	if h.graphing {
		u.historyChanged()
	}
}

func (u *UI) clearCommit() {
	h := &u.hist
	h.selected, h.detail, h.detailErr, h.detailLoading, h.diff = "", nil, nil, false, nil
	h.fileCursor = -1
	h.detailSeq++
	h.diffSeq++
}

// selectCommit shows commit i's detail.
func (u *UI) selectCommit(i int) {
	if i < 0 || i >= len(u.hist.commits) {
		return
	}
	u.showDetail()
	u.openCommit(i)
}

// openCommit selects the commit at i and reads its detail, without moving
// a narrow window to the detail pane.
func (u *UI) openCommit(i int) {
	h := &u.hist
	if i < 0 || i >= len(h.commits) {
		return
	}
	h.cursor = i
	c := h.commits[i]
	if h.selected == c.Hash && (h.detail != nil || h.detailLoading) {
		return
	}
	u.clearCommit()
	h.selected, h.detailLoading = c.Hash, true
	seq, repo := h.detailSeq, u.repo
	u.background(func() func() {
		d, err := repo.Detail(context.Background(), c)
		return func() {
			if repo != u.repo || seq != u.hist.detailSeq {
				return
			}
			h := &u.hist
			h.detailLoading, h.detailErr = false, err
			if err == nil {
				h.detail = &d
				if !u.chrome.shell && len(d.Files) > 0 {
					u.selectCommitFile(0)
				}
			}
		}
	})
}

// selectCommitFile shows the diff of file i of the selected commit.
func (u *UI) selectCommitFile(i int) {
	h := &u.hist
	if h.detail == nil || i < 0 || i >= len(h.detail.Files) {
		return
	}
	h.fileCursor = i
	f := h.detail.Files[i]
	key := changeKey{area: workflow.InCommit, path: f.Path}
	if h.diff != nil && h.diff.key == key {
		return
	}
	h.diffSeq++
	seq, repo, d := h.diffSeq, u.repo, *h.detail
	h.diff = &diffPane{key: key, loading: true, info: workflow.Diff{Change: f}}
	u.background(func() func() {
		info, err := repo.CommitFileDiff(context.Background(), d, f)
		var state = diffPane{key: key, info: info, err: err}
		if err == nil && info.HasText() {
			state.state, state.err = diffState(info.Patch, f.Path)
		}
		return func() {
			if repo != u.repo || seq != u.hist.diffSeq {
				return
			}
			u.hist.diff = &state
		}
	})
}

// defaultCommit selects the newest commit of the list, which opens its
// first file, when History is open with nothing selected: the first visit,
// a selection that is gone, or a new search. It never moves a narrow
// window to the detail, and GitOwl never opens History by itself.
func (u *UI) defaultCommit() {
	h := &u.hist
	if u.middleTab != 1 && u.middleTab != 2 || h.loading || h.selected != "" || h.detailLoading || len(h.commits) == 0 {
		return
	}
	u.openCommit(0)
}

func (u *UI) baseText(d *workflow.CommitDetail) string {
	switch d.Base {
	case workflow.BaseEmptyTree:
		return u.t("baseEmptyTree")
	case workflow.BaseFirstParent:
		return u.t("baseFirstParent", "hash", d.BaseHash[:min(7, len(d.BaseHash))])
	}
	return u.t("baseParent", "hash", d.BaseHash[:min(7, len(d.BaseHash))])
}

// commitBody returns the commit message below its subject (the first line),
// trimmed of the blank lines that separate them, so the detail shows the
// description without repeating the h1 subject.
func commitBody(message, subject string) string {
	body := strings.TrimPrefix(message, subject)
	return strings.TrimLeft(body, "\n")
}
