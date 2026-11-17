package desktop

import (
	"context"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/prefs"
	"gitowl/internal/workflow"
)

// recentState is what the window knows of the recent repositories besides
// the list in the preferences: the last read status of each, which ones a
// background read is still in flight for, and a generation number so a
// stale reply is dropped (recentstatus_ui.go).
type recentState struct {
	// status holds the last read status of each recent folder, by root.
	status map[string]workflow.RecentStatus
	// reading marks roots whose background read has not yet replied.
	reading map[string]bool
	seq     uint64
	// checked is set once a read was started since the welcome screen last
	// showed, so entering the Landing reads once per visit.
	checked bool
}

// remember puts root first in the recent list once a repository opened.
// A failure to save is said once; the list still applies for this run.
func (u *UI) remember(root string) {
	p := u.prefs
	p.Recent = prefs.AddRecent(p.Recent, root)
	u.savePreferences(p, "recentNotSaved")
}

// forget removes root from the recent list and the pinned list. The folder is
// not touched.
func (u *UI) forget(root string) {
	p := u.prefs
	p.Recent = prefs.RemoveRecent(p.Recent, root)
	p.Pinned = prefs.RemovePinned(p.Pinned, root)
	u.savePreferences(p, "recentNotSaved")
}

// canAddRepositories reports whether the rail's + can open its picker now.
func (u *UI) canAddRepositories() bool {
	return !u.switchBlocked() && !u.choosing && u.opts.ChooseDirectories != nil
}

// chooseRepositories opens the folder picker that takes several folders for
// the rail's list. Chosen folders are added, not opened.
func (u *UI) chooseRepositories() {
	if !u.canAddRepositories() {
		return
	}
	u.choosing = true
	title := u.t("chromeAddRepository")
	u.opts.Go(func() {
		paths, err := u.opts.ChooseDirectories(title)
		u.opts.Post(func() {
			u.choosing = false
			if err != nil {
				u.report(err)
				return
			}
			u.addRepositories(paths)
		})
	})
}

// addRepositories adds the folders that are Git repositories to the recent
// list, so the rail shows them. The first folder chosen ends up first. A
// folder that is not a repository is left out and counted in a toast. Each
// folder is read in the background, as the recent list reads its status.
func (u *UI) addRepositories(paths []string) {
	if len(paths) == 0 {
		return
	}
	wf := u.wf
	u.background(func() func() {
		ctx := context.Background()
		var ok []string
		skipped := 0
		for _, path := range paths {
			if wf.RecentStatus(ctx, path).Kind == workflow.RecentOK {
				ok = append(ok, path)
			} else {
				skipped++
			}
		}
		return func() {
			if len(ok) > 0 {
				p := u.prefs
				for i := len(ok) - 1; i >= 0; i-- {
					p.Recent = prefs.AddRecent(p.Recent, ok[i])
				}
				u.savePreferences(p, "recentNotSaved")
			}
			if skipped > 0 {
				u.toast("error", u.t("addRepositoriesSkipped", "count", skipped))
			}
		}
	})
}

// savePreferences is setPreferences that says in a toast when the change
// could not be saved.
func (u *UI) savePreferences(p prefs.Preferences, failKey string) {
	u.setPreferences(p)
	if u.set.saveErr != "" {
		u.toast("error", u.t(failKey))
	}
}

// checkRecent is now refreshRecentStatus in recentstatus_ui.go, which
// reads each recent repository's real branch and change count in the
// background, not only whether its folder exists.

// openRecent opens a recent repository in this window. The current one
// stays until it opens; a failure is explained where the user is.
func (u *UI) openRecent(root string) {
	if u.switchBlocked() || u.repo != nil && u.repo.Root() == root {
		return
	}
	u.OpenPath(root)
}

// closeBlock returns why the repository cannot be closed now: "busy" while
// a Git change runs, or "".
func (u *UI) closeBlock() string {
	switch {
	case u.repo == nil:
		return "errorRepositoryClosed"
	case u.writing():
		return "busy"
	}
	return ""
}

// CloseRepository returns to the welcome screen. GitOwl stays open and the
// repository is left as it is; this run keeps the worktree's words and
// position for when it opens again. Reads, opens and drafts in flight are
// cancelled or their results dropped.
func (u *UI) CloseRepository() {
	if u.closeBlock() != "" {
		return
	}
	u.saveSession()
	u.wf.Close()
	u.openSeq++
	u.opening = false
	u.resetRepositoryState()
	u.repo, u.snap, u.notice, u.loading = nil, workflow.Snapshot{}, nil, 0
	u.lay.detail, u.lay.drawerOpen = false, false
	u.titleChanged()
	u.recent.checked = false
	u.land = landingState{}
}

// recentMenu lists the recent repositories, Open Other Repository… and
// Close Repository in the title bar's Switch Repository menu.
func (u *UI) recentMenu(m *ui.Menu) {
	blocked := u.switchBlocked()
	current := ""
	if u.repo != nil {
		current = u.repo.Root()
	}
	for _, root := range u.prefs.Recent {
		label := u.recentLabel(root)
		item := m.Item(label).Checked(root == current).Disabled(blocked || root == current)
		if item.Chosen() {
			u.openRecent(root)
		}
	}
	if len(u.prefs.Recent) > 0 {
		m.Separator()
	}
	if m.Item(u.t("openOtherRepository")).Disabled(blocked).Chosen() {
		u.chooseRepository()
	}
	if u.repo != nil {
		m.Separator()
		if m.Item(u.t("closeRepository")).Disabled(u.closeBlock() != "").Chosen() {
			u.CloseRepository()
		}
	}
}

// recentLabel names a recent repository by its folder and where it is,
// with the home folder shortened, so two of the same name differ.
func (u *UI) recentLabel(root string) string {
	return filepath.Base(root) + " — " + homeShort(filepath.Dir(root))
}

func homeShort(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if p == home {
			return "~"
		}
		if rel, err := filepath.Rel(home, p); err == nil && rel != ".." && !filepath.IsAbs(rel) && len(rel) > 0 && rel[0] != '.' {
			return filepath.Join("~", rel)
		}
	}
	return p
}

// recentList and recentRow used to render the Onevra welcome list; the
// custom rail in chrome.go draws Recent repositories now.
