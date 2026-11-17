package desktop

import (
	"strings"

	"gitowl/internal/workflow"
)

// historyBranches lists every local and remote-tracking branch the branch
// column last read, as History scopes, in the column's order.
func (u *UI) historyBranches() []workflow.HistoryScope {
	var out []workflow.HistoryScope
	for _, b := range u.br.data.Local {
		out = append(out, workflow.HistoryScope{Ref: "refs/heads/" + b.Name, Name: b.Name})
	}
	for _, b := range u.br.data.Remote {
		out = append(out, workflow.HistoryScope{Ref: "refs/remotes/" + b.Name, Name: b.Name, Remote: true})
	}
	return out
}

// picked returns the branch the toolbar shows on its own, and whether there
// is one. A pick of a branch that no longer exists is no pick: All branches.
// It scans the branch lists without building them, as the chooser asks many
// times a frame.
func (u *UI) picked() (workflow.HistoryScope, bool) {
	ref := u.hist.shownRef
	if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
		for _, b := range u.br.data.Local {
			if b.Name == name {
				return workflow.HistoryScope{Ref: ref, Name: name}, true
			}
		}
	} else if name, ok := strings.CutPrefix(ref, "refs/remotes/"); ok {
		for _, b := range u.br.data.Remote {
			if b.Name == name {
				return workflow.HistoryScope{Ref: ref, Name: name, Remote: true}, true
			}
		}
	}
	return workflow.HistoryScope{}, false
}

// branchView is which branches the graph draws, read once so a loop over the
// branches does not look the pick up again for each of them.
type branchView struct {
	pick    workflow.HistoryScope
	picked  bool
	remotes bool
	// configured are the remotes, to tell which remote a branch belongs to.
	configured []workflow.Remote
}

// branchView reads the view the toolbar sets.
func (u *UI) branchView() branchView {
	v := branchView{remotes: u.hist.remotes, configured: u.sync.info.Remotes}
	v.pick, v.picked = u.picked()
	return v
}

// pickRef is the ref of the picked branch, "" for All branches.
func (v branchView) pickRef() string {
	if v.picked {
		return v.pick.Ref
	}
	return ""
}

// shows reports whether the graph draws s: the picked branch alone, or, with
// All branches, every local branch and the remote ones while Remote branches
// is on. With a local branch picked, Remote branches also draws its remote
// counterparts, the branch of the same name on each remote.
func (v branchView) shows(s workflow.HistoryScope) bool {
	if v.picked {
		if s.Ref == v.pick.Ref {
			return true
		}
		return v.remotes && isRemoteCounterpart(s, v.pick, v.configured)
	}
	return !s.Remote || v.remotes
}

// isRemoteCounterpart reports whether remote branch s is local's branch on a
// remote: origin/feature/x for feature/x.
func isRemoteCounterpart(s, local workflow.HistoryScope, remotes []workflow.Remote) bool {
	if !s.Remote || local.Remote {
		return false
	}
	return s.Name == remoteOf(s.Name, remotes)+"/"+local.Name
}

// branchShown reports whether the graph draws s.
func (u *UI) branchShown(s workflow.HistoryScope) bool {
	return u.branchView().shows(s)
}

// shownScopes are the scopes the History list walks. A detached HEAD is walked
// as well, as its own tip.
func (u *UI) shownScopes() []workflow.HistoryScope {
	v := u.branchView()
	var out []workflow.HistoryScope
	for _, s := range u.historyBranches() {
		if v.shows(s) {
			out = append(out, s)
		}
	}
	if !v.picked && u.snap.Branch == "" {
		out = append(out, workflow.HistoryScope{})
	}
	return out
}

// graphLabels are the labels a commit shows in History: its branches the graph
// draws, and its tags. A branch the graph does not draw (a remote branch with
// Remote branches off, or any other branch while one is picked) shows no label.
func (u *UI) graphLabels(hash string) []commitLabel {
	v := u.branchView()
	var out []commitLabel
	for _, l := range u.commitLabels(hash) {
		if l.tag {
			out = append(out, l)
			continue
		}
		ref := "refs/heads/" + l.name
		if l.remote {
			ref = "refs/remotes/" + l.name
		}
		if v.shows(workflow.HistoryScope{Ref: ref, Name: l.name, Remote: l.remote}) {
			out = append(out, l)
		}
	}
	return out
}

// pickBranch shows one branch in the graph; "" shows All branches again.
func (u *UI) pickBranch(ref string) {
	u.hist.shownRef = ref
	u.applySearch()
}

// remoteSource is one remote and the remote-tracking branches named after it.
type remoteSource struct {
	name     string
	branches []workflow.HistoryScope
}

// remoteSources groups the remote-tracking branches by their remote, in the
// order the remotes are configured. Remotes without branches are left out.
func (u *UI) remoteSources() []remoteSource {
	remotes := u.sync.info.Remotes
	var srcs []remoteSource
	index := map[string]int{}
	add := func(name string) int {
		if i, ok := index[name]; ok {
			return i
		}
		index[name] = len(srcs)
		srcs = append(srcs, remoteSource{name: name})
		return index[name]
	}
	for _, r := range remotes {
		add(r.Name)
	}
	for _, s := range u.historyBranches() {
		if s.Remote {
			i := add(remoteOf(s.Name, remotes))
			srcs[i].branches = append(srcs[i].branches, s)
		}
	}
	out := srcs[:0]
	for _, src := range srcs {
		if len(src.branches) > 0 {
			out = append(out, src)
		}
	}
	return out
}

// remoteOf names the remote a remote-tracking branch such as origin/main
// belongs to: the longest configured remote name followed by a slash, or the
// text before the first slash when none matches.
func remoteOf(branch string, remotes []workflow.Remote) string {
	best := ""
	for _, r := range remotes {
		if strings.HasPrefix(branch, r.Name+"/") && len(r.Name) > len(best) {
			best = r.Name
		}
	}
	if best != "" {
		return best
	}
	name, _, _ := strings.Cut(branch, "/")
	return name
}

// query is the search and view the page shows. The History page always reads
// its Graph; the Search page reads the text typed, with the criterion, once
// there is text.
func (u *UI) query() workflow.HistoryQuery {
	h := &u.hist
	q := workflow.HistoryQuery{Scopes: u.shownScopes(), Order: h.order}
	if u.middleTab == 2 && strings.TrimSpace(h.text) != "" {
		q.Field, q.Text = h.field, h.text
	}
	return q
}

// historyFieldText names a search criterion in the toolbar and its menu.
func (u *UI) historyFieldText(f workflow.HistoryField) string {
	switch f {
	case workflow.FieldAuthor:
		return u.t("fieldAuthor")
	case workflow.FieldHash:
		return u.t("fieldHash")
	}
	return u.t("fieldMessage")
}

// historyFieldPlaceholder is the hint the search text shows for a criterion.
func (u *UI) historyFieldPlaceholder(f workflow.HistoryField) string {
	switch f {
	case workflow.FieldAuthor:
		return u.t("searchAuthor")
	case workflow.FieldHash:
		return u.t("searchHash")
	}
	return u.t("searchMessage")
}

// historyScopeText names the branches a search looks in: the picked branch,
// or All branches.
func (u *UI) historyScopeText() string {
	if p, ok := u.picked(); ok {
		return p.Name
	}
	return u.t("historyAllBranches")
}

// applySearch reads the list again with what the toolbar shows, unless the
// list already shows it.
func (u *UI) applySearch() {
	q := u.query()
	if q.Equal(u.hist.query) && u.hist.loaded {
		return
	}
	u.historyChanged()
}
