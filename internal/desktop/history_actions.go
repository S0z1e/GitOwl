package desktop

// The design's commit detail has two action groups (history.md §6): Copy
// hash, a short 24-DIP button in the metadata row, and the action column
// (Checkout, Create branch…, Revert…) below it. This ticket wires Copy hash
// to the clipboard and leaves the three actions as disabled placeholders,
// each with a "coming soon" tooltip; later tickets give them behaviour.

// commitLabels returns the branch pills a commit row shows, from the branch
// panel's real refs (history.md §5.2): the local branches whose tip is this
// commit, then the remote-tracking branches, each tagged local or remote so
// the row paints the accent or the grey capsule.
type commitLabel struct {
	name   string
	remote bool
	tag    bool
	// current marks the checked-out local branch, the design's accent pill.
	current bool
}

func (u *UI) commitLabels(hash string) []commitLabel {
	var out []commitLabel
	for _, b := range u.br.data.Local {
		if b.Head == hash {
			out = append(out, commitLabel{name: b.Name, current: b.Current})
		}
	}
	for _, b := range u.br.data.Remote {
		if b.Head == hash {
			out = append(out, commitLabel{name: b.Name, remote: true})
		}
	}
	for _, t := range u.chrome.navTagRefs {
		if t.Hash == hash {
			out = append(out, commitLabel{name: t.Name, tag: true})
		}
	}
	return out
}
