package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/git"
	"gitowl/internal/launch"
	"gitowl/internal/prefs"
	"gitowl/internal/workflow"
)

// Failures of opening something in another app, as notice codes.
const (
	codeLaunchFailed workflow.Code = "LaunchFailed"
	codeFileMissing  workflow.Code = "FileMissing"
	codeNoEditor     workflow.Code = "NoEditor"
)

// worktreePath returns the absolute path of rel in the open worktree, ""
// when there is none or rel would leave it.
func (u *UI) worktreePath(rel string) string {
	if u.repo == nil {
		return ""
	}
	root := u.repo.Root()
	if rel == "" {
		return root
	}
	p := filepath.Join(root, filepath.FromSlash(rel))
	if !strings.HasPrefix(p, root+string(filepath.Separator)) {
		return ""
	}
	return p
}

// launch opens r in the background. A failure is explained where the user
// is; nothing else changes. Returning to the window reads the repository
// again, which shows edits made meanwhile.
func (u *UI) launch(r launch.Request) {
	if u.opts.Launch == nil || r.Path == "" {
		return
	}
	run := u.opts.Launch
	u.background(func() func() {
		err := run(r)
		return func() {
			switch {
			case err == nil:
			case errors.Is(err, os.ErrNotExist):
				u.notice = &notice{code: codeFileMissing, detail: r.Path}
			default:
				u.notice = &notice{code: codeLaunchFailed, detail: err.Error()}
			}
		}
	})
}

// editor is the app files and worktrees open in: the chosen one, or while
// none is chosen the editor found on this Mac. It is empty when neither
// exists.
func (u *UI) editor() prefs.Editor {
	if u.prefs.Editor.Set() {
		return u.prefs.Editor
	}
	return u.defaultEditor
}

// editorName names the editor in use, or "" when there is none.
func (u *UI) editorName() string {
	return u.editor().Name
}

// openInEditor opens the file at rel, or the worktree for "", in the
// editor. Without one a file opens in its default app, and the worktree
// asks for an editor first.
func (u *UI) openInEditor(rel string) {
	e := u.editor()
	if rel == "" && !e.Set() {
		u.notice = &notice{code: codeNoEditor}
		u.openGeneralSettings()
		return
	}
	u.launch(launch.Request{Tool: launch.Editor, App: e.Path, Path: u.worktreePath(rel)})
}

func (u *UI) openTerminal() {
	u.launch(launch.Request{Tool: launch.Terminal, Path: u.worktreePath("")})
}

func (u *UI) showInFinder(rel string) {
	u.launch(launch.Request{Tool: launch.Finder, Path: u.worktreePath(rel)})
}

func (u *UI) openGeneralSettings() {
	u.set.state.Category = "general"
	u.set.open = true
}

// editorLabel is the menu item that opens a file or the worktree in the
// editor.
func (u *UI) editorLabel(file bool) string {
	switch name := u.editorName(); {
	case name != "":
		return u.t("openInEditorNamed", "editor", name)
	case file:
		return u.t("openWithDefaultApp")
	}
	return u.t("openInEditor")
}

// fileItems adds the items that open the working tree's file at rel.
// missing disables them for a file deleted from the working tree; the
// worktree's folder can still be shown.
func (u *UI) fileItems(m *ui.Menu, rel string, missing bool) {
	if m.Item(u.editorLabel(true)).Disabled(missing).Chosen() {
		u.openInEditor(rel)
	}
	if missing {
		if m.Item(u.t("showWorktreeInFinder")).Chosen() {
			u.showInFinder("")
		}
		return
	}
	if m.Item(u.t("showInFinder")).Chosen() {
		u.showInFinder(rel)
	}
}

// missingInWorktree reports a change whose file is not in the working tree.
func missingInWorktree(ch workflow.Change) bool {
	return ch.Area == workflow.Unstaged && ch.Kind == git.Deleted || ch.Submodule
}

// chooseEditor asks for an application with the system's picker.
func (u *UI) chooseEditor() {
	choose := u.opts.ChooseApplication
	if choose == nil {
		return
	}
	title := u.t("chooseEditorTitle")
	u.opts.Go(func() {
		path, err := choose(title)
		u.opts.Post(func() {
			if err != nil {
				u.set.saveErr = err.Error()
				return
			}
			if path == "" {
				return
			}
			if !strings.HasSuffix(path, ".app") {
				u.toast("error", u.t("chooseEditorNotApp"))
				return
			}
			p := u.prefs
			p.Editor = prefs.Editor{Path: path, Name: strings.TrimSuffix(filepath.Base(path), ".app")}
			u.setPreferences(p)
		})
	})
}
