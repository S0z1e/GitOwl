package desktop

import (
	"strings"
)

// showClean reports the Changes page of a repository read as having no
// changes. A repository not read yet is not clean. A revert in progress is
// never "clean" here: the status strip must stay to continue or abort it,
// even once every conflict is resolved and the working tree is clean.
func (u *UI) showClean() bool {
	return u.middleTab == 0 && u.repo != nil && u.snap.Root != "" && u.snap.Clean() && !u.revertInProgress()
}

// hasDraft reports a message or AI draft kept for this repository.
func (u *UI) hasDraft() bool {
	return strings.TrimSpace(u.cm.message) != "" || u.ai.pending != nil
}
