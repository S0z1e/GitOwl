package desktop

import (
	"os"
)

// landingState keeps what the Landing shows besides the recent list: the
// reason a drop was refused, until the next frame replaces it or a
// successful open leaves the Landing.
type landingState struct {
	// dropError is the locale key of the latest refused drop, or "".
	dropError string
}

// onDropped opens a single dropped folder, or explains why it will not.
// It never records a Recent entry itself: a successful open records it
// through switchTo, as every open does.
func (u *UI) onDropped(files []string) {
	if u.switchBlocked() {
		// A drop while opening or writing is ignored, so a slow open is
		// not interrupted; the reason is already on screen.
		return
	}
	u.land.dropError = ""
	u.notice = nil
	switch {
	case len(files) != 1:
		u.land.dropError = "landingDropMany"
		return
	case !isDir(files[0]):
		u.land.dropError = "landingDropFile"
		return
	}
	u.OpenPath(files[0])
}

// isDir reports whether path is an existing directory. A path that cannot
// be read is not a directory, so the drop is refused as a non-folder.
func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}
