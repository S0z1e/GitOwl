package desktop

import (
	"gitowl/internal/desktop/designicons"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/prefs"
	"gitowl/internal/workflow"
)

// Layout numbers measured from the Empty state design board. The board
// lists 91 repositories and "Show 83 more", so eight rows are visible by
// default.
const (
	chromeRailWidth    = 200
	chromeTrafficInset = 58
	chromeNavWidth     = 228
	// chromeNavMinWidth and chromeNavMaxWidth bound the navigation column
	// as its grip drags it wider or narrower.
	chromeNavMinWidth = 160
	chromeNavMaxWidth = 480
	// chromeNavRowHeight is the navigation tree's row height, the design's 30.
	chromeNavRowHeight   = 30
	chromeToolbarHeight  = 52
	chromeBranchHeight   = 28
	chromeRailPadX       = 10
	chromeRailPadBottom  = 10
	chromeRowHeight      = 32
	chromeRepoVisible    = 8
	chromeWelcomeOwl     = 60
	chromeOpenHeight     = 44
	chromeWelcomeMaxText = 460
)

// chromeState is Welcome-column UI that is not a preference: the search
// query, whether Show more has expanded the repository list, and whether
// the window last had keyboard focus so a return can reread the repository.
type chromeState struct {
	query     string
	showAll   bool
	focus     ui.Handle
	winFocus  bool
	seenFocus bool
	// drawerP is the navigation drawer's progress last frame, from 0 shut to
	// 1 open. It decides whether the drawer's scrim is built (chromeDrawer).
	drawerP float32
	// navWidth is the navigation column's width as its grip set it; 0 is
	// chromeNavWidth.
	navWidth float32
	// shell is the custom product window. New() turns it on so Pull/Push,
	// drafts, amend and All commits never fall back to the removed Onevra
	// header.
	shell     bool
	navFilter string
	navSel    string
	// navItems is this frame's flattened tree; navIdx the selected row in it,
	// -1 for none; navList is the virtual list that draws it.
	navItems   []navEntry
	navIdx     int
	navList    ui.ListState
	navClosed  map[string]bool
	navTags    []string
	navTagRefs []workflow.Tag // tag names with the commits they point to
	navStashes []workflow.Stash
	navSeq     uint64
	pal        paletteState
	// composerHidden collapses the commit panel to a bar at the bottom of
	// Current changes. The message and draft are kept.
	composerHidden bool
	// composerH is the commit panel's height when it was last drawn, which
	// the slide between the bar and the panel needs.
	composerH float32
	// fetchMenuOpen opens the Fetch menu when more than one remote exists.
	fetchMenuOpen bool
}

// chromeWindow lays out the custom-drawn product shell. window() is this
// function: there is no second Onevra header wrapping UI.View.
func chromeWindow(u *UI) func(*ui.Context) {
	u.chrome.shell = true
	return func(c *ui.Context) {
		c.SetTheme(KitTheme(c))
		u.Chrome(c)
	}
}

// Chrome draws the custom frame: a repository rail, a toolbar that insets
// for the system traffic lights, and either Welcome or the open-repository
// shell (toolbar, navigation, review). GitOwl does not draw window dots.
// compactWidth / narrowWidth / railHidden / drawerOpen fold the shell. A wide
// window can hide the repository rail; a compact one shows the navigation as
// a drawer instead.
func (u *UI) Chrome(c *ui.Context) {
	w, _ := c.Size()
	u.lay.compact, u.lay.narrow = w < compactWidth, w < narrowWidth
	if !u.lay.compact {
		u.lay.drawerOpen = false
	}
	u.chromeFocusRefresh(c)
	u.chromeShortcuts(c)
	defer u.flushCopy(c)
	defer u.noteDialogs()
	defer u.menuStateChanged()
	defer u.showToasts(c)
	defer u.pickerDialog(c)
	defer u.discardDialog(c)
	tk := kitDesignTokens(c.Theme().Dark)
	root := ui.Box(c).Fill().Background(tk.bg).Clip()
	if root.FileDragOver() && !u.switchBlocked() {
		root.Background(tk.accent.Alpha(.08)).Border(1, tk.accent.Alpha(.5))
	}
	if files := root.DroppedFiles(); len(files) > 0 {
		u.onDropped(files)
	}
	root.Children(func() {
		ui.Row(c).Fill().Children(func() {
			u.chromeRailSlot(c, tk)
			ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
				u.chromeToolbar(c, tk)
				// A wide window fills the height under the toolbar. A narrow one
				// keeps the content's own height, so the commit box and the
				// file list share the room they had.
				row := ui.Row(c).Grow(1).FillWidth().MinHeight(0)
				if !u.lay.narrow {
					row.AlignItems(ui.Stretch)
				}
				row.Children(func() {
					if u.chromeNavInline() {
						u.chromeNav(c, tk)
					}
					u.chromeNavGrip(c, tk)
					main := ui.Column(c).Key("chrome-main").Grow(1).FillWidth().MinHeight(0)
					if !u.lay.narrow {
						main.FillHeight()
					}
					main.Children(func() {
						if u.repo != nil && !u.chromeNavShowing() {
							u.chromeNotice(c, tk)
						}
						if u.repo == nil {
							u.chromeWelcome(c, tk)
							return
						}
						u.chromeReview(c)
					})
				})
			})
		})
		if u.lay.compact && u.repo != nil {
			u.chromeDrawer(c, tk)
		}
	})
	u.settingsDialog(c)
	u.chromeSyncDialogs(c)
	u.chromeIdentityDialog(c)
	u.chromeBranchDialogs(c)
	u.paletteDialog(c)
	u.requestDialog(c)
}

