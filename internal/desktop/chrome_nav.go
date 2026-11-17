package desktop

import (
	"context"
	"gitowl/internal/desktop/designicons"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

func (u *UI) resetChromeNav() {
	u.chrome.navFilter = ""
	u.chrome.navSel = ""
	u.chrome.navClosed = nil
	u.chrome.navTags = nil
	u.chrome.navTagRefs = nil
	u.chrome.navStashes = nil
	u.chrome.navSeq++
}

// loadNavLists reads tags and stashes for the chrome navigation tree.
func (u *UI) loadNavLists() {
	repo := u.repo
	if repo == nil {
		return
	}
	u.chrome.navSeq++
	seq := u.chrome.navSeq
	u.background(func() func() {
		tags, tagErr := repo.Tags(context.Background())
		stashes, stashErr := repo.Stashes(context.Background())
		return func() {
			if repo != u.repo || seq != u.chrome.navSeq {
				return
			}
			if tagErr != nil {
				u.report(tagErr)
				return
			}
			if stashErr != nil {
				u.report(stashErr)
				return
			}
			names := make([]string, len(tags))
			for i, tag := range tags {
				names[i] = tag.Name
			}
			u.chrome.navTags, u.chrome.navTagRefs, u.chrome.navStashes = names, tags, stashes
		}
	})
}

func (u *UI) chromeNavFilter(c *ui.Context, tk kitTokens) {
	ui.Row(c).FillWidth().Height(kitFieldH).Radius(10).Gap(8).
		Background(tk.surf).Border(1, tk.bd2).Padding(0, 10).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, designicons.Search()).Size(14, 14).TextColor(tk.muted)
		field := ui.TextInputBase(c, &u.chrome.navFilter).Grow(1).MinWidth(0).FillWidth().
			Label(u.t("chromeNavFilter")).Placeholder(u.t("chromeNavFilter"))
		u.chromeMarkFocus(field)
	})
}

// navEntry is one row of the navigation tree. The tree is flattened into a
// slice of entries each frame so a virtual list draws only the rows on screen.
type navEntry struct {
	key   string
	label string
	// parent is the index of the folder row above this one, or -1.
	parent int
	depth  int
	folder bool
	open   bool
	// toggle opens or closes a folder; activate runs on a double click or
	// Enter; onClick runs on a plain click on the row itself.
	toggle   func()
	activate func()
	onClick  func()
	tip      string
	menu     func(m *ui.Menu)
	// draw paints the row's content inside its shell.
	draw func(c *ui.Context, tk kitTokens)
}

// navTree collects the entries of one frame's tree in display order.
type navTree struct {
	items []navEntry
}

func (n *navTree) add(e navEntry) int {
	n.items = append(n.items, e)
	return len(n.items) - 1
}

func (u *UI) chromeNavTree(c *ui.Context, tk kitTokens) {
	if u.chrome.navClosed == nil {
		// Worktrees, Remotes, Tags and Stashes open collapsed; the branch
		// folders follow the checked-out branch (chromeNavFolderOpen).
		u.chrome.navClosed = map[string]bool{
			"group:worktrees": true,
			"group:remotes":   true,
			"group:tags":      true,
			"group:stashes":   true,
		}
	}
	nav := &navTree{}
	q := u.chrome.navFilter
	u.chromeNavWorktrees(nav, q)
	u.chromeNavBranches(nav, q)
	u.chromeNavRemotes(nav, q)
	u.chromeNavTags(nav, q)
	u.chromeNavStashes(nav, q)
	items := nav.items
	u.chrome.navItems = items
	u.chrome.navIdx = -1
	for i, it := range items {
		if it.key == u.chrome.navSel {
			u.chrome.navIdx = i
			break
		}
	}
	st := &u.chrome.navList
	st.Key = func(i int) any { return items[i].key }
	st.Selected = &u.chrome.navIdx
	list := ui.List(c, st, len(items), func(i int) {
		u.chromeNavRow(c, tk, i, items[i], i == u.chrome.navIdx)
	}).Fill().Unselectable()
	if list.Changed() && u.chrome.navIdx >= 0 && u.chrome.navIdx < len(items) {
		u.chrome.navSel = items[u.chrome.navIdx].key
	}
}

// chromeNavLeft closes an open folder, or moves to the folder above row i.
func (u *UI) chromeNavLeft(i int) {
	items := u.chrome.navItems
	if i < 0 || i >= len(items) {
		return
	}
	it := items[i]
	if it.folder && it.open {
		it.toggle()
		return
	}
	if it.parent >= 0 && it.parent < len(items) {
		u.chrome.navSel = items[it.parent].key
	}
}

