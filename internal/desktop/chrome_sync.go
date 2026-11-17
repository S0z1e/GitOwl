package desktop

import (
	"context"
	"gitowl/internal/desktop/designicons"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/git"
	"gitowl/internal/workflow"
)

const chromeSyncPopoverW = 440

func (u *UI) chromeFetchButton(c *ui.Context, tk kitTokens, iconOnly, seg bool) {
	if u.sync.op == opFetch {
		u.chromeCancelButton(c, tk, iconOnly, u.t("fetching", "remote", u.sync.remote))
		return
	}
	// Only a standing reason dims Fetch. A write in flight leaves it lit,
	// so the toolbar does not flash on every stage; chromeFetch ignores the
	// click meanwhile.
	off := u.fetchDims()
	b := chromeToolButtonSeg(c, designicons.Fetch(), u.t("fetch"), tk, iconOnly, off, "", seg, false)
	// With more than one remote, Fetch opens a menu of All remotes and each
	// remote. With one, it fetches that remote at once.
	multi := len(u.sync.info.Remotes) > 1
	if off {
		if block := u.fetchBlock(); block != "" && block != "busy" {
			b.Tooltip(u.t(block))
		}
	} else if multi {
		b.Tooltip(u.fetchTip(""))
	} else {
		b.Tooltip(u.fetchTip(u.sync.info.DefaultFetch()))
	}
	u.chromeMarkFocus(b)
	if b.Clicked() {
		if multi {
			u.chrome.fetchMenuOpen = true
		} else {
			u.chromeFetch("")
		}
	}
	if multi {
		overlayPopover(c, b, &u.chrome.fetchMenuOpen, func() {
			u.chromeFetchMenu(c, tk)
		})
	}
}

// chromeFetchMenu lists All remotes, then each remote, for Fetch.
func (u *UI) chromeFetchMenu(c *ui.Context, tk kitTokens) {
	ui.Column(c).Gap(2).Padding(4).MinWidth(200).Children(func() {
		all := u.chromeFetchMenuItem(c, tk, u.t("fetchAllRemotes"), false)
		if all.Clicked() {
			u.chrome.fetchMenuOpen = false
			u.chromeFetchAll()
		}
		for _, r := range u.sync.info.Remotes {
			name := r.Name
			item := u.chromeFetchMenuItem(c, tk, name, name == u.sync.info.UpstreamRemote)
			if item.Clicked() {
				u.chrome.fetchMenuOpen = false
				u.chromeFetch(name)
			}
		}
	})
}

// chromeFetchMenuItem is one row of the Fetch menu. An upstream remote is
// marked, as the Pull and Push popovers mark it.
func (u *UI) chromeFetchMenuItem(c *ui.Context, tk kitTokens, label string, upstream bool) ui.Element {
	item := ui.ButtonBase(c).Label(label).FillWidth().Height(32).Padding(0, 12).
		Radius(6).Justify(ui.Start).AlignItems(ui.Center)
	item.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(13).SingleLine()
			if upstream {
				ui.Text(c, u.t("upstream")).FontSize(12).TextColor(tk.muted)
			}
		})
	})
	return item
}

func (u *UI) holdSyncPopover() {
	s := &u.sync.pop
	if !s.nameOpen && !s.addOpen {
		return
	}
	switch s.kind {
	case "pull":
		s.pullOpen = true
	case "push":
		s.pushOpen = true
	}
}

func (u *UI) chromePullButton(c *ui.Context, tk kitTokens, iconOnly, seg bool) {
	u.holdSyncPopover()
	if u.sync.op == opPull {
		u.chromeCancelButton(c, tk, iconOnly, u.t("pulling", "upstream", u.sync.remote))
		return
	}
	// Only a missing repository dims Pull. A write in flight leaves it lit,
	// so the toolbar does not flash on every stage; the click is ignored.
	off := u.repo == nil
	count := ""
	if u.snap.HasAheadBehind && u.snap.Behind > 0 {
		count = "↓" + strconv.Itoa(u.snap.Behind)
	}
	b := chromeToolButtonSeg(c, designicons.Pull(), u.t("pull"), tk, iconOnly, off, count, seg, seg && !iconOnly)
	b.Tooltip(u.t("pull"))
	u.chromeMarkFocus(b)
	if b.Clicked() && !off && !u.writing() {
		u.openPullPopover()
	}
	overlayPopover(c, b, &u.sync.pop.pullOpen, func() {
		u.chromePullPopover(c, tk)
	})
	u.holdSyncPopover()
}

func (u *UI) chromePushButton(c *ui.Context, tk kitTokens, iconOnly, seg bool) {
	u.holdSyncPopover()
	if u.sync.op == opPush {
		u.chromeCancelButton(c, tk, iconOnly, u.t("pushing", "target", u.sync.remote))
		return
	}
	// Only a missing repository dims Push; see chromePullButton.
	off := u.repo == nil
	count := u.chromePushAheadCount()
	b := chromeToolButtonSeg(c, designicons.Push(), u.t("push"), tk, iconOnly, off, count, seg, seg && !iconOnly)
	target := u.chromeSyncTarget()
	if target != "" {
		b.Tooltip(u.t("pushTo", "branch", u.snap.Branch, "target", target))
	} else {
		b.Tooltip(u.t("push"))
	}
	u.chromeMarkFocus(b)
	if b.Clicked() && !off && !u.writing() {
		u.openPushPopover()
	}
	overlayPopover(c, b, &u.sync.pop.pushOpen, func() {
		u.chromePushPopover(c, tk)
	})
	u.holdSyncPopover()
}