// chromeShortcuts registers the seven command-palette shortcuts and ⌘K.
// Close repository, ⌘O, ⌘F and printed popover keys are not bound.
func (u *UI) chromeShortcuts(c *ui.Context) {
	if c.Shortcut(ui.Cmd, ui.KeyK) {
		u.togglePalette()
		return
	}
	if u.chrome.pal.open {
		if c.Shortcut(0, ui.KeyEnter) {
			u.runPaletteRow()
		}
		if c.Shortcut(0, ui.KeyEscape) {
			u.closePalette()
		}
		if c.Shortcut(0, ui.KeyUp) {
			u.movePalette(-1)
		}
		if c.Shortcut(0, ui.KeyDown) {
			u.movePalette(1)
		}
		return
	}
	for _, a := range paletteActions {
		if k, ok := paletteKeys[a]; ok && c.Shortcut(k.mods, k.key) {
			u.Do(a)
		}
	}
}

func (u *UI) chromeToolbar(c *ui.Context, tk kitTokens) {
	title := "GitOwl"
	if u.repo != nil {
		title = u.repo.Name()
	}
	ui.Row(c).FillWidth().Height(chromeToolbarHeight).Padding(0, 14).Gap(8).
		AlignItems(ui.Center).Background(tk.bg).Border(1, tk.border).DragWindow().Children(func() {
		if u.repo != nil && !u.chromeRailInline() {
			// The rail is hidden: its toggle moves here, clear of the
			// traffic lights.
			ui.Box(c).Width(chromeTrafficInset).Shrink(0)
			u.chromeSidebarToggle(c, tk)
		}
		ui.Text(c, title).Bold().FontSize(14).TextColor(tk.text).SingleLine().Ellipsis("…").Shrink(0)
		if u.repo == nil {
			return
		}
		u.chromeBranch(c, tk)
		if u.snap.Unborn() {
			ui.Row(c).Height(chromeBranchHeight).Padding(0, 10).Radius(999).
				Background(tk.fill).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Text(c, u.t("noCommitsYet")).FontSize(12).TextColor(tk.muted).SingleLine()
			})
		}
		ui.Text(c, homeShort(u.repo.Root())).FontSize(12).TextColor(tk.muted).Font("Menlo").
			SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		u.chromeToolbarActions(c, tk)
	})
}

func (u *UI) chromeBranch(c *ui.Context, tk kitTokens) {
	text, bg, bd, fg := u.snap.Branch, tk.accent.Alpha(.12), tk.accent.Alpha(.12), tk.acx
	if u.snap.Detached {
		text, bg, bd, fg = u.t("worktreeDetached", "hash", shortHash(u.snap.Head)), tk.warn.Alpha(.12), tk.warn.Alpha(.3), tk.warn
	}
	if text == "" {
		return
	}
	ui.Row(c).Height(chromeBranchHeight).Padding(0, 12).Radius(999).
		Background(bg).Border(1, bd).AlignItems(ui.Center).Shrink(1).MinWidth(0).MaxWidth(220).
		Children(func() {
			ui.Text(c, text).FontSize(13).FontWeight(600).TextColor(fg).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
		})
}

