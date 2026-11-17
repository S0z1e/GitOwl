package desktop

// The sidebar toggle has one label per layout. A compact window opens the
// navigation drawer, while a wide one hides or shows the repository rail.
// Tests use these helpers so the label matches the layout they are in.

func sidebarShowLabel(u *UI) string {
	if u.lay.compact {
		return u.t("chromeShowNav")
	}
	return u.t("chromeShowRepositories")
}

func sidebarHideLabel(u *UI) string {
	if u.lay.compact {
		return u.t("chromeHideNav")
	}
	return u.t("chromeHideRepositories")
}