func (u *UI) chromeCancelButton(c *ui.Context, tk kitTokens, iconOnly bool, status string) {
	label := status + " — " + u.t("cancelRemote")
	var b ui.Element
	if iconOnly {
		b = ui.ButtonBase(c).Size(kitToolbarH, kitToolbarH).Radius(12).
			Background(tk.surf).Border(1, tk.bd2).Label(label)
		b.Children(func() { kitSpinner(c, tk, 14) })
	} else {
		b = ui.ButtonBase(c).Height(kitToolbarH).Padding(0, 10).Gap(6).Radius(12).
			Background(tk.surf).Border(1, tk.bd2).Label(label).AlignItems(ui.Center)
		b.Children(func() {
			kitSpinner(c, tk, 14)
			ui.Text(c, u.t("cancelRemote")).FontSize(13).FontWeight(500).SingleLine()
		})
	}
	b.Tooltip(status)
	if b.Clicked() {
		u.cancelRemote()
	}
}

func kitSpinner(c *ui.Context, tk kitTokens, size float32) {
	e := ui.Box(c).Size(size, size).Radius(size/2).Border(2, tk.accent)
	e.Loop("spin", time.Second, ui.Linear)
}

func (u *UI) openPullPopover() {
	if u.repo == nil || u.writing() {
		return
	}
	info := u.sync.info
	remote := info.UpstreamRemote
	if remote == "" {
		remote = info.DefaultFetch()
	}
	u.sync.pop = syncPopover{
		kind:      "pull",
		pullOpen:  true,
		remote:    remote,
		branch:    info.UpstreamBranch,
		strategy:  workflow.PullMerge,
		autostash: true,
	}
	u.loadPopoverCommits()
}

// chromeRemoteItems is the context menu of a remote's folder in the Remotes
// group. Pull opens the Pull popover on that remote, so the run is still
// confirmed there.
func (u *UI) chromeRemoteItems(m *ui.Menu, remote string) {
	if m.Item(u.t("remoteMenuPull", "remote", remote)).Disabled(u.writing() || u.repo == nil).Chosen() {
		u.openPullPopoverFor(remote)
	}
}

// openPullPopoverFor opens the Pull popover on remote. The branch is the
// upstream branch when remote is the upstream remote, otherwise the current
// branch name on remote, or its first branch.
func (u *UI) openPullPopoverFor(remote string) {
	if u.repo == nil || u.writing() {
		return
	}
	u.sync.pop = syncPopover{
		kind:      "pull",
		pullOpen:  true,
		strategy:  workflow.PullMerge,
		autostash: true,
	}
	u.selectPopoverRemote(remote)
	u.loadPopoverCommits()
}

// remoteHasBranch reports whether remote already has a branch named branch.
func (u *UI) remoteHasBranch(remote, branch string) bool {
	ref := remote + "/" + branch
	for _, rb := range u.br.data.Remote {
		if rb.Name == ref {
			return true
		}
	}
	return false
}

// pushCreatesBranch reports whether the push in the popover makes a new
// branch on the remote, because the remote has no branch of that name.
func (u *UI) pushCreatesBranch() bool {
	s := &u.sync.pop
	return s.kind == "push" && s.remote != "" && s.branch != "" && !u.remoteHasBranch(s.remote, s.branch)
}

func (u *UI) openPushPopover() {
	if u.repo == nil || u.writing() {
		return
	}
	info := u.sync.info
	remote, branch := info.UpstreamRemote, info.UpstreamBranch
	if p := info.Push; p != nil {
		remote, branch = p.Remote, p.Branch
	}
	if remote == "" {
		remote = info.PublishRemote()
	}
	u.sync.pop = syncPopover{
		kind:     "push",
		pushOpen: true,
		remote:   remote,
		branch:   branch,
	}
	// A branch with no upstream goes to the remote under its own name. The
	// remote branch is created when the remote has no branch of that name,
	// and it becomes the upstream, so one Push publishes the branch.
	if info.UpstreamRemote == "" && u.snap.Branch != "" && remote != "" {
		s := &u.sync.pop
		s.branch, s.setUpstream = u.snap.Branch, true
		s.newBranch = !slices.Contains(u.popoverBranches(remote), s.branch)
	}
	u.loadPopoverCommits()
}

func (u *UI) chromePullPopover(c *ui.Context, tk kitTokens) {
	s := &u.sync.pop
	target := u.chromeSyncTarget()
	ui.Column(c).Width(chromeSyncPopoverW).Gap(0).Radius(16).Background(tk.surf).Clip().Children(func() {
		u.chromeSyncHeader(c, tk, designicons.Pull(), u.t("pull"), u.t("pullTarget",
			"remote", s.remote, "branch", s.branch, "local", u.snap.Branch))
		u.chromeRemoteBranchPickers(c, tk)
		ui.Box(c).Height(8)
		ui.Box(c).Padding(10, 18, 6, 18).Children(func() {
			chromeSyncSection(c, tk, u.t("pullStrategy"))
		})
		ui.Row(c).Padding(0, 18, 4, 18).Children(func() {
			mode := int(s.strategy)
			ui.Segmented(c, &mode, u.t("pullMerge"), u.t("pullRebase"), u.t("pullFastForward"))
			s.strategy = workflow.PullStrategy(mode)
		})
		ui.Box(c).Padding(6, 8, 0, 8).Children(func() {
			u.chromeSyncCheck(c, tk, &s.autostash, u.t("pullAutostash"), tk.text)
			u.chromeSyncCheck(c, tk, &s.prune, u.t("pullPrune"), tk.text)
		})
		ui.Box(c).Padding(12, 18, 6, 18).Children(func() {
			chromeSyncSection(c, tk, u.t("incoming"))
		})
		u.chromeSyncCommits(c, tk, s.incoming, u.chromeIncomingSummary())
		u.chromeSyncFooter(c, tk, designicons.Pull(), u.t("pull"), u.pullActionReason(), func() {
			u.chromeConfirmPull()
		}, u.t("pullRun", "target", target), u.chromeIncomingSummary())
	})
}