func (u *UI) chromeToolbarActions(c *ui.Context, tk kitTokens) {
	iconOnly := u.lay.narrow
	chromeSegGroup(c, tk, func() {
		u.chromeFetchButton(c, tk, iconOnly, true)
		chromeSegDivider(c, tk)
		u.chromePullButton(c, tk, iconOnly, true)
		chromeSegDivider(c, tk)
		u.chromePushButton(c, tk, iconOnly, true)
		chromeSegDivider(c, tk)
		// Only nothing to stash dims Stash. A write in flight leaves it lit;
		// Do ignores the press meanwhile.
		stashOff := u.repo == nil || u.snap.Clean()
		// Disabled controls hide tooltips, so Stash stays live and dims; its
		// tooltip says why it cannot run.
		stash := chromeToolButtonSeg(c, designicons.Stash(), u.t("stash"), tk, iconOnly, false, "", true, false)
		if stashOff {
			if u.snap.Clean() {
				stash.Tooltip(u.t("errorNothingToStash")).Description(u.t("errorNothingToStash"))
			}
			stash.Opacity(.45)
		}
		u.chromeMarkFocus(stash)
		if stash.Clicked() {
			u.Do(ActionStash)
		}
	})
	ui.Box(c).Width(8).Shrink(0)
	chromeSegGroup(c, tk, func() {
		ed := chromeToolButtonSeg(c, designicons.Editor(), u.t("kitEditor"), tk, iconOnly, false, "", true, false)
		u.chromeMarkFocus(ed)
		if ed.Clicked() {
			u.Do(ActionEditor)
		}
		chromeSegDivider(c, tk)
		term := chromeToolButtonSeg(c, designicons.Terminal(), u.t("kitTerminal"), tk, iconOnly, false, "", true, false)
		u.chromeMarkFocus(term)
		if term.Clicked() {
			u.Do(ActionTerminal)
		}
		chromeSegDivider(c, tk)
		finder := chromeToolButtonSeg(c, designicons.Finder(), u.t("kitFinder"), tk, iconOnly, false, "", true, false)
		u.chromeMarkFocus(finder)
		if finder.Clicked() {
			u.Do(ActionFinder)
		}
	})
}

// chromeSegGroup draws toolbar buttons that share one border, as the design's
// Fetch/Pull/Push/Stash and Editor/Terminal/Finder groups do.
func chromeSegGroup(c *ui.Context, tk kitTokens, body func()) {
	ui.Row(c).Height(kitToolbarH).Radius(12).Border(1, tk.bd2).Background(tk.surf).
		AlignItems(ui.Center).Shrink(0).Clip().Children(body)
}

// chromeSegDivider is the 1 DIP rule between two buttons of a group.
func chromeSegDivider(c *ui.Context, tk kitTokens) {
	ui.Box(c).Width(1).Height(kitToolbarH).Background(tk.bd2).Shrink(0)
}

func chromeToolButton(c *ui.Context, icon *ui.SVG, label string, tk kitTokens, iconOnly, disabled bool, count string) ui.Element {
	return chromeToolButtonSeg(c, icon, label, tk, iconOnly, disabled, count, false, false)
}

// chromeToolButtonSeg draws one toolbar button. In a group (seg) it has no
// border of its own. chevron adds the menu chevron that Pull and Push show.
func chromeToolButtonSeg(c *ui.Context, icon *ui.SVG, label string, tk kitTokens, iconOnly, disabled bool, count string, seg, chevron bool) ui.Element {
	if !seg && iconOnly {
		return kitIconButton(c, icon, label, tk, disabled)
	}
	color := tk.text
	if disabled {
		color = tk.dis
	}
	b := ui.ButtonBase(c).Label(label).Disabled(disabled).AlignItems(ui.Center)
	switch {
	case seg && iconOnly:
		b.Size(kitToolbarH, kitToolbarH)
	case seg:
		b.Height(kitToolbarH).Padding(0, 12).Gap(6)
	default:
		b.Height(kitToolbarH).Padding(0, 10).Gap(6).Radius(12).Background(tk.surf).Border(1, tk.bd2)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(color)
		if iconOnly {
			return
		}
		ui.Text(c, label).FontSize(13).FontWeight(500).SingleLine()
		if count != "" {
			ui.Text(c, count).FontSize(11).TextColor(tk.muted).Font("Menlo").SingleLine()
		}
		if chevron {
			ui.Icon(c, designicons.ChevronDown()).Size(12, 12).TextColor(tk.muted)
		}
	})
	return b
}

