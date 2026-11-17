package git

import (
	"strings"
)

// Worktree is one entry of git worktree list.
type Worktree struct {
	Path string
	// Head is the checked-out commit, empty before the first commit.
	Head string
	// Branch is the checked-out branch without refs/heads/, empty when
	// detached.
	Branch   string
	Detached bool
	Bare     bool
	// Locked is set when the worktree is locked; LockReason may be empty.
	Locked     bool
	LockReason string
	// Prunable is set when Git would prune the entry, with its reason.
	Prunable       bool
	PrunableReason string
}

// WorktreeArgs are the arguments ParseWorktrees expects.
var WorktreeArgs = []string{"worktree", "list", "--porcelain", "-z"}

// ParseWorktrees parses NUL-separated porcelain output: attributes end in
// NUL and an empty attribute ends an entry. The first entry is the main
// worktree.
func ParseWorktrees(out []byte) []Worktree {
	var list []Worktree
	var cur *Worktree
	for _, attr := range strings.Split(string(out), "\x00") {
		if attr == "" {
			cur = nil
			continue
		}
		key, value, _ := strings.Cut(attr, " ")
		if key == "worktree" {
			list = append(list, Worktree{Path: value})
			cur = &list[len(list)-1]
			continue
		}
		if cur == nil {
			continue
		}
		switch key {
		case "HEAD":
			if strings.Trim(value, "0") != "" {
				cur.Head = value
			}
		case "branch":
			cur.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "detached":
			cur.Detached = true
		case "bare":
			cur.Bare = true
		case "locked":
			cur.Locked, cur.LockReason = true, value
		case "prunable":
			cur.Prunable, cur.PrunableReason = true, value
		}
	}
	return list
}
