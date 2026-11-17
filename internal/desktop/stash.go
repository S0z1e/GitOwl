package desktop

import (
	"context"
	"os"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"gitowl/internal/workflow"
)

// stashView is the read-only stash detail in the main column.
type stashView struct {
	open    bool
	item    workflow.Stash
	detail  workflow.StashDetail
	cursor  int
	diff    *diffPane
	seq     uint64
	err     error
	loading bool
	busy    bool
}

func (u *UI) leaveStash() {
	u.stv.seq++
	u.stv = stashView{seq: u.stv.seq}
}

func (u *UI) chromeStash() {
	if u.repo == nil || u.snap.Clean() || u.writing() {
		return
	}
	repo := u.repo
	u.stv.busy = true
	u.background(func() func() {
		snap, _, err := repo.Stash(context.Background(), true)
		return func() {
			if repo != u.repo {
				return
			}
			u.stv.busy = false
			if snap.Root != "" {
				u.setSnapshot(snap)
			}
			u.loadNavLists()
			if err != nil {
				u.report(err)
			}
		}
	})
}

func (u *UI) openStash(st workflow.Stash) {
	repo := u.repo
	if repo == nil {
		return
	}
	u.leaveCompare()
	u.stv.seq++
	seq := u.stv.seq
	u.stv = stashView{open: true, item: st, seq: seq, cursor: -1, loading: true}
	u.chrome.navSel = "stash:" + st.Ref
	u.background(func() func() {
		detail, err := repo.StashShow(context.Background(), st.Ref)
		return func() {
			if seq != u.stv.seq || repo != u.repo {
				return
			}
			u.stv.loading, u.stv.detail, u.stv.err = false, detail, err
			if err == nil && len(detail.Files) > 0 {
				u.selectStashFile(0)
			}
		}
	})
}

func (u *UI) selectStashFile(i int) {
	if i < 0 || i >= len(u.stv.detail.Files) {
		return
	}
	u.stv.cursor = i
	u.loadStashDiff(u.stv.detail.Files[i])
}

func (u *UI) loadStashDiff(f workflow.Change) {
	repo := u.repo
	if repo == nil {
		return
	}
	u.stv.seq++
	seq, ref := u.stv.seq, u.stv.item.Ref
	key := changeKey{area: workflow.InCommit, path: f.Path}
	u.stv.diff = &diffPane{key: key, caretHunk: -1, scrollHunk: -1, loading: true}
	u.background(func() func() {
		info, err := repo.StashDiff(context.Background(), ref, f.Path)
		var state *editor.DiffState
		if err == nil && info.HasText() {
			state, err = diffState(info.Patch, f.Path)
		}
		return func() {
			if seq != u.stv.seq || repo != u.repo {
				return
			}
			u.stv.diff = &diffPane{key: key, info: info, state: state, err: err, caretHunk: -1, scrollHunk: -1}
		}
	})
}

func (u *UI) chromeStashDetail(c *ui.Context) {
	tk := kitDesignTokens(c.Theme().Dark)
	ui.Column(c).Fill().MinWidth(0).Background(tk.bg).Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Padding(12, 14, 8).Gap(8).Children(func() {
			ui.Text(c, u.stv.item.Message).Bold().FontSize(16).TextColor(tk.text).
				SingleLine().Ellipsis("…")
			u.chromeCompareControls(c, tk)
			ui.Row(c).Gap(8).Wrap().Children(func() {
				off := u.writing()
				apply := kitSecondaryButton(c, u.t("stashApply"), tk, false)
				if off {
					apply.Opacity(.45)
				}
				if apply.Clicked() && !off {
					u.applyStash(false)
				}
				pop := kitSecondaryButton(c, u.t("stashPop"), tk, false)
				if off {
					pop.Opacity(.45)
				}
				if pop.Clicked() && !off {
					u.applyStash(true)
				}
			})
		})
		if u.stv.err != nil {
			ui.Text(c, u.t("error"+string(noticeOf(u.stv.err).code))).Padding(12, 14).TextColor(tk.del).Wrap()
			return
		}
		ui.Row(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).Width(260).FillHeight().MinWidth(0).Border(1, tk.border).Children(func() {
				u.chromeStashFiles(c, tk)
			})
			ui.Column(c).Grow(1).FillHeight().MinWidth(0).Children(func() {
				if p := u.stv.diff; p != nil {
					u.diffBodyDesign(c, p)
				}
			})
		})
	})
}

func (u *UI) chromeStashFiles(c *ui.Context, tk kitTokens) {
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Padding(6).Gap(2).Children(func() {
			for i, f := range u.stv.detail.Files {
				u.chromeCompareFileRowFor(c, tk, i, f, i == u.stv.cursor, func() { u.selectStashFile(i) })
			}
		})
	})
}