func (u *UI) chromeRail(c *ui.Context, tk kitTokens) {
	ui.Column(c).Width(chromeRailWidth).FillHeight().Shrink(0).
		Background(tk.rail).Border(1, tk.border).
		Padding(0, chromeRailPadX, chromeRailPadBottom).Children(func() {
		// System traffic lights sit here. GitOwl leaves the inset and
		// does not draw the artboard's red, yellow and green dots.
		ui.Row(c).FillWidth().Height(chromeToolbarHeight).Padding(0, 0, 0, 6).
			AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Box(c).Grow(1)
			if u.repo != nil {
				u.chromeSidebarToggle(c, tk)
			}
		})
		u.chromeSearch(c, tk)
		q := strings.TrimSpace(u.chrome.query)
		pinned := u.chromeFilter(u.prefs.Pinned, q)
		repos := u.chromeFilter(u.chromeUnpinned(), q)
		shown, hidden := u.chromeShownRepos(repos, q)
		ui.Column(c).FillWidth().Grow(1).MinHeight(0).Clip().Children(func() {
			u.chromeGroups(c, tk, q, pinned, shown, repos)
		})
		if hidden > 0 {
			label := u.t("chromeShowMore", "count", hidden)
			more := ui.ButtonBase(c).Key("chrome-show-more").FillWidth().Height(26).Padding(6, 10).Justify(ui.Start).Label(label)
			more.Children(func() {
				ui.Text(c, label).FontSize(12).TextColor(tk.acx)
			})
			if more.Clicked() {
				u.chrome.showAll = true
			}
		}
		ui.Box(c).Padding(0, 2).Children(func() {
			gear := kitIconButton(c, designicons.Settings(), u.t("settings"), tk, false)
			u.chromeMarkFocus(gear)
			u.focusReturn(gear, "settings")
			if gear.Clicked() {
				u.Do(ActionSettings)
			}
		})
	})
}

func (u *UI) chromeSearch(c *ui.Context, tk kitTokens) {
	ui.Row(c).FillWidth().Height(kitFieldH).Radius(10).Gap(8).
		Background(tk.surf).Border(1, tk.bd2).Padding(0, 10).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, designicons.Search()).Size(14, 14).TextColor(tk.muted)
		field := ui.TextInputBase(c, &u.chrome.query).Grow(1).MinWidth(0).FillWidth().
			Label(u.t("chromeSearch")).Placeholder(u.t("chromeSearch"))
		u.chromeMarkFocus(field)
		ui.Text(c, "⌘F").Font("monospace").FontSize(11).TextColor(tk.muted)
	})
}

func (u *UI) chromeGroups(c *ui.Context, tk kitTokens, q string, pinned, shown, repos []string) {
	if q == "" || len(pinned) > 0 {
		u.chromeGroup(c, tk, u.t("chromePinned"), pinned, true, len(pinned))
	}
	if q == "" || len(repos) > 0 {
		u.chromeGroup(c, tk, u.t("chromeRepositories"), shown, false, len(repos))
	}
}

func (u *UI) chromeGroup(c *ui.Context, tk kitTokens, title string, roots []string, pinned bool, total int) {
	ui.Row(c).FillWidth().Padding(14, 8, 6).Justify(ui.SpaceBetween).Children(func() {
		ui.Text(c, title).FontSize(11).FontWeight(600).LetterSpacing(.05).TextColor(tk.muted)
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, strconv.Itoa(total)).FontSize(11).TextColor(tk.muted)
			if !pinned {
				u.chromeAddRepositories(c, tk)
			}
		})
	})
	for _, root := range roots {
		u.chromeRow(c, tk, root, pinned)
	}
}

// chromeAddRepositories is the + in the Repositories heading. Its press opens
// a folder picker that takes several folders at once.
func (u *UI) chromeAddRepositories(c *ui.Context, tk kitTokens) {
	label := u.t("chromeAddRepository")
	add := ui.ButtonBase(c).Size(20, 20).Radius(6).Label(label).Shrink(0).
		Disabled(!u.canAddRepositories()).Tooltip(label)
	add.Children(func() {
		ui.Icon(c, designicons.Plus()).Size(12, 12).TextColor(tk.muted)
	})
	if add.Clicked() {
		u.chooseRepositories()
	}
}