func (u *UI) chromePushPopover(c *ui.Context, tk kitTokens) {
	s := &u.sync.pop
	target := u.chromeSyncTarget()
	ui.Column(c).Width(chromeSyncPopoverW).Gap(0).Radius(16).Background(tk.surf).Clip().Children(func() {
		u.chromeSyncHeader(c, tk, designicons.Push(), u.t("push"), u.t("pushTargetHeader",
			"local", u.snap.Branch, "remote", s.remote, "branch", s.branch))
		u.chromeRemoteBranchPickers(c, tk)
		ui.Box(c).Padding(12, 18, 4, 18).Children(func() {
			chromeSyncSection(c, tk, u.t("pushOptions"))
		})
		ui.Box(c).Padding(6, 8, 0, 8).Children(func() {
			u.chromeSyncCheck(c, tk, &s.setUpstream, u.t("pushSetUpstream", "branch", u.snap.Branch), tk.text)
			u.chromeSyncCheck(c, tk, &s.pushTags, u.t("pushTags"), tk.text)
			u.chromeSyncCheck(c, tk, &s.forceLease, u.t("pushForceLease"), tk.del)
		})
		ui.Box(c).Padding(12, 18, 6, 18).Children(func() {
			chromeSyncSection(c, tk, u.t("outgoing"))
		})
		u.chromeSyncCommits(c, tk, s.outgoing, u.chromeOutgoingSummary())
		run := ""
		if target != "" {
			run = u.t("pushTo", "branch", u.snap.Branch, "target", target)
		}
		action := u.t("push")
		if u.pushCreatesBranch() {
			action = u.t("pushAndCreate")
		}
		u.chromeSyncFooter(c, tk, designicons.Push(), action, u.pushActionReason(), func() {
			u.chromeConfirmPush()
		}, run, u.chromeOutgoingSummary())
	})
}

func (u *UI) chromeSyncHeader(c *ui.Context, tk kitTokens, icon *ui.SVG, title, subtitle string) {
	ui.Row(c).Gap(12).Padding(16, 18, 4, 18).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(30, 30).Radius(9).Background(tk.accent.Alpha(.1)).Center().Children(func() {
			ui.Icon(c, icon).Size(16, 16).TextColor(tk.acx)
		})
		ui.Column(c).Gap(2).MinWidth(0).Grow(1).Children(func() {
			ui.Text(c, title).FontWeight(600).FontSize(15).TextColor(tk.text)
			if subtitle != "" {
				ui.Text(c, subtitle).FontSize(12).TextColor(tk.muted).Font("Menlo").SingleLine().Ellipsis("…")
			}
		})
	})
}

func (u *UI) chromeRemoteBranchPickers(c *ui.Context, tk kitTokens) {
	s := &u.sync.pop
	var remotes []string
	for _, r := range u.sync.info.Remotes {
		remotes = append(remotes, r.Name)
	}
	ui.Row(c).Gap(8).Padding(0, 18).AlignItems(ui.Center).Children(func() {
		if len(remotes) == 0 {
			u.chromeRemotePicker(c, tk)
		} else {
			idx := slices.Index(remotes, s.remote)
			if idx < 0 {
				// Pull and Push start on the first remote, as the design does.
				u.selectPopoverRemote(remotes[0])
				idx = 0
			}
			mode := idx
			ui.Segmented(c, &mode, remotes...)
			if mode != idx && mode >= 0 && mode < len(remotes) {
				u.selectPopoverRemote(remotes[mode])
				u.loadPopoverCommits()
			}
		}
		ui.Box(c).Grow(1)
		add := ui.ButtonBase(c).Height(26).Padding(0, 12).Gap(5).Radius(999).
			Background(tk.fill).Label(u.t("addRemote")).AlignItems(ui.Center)
		add.Children(func() {
			ui.Icon(c, designicons.Plus()).Size(12, 12).TextColor(tk.text)
			ui.Text(c, u.t("addRemote")).FontSize(12).FontWeight(500).TextColor(tk.text).SingleLine()
		})
		u.focusReturn(add, "addRemote")
		u.chromeMarkFocus(add)
		if add.Clicked() {
			s.addOpen, s.addName, s.addURL, s.formErr = true, "", "", nil
		}
	})
	ui.Box(c).Height(10)
	ui.Row(c).FillWidth().Padding(0, 18).Children(func() {
		ui.Row(c).FillWidth().Height(34).Radius(10).Gap(8).Background(tk.surf).
			Border(1, tk.bd2).Padding(0, 12).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, designicons.Search()).Size(14, 14).TextColor(tk.muted)
			field := ui.TextInputBase(c, &s.branchFilter).Grow(1).MinWidth(0).FillWidth().
				Label(u.t("popoverFilterBranches")).Placeholder(u.t("popoverFilterBranches"))
			u.chromeMarkFocus(field)
		})
	})
	ui.Box(c).Height(6)
	names := popoverFilter(u.popoverBranches(s.remote), s.branchFilter)
	// A long list scrolls within six rows, so the popover keeps its size.
	ui.Scroll(c).Height(float32(min(len(names), 6) * 36)).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Padding(0, 10).Gap(2).Children(func() {
			for _, name := range names {
				u.chromeBranchChoice(c, tk, name)
			}
		})
	})
	if s.kind == "push" {
		nb := ui.ButtonBase(c).FillWidth().Height(32).Padding(0, 18).Gap(8).Justify(ui.Start).
			AlignItems(ui.Center).Label(u.t("newBranchOnOrigin"))
		nb.Children(func() {
			ui.Icon(c, designicons.Plus()).Size(14, 14).TextColor(tk.acx)
			ui.Text(c, u.t("newBranchOnOrigin")).FontSize(13).TextColor(tk.acx).SingleLine()
		})
		u.focusReturn(nb, "newBranchOnOrigin")
		u.chromeMarkFocus(nb)
		if nb.Clicked() {
			s.nameOpen, s.name, s.formErr = true, "", nil
		}
	}
}

