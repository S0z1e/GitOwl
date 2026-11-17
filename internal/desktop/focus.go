package desktop

import "github.com/egoist/mygo/ui"

// The keyboard focus goes back to what had it when a dialog opened, once
// the dialog closes: the button, row or list that opened it, or the
// control the focus was in when a shortcut did. MyGo gives the focus
// back only when the focused element is gone, which the dialog's exit
// animation delays past the moment it has moved on, so GitOwl does it
// itself for the controls that open dialogs. Each of them calls
// focusReturn as it is built, under a name of its own.

// focusReturn notes that e, the control named key, has the focus, and
// focuses it again when the dialog opened from it has closed.
func (u *UI) focusReturn(e ui.Element, key string) {
	if e.FocusWithin() && !u.dialogOpen() {
		u.focusKey = key
	}
	if u.returnFocus == key && !u.dialogOpen() {
		u.returnFocus = ""
		e.Focus()
	}
}

// noteDialogs runs at the end of a frame: a dialog that just opened
// returns the focus, when it closes, to the control that had it.
func (u *UI) noteDialogs() {
	open := u.dialogOpen()
	if open && !u.wasDialogOpen {
		u.returnFocus = u.focusKey
	}
	u.wasDialogOpen = open
}

// dialogOpen reports whether a dialog of the window is open.
func (u *UI) dialogOpen() bool {
	return u.set.open || u.br.createOpen || u.br.deleteOpen || u.trees.createOpen || u.trees.removeOpen ||
		u.trees.pruneOpen || u.sync.publishOpen || u.discard.open || u.ai.inspectOpen || u.pick.open ||
		u.sync.pop.nameOpen || u.sync.pop.addOpen || u.br.form.open || u.br.confirm.open ||
		u.ident.open || u.chrome.pal.open
}
