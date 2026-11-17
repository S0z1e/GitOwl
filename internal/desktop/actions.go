package desktop

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// writing reports a Git change GitOwl is making in the current repository:
// staging, a commit, a branch or worktree change, or a remote operation.
// One runs at a time, and every entry point to another one, whether a
// button, a menu item or a shortcut, is unavailable meanwhile.
func (u *UI) writing() bool {
	return u.wt.busy != nil || u.wt.bulk != "" || u.cm.committing || u.br.busy != "" || u.trees.busy || u.sync.op != "" || u.discard.busy || u.stv.busy
}

// switchBlocked reports work that opening another repository or worktree,
// or closing this one, would drop or interrupt: an open in flight, or any
// Git change.
func (u *UI) switchBlocked() bool { return u.opening || u.writing() }

// Action is a window command that a menu item, a shortcut and a button
// share: each asks Can and runs Do, so none gets around the others'
// protection.
type Action string

const (
	ActionOpen            Action = "open"
	ActionRecent          Action = "recent"
	ActionClose           Action = "close"
	ActionRefresh         Action = "refresh"
	ActionWorktrees       Action = "worktrees"
	ActionFetch           Action = "fetch"
	ActionPull            Action = "pull"
	ActionPush            Action = "push"
	ActionCommit          Action = "commit"
	ActionEditor          Action = "editor"
	ActionTerminal        Action = "terminal"
	ActionFinder          Action = "finder"
	ActionSettings        Action = "settings"
	ActionStash           Action = "stash"
	ActionDiscardUnstaged Action = "discardUnstaged"
)

// Actions lists the actions in menu order.
var Actions = []Action{ActionOpen, ActionRecent, ActionClose, ActionRefresh, ActionWorktrees, ActionFetch, ActionPull, ActionPush}

// Can reports whether a runs now.
func (u *UI) Can(a Action) bool {
	switch a {
	case ActionOpen:
		return !u.switchBlocked() && !u.choosing && u.opts.ChooseDirectory != nil
	case ActionRecent:
		return !u.switchBlocked()
	case ActionClose:
		return u.closeBlock() == ""
	case ActionRefresh:
		return u.repo != nil
	case ActionWorktrees:
		return u.repo != nil && !u.switchBlocked()
	case ActionFetch:
		return u.fetchBlock() == "" && u.sync.loaded && len(u.sync.info.Remotes) > 0
	case ActionPull:
		if u.chrome.shell {
			return u.repo != nil && !u.writing()
		}
		return u.pullBlock() == ""
	case ActionPush:
		if u.chrome.shell {
			return u.repo != nil && !u.writing()
		}
		return u.pushBlock() == ""
	case ActionCommit:
		return u.commitBlock() == ""
	case ActionEditor, ActionTerminal, ActionFinder:
		return u.repo != nil
	case ActionSettings:
		return true
	case ActionStash:
		return u.repo != nil && !u.snap.Clean() && !u.writing()
	case ActionDiscardUnstaged:
		return u.repo != nil && len(u.snap.Unstaged) > 0 && !u.writing()
	}
	return false
}

// repeatWindow is how soon the same action counts as the same key press,
// as when both the menu's key equivalent and the window see one shortcut.
const repeatWindow = 150 * time.Millisecond

// Do runs a when it can run now. A repeat of the same action within
// repeatWindow is the same request and is ignored.
func (u *UI) Do(a Action) {
	now := time.Now()
	if a == u.lastAction && now.Sub(u.lastActionAt) < repeatWindow {
		return
	}
	u.lastAction, u.lastActionAt = a, now
	if !u.Can(a) {
		return
	}
	switch a {
	case ActionOpen:
		u.chooseRepository()
	case ActionRecent:
		u.openPicker(pickRecent)
	case ActionClose:
		u.CloseRepository()
	case ActionRefresh:
		u.Refresh()
	case ActionWorktrees:
		u.openPicker(pickWorktree)
	case ActionFetch:
		u.fetch("")
	case ActionPull:
		if u.chrome.shell {
			u.openPullPopover()
		} else {
			u.pull()
		}
	case ActionPush:
		if u.chrome.shell {
			u.openPushPopover()
		} else if p := u.sync.info.Push; p != nil {
			u.push(*p, false)
		} else {
			u.openPublish()
		}
	case ActionCommit:
		u.commit()
	case ActionEditor:
		u.openInEditor("")
	case ActionTerminal:
		u.openTerminal()
	case ActionFinder:
		u.showInFinder("")
	case ActionSettings:
		u.OpenSettings()
	case ActionStash:
		u.chromeStash()
	case ActionDiscardUnstaged:
		u.discardUnstaged()
	}
}

// shortcuts lists each action's keys. The menu bar shows them; the window
// registers them too, for when no menu bar handles them.
var shortcuts = map[Action]struct {
	mods ui.Modifiers
	key  ui.Key
}{
	ActionOpen:      {ui.Cmd, ui.KeyO},
	ActionRecent:    {ui.Cmd | ui.Shift, ui.KeyO},
	ActionClose:     {ui.Cmd | ui.Shift, ui.KeyW},
	ActionRefresh:   {ui.Cmd, ui.KeyR},
	ActionWorktrees: {ui.Cmd | ui.Shift, ui.KeyT},
}

// Accelerators are the menu bar's names for the shortcuts.
var Accelerators = map[Action]string{
	ActionOpen:      "CmdOrCtrl+O",
	ActionRecent:    "CmdOrCtrl+Shift+O",
	ActionClose:     "CmdOrCtrl+Shift+W",
	ActionRefresh:   "CmdOrCtrl+R",
	ActionWorktrees: "CmdOrCtrl+Shift+T",
}

// ActionLabel names a for menus.
func (u *UI) ActionLabel(a Action) string {
	return u.t(map[Action]string{
		ActionOpen: "openRepository", ActionRecent: "openRecentMenu", ActionClose: "closeRepository",
		ActionRefresh: "refresh", ActionWorktrees: "switchWorktree", ActionFetch: "fetch", ActionPull: "pull", ActionPush: "push",
	}[a])
}

// registerShortcuts registers the actions' keys in the window.
func (u *UI) registerShortcuts(c *ui.Context) {
	for _, a := range Actions {
		if k, ok := shortcuts[a]; ok && c.Shortcut(k.mods, k.key) {
			u.Do(a)
		}
	}
}

// menuStateChanged tells the menu bar which actions can run, when that
// changed since it was last told.
func (u *UI) menuStateChanged() {
	if u.opts.MenuState == nil {
		return
	}
	state := make(map[Action]bool, len(Actions))
	same := u.menuState != nil
	for _, a := range Actions {
		state[a] = u.Can(a)
		same = same && u.menuState[a] == state[a]
	}
	if !same {
		u.menuState = state
		u.opts.MenuState(state)
	}
}
