package desktop

import (
	"context"
	"strings"

	"gitowl/internal/workflow"
)

// summaryLimit is the design's 72-character Summary hint (commit-b.md
// "n / 72"). It is a hint only: a longer Summary changes the count's color
// but never blocks the commit.
const summaryLimit = 72

// composer is the commit message editor of the open repository. The
// message is the single source of truth GitOwl commits and other features
// (AI, sessions) set; summary and description are the two fields the design
// edits, split from the message and composed back into it (see splitMessage
// and composeMessage).
type composer struct {
	message string
	// summary is the first line of the message; description the body after
	// a blank line. composed caches the message these two last produced, so
	// composerView knows when message was set from the outside and must be
	// split again.
	summary     string
	description string
	composed    string
	// committing blocks a second commit while one runs.
	committing bool
	// hint explains why the commit button did nothing, or what happened.
	hint *notice
	// last is the latest commit GitOwl created in this repository.
	last *workflow.Commit
	// push holds a commit-and-push that committed but could not push: the
	// local commit stands and Retry push may run again. It is nil when no
	// push is pending retry.
	push *commitPush
	// showWhenClean unfolds the editor while the working tree is clean, to
	// keep working on a kept message.
	showWhenClean bool
	// menuOpen tracks the commit split-button's menu for later slices
	// (Amend, Commit and push); this slice's menu holds only Commit.
	menuOpen bool
	// amend turns the panel to amend the last commit instead of making a
	// new one. amendInfo is the target as last read (AmendTarget): its hash,
	// subject, message and whether it may be amended. amendLoaded is set
	// once a read completed for the current HEAD; amendSeq drops stale reads.
	amend       bool
	amendInfo   *workflow.Amendable
	amendLoaded bool
	amendSeq    uint64
	// amendHead is the HEAD the target was last loaded for; a change means
	// the row reloads (the checked-out branch moved or switched). amendAsked
	// records that a read for amendHead was already requested, so the row
	// does not re-request every frame while one is in flight.
	amendHead  string
	amendAsked bool
	// stash holds the normal (non-amend) draft while amend mode is on, so
	// turning amend off restores exactly what the user was writing. It is
	// only meaningful while amend is true.
	stash draft
}

// draft is one commit message split the design's way, kept aside while the
// other draft is edited (amend vs normal).
type draft struct{ summary, description, message, composed string }

// composeMessage joins a summary and a description into one commit message:
// the summary alone, or the summary, a blank line and the description when
// the description has content. Trailing blank lines on either are trimmed so
// an empty description never adds a separator.
func composeMessage(summary, description string) string {
	s := strings.TrimRight(summary, "\n")
	d := strings.TrimRight(description, "\n")
	if strings.TrimSpace(d) == "" {
		return s
	}
	return s + "\n\n" + d
}

// splitMessage is composeMessage reversed: the first paragraph is the
// summary, the rest (after one blank line) the description. A message
// without a blank line is all summary.
func splitMessage(message string) (summary, description string) {
	if i := strings.Index(message, "\n\n"); i >= 0 {
		return message[:i], message[i+2:]
	}
	return message, ""
}

// syncFields splits the message into summary and description when the
// message was set from the outside (AI, a session, a test) since the fields
// last composed it. composeView calls this before binding the editors.
func (cm *composer) syncFields() {
	if cm.message != cm.composed {
		cm.summary, cm.description = splitMessage(cm.message)
		cm.composed = cm.message
	}
}

// recompose rebuilds the message from the edited fields and remembers it as
// the composed value, so the next syncFields leaves the fields alone.
func (cm *composer) recompose() {
	cm.message = composeMessage(cm.summary, cm.description)
	cm.composed = cm.message
}

// commitBlock returns the message key explaining why Commit cannot run
// now, or "" when it can. "busy" means another operation must finish
// first; the spinner shows it.
func (u *UI) commitBlock() string {
	switch {
	case u.repo == nil:
		return "errorRepositoryClosed"
	case u.cm.committing || u.indexBusy():
		return "busy"
	}
	return u.commitReason()
}

// commitReason is why Commit cannot run once no Git change is in flight, or
// "". The status line reads it during a write, so a write does not swap the
// real reason for a generic line and back.
func (u *UI) commitReason() string {
	switch {
	case u.repo == nil:
		return "errorRepositoryClosed"
	case u.revertInProgress():
		return "errorRevertInProgress"
	}
	if u.amending() && !u.chrome.shell {
		return u.amendBlock()
	}
	switch {
	case len(u.snap.Staged) == 0:
		return "errorNothingStaged"
	case strings.TrimSpace(u.cm.message) == "":
		return "errorEmptyMessage"
	}
	return ""
}

// commit creates a commit from the staged changes and the message, as the
// user wrote it. It does nothing else: no staging, no pushing.
func (u *UI) commit() {
	if u.repo == nil || u.cm.committing || u.indexBusy() {
		return
	}
	if u.amending() {
		u.amend()
		return
	}
	switch {
	case len(u.snap.Staged) == 0:
		u.cm.hint = &notice{code: workflow.CodeNothingStaged}
		return
	case strings.TrimSpace(u.cm.message) == "":
		u.cm.hint = &notice{code: workflow.CodeEmptyMessage}
		return
	}
	u.cm.committing, u.cm.hint = true, nil
	repo, message, staged := u.repo, u.cm.message, u.snap.StagedFingerprint
	u.background(func() func() {
		snap, c, err := repo.Commit(context.Background(), message, staged)
		return func() {
			if repo != u.repo {
				return
			}
			u.cm.committing = false
			if err != nil {
				n := &notice{code: workflow.CodeOf(err)}
				if e, ok := err.(*workflow.Error); ok {
					n.detail = e.Detail
				}
				u.cm.hint = n
				if snap.Root != "" {
					u.setSnapshot(snap)
				}
				return
			}
			// Only a successful commit clears the message.
			if u.cm.message == message {
				u.cm.message, u.cm.summary, u.cm.description, u.cm.composed = "", "", "", ""
			}
			u.cm.last = &c
			// The draft belonged to the commit just made.
			u.ai.applied, u.ai.pending, u.ai.last, u.ai.draftedByAI = "", nil, nil, false
			u.setSnapshot(snap)
			u.committed()
		}
	})
}

// committed lets other views follow a new commit; history follows the new
// HEAD through setSnapshot.
func (u *UI) committed() {}

// answerHint clears a refusal hint once the user edits the message again,
// and drops the Drafted by AI marker: an edited message is no longer the
// AI's draft.
func (u *UI) answerHint() {
	u.ai.draftedByAI = false
	if u.cm.hint != nil && !u.cm.committing {
		u.cm.hint = nil
	}
}

// commitShortcut runs the same commit the button does, under the same
// protection, so ⌘Return never gets around validation or commits twice.
func (u *UI) commitShortcut() {
	if u.commitBlock() == "" {
		u.commit()
	}
}

// commitStatus is the one short line under the editor: what is missing
// before Commit, else what Commit will do.
func (u *UI) commitStatus() string {
	target := u.snap.Branch
	if u.snap.Detached {
		target = u.t("detachedHead")
	}
	if u.amending() && !u.chrome.shell {
		return u.amendStatus()
	}
	switch u.commitReason() {
	case "errorNothingStaged":
		return u.t("commitNeedsStaged")
	case "errorEmptyMessage":
		return u.t("commitNeedsMessage")
	case "":
		return u.t("commitReady", "branch", target)
	default:
		return u.t(u.commitReason())
	}
}
