package desktop

import (
	"context"
	"gitowl/internal/desktop/designicons"
	"strings"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

// chromeComposer is the Current changes commit box: branch, summary,
// description, Generate Draft, Amend previous commit and one Commit
// button. It stays under the file list at every width.
func (u *UI) chromeComposer(c *ui.Context) {
	if u.repo == nil {
		return
	}
	u.cm.syncFields()
	tk := kitDesignTokens(c.Theme().Dark)
	// The bar and the panel share one slot that slides between their heights
	// over composerSlide. The bar fades out as the panel fades in.
	target := float32(1)
	if u.chrome.composerHidden {
		target = 0
	}
	slot := ui.Column(c).FillWidth().Shrink(0).Clip()
	p := slot.Animate("composer", target, composerSlide)
	barH := float32(kitToolbarH + 20)
	cardH := max(u.chrome.composerH, barH)
	if p < 1 {
		// At rest the slot takes its natural height, so a panel not yet
		// measured is never clipped.
		slot.Height(barH*(1-p) + cardH*p)
	}
	slot.Children(func() {
		if p < 1 {
			u.chromeComposerBar(c, tk, barH, p)
		}
		if p > 0 {
			u.chromeComposerCard(c, tk, p)
		}
	})
}

// chromeComposerCard draws the commit panel card at opacity p, and keeps its
// height for the slide.
func (u *UI) chromeComposerCard(c *ui.Context, tk kitTokens, p float32) {
	branch := u.snap.Branch
	if u.snap.Detached {
		branch = u.t("detachedHead")
	}
	// The composer is a bordered card, as the design's commit panel is.
	card := ui.Column(c).FillWidth().Shrink(0).Padding(10, 12).Opacity(p)
	card.Children(func() {
		ui.Column(c).FillWidth().Padding(12).Gap(10).Radius(14).
			Background(tk.surf).Border(1, tk.border).Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, branch).FontSize(12).FontWeight(600).TextColor(tk.muted).
					SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
				// Close keeps the message and draft; the bar below reopens them.
				closeBtn := kitIconButton(c, designicons.Close(), u.t("commitPanelClose"), tk, false)
				if closeBtn.Clicked() {
					u.chrome.composerHidden = true
				}
			})
			u.chromeDraftCard(c, tk)
			u.chromeSummaryRow(c, tk)
			desc := ui.TextArea(c, &u.cm.description).FillWidth().
				Label(u.t("commitBDescription")).Placeholder(u.t("commitBDescriptionHint"))
			u.chromeMarkFocus(desc)
			if desc.Changed() {
				u.cm.recompose()
				u.answerHint()
			}
			desc.OnShortcut(ui.Cmd, ui.KeyEnter, u.commitShortcut)
			u.chromeCommitResult(c, tk)
			ui.Text(c, u.commitStatus()).FontSize(12).TextColor(tk.muted).Wrap()
			u.chromeIdentityLine(c, tk)
			u.chromeCommitActions(c, tk)
		})
	})
	u.chrome.composerH = card.Bounds().H
}

func (u *UI) chromeSummaryRow(c *ui.Context, tk kitTokens) {
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		box := ui.TextInput(c, &u.cm.summary).Grow(1).MinWidth(0).FillWidth().
			Height(kitFieldH).Label(u.t("commitBSummary")).Placeholder(u.t("commitBSummaryHint"))
		u.chromeMarkFocus(box)
		if box.Changed() {
			u.cm.recompose()
			u.answerHint()
		}
		box.OnShortcut(ui.Cmd, ui.KeyEnter, u.commitShortcut)
		n := len([]rune(u.cm.summary))
		count := ui.Text(c, u.t("commitBCount", "n", n)).Font("monospace").
			FontSize(11).SingleLine().Shrink(0)
		if n > summaryLimit {
			count.TextColor(tk.warn)
		} else {
			count.TextColor(tk.muted)
		}
	})
}

func (u *UI) chromeDraftCard(c *ui.Context, tk kitTokens) {
	d := u.ai.pending
	if d == nil || u.ai.generating {
		return
	}
	ui.Column(c).FillWidth().Padding(10).Gap(8).Radius(12).
		Background(tk.surf).Border(1, tk.bd2).Children(func() {
		ui.Text(c, d.Text).FontSize(13).TextColor(tk.muted).Wrap().MaxLines(3).Ellipsis("…")
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if u.suggestStale(d) {
				if kitSecondaryButton(c, u.t("draftUseAnyway"), tk, false).Clicked() {
					u.useDraft(*d)
				}
				if kitSecondaryButton(c, u.t("draftRegenerate"), tk, false).Clicked() {
					u.generateDraft()
				}
				return
			}
			if kitSecondaryButton(c, u.t("draftReplace"), tk, false).Clicked() {
				u.useDraft(*d)
			}
			if kitSecondaryButton(c, u.t("draftDiscard"), tk, false).Clicked() {
				u.ai.pending = nil
			}
		})
	})
}