// popoverFilter keeps the names that contain text, ignoring case. An empty
// text keeps them all.
func popoverFilter(names []string, text string) []string {
	if text == "" {
		return names
	}
	var out []string
	for _, n := range names {
		if strings.Contains(strings.ToLower(n), strings.ToLower(text)) {
			out = append(out, n)
		}
	}
	return out
}

// chromeSyncSection is a popover section's label: small, uppercase and
// muted, as the design's section labels are.
func chromeSyncSection(c *ui.Context, tk kitTokens, label string) {
	ui.Text(c, strings.ToUpper(label)).FontSize(11).FontWeight(600).LetterSpacing(.05).
		TextColor(tk.muted).SingleLine()
}

func (u *UI) chromeRemotePicker(c *ui.Context, tk kitTokens) {
	s := &u.sync.pop
	label := s.remote
	if label == "" {
		label = u.t("publishChooseRemote")
	}
	trigger := ui.ButtonBase(c).Height(32).Padding(0, 10).Gap(5).Radius(10).
		Background(tk.fill).Border(1, tk.bd2).Label(u.t("publishRemote")).AlignItems(ui.Center)
	trigger.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(500).SingleLine()
		if s.remote != "" && s.remote == u.sync.info.UpstreamRemote {
			ui.Text(c, u.t("upstream")).FontSize(12).TextColor(tk.muted).SingleLine()
		}
	})
	open := ui.Local(trigger, "remotes", func() bool { return false })
	if open == nil {
		return
	}
	if trigger.Clicked() {
		*open = true
	}
	overlayPopover(c, trigger, open, func() {
		ui.Column(c).Gap(2).Padding(4).MinWidth(160).Children(func() {
			for _, r := range u.sync.info.Remotes {
				name := r.Name
				item := ui.ButtonBase(c).Label(name).FillWidth().Height(32).Padding(0, 12).
					Radius(6).Justify(ui.Start).AlignItems(ui.Center)
				if name == s.remote {
					item.Background(tk.sel)
				}
				item.Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, name).FontSize(13).SingleLine()
						if name == u.sync.info.UpstreamRemote {
							ui.Text(c, u.t("upstream")).FontSize(12).TextColor(tk.muted)
						}
					})
				})
				if item.Clicked() {
					u.selectPopoverRemote(name)
					*open = false
				}
			}
			item := ui.ButtonBase(c).Label(u.t("addRemote")).FillWidth().Height(32).Padding(0, 12).
				Radius(6).Justify(ui.Start).AlignItems(ui.Center)
			item.Children(func() {
				ui.Text(c, u.t("addRemote")).FontSize(13).TextColor(tk.acx).SingleLine()
			})
			if item.Clicked() {
				s.addOpen, s.addName, s.addURL, s.formErr = true, "", "", nil
				*open = false
			}
		})
	})
}

func (u *UI) chromeBranchChoice(c *ui.Context, tk kitTokens, name string) {
	s := &u.sync.pop
	on := name == s.branch
	row := ui.ButtonBase(c).FillWidth().Height(34).Padding(0, 10).Gap(10).Radius(10).
		Justify(ui.Start).Label(name).AlignItems(ui.Center)
	if on {
		row.Background(tk.accent.Alpha(.12))
	}
	row.Children(func() {
		chromeRadioFace(c, on, tk)
		ui.Icon(c, designicons.Branch()).Size(14, 14).TextColor(tk.muted)
		weight := 500
		if on {
			weight = 600
		}
		ui.Text(c, name).FontSize(13).FontWeight(weight).SingleLine()
		extra := u.chromeBranchMeta(name)
		newOnRemote := s.kind == "push" && !u.remoteHasBranch(s.remote, name)
		color := tk.muted
		if newOnRemote {
			extra, color = u.t("newRemoteBranchBadge"), tk.acx
		}
		if extra != "" {
			ui.Box(c).Grow(1)
			ui.Text(c, extra).FontSize(12).TextColor(color).SingleLine()
		}
	})
	if row.Clicked() {
		s.branch, s.newBranch = name, false
		u.loadPopoverCommits()
	}
}

