package desktop

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

// identityPanel is the Commit identity dialog and the identity the commit
// panel shows for the open repository.
type identityPanel struct {
	open bool
	// name and email are the dialog's fields.
	name, email string
	// current is the identity Git reads for the open repository; loaded is
	// set once a read for it completed.
	current workflow.Identity
	loaded  bool
	// loadSeq drops reads that a later read or save has overtaken.
	loadSeq uint64
	// saving blocks a second save while one runs.
	saving bool
	// global makes the next save change Git's global settings, for every
	// repository, instead of this repository's own. Off each time the dialog
	// opens.
	global bool
	// err explains a save that failed; the dialog stays open.
	err *notice
}

// loadIdentity reads the identity of the open repository off the UI thread.
// When fill is set, the dialog's fields take the value read, as they do when
// the dialog opens.
func (u *UI) loadIdentity(fill bool) {
	repo := u.repo
	if repo == nil {
		return
	}
	u.ident.loadSeq++
	seq := u.ident.loadSeq
	u.background(func() func() {
		id, err := repo.Identity(context.Background())
		return func() {
			if repo != u.repo || seq != u.ident.loadSeq || err != nil {
				return
			}
			u.ident.current, u.ident.loaded = id, true
			if fill && u.ident.open {
				u.ident.name, u.ident.email = id.Name, id.Email
			}
		}
	})
}

// openIdentity shows the Commit identity dialog, filled with the identity
// Git reads now.
func (u *UI) openIdentity() {
	if u.repo == nil {
		return
	}
	u.ident.open, u.ident.err, u.ident.global = true, nil, false
	u.ident.name, u.ident.email = u.ident.current.Name, u.ident.current.Email
	u.loadIdentity(true)
}

// saveIdentity records name and email for the open repository, or in Git's
// global settings when global is set. Empty values remove the identity from
// that scope. The dialog closes when the save works.
func (u *UI) saveIdentity(name, email string, global bool) {
	repo := u.repo
	if repo == nil || u.ident.saving {
		return
	}
	// A save overtakes any read still in flight, so an older identity cannot
	// land after the new one.
	u.ident.loadSeq++
	u.ident.saving, u.ident.err = true, nil
	name, email = strings.TrimSpace(name), strings.TrimSpace(email)
	u.background(func() func() {
		id, err := repo.SetIdentity(context.Background(), name, email, global)
		return func() {
			if repo != u.repo {
				return
			}
			u.ident.saving = false
			if err != nil {
				u.ident.err = noticeOf(err)
				return
			}
			u.ident.current, u.ident.loaded = id, true
			u.ident.open, u.ident.name, u.ident.email, u.ident.global = false, "", "", false
		}
	})
}

// closeIdentity dismisses the Commit identity dialog and drops its fields.
func (u *UI) closeIdentity() {
	u.ident.open, u.ident.name, u.ident.email, u.ident.err, u.ident.global = false, "", "", nil, false
}

// identityText is the name and email as Git writes them on a commit; the
// email part is left out when it is empty.
func identityText(id workflow.Identity) string {
	if id.Email == "" {
		return id.Name
	}
	return id.Name + " <" + id.Email + ">"
}

// chromeIdentityDialog is the Commit identity form: the name and email Git
// records on this repository's commits.
func (u *UI) chromeIdentityDialog(c *ui.Context) {
	p := &u.ident
	if !p.open {
		return
	}
	u.chromeFormDialog(c, u.t("identityTitle"), func(tk kitTokens) {
		ui.Text(c, u.t("identityTitle")).FontWeight(600).FontSize(15).TextColor(tk.text)
		ui.Text(c, u.t("identityBody")).Wrap().FontSize(13).TextColor(tk.muted)
		ui.Text(c, u.t("identityName")).FontSize(12).TextColor(tk.muted)
		name := ui.TextInput(c, &p.name).Height(kitFieldH).FillWidth().
			Label(u.t("identityName")).AutoFocus()
		ui.Text(c, u.t("identityEmailOptional")).FontSize(12).TextColor(tk.muted)
		email := ui.TextInput(c, &p.email).Height(kitFieldH).FillWidth().Label(u.t("identityEmailOptional"))
		ui.Text(c, u.identitySource()).FontSize(12).TextColor(tk.muted).Wrap()
		if p.err != nil {
			ui.Text(c, u.t("error"+string(p.err.code))).Wrap().TextColor(tk.del)
			if p.err.detail != "" {
				ui.Text(c, p.err.detail).FontSize(12).Wrap().TextColor(tk.muted)
			}
		}
		submit := name.Submitted() || email.Submitted()
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			on := p.global
			box := kitCheckbox(c, &on, u.t("identityGlobal"), tk, p.saving)
			if box.Clicked() && !p.saving {
				p.global = !p.global
			}
			ui.Text(c, u.t("identityGlobal")).FontSize(13).TextColor(tk.text).Wrap().PassThrough()
		})
		if p.current.Local && p.loaded {
			ui.Row(c).FillWidth().Justify(ui.Start).Children(func() {
				if kitSecondaryButton(c, u.t("identityUseGit"), tk, p.saving).Clicked() && !p.saving {
					u.saveIdentity("", "", false)
				}
			})
		}
		ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
			if kitSecondaryButton(c, u.t("cancel"), tk, false).Clicked() {
				u.closeIdentity()
			}
			if kitPrimaryButton(c, u.t("identitySave"), tk, p.saving).Clicked() || submit {
				if !p.saving {
					u.saveIdentity(p.name, p.email, p.global)
				}
			}
		})
	}, u.closeIdentity)
}

// identitySource says where the identity the dialog shows comes from.
func (u *UI) identitySource() string {
	switch {
	case u.ident.current.Local:
		return u.t("identitySourceLocal")
	case u.ident.current.Set():
		return u.t("identitySourceGlobal")
	}
	return u.t("identitySourceNone")
}

// chromeIdentityLine is the commit panel's line naming the identity commits
// record, with a Change button that opens the Commit identity dialog.
func (u *UI) chromeIdentityLine(c *ui.Context, tk kitTokens) {
	if !u.prefs.CommitIdentity || !u.ident.loaded {
		return
	}
	label, action := u.t("identityNotSet"), u.t("identitySetShort")
	if u.ident.current.Set() {
		label, action = u.t("identityCurrent", "identity", identityText(u.ident.current)), u.t("identityChange")
	}
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(tk.muted).Wrap().Grow(1).MinWidth(0)
		b := ui.ButtonBase(c).Key("commitIdentity").Height(24).Padding(0, 8).Radius(6).
			AlignItems(ui.Center).Justify(ui.Center).Label(action).Shrink(0)
		if b.Hovered() {
			b.Background(tk.text.Alpha(.05))
		}
		b.Children(func() {
			ui.Text(c, action).FontSize(12).FontWeight(600).TextColor(tk.acx).SingleLine()
		})
		u.focusReturn(b, "commitIdentity")
		u.chromeMarkFocus(b)
		if b.Clicked() {
			u.openIdentity()
		}
	})
}
