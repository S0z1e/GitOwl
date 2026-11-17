package desktop

import (
	"context"
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

// discardState is the Discard Unstaged Changes confirmation.
type discardState struct {
	open bool
	plan workflow.DiscardPlan
	// busy is set while the plan is read or the discard runs.
	busy bool
	err  *notice
	seq  uint64
}

// planDiscard reads what discarding ch would do and asks to confirm it.
// Changes that cannot be discarded are explained instead.
func (u *UI) planDiscard(ch workflow.Change) {
	repo := u.repo
	if repo == nil || u.writing() {
		return
	}
	d := &u.discard
	d.seq++
	seq := d.seq
	u.background(func() func() {
		p, err := repo.PlanDiscard(context.Background(), ch)
		return func() {
			if repo != u.repo || seq != u.discard.seq {
				return
			}
			if err != nil {
				u.report(err)
				return
			}
			u.discard = discardState{open: true, plan: p, seq: seq}
		}
	})
}

// confirmDiscard runs the confirmed discard. If anything changed since the
// confirmation showed, nothing is discarded and the dialog shows the file
// as it is now, to confirm again.
func (u *UI) confirmDiscard() {
	repo, d := u.repo, &u.discard
	if repo == nil || d.busy || u.writing() {
		return
	}
	d.busy, d.err = true, nil
	p, seq := d.plan, d.seq
	u.background(func() func() {
		snap, err := repo.Discard(context.Background(), p)
		var again workflow.DiscardPlan
		var againErr error
		if workflow.CodeOf(err) == workflow.CodeDiscardStale {
			again, againErr = repo.PlanDiscard(context.Background(), p.Change)
		}
		return func() {
			if repo != u.repo || seq != u.discard.seq {
				return
			}
			d := &u.discard
			d.busy = false
			if snap.Root != "" {
				key := changeKey{area: p.Change.Area, path: p.Change.Path}
				if sel := u.wt.selected; sel != nil && *sel == key && !inArea(snap, key.area, key.path) {
					u.wt.selected = u.followFile(snap, p.Change)
				}
				u.setSnapshot(snap)
			}
			switch {
			case err == nil:
				d.open = false
				u.notice = nil
				u.toast("success", u.t("discardedToast", "path", p.Change.Path))
			case workflow.CodeOf(err) == workflow.CodeDiscardStale && againErr == nil:
				d.plan, d.err = again, noticeOf(err)
			default:
				d.open = false
				u.report(err)
			}
		}
	})
}

func (u *UI) discardDialog(c *ui.Context) {
	d := &u.discard
	if u.repo == nil || !d.open {
		return
	}
	p := d.plan
	title := u.t("discardTitle", "name", filepath.Base(p.Change.Path))
	u.chromeFormDialog(c, title, func(tk kitTokens) {
		ui.Text(c, title).FontWeight(600).FontSize(15).TextColor(tk.text)
		ui.Text(c, u.t("discardFile")).FontSize(12).TextColor(tk.muted)
		ui.Text(c, p.Change.Path).Wrap().Font("monospace").FontSize(13).TextColor(tk.text).Selectable()
		if p.Path != "" && p.Path != p.Change.Path {
			ui.Text(c, p.Path).Wrap().Font("monospace").FontSize(12).TextColor(tk.muted).Selectable()
		}
		body := u.t("discardBody")
		switch {
		case p.Deleted:
			body = u.t("discardBodyDeleted")
		case p.Staged:
			body = u.t("discardBodyStaged")
		}
		ui.Text(c, body).Wrap().FontSize(13).TextColor(tk.text)
		if p.Binary || p.TooLarge {
			ui.Text(c, u.t("discardBodyBinary")).Wrap().FontSize(13).TextColor(tk.warn)
		}
		ui.Text(c, u.t("discardUndo")).Wrap().FontSize(12).TextColor(tk.muted)
		if e := d.err; e != nil {
			ui.Text(c, u.t("error"+string(e.code))).Wrap().TextColor(tk.del)
		}
		if d.busy {
			kitSpinner(c, tk, 14)
		}
		u.chromeFormButtons(c, tk, u.t("discardConfirm"), func() {
			d.open, d.err = false, nil
		}, u.confirmDiscard, false)
	}, func() { d.open, d.err = false, nil })
}