func (u *UI) chromeCommitResult(c *ui.Context, tk kitTokens) {
	switch {
	case u.cm.hint != nil:
		h := u.cm.hint
		ui.Text(c, u.t("error"+string(h.code))).Wrap().TextColor(tk.del)
		if h.detail != "" {
			ui.Text(c, h.detail).FontSize(12).Wrap().TextColor(tk.muted)
		}
		if h.code == workflow.CodeIdentityMissing && u.prefs.CommitIdentity {
			set := kitSecondaryButton(c, u.t("identitySet"), tk, false)
			u.focusReturn(set, "identityFix")
			u.chromeMarkFocus(set)
			if set.Clicked() {
				u.openIdentity()
			}
		}
	case u.ai.err != nil:
		e := u.ai.err
		ui.Text(c, u.t("error"+string(e.code))).Wrap().TextColor(tk.del)
		if e.detail != "" {
			ui.Text(c, e.detail).FontSize(12).Wrap().TextColor(tk.muted)
		}
	case u.cm.last != nil:
		ui.Text(c, u.t("committedAs", "hash", u.cm.last.Short(), "subject", u.cm.last.Subject)).
			Wrap().TextColor(tk.add)
	}
}

func (u *UI) chromeCommitActions(c *ui.Context, tk kitTokens) {
	staged := len(areaChanges(u.snap, workflow.Staged))
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
		u.chromeGenerateDraft(c, tk)
		ui.Text(c, u.t("aiSuggestUses", "count", staged)).FontSize(12).TextColor(tk.muted).SingleLine()
	})
	u.chromeAmendToggle(c, tk)
	// Commit spans the card's width, as the design's primary action does.
	block := u.commitBlock()
	off := block != ""
	btn := kitPrimaryButton(c, u.t("commitAction"), tk, false).FillWidth()
	if off && block != "busy" {
		btn.Tooltip(u.commitStatus()).Description(u.commitStatus())
	}
	if off {
		btn.Opacity(.45)
	}
	if btn.Clicked() && !off {
		u.commit()
	}
}

func (u *UI) chromeGenerateDraft(c *ui.Context, tk kitTokens) {
	if u.ai.generating {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			kitSpinner(c, tk, 14)
			ui.Text(c, u.t("draftWriting")).FontSize(12).TextColor(tk.muted).SingleLine()
		})
		return
	}
	block := u.draftBlock()
	off := block != ""
	b := kitSecondaryButtonIcon(c, designicons.Sparkle(), u.t("draftGenerate"), tk)
	if off && block != "busy" {
		b.Tooltip(u.t(block)).Description(u.t(block))
	}
	if off {
		b.Opacity(.45)
	}
	if b.Clicked() && !off {
		u.generateDraft()
	}
}

func (u *UI) chromeAmendToggle(c *ui.Context, tk kitTokens) {
	disabled := u.snap.Unborn()
	on := u.cm.amend
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		box := kitCheckbox(c, &on, u.t("amendToggle"), tk, disabled)
		if disabled {
			box.Tooltip(u.t("errorAmendUnborn"))
		}
		if box.Clicked() && !disabled {
			u.chromeToggleAmend(!u.cm.amend)
		}
		ui.Text(c, u.t("amendToggle")).FontSize(12).TextColor(tk.muted).SingleLine().PassThrough()
	})
}

// chromeToggleAmend turns amend on or off without loading the previous
// commit's message or swapping drafts.
func (u *UI) chromeToggleAmend(on bool) {
	if on == u.cm.amend {
		return
	}
	u.cm.amend = on
	u.cm.hint = nil
}

func (u *UI) chromeDraftAppliesDirectly() bool {
	msg := strings.TrimSpace(u.cm.message)
	if msg == "" {
		return true
	}
	return u.ai.draftedByAI && msg == strings.TrimSpace(u.ai.applied)
}

// chromeAmend rewrites HEAD with the current message and staged files.
// Unlike the Onevra path, a message-only amend is not a commit: staged
// files are required. workflow.Amend still refuses a pushed commit.
func (u *UI) chromeAmend() {
	if u.snap.Unborn() || strings.TrimSpace(u.cm.message) == "" || len(u.snap.Staged) == 0 {
		return
	}
	u.cm.committing, u.cm.hint = true, nil
	repo, message := u.repo, u.cm.message
	req := workflow.AmendRequest{
		Message:     message,
		Head:        u.snap.Head,
		Staged:      u.snap.StagedFingerprint,
		MessageOnly: false,
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
			if u.cm.message == message {
				u.cm.amend = false
				u.cm.stash = draft{}
				u.cm.message, u.cm.summary, u.cm.description, u.cm.composed = "", "", "", ""
			}
			u.cm.last = &c
			u.ai.applied, u.ai.pending, u.ai.last, u.ai.draftedByAI = "", nil, nil, false
			u.setSnapshot(snap)
			u.committed()
		}
	})
}

// chromeComposerBar is the commit panel shut to a bar. Its press opens the
// panel again with the message and draft it kept. At progress p it is
// barH*(1-p) tall and fades out, as the panel opens over composerSlide.
func (u *UI) chromeComposerBar(c *ui.Context, tk kitTokens, barH, p float32) {
	bar := ui.Column(c).FillWidth().Shrink(0).Height(barH * (1 - p)).Clip().Opacity(1 - p)
	bar.Children(func() {
		ui.Column(c).FillWidth().Padding(10, 12).Children(func() {
			show := ui.ButtonBase(c).FillWidth().Height(kitToolbarH).Padding(0, 12).Gap(6).Radius(12).
				Background(tk.surf).Border(1, tk.bd2).AlignItems(ui.Center).Label(u.t("commitPanelShow"))
			show.Children(func() {
				ui.Text(c, u.t("commitPanelShow")).FontSize(13).FontWeight(500).TextColor(tk.text).SingleLine()
				ui.Icon(c, designicons.ChevronDown()).Size(12, 12).TextColor(tk.muted).Rotate(180)
			})
			if show.Clicked() {
				u.chrome.composerHidden = false
			}
		})
	})
}