func (u *UI) chromeStashItems(m *ui.Menu, st workflow.Stash) {
	m.Item(st.Message).Disabled(true)
	m.Separator()
	if m.Item(u.t("stashApply")).Disabled(u.writing()).Chosen() {
		u.openStash(st)
		u.applyStash(false)
	}
	if m.Item(u.t("stashPop")).Disabled(u.writing()).Chosen() {
		u.openStash(st)
		u.applyStash(true)
	}
	m.Separator()
	if m.Item(u.t("stashNewBranch")).Disabled(u.writing()).Chosen() {
		u.openStashBranchForm(st)
	}
	m.Separator()
	if m.Item(u.t("stashCompare")).Chosen() {
		head := u.snap.Head
		if u.snap.Branch != "" && !u.snap.Detached {
			head = u.snap.Branch
		}
		u.startCompareAs(head, st.Ref, head, st.Message)
	}
	if m.Item(u.t("stashCompareWorking")).Chosen() {
		u.startCompareAs(st.Ref, "", st.Message, u.t("workingDirectory"))
	}
	if m.Item(u.t("stashSavePatch")).Chosen() {
		u.saveStashPatch(st)
	}
	m.Separator()
	if m.Item(u.t("stashRename")).Disabled(u.writing()).Chosen() {
		u.openStashRenameForm(st)
	}
	if m.Item(u.t("stashDrop")).Disabled(u.writing()).Chosen() {
		u.openBranchConfirm("dropStash", st.Message, "", st.Ref)
	}
	m.Separator()
	if m.Item(u.t("stashCopyName")).Chosen() {
		u.copyPending = st.Message
	}
}

func (u *UI) applyStash(pop bool) {
	st := u.stv.item
	if st.Ref == "" || u.writing() || u.repo == nil {
		return
	}
	repo := u.repo
	u.stv.busy = true
	u.background(func() func() {
		var snap workflow.Snapshot
		var err error
		if pop {
			snap, err = repo.PopStash(context.Background(), st.Ref)
		} else {
			snap, err = repo.ApplyStash(context.Background(), st.Ref)
		}
		return func() {
			if repo != u.repo {
				return
			}
			u.stv.busy = false
			if snap.Root != "" {
				u.setSnapshot(snap)
			}
			u.loadNavLists()
			if err != nil {
				u.report(err)
				return
			}
			if pop {
				u.leaveStash()
			}
		}
	})
}

func (u *UI) dropStash(ref string) {
	if ref == "" || u.writing() || u.repo == nil {
		return
	}
	repo := u.repo
	u.stv.busy = true
	u.background(func() func() {
		snap, err := repo.DropStash(context.Background(), ref)
		return func() {
			if repo != u.repo {
				return
			}
			u.stv.busy = false
			if snap.Root != "" {
				u.setSnapshot(snap)
			}
			u.loadNavLists()
			if err != nil {
				u.report(err)
				return
			}
			if u.stv.open && u.stv.item.Ref == ref {
				u.leaveStash()
			}
		}
	})
}

func (u *UI) openStashBranchForm(st workflow.Stash) {
	u.br.confirm = branchConfirm{}
	u.br.form = branchForm{kind: "stashBranch", open: true, from: st.Ref, label: st.Message, switchAfter: true}
}

func (u *UI) openStashRenameForm(st workflow.Stash) {
	u.br.confirm = branchConfirm{}
	u.br.form = branchForm{kind: "stashRename", open: true, from: st.Ref, name: st.Message, label: st.Message}
}

func (u *UI) saveStashPatch(st workflow.Stash) {
	fn := u.opts.ChooseSaveFile
	if fn == nil || u.repo == nil {
		return
	}
	repo := u.repo
	suggested := strings.TrimSpace(st.Message) + ".patch"
	u.background(func() func() {
		path, err := fn(u.t("stashSavePatch"), suggested)
		if err != nil || path == "" {
			return func() {
				if err != nil {
					u.report(err)
				}
			}
		}
		detail, err := repo.StashShow(context.Background(), st.Ref)
		if err != nil {
			return func() { u.report(err) }
		}
		var b strings.Builder
		for _, f := range detail.Files {
			d, derr := repo.StashDiff(context.Background(), st.Ref, f.Path)
			if derr != nil {
				return func() { u.report(derr) }
			}
			b.WriteString(d.Patch)
			if !strings.HasSuffix(d.Patch, "\n") {
				b.WriteByte('\n')
			}
		}
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			return func() { u.report(err) }
		}
		return nil
	})
}