func (u *UI) chromeRow(c *ui.Context, tk kitTokens, root string, pinned bool) {
	name := filepath.Base(root)
	current := u.repo != nil && u.repo.Root() == root
	line := ui.Row(c).FillWidth().Height(chromeRowHeight).Gap(8).Padding(0, 6, 0, 10).
		Radius(9).AlignItems(ui.Center).Background(chromeRowFill(tk, current))
	line.ContextMenu(func(m *ui.Menu) { u.chromeRowItems(m, root, pinned) })
	line.Children(func() {
		u.chromeAvatar(c, name)
		// ButtonBase centers its children, so Justify(Start) keeps the
		// name against the avatar. Long names still ellipsize.
		row := ui.ButtonBase(c).Grow(1).MinWidth(0).FillHeight().Justify(ui.Start).AlignItems(ui.Center).Label(name)
		// A row is not dimmed while a Git change writes: each stage would
		// dim and restore it, which flashes. openRecent ignores the click
		// during a write instead.
		row.Children(func() {
			ui.Text(c, name).TextColor(tk.text).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		})
		u.chromeMarkFocus(row)
		if row.Clicked() {
			u.openRecent(root)
		}
		pinLabel := u.t("chromePin", "name", name)
		// Pinned rows show a filled accent pin; unpinned rows show the
		// outline pin in muted text, as the Empty state board does.
		icon, iconColor := designicons.Pin(), tk.muted
		if pinned {
			pinLabel = u.t("chromeUnpin", "name", name)
			icon, iconColor = designicons.PinFilled(), tk.acx
		}
		pin := ui.ButtonBase(c).Size(22, 22).Radius(6).Label(pinLabel)
		pin.Children(func() {
			ui.Icon(c, icon).Size(14, 14).TextColor(iconColor)
		})
		if pin.Clicked() {
			if pinned {
				u.unpinRepo(root)
			} else {
				u.pinRepo(root)
			}
		}
	})
}

// chromeRowItems is the context menu of a repository row. Pin and Unpin change
// only the pinned list. Remove takes the repository off both lists; the
// folder stays on disk and comes back when it is opened again.
func (u *UI) chromeRowItems(m *ui.Menu, root string, pinned bool) {
	name := filepath.Base(root)
	if pinned {
		if m.Item(u.t("chromeUnpin", "name", name)).Chosen() {
			u.unpinRepo(root)
		}
	} else if m.Item(u.t("chromePin", "name", name)).Chosen() {
		u.pinRepo(root)
	}
	m.Separator()
	if m.Item(u.t("chromeRemoveRepo", "name", name)).Chosen() {
		u.forget(root)
	}
}

func chromeRowFill(tk kitTokens, current bool) ui.Color {
	if current {
		return tk.sel
	}
	return ui.Color{}
}

func (u *UI) chromeAvatar(c *ui.Context, name string) {
	letter := "?"
	if r, _ := utf8.DecodeRuneInString(name); r != utf8.RuneError && unicode.IsLetter(r) {
		letter = strings.ToUpper(string(unicode.ToUpper(r)))
	} else if name != "" {
		letter = strings.ToUpper(name[:1])
	}
	palette := []ui.Color{
		ui.Hex("#2563EB"), ui.Hex("#1F9D62"), ui.Hex("#6B5BD6"),
		ui.Hex("#DB2777"), ui.Hex("#7C3AED"), ui.Hex("#D97706"),
	}
	h := 0
	for i := 0; i < len(name); i++ {
		h = h*31 + int(name[i])
	}
	if h < 0 {
		h = -h
	}
	bg := palette[h%len(palette)]
	ui.Box(c).Size(20, 20).Radius(6).Background(bg).Center().Shrink(0).Children(func() {
		ui.Text(c, letter).FontSize(11).FontWeight(700).TextColor(ui.Hex("#FFFFFF"))
	})
}