// chromeNavRight opens a closed folder, or moves to the first row under an
// open one.
func (u *UI) chromeNavRight(i int) {
	items := u.chrome.navItems
	if i < 0 || i >= len(items) {
		return
	}
	it := items[i]
	switch {
	case it.folder && !it.open:
		it.toggle()
	case it.folder && i+1 < len(items) && items[i+1].parent == i:
		u.chrome.navSel = items[i+1].key
	}
}

// chromeNavRow draws tree row i in its shell: its indent, its selection tint,
// its content, and its keys. A row is focusable as it was before the list was
// virtualised, so Tab, the menu key and focus return still reach it.
func (u *UI) chromeNavRow(c *ui.Context, tk kitTokens, i int, it navEntry, selected bool) {
	bg := ui.Color{}
	if selected {
		bg = tk.sel
	}
	depth := min(it.depth, maxTreeDepth-1)
	row := ui.Row(c).Key(it.key).Focusable().FocusRing(true).FillWidth().MinWidth(0).Height(chromeNavRowHeight).Gap(8).AlignItems(ui.Center).
		Padding(0, 10, 0, 10+18*float32(depth)).Radius(9).Background(bg).
		Role(ui.RoleTreeItem).Level(depth + 1).Label(it.label).Transition(navRowTransition)
	row.Unselectable()
	row.OnClick(func() {
		u.chrome.navSel = it.key
		row.Focus()
		if it.onClick != nil {
			it.onClick()
		}
	})
	if it.activate != nil && row.DoubleClicked() {
		it.activate()
	}
	row.OnShortcut(0, ui.KeyLeft, func() { u.chromeNavLeft(i) })
	row.OnShortcut(0, ui.KeyRight, func() { u.chromeNavRight(i) })
	if it.activate != nil {
		row.OnShortcut(0, ui.KeyEnter, it.activate)
	}
	u.focusReturn(row, it.key)
	if it.folder {
		row.Expanded(it.open)
	}
	if it.tip != "" {
		row.Tooltip(it.tip)
	}
	if it.menu != nil {
		row.ContextMenu(it.menu)
	}
	row.Children(func() {
		it.draw(c, tk)
	})
}

// navRowTransition is how the rows of a tree move as a folder opens and
// shuts: the rows below slide to their places, a row that appears slides
// down into place as it fades in, and one that goes folds away.
var navRowTransition = ui.ElementTransition{
	Duration: 180 * time.Millisecond,
	Position: true,
	Enter:    &ui.Motion{Y: -6, Opacity: 0},
	Exit:     &ui.Motion{Opacity: 0, Collapse: true},
}

// chevronTurn is how far a disclosure chevron is turned, in degrees: 0 while
// it points right (closed), 90 while it points down (open), easing between
// the two as the folder opens and shuts.
func chevronTurn(e ui.Element, open bool) float32 {
	target := float32(0)
	if open {
		target = 1
	}
	return 90 * e.Animate("turn", target, 150*time.Millisecond)
}

// navSection adds a section row (Worktrees, Branches, Remotes, Tags, Stashes):
// a chevron, the icon, the title and an optional count. A click or Enter
// toggles it. It returns the row's index and whether the section is open.
func (u *UI) navSection(nav *navTree, key string, icon *ui.SVG, title, count string) (int, bool) {
	open := !u.chrome.navClosed[key]
	toggle := func() {
		if u.chrome.navClosed == nil {
			u.chrome.navClosed = map[string]bool{}
		}
		u.chrome.navClosed[key] = open
	}
	i := nav.add(navEntry{
		key: key, label: title, parent: -1, folder: true, open: open,
		toggle: toggle, activate: toggle, onClick: toggle,
		draw: func(c *ui.Context, tk kitTokens) {
			chevron := ui.Icon(c, designicons.ChevronRight()).Size(12, 12).TextColor(tk.muted).Shrink(0)
			chevron.Rotate(chevronTurn(chevron, open))
			ui.Icon(c, icon).Size(16, 16).TextColor(tk.muted).Shrink(0)
			ui.Text(c, title).FontSize(13).TextColor(tk.text).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			if count != "" {
				ui.Text(c, count).FontSize(12).TextColor(tk.muted).Shrink(0)
			}
		},
	})
	return i, open
}