// popoverCountMeta is a non-target branch's meta: the count that the run would
// move, with Pull showing what it brings in and Push what it sends out. A
// branch with no local counterpart shows nothing.
func (u *UI) popoverCountMeta(name string) string {
	s := u.sync.pop
	c, ok := s.counts[name]
	if !ok {
		return ""
	}
	if s.kind == "pull" {
		if c.behind > 0 {
			return u.t("branchBehind", "count", c.behind)
		}
	} else if c.ahead > 0 {
		return u.t("branchAhead", "count", c.ahead)
	}
	if c.behind > 0 {
		return u.t("branchBehind", "count", c.behind)
	}
	if c.ahead > 0 {
		return u.t("branchAhead", "count", c.ahead)
	}
	return u.t("upToDateShort")
}

func (u *UI) chromeBranchMeta(name string) string {
	if !u.syncTargetIs(u.sync.pop.remote, name) {
		return u.popoverCountMeta(name)
	}
	if u.snap.HasAheadBehind && u.snap.Behind > 0 && u.sync.pop.kind == "pull" {
		return u.t("trackingBehind", "count", u.snap.Behind)
	}
	if u.snap.HasAheadBehind && u.snap.Ahead > 0 && u.sync.pop.kind == "push" {
		return u.t("trackingAhead", "count", u.snap.Ahead)
	}
	if u.snap.HasAheadBehind {
		return u.t("upToDateShort")
	}
	return ""
}

// chromeSyncCheck is one option of a popover: the design's 18 check box and
// its label, in a 30-high row that toggles on a click anywhere in it.
func (u *UI) chromeSyncCheck(c *ui.Context, tk kitTokens, on *bool, label string, color ui.Color) {
	row := ui.ButtonBase(c).FillWidth().Height(30).Padding(0, 10).Gap(10).Radius(10).
		Justify(ui.Start).AlignItems(ui.Center).Label(label)
	row.Children(func() {
		chromeCheckFace(c, *on, tk)
		ui.Text(c, label).FontSize(13).TextColor(color).SingleLine()
	})
	if row.Clicked() {
		*on = !*on
	}
}

// chromeRadioFace draws the design's radio circle, with the accent dot when
// it is the chosen one. It passes presses to the row that holds it.
func chromeRadioFace(c *ui.Context, on bool, tk kitTokens) {
	border := tk.cxb
	if on {
		border = tk.accent
	}
	face := ui.Box(c).Size(18, 18).Radius(9).Border(1, border).Background(tk.surf).Center().Shrink(0).PassThrough()
	if on {
		face.Children(func() {
			ui.Box(c).Size(8, 8).Radius(4).Background(tk.accent).PassThrough()
		})
	}
}

// chromeCheckFace draws the design's .cx box, checked with the accent. It
// passes presses to the row that holds it.
func chromeCheckFace(c *ui.Context, on bool, tk kitTokens) {
	border, fill := tk.cxb, ui.Color{}
	if on {
		border, fill = tk.acx, tk.accent
	}
	face := ui.Box(c).Size(18, 18).Radius(6).Border(1, border).Background(fill).Center().Shrink(0).PassThrough()
	if on {
		face.Children(func() {
			ui.Icon(c, designicons.StageCheck()).Size(12, 12).TextColor(tk.onAc)
		})
	}
}

func (u *UI) chromeSyncFooter(c *ui.Context, tk kitTokens, icon *ui.SVG, action, reason string, run func(), runLabel, summary string) {
	ui.Row(c).FillWidth().Gap(8).Padding(12, 18).AlignItems(ui.Center).
		Border(1, tk.border).Children(func() {
		// The reason a run cannot start reads on the left. A ready run shows
		// its count summary there, as the design's footer does; the run label
		// stays off the screen so tests and the button name it uniquely.
		left := reason
		if left == "" {
			left = summary
		}
		if left != "" {
			t := ui.Text(c, left).FontSize(12).TextColor(tk.muted).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			if reason != "" {
				t.Tooltip(reason)
			}
		} else {
			ui.Box(c).Grow(1).MinWidth(0)
		}
		if kitSecondaryButton(c, u.t("cancel"), tk, false).Clicked() {
			u.sync.pop.pullOpen, u.sync.pop.pushOpen, u.sync.pop.kind = false, false, ""
		}
		off := reason != ""
		// Unique accessible name so tests can click the popover action
		// without hitting the toolbar button that opened it.
		acc := action
		if !off && runLabel != "" {
			acc = runLabel
		}
		fill, fg := tk.accent, tk.onAc
		if off {
			fill, fg = tk.dis, tk.surf
		}
		b := ui.ButtonBase(c).Height(34).Padding(0, 16).Radius(12).Gap(6).AlignItems(ui.Center).
			Background(fill).TextColor(fg).Label(acc).Disabled(off)
		b.Children(func() {
			ui.Icon(c, icon).Size(14, 14).TextColor(fg)
			ui.Text(c, action).FontWeight(600).FontSize(13).SingleLine()
		})
		if off {
			b.Tooltip(reason)
		}
		if b.Clicked() && !off {
			run()
		}
	})
}

func (u *UI) chromeSyncDialogs(c *ui.Context) {
	u.chromeNameDialog(c)
	u.chromeAddRemoteDialog(c)
}

