package desktop

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// overlayFor is how long a popover or dialog takes to show or hide. MyGo draws
// both without a transition, so each one that GitOwl opens sets one of the
// motions below.
const overlayFor = 150 * time.Millisecond

// overlayPopover is ui.Popover with popoverMotion.
func overlayPopover(c *ui.Context, anchor ui.Element, open *bool, fn func()) {
	ui.Popover(c, anchor, open, fn).Transition(popoverMotion)
}

// popoverMotion drops a popover in from just above its place and fades it,
// and fades it out the same way when it closes.
var popoverMotion = ui.ElementTransition{
	Duration: overlayFor,
	Enter:    &ui.Motion{Y: -6, Opacity: 0},
	Exit:     &ui.Motion{Y: -6, Opacity: 0},
}

// dialogFade fades a dialog's backdrop in and out. The dialog panel sits inside
// the backdrop, so it fades with it.
var dialogFade = ui.ElementTransition{
	Duration: overlayFor,
	Enter:    &ui.Motion{Opacity: 0},
	Exit:     &ui.Motion{Opacity: 0},
}
