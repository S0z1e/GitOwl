// Package desktop builds GitOwl's native window with MyGo. All UI
// state is owned by the UI thread; workflow calls run through Options.Go and
// return their results through Options.Post.
package desktop

import (
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/git"
	"gitowl/internal/launch"
	"gitowl/internal/prefs"
	"gitowl/internal/workflow"
)

// Options connects the UI to the rest of the application.
type Options struct {
	// Workflow is the application workflow; required.
	Workflow *workflow.Application
	// Post runs fn on the UI thread and requests a frame. Nil runs fn at
	// once, as synchronous tests need.
	Post func(fn func())
	// Go runs background work. Nil starts a goroutine.
	Go func(fn func())
	// Preferred lists the system's preferred languages, most preferred first.
	Preferred []string
	// ChooseDirectory shows a directory picker and returns the chosen path,
	// or "" when cancelled. It is called off the UI thread.
	ChooseDirectory func(title string) (string, error)
	// ChooseDirectories shows a directory picker that accepts several
	// folders, and returns the chosen paths; none when cancelled. It is
	// called off the UI thread. Nil disables adding folders to the list.
	ChooseDirectories func(title string) ([]string, error)
	// SetTitle updates the native window title. Nil ignores titles.
	SetTitle func(title string)
	// Preferences are the preferences loaded at startup.
	Preferences prefs.Preferences
	// SavePreferences persists changed preferences. Nil keeps them in memory.
	SavePreferences func(prefs.Preferences) error
	// ApplyPreferences applies changed preferences outside the window, such
	// as the app's appearance, MyGo's language and the menus.
	ApplyPreferences func(prefs.Preferences)
	// Version is the app version shown in About.
	Version string
	// Launch opens a path in an editor, Terminal or Finder. It is called off
	// the UI thread. Nil disables those actions.
	Launch func(launch.Request) error
	// OpenURL opens a URL in the system browser. Nil uses /usr/bin/open.
	OpenURL func(url string) error
	// ChooseApplication shows an application picker and returns the chosen
	// .app, or "" when cancelled. It is called off the UI thread.
	ChooseApplication func(title string) (string, error)
	// DefaultEditor finds the editor to use while the user has chosen none.
	// It is called once, by New. Nil finds none.
	DefaultEditor func() (prefs.Editor, bool)
	// ChooseSaveFile shows a save panel and returns the chosen path, or ""
	// when cancelled. It is called off the UI thread.
	ChooseSaveFile func(title, suggested string) (string, error)
	// MenuState tells the menu bar which actions can run now, whenever that
	// changes. Nil ignores it.
	MenuState func(map[Action]bool)
	// AITimeout bounds one draft request; zero uses ai.DefaultTimeout.
	AITimeout time.Duration
}

// UI is the state behind GitOwl's main window.
type UI struct {
	opts      Options
	wf        *workflow.Application
	preferred []string
	// defaultEditor is the editor found at startup, used while the user
	// has chosen none.
	defaultEditor prefs.Editor

	repo    *workflow.Repository
	snap    workflow.Snapshot
	opening bool
	// openSeq numbers open requests, so only the latest one applies.
	openSeq uint64
	// loading counts status reads in flight for the current repository.
	loading int
	// notice is the latest failure shown to the user.
	notice *notice

	// middleTab selects the middle column's view: 0 changes, 1 history, 2 search.
	middleTab int
	// diffSplit shows the diff side by side rather than unified; diffWrap
	// soft-wraps long lines rather than scrolling them. Both are the
	// user's choice, kept for the window and shared by Changes and
	// History; a narrow window forces Unified until it widens again.
	diffSplit, diffWrap bool
	wt                  workingTree
	cm                  composer
	hist                history
	br                  branchPanel
	co                  checkoutState
	trees               worktreePanel
	sync                syncState
	// toasts are confirmations to show in the next frame.
	toasts []ui.Toast
	// sessions keeps each worktree's words and position while another one
	// is open, by root.
	sessions map[string]savedSession
	// createdWorktree is a worktree GitOwl created and has not opened yet.
	createdWorktree string
	set             settingsPanel
	lay             layout
	prefs           prefs.Preferences
	ai              aiState
	recent          recentState
	land            landingState
	chrome          chromeState
	pick            picker
	discard         discardState
	ident           identityPanel
	rev             revertUIState
	cmp             compareState
	stv             stashView
	// copyPending is text to write to the clipboard on the next frame,
	// after a menu item that has no drawing context.
	copyPending string
	// choosing is set while the folder picker is open.
	choosing bool
	// The last action run, to ignore one key press seen twice, and what
	// the menu bar was last told.
	lastAction   Action
	lastActionAt time.Time
	menuState    map[Action]bool
	form         aiForm
	// focusKey names the control with the focus, returnFocus the one to
	// focus again when the open dialog closes (focus.go).
	focusKey, returnFocus string
	wasDialogOpen         bool

	leftWidth, middleWidth float32
}

// notice is a failure explained in the interface.
type notice struct {
	code   workflow.Code
	detail string
}

// New returns the UI for one main window.
func New(opts Options) *UI {
	if opts.Post == nil {
		opts.Post = func(fn func()) { fn() }
	}
	if opts.Go == nil {
		opts.Go = func(fn func()) { go fn() }
	}
	p := opts.Preferences
	if p.Equal(prefs.Preferences{}) {
		p = prefs.Default()
	}
	p.Normalize()
	u := &UI{opts: opts, wf: opts.Workflow, preferred: opts.Preferred, prefs: p,
		form: aiForm{baseURL: p.AI.BaseURL, model: p.AI.Model, prompt: p.AI.Prompt}, leftWidth: 240, middleWidth: 360, wt: workingTree{cursor: -1}, hist: newHistory(), br: newBranchPanel(), co: newCheckoutState(), chrome: chromeState{shell: true}}
	if opts.DefaultEditor != nil {
		if e, ok := opts.DefaultEditor(); ok {
			u.defaultEditor = e
		}
	}
	return u
}

// SetPreferredLanguages updates the system languages, for "follow system".
func (u *UI) SetPreferredLanguages(langs []string) { u.preferred = langs }

// Title returns the window title.
func (u *UI) Title() string {
	if u.repo != nil {
		return u.repo.Name() + " — GitOwl"
	}
	return "GitOwl"
}

func (u *UI) titleChanged() {
	if u.opts.SetTitle != nil {
		u.opts.SetTitle(u.Title())
	}
}

// background runs work off the UI thread and applies the function it returns
// on the UI thread.
func (u *UI) background(work func() func()) {
	u.opts.Go(func() {
		apply := work()
		if apply != nil {
			u.opts.Post(apply)
		}
	})
}

func (u *UI) report(err error) {
	if err == nil {
		u.notice = nil
		return
	}
	n := &notice{code: workflow.CodeOf(err)}
	if e, ok := err.(*workflow.Error); ok {
		n.detail = git.Redact(e.Detail)
	} else {
		n.detail = git.Redact(err.Error())
	}
	u.notice = n
}

// T formats a message in the interface language, for text outside the
// window such as menus.
func (u *UI) T(key string) string { return u.t(key) }

func (u *UI) chooseRepository() {
	if u.switchBlocked() || u.choosing || u.opts.ChooseDirectory == nil {
		return
	}
	u.choosing = true
	title := u.t("openRepositoryTitle")
	u.opts.Go(func() {
		path, err := u.opts.ChooseDirectory(title)
		u.opts.Post(func() {
			u.choosing = false
			if err != nil {
				u.report(err)
				return
			}
			if path != "" {
				u.OpenPath(path)
			}
		})
	})
}
