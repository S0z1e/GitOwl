package desktop

import (
	"strings"

	"gitowl/internal/workflow"
)

// browseState tracks the branch filter and the branch being browsed. A
// browsed branch sets the History scope without checking anything out; the
// checked-out branch stays whatever the worktree holds.
type browseState struct {
	// filter is the text of the branch filter box, matched without case
	// against full branch names.
	filter string
	// browsing is the key ("local:…" or "remote:…") of the branch being
	// browsed, "" when History follows the current checkout (HEAD).
	browsing string
	// gen numbers browse requests so a quick run of clicks shows only the
	// latest branch's history, never an earlier click's late result.
	gen uint64
	// gone is set when the browsed branch disappeared and History was
	// returned to the current checkout; it clears on the next browse.
	gone bool
}

// matchFilter reports whether name matches the filter text, without case.
// An empty filter matches everything.
func matchFilter(name, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	return strings.Contains(strings.ToLower(name), strings.ToLower(filter))
}

// filterNames keeps the names that match the filter, in their original
// order. An empty filter returns all the names.
func filterNames(names []string, filter string) []string {
	if strings.TrimSpace(filter) == "" {
		return names
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if matchFilter(n, filter) {
			out = append(out, n)
		}
	}
	return out
}

// filterLocal keeps the local branches whose name matches the filter.
func filterLocal(bs []workflow.Branch, filter string) []workflow.Branch {
	if strings.TrimSpace(filter) == "" {
		return bs
	}
	out := make([]workflow.Branch, 0, len(bs))
	for _, b := range bs {
		if matchFilter(b.Name, filter) {
			out = append(out, b)
		}
	}
	return out
}

// filterRemote keeps the remote-tracking branches whose name matches.
func filterRemote(rs []workflow.RemoteBranch, filter string) []workflow.RemoteBranch {
	if strings.TrimSpace(filter) == "" {
		return rs
	}
	out := make([]workflow.RemoteBranch, 0, len(rs))
	for _, r := range rs {
		if matchFilter(r.Name, filter) {
			out = append(out, r)
		}
	}
	return out
}

// localScope is the History scope of a local branch.
func localScope(name string) workflow.HistoryScope {
	return workflow.HistoryScope{Ref: "refs/heads/" + name, Name: name}
}

// remoteScope is the History scope of a remote-tracking branch.
func remoteScope(name string) workflow.HistoryScope {
	return workflow.HistoryScope{Ref: "refs/remotes/" + name, Name: name, Remote: true}
}

// browseScope shows the history of scope without checking anything out: it
// opens History and draws only that branch, hiding the others in the chooser.
// A new generation makes any earlier click's late result moot. key identifies
// the browsed row so the tree can mark it.
func (u *UI) browseScope(key string, scope workflow.HistoryScope) {
	u.br.bro.gen++
	u.br.bro.browsing = key
	u.br.bro.gone = false
	h := &u.hist
	h.shownRef = scope.Ref
	u.middleTab = 1
	u.applySearch()
}

// browseCurrent returns History to the current checkout, as after a switch
// or when a browsed branch is gone: every branch is drawn again.
func (u *UI) browseCurrent() {
	u.br.bro.gen++
	u.br.bro.browsing = ""
	u.hist.shownRef = ""
	u.applySearch()
}

// browseKeyGone reports whether the browsed branch no longer exists in the
// branches last read, so its History scope would be missing.
func (u *UI) browseKeyGone() bool {
	key := u.br.bro.browsing
	switch {
	case key == "":
		return false
	case strings.HasPrefix(key, "local:"):
		name := strings.TrimPrefix(key, "local:")
		for _, b := range u.br.data.Local {
			if b.Name == name {
				return false
			}
		}
		return true
	case strings.HasPrefix(key, "remote:"):
		name := strings.TrimPrefix(key, "remote:")
		for _, r := range u.br.data.Remote {
			if r.Name == name {
				return false
			}
		}
		return true
	}
	return false
}

// browseRecover returns to the current checkout when the browsed branch is
// gone from the branches last read, so History never rests on a missing
// scope. It reports whether it recovered.
func (u *UI) browseRecover() bool {
	if u.br.bro.browsing == "" || !u.br.loaded || !u.browseKeyGone() {
		return false
	}
	u.browseCurrent()
	u.br.bro.gone = true
	return true
}