func (u *UI) chromeWelcome(c *ui.Context, tk kitTokens) {
	ui.Column(c).Fill().Center().Gap(8).Padding(0, 0, 40, 0).Children(func() {
		ui.Image(c, OwlImage()).Size(chromeWelcomeOwl, chromeWelcomeOwl).Label("GitOwl brand")
		ui.Box(c).Padding(6, 0, 0).Children(func() {
			ui.Text(c, u.t("noRepositoryTitle")).FontWeight(700).FontSize(28).LetterSpacing(-.02).TextColor(tk.text)
		})
		ui.Box(c).MaxWidth(chromeWelcomeMaxText).Padding(0, 0, 16, 0).Children(func() {
			ui.Text(c, u.t("chromeWelcomeBody")).FontSize(14).TextColor(tk.muted).Wrap().TextAlign(ui.Center)
		})
		msg := u.chromeAlert()
		if msg != "" {
			ui.Text(c, msg).Wrap().TextColor(tk.del)
		}
		btn := ui.ButtonBase(c).Height(chromeOpenHeight).Padding(0, 26).Radius(12).Gap(8).
			Background(tk.accent).TextColor(tk.onAc).AlignItems(ui.Center).Label(u.t("openRepository"))
		if u.opening {
			btn.Disabled(true).Background(tk.accent.Alpha(.6))
		}
		btn.Children(func() {
			ui.Icon(c, designicons.Folder()).Size(16, 16).TextColor(tk.onAc)
			ui.Text(c, u.t("openRepository")).FontWeight(600).FontSize(14)
		})
		if btn.Clicked() {
			u.chooseRepository()
		}
		ui.Box(c).Padding(6, 0, 0).Children(func() {
			ui.Text(c, u.t("landingDropHint")).TextColor(tk.muted)
		})
	})
}

func (u *UI) chromeAlert() string {
	if u.land.dropError != "" {
		return u.t(u.land.dropError)
	}
	if u.notice != nil {
		return u.t("error" + string(u.notice.code))
	}
	return ""
}

// chromeSidebarToggle is the sidebar icon. In a wide window it hides or shows
// the repository rail; in a compact one it opens the navigation drawer.
func (u *UI) chromeSidebarToggle(c *ui.Context, tk kitTokens) {
	b := kitIconButton(c, designicons.Sidebar(), u.chromeSidebarLabel(), tk, false)
	u.chromeMarkFocus(b)
	if b.Clicked() {
		u.toggleSidebar()
	}
}

func (u *UI) chromeSidebarLabel() string {
	switch {
	case u.lay.compact && u.lay.drawerOpen:
		return u.t("chromeHideNav")
	case u.lay.compact:
		return u.t("chromeShowNav")
	case u.lay.railHidden:
		return u.t("chromeShowRepositories")
	default:
		return u.t("chromeHideRepositories")
	}
}

// chromeRailInline reports whether the repository rail sits beside the
// content. Welcome always shows it. A wide window with a repository open
// can hide it; a compact window keeps it.
func (u *UI) chromeRailInline() bool {
	return u.repo == nil || u.lay.compact || !u.lay.railHidden
}

// Times the repository rail and the navigation drawer take to slide.
const (
	railSlide     = 200 * time.Millisecond
	drawerSlide   = 200 * time.Millisecond
	composerSlide = 200 * time.Millisecond
)

// chromeRailSlot is the repository rail's place beside the content. Showing or
// hiding the rail slides it over railSlide: the slot's width follows the
// rail's progress, and the rail moves with the slot's edge, clipped to it.
// The slot is always built so its progress persists between frames.
func (u *UI) chromeRailSlot(c *ui.Context, tk kitTokens) {
	target := float32(0)
	if u.chromeRailInline() {
		target = 1
	}
	slot := ui.Row(c).Shrink(0).FillHeight().Clip()
	p := slot.Animate("rail", target, railSlide)
	slot.Width(chromeRailWidth * p)
	if p == 0 {
		return
	}
	slot.Children(func() {
		ui.Row(c).Width(chromeRailWidth).FillHeight().Shrink(0).Translate(-chromeRailWidth*(1-p), 0).Children(func() {
			u.chromeRail(c, tk)
		})
	})
}