// navFolder adds a folder inside a tree: a chevron, the folder icon and its
// name. The chevron toggles it; a double click or Enter does too.
func (u *UI) navFolder(nav *navTree, key, name string, depth, parent int, open bool, toggle func(), menu func(m *ui.Menu)) int {
	return nav.add(navEntry{
		key: key, label: name, parent: parent, depth: depth, folder: true, open: open,
		toggle: toggle, activate: toggle, tip: name, menu: menu,
		draw: func(c *ui.Context, tk kitTokens) {
			arrow := ui.Box(c).Shrink(0).Children(func() {
				chevron := ui.Icon(c, designicons.ChevronRight()).Size(12, 12).TextColor(tk.muted)
				chevron.Rotate(chevronTurn(chevron, open))
			})
			arrow.OnClick(toggle)
			ui.Icon(c, designicons.Folder()).Size(16, 16).TextColor(tk.muted).Shrink(0)
			ui.Text(c, name).FontSize(13).TextColor(tk.text).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		},
	})
}

func (u *UI) chromeNavWorktrees(nav *navTree, q string) {
	var shown []workflow.Worktree
	for _, w := range u.trees.list {
		if matchFilter(w.Name(), q) {
			shown = append(shown, w)
		}
	}
	if len(shown) == 0 {
		return
	}
	parent, open := u.navSection(nav, "group:worktrees", designicons.Folder(), u.t("worktreesTitle"), strconv.Itoa(len(u.trees.list)))
	if !open || (len(u.trees.list) < 2 && !u.snap.Detached) {
		return
	}
	for _, w := range shown {
		u.chromeNavWorktreeRow(nav, w, parent)
	}
}

func (u *UI) chromeNavWorktreeRow(nav *navTree, w workflow.Worktree, parent int) {
	key := "worktree:" + w.Path
	access := w.Name()
	if w.Current {
		access = u.t("currentWorktreeName", "name", w.Name())
	}
	var activate func()
	if !w.Current && !w.Prunable {
		activate = func() { u.openWorktree(w) }
	}
	nav.add(navEntry{
		key: key, label: access, parent: parent, depth: 1, activate: activate,
		tip: u.worktreeTip(w),
		draw: func(c *ui.Context, tk kitTokens) {
			if w.Current {
				ui.Icon(c, designicons.Check()).Size(16, 16).TextColor(tk.accent).Shrink(0)
			}
			name := ui.Text(c, w.Name()).FontSize(13).TextColor(tk.text).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
			if w.Current {
				name.Bold()
			}
			switch {
			case w.Prunable:
				ui.Text(c, u.t("worktreePrunable")).FontSize(12).TextColor(tk.muted).SingleLine().Shrink(0)
			case w.Locked:
				ui.Text(c, u.t("worktreeLocked")).FontSize(12).TextColor(tk.muted).SingleLine().Shrink(0)
			case w.Main:
				ui.Text(c, u.t("worktreeMain")).FontSize(12).TextColor(tk.muted).SingleLine().Shrink(0)
			}
			ui.Text(c, u.worktreeState(w)).FontSize(12).TextColor(tk.muted).SingleLine().Ellipsis("…").Shrink(0)
		},
	})
}

func (u *UI) chromeNavBranches(nav *navTree, q string) {
	local := filterLocal(u.br.data.Local, q)
	if len(local) == 0 {
		return
	}
	parent, open := u.navSection(nav, "group:branches", designicons.Branch(), u.t("branchesTitle"), "")
	if !open {
		return
	}
	u.chromeNavRefRows(nav, localTree(local), "local:", 1, parent)
}

func (u *UI) chromeNavRemotes(nav *navTree, q string) {
	remote := filterRemote(u.br.data.Remote, q)
	if len(remote) == 0 {
		return
	}
	parent, open := u.navSection(nav, "group:remotes", designicons.Cloud(), u.t("chromeRemotes"), "")
	if !open {
		return
	}
	u.chromeNavRefRows(nav, remoteTree(remote), "remote:", 1, parent)
}

func (u *UI) chromeNavTags(nav *navTree, q string) {
	names := filterNames(u.chrome.navTags, q)
	if len(names) == 0 {
		return
	}
	parent, open := u.navSection(nav, "group:tags", designicons.Tag(), u.t("chromeTags"), strconv.Itoa(len(u.chrome.navTags)))
	if !open {
		return
	}
	u.chromeNavRefRows(nav, refTree(names), "tag:", 1, parent)
}

