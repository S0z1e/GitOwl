package desktop

import (
	"context"
	"strings"

	"gitowl/internal/workflow"
)

// commit_mode.go holds the commit mode region: the design's "Amend previous
// commit" checkbox row (commit-b.md .am) and everything amending needs —
// loading the target, toggling the two drafts, and the Amend action button.
// It shares the one commit message and the operation protection commit.go
// set up, so ⌘Return and the button run one guarded operation.

// amending reports whether the commit button should amend the last commit
// instead of making a new one.
func (u *UI) amending() bool { return u.cm.amend }

// loadAmendTarget reads whether the current branch's last commit may be
// amended (workflow.AmendTarget) in the background and keeps the result for
// the Amend row. It is called when HEAD or the branch changes; a stale read,
// left behind by a switch, is dropped by amendSeq. The target is always the
// checked-out branch, never the one being browsed in history.
func (u *UI) loadAmendTarget() {
	repo := u.repo
	if repo == nil {
		u.cm.amendInfo, u.cm.amendLoaded, u.cm.amendAsked = nil, false, false
		return
	}
	// Reload only when a read for this HEAD was not asked yet: the target
	// is the checked-out HEAD, so an unchanged HEAD keeps the same answer.
	if u.cm.amendAsked && u.cm.amendHead == u.snap.Head {
		return
	}
	u.cm.amendSeq++
	seq := u.cm.amendSeq
	u.cm.amendHead = u.snap.Head
	u.cm.amendAsked = true
	u.cm.amendLoaded = false
	u.background(func() func() {
		info, err := repo.AmendTarget(context.Background())
		return func() {
			if repo != u.repo || seq != u.cm.amendSeq || err != nil {
				return
			}
			got := info
			u.cm.amendInfo, u.cm.amendLoaded = &got, true
			// A target that turned un-amendable, or moved, leaves amend mode
			// so the user cannot act on a stale target. The custom shell only
			// drops amend when there is no commit yet; pushed and detached
			// stay checked.
			if u.cm.amend {
				if u.chrome.shell {
					if info.Reason == workflow.CodeAmendUnborn {
						u.cm.amend = false
					}
				} else if !info.CanAmend || info.Hash == "" {
					u.toggleAmend(false)
				}
			}
		}
	})
}

// toggleAmend turns amend mode on or off, swapping the two drafts. Turning
// on stashes the normal draft and loads the target's message into the
// fields; turning off restores the stashed normal draft. Each draft keeps
// its own summary, description and message (commit-b.md: the two are kept
// apart and survive a worktree switch).
func (u *UI) toggleAmend(on bool) {
	cm := &u.cm
	if on == cm.amend {
		return
	}
	cm.hint = nil
	if on {
		cm.stash = draft{cm.summary, cm.description, cm.message, cm.composed}
		cm.amend = true
		if info := cm.amendInfo; info != nil {
			msg := strings.TrimRight(info.Message, "\n")
			cm.message = msg
			cm.summary, cm.description = splitMessage(msg)
			cm.composed = msg
		}
		return
	}
	cm.amend = false
	s := cm.stash
	cm.summary, cm.description, cm.message, cm.composed = s.summary, s.description, s.message, s.composed
	cm.stash = draft{}
}

// amendReason is the message key explaining why the Amend toggle is
// disabled, from the target's Reason code; empty keys fall back to a
// generic line. It is shown under the row, muted (commit-b.md: the reason
// sits beside the disabled control).
func (u *UI) amendReason() string {
	info := u.cm.amendInfo
	if info == nil {
		return u.t("amendUnavailable")
	}
	switch info.Reason {
	case workflow.CodeAmendDetached:
		return u.t("errorAmendDetached")
	case workflow.CodeAmendUnborn:
		return u.t("errorAmendUnborn")
	case workflow.CodeAmendPushed:
		return u.t("errorAmendPushed")
	case workflow.CodeAmendUnknown:
		return u.t("errorAmendUnknown")
	default:
		return u.t("amendUnavailable")
	}
}

// amendBlock returns the message key explaining why Amend cannot run now, or
// "" when it can. Amend needs a message; it does not need a staged change (a
// message-only reword is allowed). The repo and busy checks are done by
// commitBlock before this is reached.
func (u *UI) amendBlock() string {
	info := u.cm.amendInfo
	switch {
	case info == nil || !info.CanAmend:
		return "errorAmendNotAllowed"
	case strings.TrimSpace(u.cm.message) == "":
		return "errorEmptyMessage"
	}
	return ""
}

// amendStatus is the one short line under the Amend button: what is missing,
// else that it will reword the last commit (and fold in staged changes when
// any are staged).
func (u *UI) amendStatus() string {
	switch u.amendBlock() {
	case "errorEmptyMessage":
		return u.t("commitNeedsMessage")
	case "errorAmendNotAllowed":
		return u.amendReason()
	case "", "busy":
		if len(u.snap.Staged) == 0 {
			return u.t("amendReadyMessage")
		}
		return u.t("amendReadyStaged", "count", len(u.snap.Staged))
	default:
		return u.t(u.amendBlock())
	}
}

// amend rewrites the last commit with the current message, folding in the
// staged changes when any are staged (otherwise a message-only reword). It
// re-reads and re-checks HEAD and staged under the workflow lock; a refusal
// keeps the draft and the files. It never pushes.
func (u *UI) amend() {
	if u.chrome.shell {
		u.chromeAmend()
		return
	}
	info := u.cm.amendInfo
	if info == nil || !info.CanAmend {
		u.cm.hint = &notice{code: workflow.CodeAmendNotAllowed}
		return
	}
	if strings.TrimSpace(u.cm.message) == "" {
		u.cm.hint = &notice{code: workflow.CodeEmptyMessage}
		return
	}
	u.cm.committing, u.cm.hint = true, nil
	repo, message := u.repo, u.cm.message
	req := workflow.AmendRequest{
		Message:     message,
		Head:        u.snap.Head,
		Staged:      u.snap.StagedFingerprint,
		MessageOnly: len(u.snap.Staged) == 0,
	}
	u.background(func() func() {
		snap, c, err := repo.Amend(context.Background(), req)
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
			// The amend consumed the draft: leave amend mode and clear both
			// drafts, as a new commit would.
			if u.cm.message == message {
				u.cm.amend = false
				u.cm.stash = draft{}
				u.cm.message, u.cm.summary, u.cm.description, u.cm.composed = "", "", "", ""
			}
			u.cm.last = &c
			u.ai.applied, u.ai.pending, u.ai.last = "", nil, nil
			u.setSnapshot(snap)
			u.committed()
		}
	})
}