func (u *UI) chromeNameDialog(c *ui.Context) {
	s := &u.sync.pop
	if !s.nameOpen {
		return
	}
	u.chromeFormDialog(c, u.t("newBranch"), func(tk kitTokens) {
		ui.Text(c, u.t("newBranch")).FontWeight(600).FontSize(15).TextColor(tk.text)
		ui.Text(c, u.t("newBranchOnOriginFrom", "remote", s.remote)).Wrap().FontSize(13).TextColor(tk.muted)
		ui.Text(c, u.t("branchName")).FontSize(12).TextColor(tk.muted)
		field := ui.TextInput(c, &s.name).Height(kitFieldH).FillWidth().
			Label(u.t("branchName")).Placeholder("feature/name").AutoFocus()
		submit := field.Submitted()
		u.chromeFormButtons(c, tk, u.t("createBranch"), func() {
			s.nameOpen, s.name = false, ""
		}, func() {
			name := strings.TrimSpace(s.name)
			if name == "" {
				return
			}
			s.branch, s.newBranch, s.nameOpen, s.name = name, true, false, ""
		}, submit)
	}, func() {
		s.nameOpen, s.name = false, ""
	})
}

func (u *UI) chromeAddRemoteDialog(c *ui.Context) {
	s := &u.sync.pop
	if !s.addOpen {
		return
	}
	u.chromeFormDialog(c, u.t("addRemoteTitle"), func(tk kitTokens) {
		ui.Text(c, u.t("addRemoteTitle")).FontWeight(600).FontSize(15).TextColor(tk.text)
		ui.Text(c, u.t("addRemoteFrom")).Wrap().FontSize(13).TextColor(tk.muted)
		ui.Text(c, u.t("remoteName")).FontSize(12).TextColor(tk.muted)
		name := ui.TextInput(c, &s.addName).Height(kitFieldH).FillWidth().
			Label(u.t("remoteName")).AutoFocus()
		ui.Text(c, u.t("remoteURL")).FontSize(12).TextColor(tk.muted)
		url := ui.TextInput(c, &s.addURL).Height(kitFieldH).FillWidth().Label(u.t("remoteURL"))
		if s.formErr != nil {
			ui.Text(c, u.t("error"+string(s.formErr.code))).Wrap().TextColor(tk.del)
			if s.formErr.detail != "" {
				ui.Text(c, git.Redact(s.formErr.detail)).Wrap().FontSize(12).TextColor(tk.muted)
			}
		}
		u.chromeFormButtons(c, tk, u.t("addRemoteAction"), func() {
			s.addOpen, s.addName, s.addURL, s.formErr = false, "", "", nil
		}, u.confirmAddRemote, name.Submitted() || url.Submitted())
	}, func() {
		s.addOpen, s.addName, s.addURL, s.formErr = false, "", "", nil
	})
}

func (u *UI) chromeFormDialog(c *ui.Context, label string, body func(tk kitTokens), dismiss func()) {
	tk := kitDesignTokens(c.Theme().Dark)
	ui.Box(c).Key("chromeForm:" + label).Children(func() {
		ui.Overlay(c, func() {
			w, h := c.Size()
			width := min(float32(420), w-40)
			back := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).
				Background(ui.RGBA(0, 0, 0, .18)).Modal().Transition(dialogFade)
			if back.Clicked() || back.OverlayShortcut(0, ui.KeyEscape) {
				dismiss()
			}
			back.Children(func() {
				panel := ui.Column(c).Role(ui.RoleDialog).Label(label).
					Absolute().Left(max(0, (w-width)/2)).Top(max(float32(80), (h-280)/2)).
					Width(width).Padding(20).Gap(12).Radius(14).Background(tk.bg).Clip().
					Shadow(0, 20, 60, 0, ui.RGBA(0, 0, 0, 0.25))
				panel.Clicked()
				panel.Children(func() { body(tk) })
			})
		})
	})
}

func (u *UI) chromeFormButtons(c *ui.Context, tk kitTokens, primary string, cancel, confirm func(), submit bool) {
	ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
		if kitSecondaryButton(c, u.t("cancel"), tk, false).Clicked() {
			cancel()
		}
		if kitPrimaryButton(c, primary, tk, false).Clicked() || submit {
			confirm()
		}
	})
}

func (u *UI) confirmAddRemote() {
	s := &u.sync.pop
	repo := u.repo
	if repo == nil {
		return
	}
	name, url := strings.TrimSpace(s.addName), strings.TrimSpace(s.addURL)
	info, err := repo.AddRemote(context.Background(), name, url)
	if err != nil {
		s.formErr = noticeOf(err)
		return
	}
	u.sync.info, u.sync.loaded = info, true
	s.remote, s.branch, s.newBranch = name, "", false
	s.addOpen, s.addName, s.addURL, s.formErr = false, "", "", nil
}

func (u *UI) selectPopoverRemote(name string) {
	s := &u.sync.pop
	s.remote, s.newBranch = name, false
	if name == u.sync.info.UpstreamRemote {
		s.branch = u.sync.info.UpstreamBranch
		return
	}
	if bs := u.popoverBranches(name); len(bs) > 0 {
		s.branch = bs[0]
		return
	}
	s.branch = ""
}