func (u *UI) chromeNavStashes(nav *navTree, q string) {
	var shown []workflow.Stash
	for _, st := range u.chrome.navStashes {
		if matchFilter(st.Message, q) {
			shown = append(shown, st)
		}
	}
	if len(shown) == 0 {
		return
	}
	parent, open := u.navSection(nav, "group:stashes", designicons.Stash(), u.t("chromeStashes"), strconv.Itoa(len(u.chrome.navStashes)))
	if !open {
		return
	}
	for _, st := range shown {
		st := st
		open := func() { u.openStash(st) }
		nav.add(navEntry{
			key: "stash:" + st.Ref, label: st.Message, parent: parent, depth: 1,
			activate: open, onClick: open,
			tip:  st.Message,
			menu: func(m *ui.Menu) { u.chromeStashItems(m, st) },
			draw: func(c *ui.Context, tk kitTokens) {
				ui.Text(c, st.Message).FontSize(13).TextColor(tk.text).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			},
		})
	}
}

// chromeNavFolderOpen reports whether a branch-tree folder is open. A toggle
// the user made wins. Otherwise a local folder opens only when it holds the
// checked-out branch, and remote folders start closed.
func (u *UI) chromeNavFolderOpen(key, path string, local bool) bool {
	if closed, ok := u.chrome.navClosed[key]; ok {
		return !closed
	}
	if !local || u.snap.Detached {
		return false
	}
	return strings.HasPrefix(u.snap.Branch, path+"/")
}

func (u *UI) chromeNavRefRows(nav *navTree, nodes []*refNode, kind string, depth, parent int) {
	for _, n := range nodes {
		if n.folder {
			fkey := kind + "folder:" + n.path
			open := u.chromeNavFolderOpen(fkey, n.path, kind == "local:")
			toggle := func() {
				if u.chrome.navClosed == nil {
					u.chrome.navClosed = map[string]bool{}
				}
				u.chrome.navClosed[fkey] = open
			}
			// The top folder of the Remotes group is a remote, by name.
			var menu func(m *ui.Menu)
			if kind == "remote:" && depth == 1 {
				remote := n.path
				menu = func(m *ui.Menu) { u.chromeRemoteItems(m, remote) }
			}
			i := u.navFolder(nav, fkey, n.label, depth, parent, open, toggle, menu)
			if open {
				u.chromeNavRefRows(nav, n.children, kind, depth+1, i)
			}
			continue
		}
		key := kind + n.path
		switch {
		case strings.HasPrefix(kind, "tag:"):
			tag := n.path
			nav.add(navEntry{
				key: key, label: n.path, parent: parent, depth: depth, tip: n.path,
				menu: func(m *ui.Menu) { u.chromeTagItems(m, tag) },
				draw: func(c *ui.Context, tk kitTokens) {
					ui.Text(c, n.label).FontSize(13).TextColor(tk.text).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
				},
			})
		case n.remote != nil:
			ref := n.remote.Name
			nav.add(navEntry{
				key: key, label: n.path, parent: parent, depth: depth, tip: n.path,
				menu: func(m *ui.Menu) { u.chromeRemoteBranchItems(m, ref) },
				draw: func(c *ui.Context, tk kitTokens) {
					ui.Text(c, n.label).FontSize(13).TextColor(tk.muted).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
				},
			})
		case n.local != nil:
			u.chromeNavBranchRow(nav, *n.local, n.label, key, depth, parent)
		default:
			nav.add(navEntry{
				key: key, label: n.path, parent: parent, depth: depth, tip: n.path,
				draw: func(c *ui.Context, tk kitTokens) {
					ui.Text(c, n.label).FontSize(13).TextColor(tk.text).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
				},
			})
		}
	}
}

func (u *UI) chromeNavBranchRow(nav *navTree, br workflow.Branch, label, key string, depth, parent int) {
	var activate func()
	if !br.Current {
		activate = func() {
			if !u.switchBlocked() {
				u.switchBranch(br.Name)
			}
		}
	}
	access := br.Name
	if br.Current {
		access = u.t("currentBranchName", "branch", br.Name)
	}
	nav.add(navEntry{
		key: key, label: access, parent: parent, depth: depth, activate: activate,
		tip:  br.Name,
		menu: func(m *ui.Menu) { u.chromeBranchItems(m, br.Name) },
		draw: func(c *ui.Context, tk kitTokens) {
			if br.Current {
				ui.Icon(c, designicons.Check()).Size(16, 16).TextColor(tk.accent).Shrink(0)
			} else {
				ui.Icon(c, designicons.Branch()).Size(16, 16).TextColor(tk.muted).Shrink(0)
			}
			name := ui.Text(c, label).FontSize(13).TextColor(tk.text).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			if br.Current {
				name.Bold()
			}
		},
	})
}