// chromeDrawer is the navigation drawer of a compact window. Opening it slides
// the nav column in from the rail's edge over a scrim of the content; closing
// slides it back out over drawerSlide. The drawer is always built, zero wide
// while shut, so its progress persists; the scrim is built only while the
// drawer shows, so a shut drawer takes no clicks.
func (u *UI) chromeDrawer(c *ui.Context, tk kitTokens) {
	target := float32(0)
	if u.lay.drawerOpen {
		target = 1
	}
	if u.chrome.drawerP > 0 {
		scrim := ui.Box(c).Absolute().Top(0).Left(chromeRailWidth).Right(0).Bottom(0).
			Background(ui.RGBA(0, 0, 0, .25*u.chrome.drawerP))
		scrim.OnClick(func() { u.lay.drawerOpen = false })
	}
	sheet := ui.Row(c).Absolute().Top(chromeToolbarHeight).Left(chromeRailWidth).Bottom(0).
		Clip().Shadow(0, 0, 24, 0, ui.RGBA(0, 0, 0, .25))
	p := sheet.Animate("drawer", target, drawerSlide)
	u.chrome.drawerP = p
	sheet.Width(u.chromeNavW() * p)
	if p == 0 {
		return
	}
	sheet.Children(func() {
		ui.Column(c).Width(u.chromeNavW()).FillHeight().Shrink(0).Background(tk.nav).
			Translate(-u.chromeNavW()*(1-p), 0).Children(func() {
			u.chromeNav(c, tk)
		})
	})
}

// chromeNavInline reports whether the navigation column sits beside the
// content. A wide window with a repository open always shows it.
func (u *UI) chromeNavInline() bool {
	return u.repo != nil && !u.lay.compact
}

// chromeNavW is the navigation column's width: chromeNavWidth until its grip
// moves it.
func (u *UI) chromeNavW() float32 {
	if u.chrome.navWidth > 0 {
		return u.chrome.navWidth
	}
	return chromeNavWidth
}

// chromeNavGrip is the handle on the navigation column's right edge. Dragging
// it right widens the column and dragging left narrows it, within
// chromeNavMinWidth and chromeNavMaxWidth.
func (u *UI) chromeNavGrip(c *ui.Context, tk kitTokens) {
	grip := ui.Box(c).Width(6).FillHeight().Shrink(0).Cursor(ui.CursorResizeEW)
	if grip.Hovered() {
		grip.Background(tk.border)
	}
	dx, _, held := grip.Dragged()
	if held && dx != 0 {
		u.chrome.navWidth = min(max(u.chromeNavW()+dx, chromeNavMinWidth), chromeNavMaxWidth)
	}
}

func (u *UI) chromeNavShowing() bool {
	if u.repo == nil {
		return false
	}
	if u.lay.compact {
		return u.lay.drawerOpen
	}
	return true
}

func (u *UI) chromeNav(c *ui.Context, tk kitTokens) {
	ui.Column(c).Width(u.chromeNavW()).FillHeight().Shrink(0).
		Background(tk.nav).Border(1, tk.border).BorderWidth(0, 1, 0, 0).Padding(10, 8).Children(func() {
		u.chromeNotice(c, tk)
		u.chromeNavItem(c, tk, designicons.Changes(), u.t("chromeCurrentChanges"), u.middleTab == 0, 0)
		u.chromeNavItem(c, tk, designicons.Commits(), u.t("chromeAllCommits"), u.middleTab == 1, 1)
		u.chromeNavItem(c, tk, designicons.Search(), u.t("chromeSearch"), u.middleTab == 2, 2)
		// The tree scrolls on its own so a long branch or tag list never
		// pushes the filter off the bottom or clips the rows.
		ui.Column(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
			u.chromeNavTree(c, tk)
		})
		u.chromeNavFilter(c, tk)
	})
}

func (u *UI) chromeNavItem(c *ui.Context, tk kitTokens, icon *ui.SVG, label string, selected bool, tab int) {
	bg := ui.Color{}
	if selected && !u.cmp.open {
		bg = tk.sel
	}
	b := ui.ButtonBase(c).FillWidth().Height(chromeNavRowHeight).Padding(0, 10).Gap(8).
		Radius(9).Background(bg).Label(label).AlignItems(ui.Center)
	b.Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(tk.text)
		text := ui.Text(c, label).FontSize(13).TextColor(tk.text).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		if selected {
			text.FontWeight(600)
		}
		if tab == 1 {
			ui.Text(c, strconv.Itoa(u.chromeHistoryCount())).FontSize(12).TextColor(tk.muted)
		}
	})
	u.chromeMarkFocus(b)
	if b.Clicked() {
		u.leaveCompare()
		u.leaveStash()
		u.middleTab = tab
		if tab != 0 {
			// The History and Search pages read their own query: All commits
			// browses the shown branches, Search reads the text typed.
			u.applySearch()
		}
	}
}