func (u *UI) popoverBranches(remote string) []string {
	prefix := remote + "/"
	var out []string
	seen := map[string]bool{}
	for _, rb := range u.br.data.Remote {
		if strings.HasPrefix(rb.Name, prefix) {
			name := strings.TrimPrefix(rb.Name, prefix)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	s := u.sync.pop
	if s.newBranch && s.remote == remote && s.branch != "" && !seen[s.branch] {
		out = append(out, s.branch)
	}
	// The current branch leads the list, so it is the first one a run uses.
	if cur := u.snap.Branch; cur != "" {
		if i := slices.Index(out, cur); i > 0 {
			out = append([]string{cur}, slices.Delete(out, i, i+1)...)
		}
	}
	return out
}

// popoverCommitLimit is how many incoming or outgoing commits the popover lists.
const popoverCommitLimit = 5

// popoverCount is how far one remote branch is ahead of and behind the local
// branch of the same name.
type popoverCount struct {
	ahead, behind int
}

// loadPopoverCommits reads the commits the popover's run would move: Pull's
// incoming commits or Push's outgoing ones, and each remote branch's counts
// against its local branch of the same name. It runs in the background when
// the popover opens and when its remote or branch changes, never on every
// frame, and only the latest read applies. A target that does not exist yet
// lists nothing, and the count summary says so instead.
func (u *UI) loadPopoverCommits() {
	s := &u.sync.pop
	s.incoming, s.outgoing, s.counts = nil, nil, nil
	repo := u.repo
	if repo == nil {
		return
	}
	remote, kind := s.remote, s.kind
	target := u.chromeSyncTarget()
	upstream := u.syncTargetIsUpstream()
	// The branch lists live on the UI thread, so the names are gathered here.
	var names []string
	if remote != "" {
		local := map[string]bool{}
		for _, b := range u.br.data.Local {
			local[b.Name] = true
		}
		for _, name := range u.popoverBranches(remote) {
			if local[name] {
				names = append(names, name)
			}
		}
	}
	u.sync.popSeq++
	seq := u.sync.popSeq
	u.background(func() func() {
		ctx := context.Background()
		counts := map[string]popoverCount{}
		for _, name := range names {
			ahead, behind, err := repo.RangeCounts(ctx, name, remote+"/"+name)
			if err == nil {
				counts[name] = popoverCount{ahead: ahead, behind: behind}
			}
		}
		var incoming, outgoing []workflow.CommitInfo
		if target != "" && upstream {
			switch kind {
			case "pull":
				incoming, _ = repo.RangeCommits(ctx, "HEAD", target, popoverCommitLimit)
			case "push":
				outgoing, _ = repo.RangeCommits(ctx, target, "HEAD", popoverCommitLimit)
			}
		}
		return func() {
			if repo != u.repo || seq != u.sync.popSeq {
				return
			}
			s := &u.sync.pop
			s.counts, s.incoming, s.outgoing = counts, incoming, outgoing
		}
	})
}

// chromeSyncCommits lists the commits of a popover section, each with its
// short hash in monospace, or the section's summary when there are none.
func (u *UI) chromeSyncCommits(c *ui.Context, tk kitTokens, commits []workflow.CommitInfo, summary string) {
	if len(commits) == 0 {
		ui.Box(c).Padding(0, 18, 4, 18).Children(func() {
			ui.Text(c, summary).FontSize(12).TextColor(tk.muted).Wrap()
		})
		return
	}
	ui.Column(c).FillWidth().Padding(0, 0, 4, 0).Children(func() {
		for _, cm := range commits {
			ui.Row(c).FillWidth().Height(26).Padding(0, 18).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Width(58).Shrink(0).Children(func() {
					ui.Text(c, cm.Short()).Font("Menlo").FontSize(12).TextColor(tk.muted).SingleLine()
				})
				ui.Text(c, cm.Subject).FontSize(13).TextColor(tk.text).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			})
		}
	})
}

func (u *UI) chromeSyncTarget() string {
	s := u.sync.pop
	if s.remote == "" || s.branch == "" {
		if p := u.sync.info.Push; p != nil {
			return p.String()
		}
		if u.sync.info.UpstreamRemote != "" {
			return u.sync.info.UpstreamRemote + "/" + u.sync.info.UpstreamBranch
		}
		return ""
	}
	return s.remote + "/" + s.branch
}

func (u *UI) syncTargetIsUpstream() bool {
	return u.syncTargetIs(u.sync.pop.remote, u.sync.pop.branch)
}

func (u *UI) syncTargetIs(remote, branch string) bool {
	if u.sync.pop.newBranch || remote == "" || branch == "" {
		return false
	}
	return remote == u.sync.info.UpstreamRemote && branch == u.sync.info.UpstreamBranch
}

func (u *UI) chromePushAheadCount() string {
	if !u.syncTargetIsUpstream() && u.sync.pop.kind != "" {
		return ""
	}
	return u.aheadCount()
}

func (u *UI) chromeIncomingSummary() string {
	target := u.chromeSyncTarget()
	if !u.syncTargetIsUpstream() || target == "" {
		return u.t("noUpstreamCommits")
	}
	if u.snap.HasAheadBehind && u.snap.Behind > 0 {
		return u.t("pullBehindCount", "count", u.snap.Behind, "target", target)
	}
	return u.t("upToDateShort")
}

func (u *UI) chromeOutgoingSummary() string {
	target := u.chromeSyncTarget()
	if u.pushCreatesBranch() && target != "" {
		return u.t("pushCreatesBranch", "target", target)
	}
	if !u.syncTargetIsUpstream() {
		return u.t("noUpstreamCommits")
	}
	if u.snap.HasAheadBehind && u.snap.Ahead > 0 {
		return u.t("pushAheadCount", "count", u.snap.Ahead, "target", target)
	}
	return u.t("upToDateShort")
}

