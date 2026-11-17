// Command gitowl is GitOwl's macOS desktop application.
//
//	gitowl [repository]
//
// opens the repository at the optional path at startup.
package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"gitowl/internal/ai"
	"gitowl/internal/desktop"
	"gitowl/internal/git"
	"gitowl/internal/launch"
	_ "gitowl/internal/locales"
	"gitowl/internal/prefs"
	"gitowl/internal/secrets"
	"gitowl/internal/workflow"
)

func main() {
	mygo.App.SetName("GitOwl")
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	store := prefs.Store{}
	if path, err := prefs.DefaultPath(); err == nil {
		store.Path = path
	} else {
		log.Printf("preferences: %v", err)
	}
	p, err := store.Load()
	if err != nil {
		log.Printf("preferences: %v", err)
	}
	// Start in the saved appearance rather than switching after launch.
	mygo.Theme.SetSource(mygo.ThemeSource(p.Appearance))
	applyLocale(p)

	runner, err := git.Find()
	if err != nil {
		log.Printf("git: %v", err)
	}
	wf := workflow.New(runner)
	wf.SetAI(workflow.AIConfig{Client: &ai.Client{}, Secrets: secrets.Keychain{}})
	var win *mygo.Window
	var u *desktop.UI
	u = desktop.New(desktop.Options{
		Workflow:    wf,
		Preferences: p,
		Version:     appVersion(),
		SavePreferences: func(p prefs.Preferences) error {
			if store.Path == "" {
				return nil
			}
			return store.Save(p)
		},
		ApplyPreferences: func(p prefs.Preferences) {
			mygo.Theme.SetSource(mygo.ThemeSource(p.Appearance))
			applyLocale(p)
			installMenus(u)
		},
		Post: func(fn func()) {
			if win != nil {
				win.Update(fn)
			}
		},
		ChooseDirectory: func(title string) (string, error) {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: title, Directory: true})
			if err != nil || len(paths) == 0 {
				return "", err
			}
			return paths[0], nil
		},
		ChooseDirectories: func(title string) ([]string, error) {
			return mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: title, Directory: true, Multiple: true})
		},
		ChooseApplication: func(title string) (string, error) {
			// No type filter: a filter on the .app type greyed out every app in
			// the panel, so none could be chosen. chooseEditor checks the name.
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: title, DefaultPath: "/Applications"})
			if err != nil || len(paths) == 0 {
				return "", err
			}
			return paths[0], nil
		},
		DefaultEditor: func() (prefs.Editor, bool) {
			dirs := []string{"/Applications"}
			if home, err := os.UserHomeDir(); err == nil {
				dirs = append(dirs, filepath.Join(home, "Applications"))
			}
			return launch.FindEditor(dirs...)
		},
		ChooseSaveFile: func(title, suggested string) (string, error) {
			return mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: win, Title: title, DefaultPath: suggested})
		},
		Launch: func(r launch.Request) error { return launch.Run(r, mygo.Shell) },
		MenuState: func(state map[desktop.Action]bool) {
			for a, on := range state {
				if it := menuItems[a]; it != nil {
					it.SetEnabled(on)
				}
			}
			menuEnabled = state
		},
		SetTitle: func(title string) {
			if win != nil {
				win.SetTitle(title)
			}
		},
	})
	mygo.App.WhenReady(func() {
		u.SetPreferredLanguages(mygo.App.PreferredLanguages())
		mygo.App.OnLocaleChanged(func(string) {
			u.SetPreferredLanguages(mygo.App.PreferredLanguages())
			installMenus(u)
			if win != nil {
				win.Invalidate()
			}
		})
		installMenus(u)
		win = mygo.NewWindow(mygo.WindowOptions{
			Title: u.Title(), Width: 1280, Height: 800, MinWidth: 560, MinHeight: 480,
			StateKey: "main", Hidden: true, TitleBarStyle: mygo.TitleBarHiddenInset,
			TrafficLightPosition: &mygo.Point{X: 16, Y: 16},
			Content: ui.View(func(c *ui.Context) {
				c.SetTheme(desktop.KitTheme(c))
				u.Chrome(c)
			}),
		})
		post = func(fn func()) { win.Update(fn) }
		win.OnClosed(mygo.App.Quit)
		// Returning to the window shows changes made outside GitOwl.
		win.OnFocus(u.Refresh)
		win.Show()
		win.Focus()
		if path := startPath(os.Args[1:]); path != "" {
			u.OpenPath(path)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// applyLocale makes MyGo's own strings follow the interface language;
// "system" follows the user's languages.
func applyLocale(p prefs.Preferences) {
	tag := p.Language
	if tag == prefs.System {
		tag = ""
	}
	mygo.App.SetLocale(tag)
}

// menuItems are the menu bar's items of the window's actions, and
// menuEnabled what the window last said of them.
var (
	menuItems   = map[desktop.Action]*mygo.MenuItem{}
	menuEnabled map[desktop.Action]bool
	// post runs a function on the window's UI thread.
	post func(func())
)

func installMenus(u *desktop.UI) {
	about := &mygo.MenuItem{ID: "about", Label: u.T("menuAbout"),
		Click: func(*mygo.MenuItem, *mygo.Window) {
			mygo.App.ShowAboutPanel(mygo.AboutPanelOptions{
				ApplicationName: "GitOwl", ApplicationVersion: appVersion(), Copyright: "© 2026 S0z1e",
			})
		}}
	settings := &mygo.MenuItem{ID: "settings", Label: u.T("settings") + "…", Accelerator: "CmdOrCtrl+,",
		Click: func(*mygo.MenuItem, *mygo.Window) { u.OpenSettings() }}
	quit := &mygo.MenuItem{ID: "quit", Role: mygo.RoleQuit, Label: u.T("quit")}
	item := func(a desktop.Action) *mygo.MenuItem {
		it := &mygo.MenuItem{ID: string(a), Label: u.ActionLabel(a), Accelerator: desktop.Accelerators[a],
			Disabled: menuEnabled != nil && !menuEnabled[a],
			Click: func(*mygo.MenuItem, *mygo.Window) {
				// The window decides on its own thread whether it can run.
				if post != nil {
					post(func() { u.Do(a) })
				}
			}}
		menuItems[a] = it
		return it
	}
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{about, mygo.Separator(), settings, mygo.Separator(), quit}},
		{Label: u.T("menuFile"), Submenu: []*mygo.MenuItem{
			item(desktop.ActionOpen), item(desktop.ActionRecent), mygo.Separator(),
			item(desktop.ActionClose), {Role: mygo.RoleClose},
		}},
		{Role: mygo.RoleEditMenu},
		{Label: u.T("menuView"), Submenu: []*mygo.MenuItem{item(desktop.ActionRefresh)}},
		{Label: u.T("menuRepository"), Submenu: []*mygo.MenuItem{
			item(desktop.ActionWorktrees), mygo.Separator(),
			item(desktop.ActionFetch), item(desktop.ActionPull), item(desktop.ActionPush),
		}},
		{Role: mygo.RoleWindowMenu},
	}))
}

// devVersion is shown when the app runs outside a packaged bundle, where
// MyGo reports no version. Keep it equal to "version" in mygo.json.
const devVersion = "1.0.0"

// appVersion is the packaged app's version, or devVersion when there is none.
func appVersion() string {
	if v := mygo.App.Version(); v != "" {
		return v
	}
	return devVersion
}

// startPath returns the first argument that is not a flag, such as the
// -psn_ argument macOS adds when Finder starts an app.
func startPath(args []string) string {
	for _, a := range args {
		if a != "" && !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}