func (u *UI) chromeNotice(c *ui.Context, tk kitTokens) {
	if u.notice == nil {
		return
	}
	ui.Column(c).FillWidth().Padding(8).Gap(4).Radius(9).Background(tk.del.Alpha(.12)).Shrink(0).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, u.t("error"+string(u.notice.code))).Wrap().Grow(1).TextColor(tk.del)
			d := ui.ButtonBase(c).Label(u.t("dismiss")).Padding(4, 6).Radius(8)
			d.Children(func() {
				ui.Text(c, u.t("dismiss")).FontSize(12).TextColor(tk.muted)
			})
			if d.Clicked() {
				u.notice = nil
			}
		})
		if u.notice != nil && u.notice.detail != "" {
			ui.Text(c, u.notice.detail).Wrap().FontSize(12).TextColor(tk.muted)
		}
	})
}

func (u *UI) chromeFocusRefresh(c *ui.Context) {
	focused := u.chrome.focus.Focused(c)
	if focused && u.chrome.seenFocus && !u.chrome.winFocus {
		u.Refresh()
	}
	u.chrome.winFocus = focused
	u.chrome.seenFocus = true
}

func (u *UI) chromeMarkFocus(e ui.Element) {
	if e.Focused() {
		e.Bind(&u.chrome.focus)
	}
}

// chromeReview draws Current changes or All commits in this shell. Mode
// switching is the navigation column.
func (u *UI) chromeReview(c *ui.Context) {
	if u.stv.open {
		u.chromeStashDetail(c)
		return
	}
	if u.cmp.open {
		u.chromeCompare(c)
		return
	}
	if u.middleTab == 1 {
		u.chromeHistory(c)
		return
	}
	if u.middleTab == 2 {
		u.chromeSearchPage(c)
		return
	}
	u.chromeEnsureFileSelection()
	switch {
	case u.lay.narrow && u.lay.detail:
		u.chromeDiff(c)
	case u.lay.narrow:
		u.chromeMiddle(c)
	default:
		ui.Split(c, &u.middleWidth, func() { u.chromeMiddle(c) }, func() { u.chromeDiff(c) }).Fill()
	}
}

func (u *UI) chromeMiddle(c *ui.Context) {
	ui.Column(c).Fill().MinWidth(0).Children(func() {
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() { u.chromeChanges(c) })
		u.chromeComposer(c)
	})
}

func (u *UI) chromeUnpinned() []string {
	pin := make(map[string]bool, len(u.prefs.Pinned))
	for _, root := range u.prefs.Pinned {
		pin[root] = true
	}
	var out []string
	for _, root := range u.prefs.Recent {
		if !pin[root] {
			out = append(out, root)
		}
	}
	// Recent is most-recently-opened first, and opening a repository moves
	// it to the top. Sorting by name keeps the row under the pointer in
	// place when it is clicked.
	sort.SliceStable(out, func(i, j int) bool {
		a, b := strings.ToLower(filepath.Base(out[i])), strings.ToLower(filepath.Base(out[j]))
		if a != b {
			return a < b
		}
		return out[i] < out[j]
	})
	return out
}

func (u *UI) chromeFilter(roots []string, q string) []string {
	if q == "" {
		return roots
	}
	needle := strings.ToLower(q)
	var out []string
	for _, root := range roots {
		if strings.Contains(strings.ToLower(filepath.Base(root)), needle) {
			out = append(out, root)
		}
	}
	return out
}

func (u *UI) chromeShownRepos(repos []string, q string) (shown []string, hidden int) {
	if q != "" || u.chrome.showAll || len(repos) <= chromeRepoVisible {
		return repos, 0
	}
	return repos[:chromeRepoVisible], len(repos) - chromeRepoVisible
}

func (u *UI) pinRepo(root string) {
	p := u.prefs
	p.Pinned = prefs.AddPinned(p.Pinned, root)
	u.savePreferences(p, "chromePinNotSaved")
}

func (u *UI) unpinRepo(root string) {
	p := u.prefs
	p.Pinned = prefs.RemovePinned(p.Pinned, root)
	u.savePreferences(p, "chromePinNotSaved")
}
