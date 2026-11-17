package desktop

// Window widths, in DIPs, below which the layout folds.
const (
	// compactWidth hides the repository column behind a toggle.
	compactWidth = 1100
	// narrowWidth shows one of the list and the diff at a time.
	narrowWidth = 760
)

// layout is how the window folds at its current width.
type layout struct {
	compact, narrow bool
	// railHidden hides the repository rail in a wide window.
	railHidden bool
	// drawerOpen shows the navigation column over the content in a
	// compact window.
	drawerOpen bool
	// detail shows the diff or commit pane instead of the list in a narrow
	// window.
	detail bool
}

// showDetail moves a narrow window to the detail pane after a selection.
func (u *UI) showDetail() { u.lay.detail = true }

// toggleSidebar hides or shows the repository rail in a wide window, and
// the navigation drawer in a compact one.
func (u *UI) toggleSidebar() {
	if u.lay.compact {
		u.lay.drawerOpen = !u.lay.drawerOpen
	} else {
		u.lay.railHidden = !u.lay.railHidden
	}
}