func (u *UI) pullActionReason() string {
	s := &u.sync.pop
	switch {
	case u.snap.Detached:
		return u.t("errorPushDetached")
	case u.snap.Unborn():
		return u.t("errorPushUnborn")
	case len(u.sync.info.Remotes) == 0:
		return u.t("errorNoRemote")
	case s.remote == "" || s.branch == "":
		return u.t("errorChooseRemote")
	case u.revertInProgress():
		return u.t("errorRevertInProgress")
	}
	for _, ch := range u.snap.Unstaged {
		if ch.Conflicted {
			return u.t("errorPullConflicts")
		}
	}
	if !s.autostash && !u.snap.Clean() {
		return u.t("errorPullDirty")
	}
	if u.syncTargetIsUpstream() {
		if s.strategy == workflow.PullFastForwardOnly && u.diverged() {
			return u.t("errorPullDiverged")
		}
		if u.snap.HasAheadBehind && u.snap.Behind == 0 {
			return u.t("pullUpToDate", "branch", u.snap.Branch, "upstream", u.chromeSyncTarget())
		}
	}
	return ""
}

func (u *UI) pushActionReason() string {
	s := &u.sync.pop
	switch {
	case u.snap.Detached:
		return u.t("errorPushDetached")
	case u.snap.Unborn():
		return u.t("errorPushUnborn")
	case len(u.sync.info.Remotes) == 0:
		return u.t("errorNoRemote")
	case s.remote == "" || s.branch == "":
		return u.t("errorChooseRemote")
	}
	if s.newBranch {
		return ""
	}
	if u.syncTargetIsUpstream() && u.snap.HasAheadBehind && u.snap.Ahead == 0 {
		return u.t("pushUpToDate", "target", u.chromeSyncTarget())
	}
	return ""
}

func (u *UI) chromeFetch(remote string) {
	if u.fetchBlock() != "" {
		return
	}
	if remote == "" {
		remote = u.sync.info.DefaultFetch()
	}
	if remote == "" {
		// Several remotes and no upstream or origin: nothing to pick from
		// here, so every remote is fetched.
		u.chromeFetchAll()
		return
	}
	u.remoteOp(opFetch, remote, func(ctx context.Context, r *workflow.Repository) (workflow.Snapshot, error) {
		return r.Fetch(ctx, remote)
	}, func(err error) {
		u.chromeRemoteDone(err, false, "fetchedToast", "fetchedToast", "fetchCancelled", "remote", remote)
	})
}

// chromeFetchAll fetches every configured remote.
func (u *UI) chromeFetchAll() {
	if u.fetchBlock() != "" {
		return
	}
	u.remoteOp(opFetch, u.t("fetchAllRemotes"), func(ctx context.Context, r *workflow.Repository) (workflow.Snapshot, error) {
		return r.FetchAll(ctx)
	}, func(err error) {
		u.chromeRemoteDone(err, false, "fetchedAllToast", "fetchedAllToast", "fetchCancelled")
	})
}

func (u *UI) chromeConfirmPull() {
	if u.pullActionReason() != "" || u.writing() {
		return
	}
	s := u.sync.pop
	opts := workflow.PullOptions{
		Remote:    s.remote,
		Branch:    s.branch,
		Strategy:  s.strategy,
		Autostash: s.autostash,
		Prune:     s.prune,
	}
	u.sync.pop.pullOpen, u.sync.pop.kind = false, ""
	var res workflow.PullResult
	target := opts.Remote + "/" + opts.Branch
	u.remoteOp(opPull, target, func(ctx context.Context, r *workflow.Repository) (workflow.Snapshot, error) {
		snap, rr, err := r.PullWith(ctx, opts)
		res = rr
		return snap, err
	}, func(err error) {
		u.chromeRemoteDone(err, res.Commits == 0, "pulledToast", "pullUpToDate", "pullCancelled",
			"count", res.Commits, "branch", u.snap.Branch, "upstream", target)
	})
}

func (u *UI) chromeConfirmPush() {
	if u.pushActionReason() != "" || u.writing() {
		return
	}
	s := u.sync.pop
	opts := workflow.PushOptions{
		Remote:         s.remote,
		Branch:         s.branch,
		SetUpstream:    s.setUpstream,
		PushTags:       s.pushTags,
		ForceWithLease: s.forceLease,
	}
	u.sync.pop.pushOpen, u.sync.pop.kind = false, ""
	var res workflow.PushResult
	target := opts.Remote + "/" + opts.Branch
	u.remoteOp(opPush, target, func(ctx context.Context, r *workflow.Repository) (workflow.Snapshot, error) {
		snap, rr, err := r.PushWith(ctx, opts)
		res = rr
		return snap, err
	}, func(err error) {
		u.chromeRemoteDone(err, res.UpToDate, "pushedToast", "pushUpToDate", "pushCancelled",
			"branch", u.snap.Branch, "target", target)
	})
}

func (u *UI) chromeRemoteDone(err error, upToDate bool, okKey, evenKey, cancelKey string, kv ...any) {
	switch {
	case err == nil && upToDate:
		u.notice = nil
		u.toast("success", u.t(evenKey, kv...))
	case err == nil:
		u.notice = nil
		u.toast("success", u.t(okKey, kv...))
	case workflow.CodeOf(err) == workflow.CodeCancelled:
		u.toast("", u.t(cancelKey))
	default:
		u.report(err)
	}
}