// chromeTagItems is the context menu of a tag. Checkout detaches HEAD at
// the tag. Delete removes the local tag only; a copy on a remote stays.
func (u *UI) chromeTagItems(m *ui.Menu, name string) {
	ours := u.snap.Branch
	if ours == "" {
		ours = "HEAD"
	}
	busy := u.writing()
	m.Item(name).Disabled(true)
	m.Separator()
	if m.Item(u.t("branchMenuCheckout")).Disabled(busy || u.switchBlocked()).Chosen() {
		u.chromeCheckoutCommit("refs/tags/" + name)
	}
	if m.Item(u.t("branchMenuNewFromHere")).Disabled(busy).Chosen() {
		u.openBranchForm("createFrom", name)
	}
	m.Separator()
	integrateOff := busy || u.snap.Detached
	if m.Item(u.t("branchMenuMerge", "theirs", name, "ours", ours)).Disabled(integrateOff).Chosen() {
		u.openBranchConfirm("merge", name, ours, name)
	}
	if m.Item(u.t("branchMenuRebase", "branch", ours, "onto", name)).Disabled(integrateOff).Chosen() {
		u.openBranchConfirm("rebase", name, ours, name)
	}
	if m.Item(u.t("branchMenuCompare")).Chosen() {
		u.startCompare(ours, name)
	}
	m.Separator()
	if m.Item(u.t("tagMenuCopyName")).Chosen() {
		u.copyPending = name
	}
	if len(u.sync.info.Remotes) > 0 {
		m.Separator()
	}
	u.tagRemoteItems(m, busy, "tagMenuPush", "tagMenuPushGroup", func(remote string) {
		u.pushTag(name, remote)
	})
	m.Separator()
	if m.Item(u.t("tagMenuDelete")).Disabled(busy).Chosen() {
		u.openBranchConfirm("deleteTag", name, "", "")
	}
	u.tagRemoteItems(m, busy, "tagMenuDeleteRemote", "tagMenuDeleteRemoteGroup", func(remote string) {
		u.openBranchConfirm("deleteTagRemote", name, "", remote)
	})
}

// tagRemoteItems lists act for the remotes of a tag. With one remote, it is
// a single item named by single; with several, a submenu named by group
// lists the remotes.
func (u *UI) tagRemoteItems(m *ui.Menu, busy bool, single, group string, act func(remote string)) {
	remotes := u.sync.info.Remotes
	switch len(remotes) {
	case 0:
		return
	case 1:
		remote := remotes[0].Name
		if m.Item(u.t(single, "remote", remote)).Disabled(busy).Chosen() {
			act(remote)
		}
	default:
		m.Submenu(u.t(group), func(sub *ui.Menu) {
			for _, r := range remotes {
				remote := r.Name
				if sub.Item(remote).Disabled(busy).Chosen() {
					act(remote)
				}
			}
		}).Disabled(busy)
	}
}

// pushTag pushes the local tag to remote in the background.
func (u *UI) pushTag(name, remote string) {
	u.remoteOp(opPush, remote, func(ctx context.Context, r *workflow.Repository) (workflow.Snapshot, error) {
		return r.PushTag(ctx, remote, name)
	}, func(err error) {
		u.chromeRemoteDone(err, false, "tagPushedToast", "tagPushedToast", "pushCancelled",
			"tag", name, "remote", remote)
	})
}

// deleteRemoteTag deletes the tag on remote and locally, once confirmed.
func (u *UI) deleteRemoteTag(name, remote string) {
	u.remoteOp(opPush, remote, func(ctx context.Context, r *workflow.Repository) (workflow.Snapshot, error) {
		return r.DeleteRemoteTag(ctx, remote, name)
	}, func(err error) {
		u.loadNavLists()
		u.chromeRemoteDone(err, false, "tagDeletedRemoteToast", "tagDeletedRemoteToast", "pushCancelled",
			"tag", name, "remote", remote)
	})
}

// deleteTag deletes a local tag once the user confirms it, then reads the
// tag list again.
func (u *UI) deleteTag(name string) {
	repo := u.repo
	if repo == nil || u.writing() {
		return
	}
	u.br.busy = name
	u.background(func() func() {
		err := repo.DeleteTag(context.Background(), name)
		return func() {
			if repo != u.repo {
				return
			}
			u.br.busy = ""
			u.loadNavLists()
			u.report(err)
		}
	})
}
