package git

import (
	"bytes"
	"fmt"
	"strconv"
)

// Numstat is one path of git diff --numstat -z. Binary marks a change Git
// counts as "-" lines rather than a number, so Added and Removed are then 0
// and not meaningful.
type Numstat struct {
	Path, OrigPath string
	Added, Removed int
	Binary         bool
}

// NumstatArgs are the arguments ParseNumstat expects, less the range or
// --cached selector the caller adds first.
var NumstatArgs = []string{"--numstat", "-z", "--no-ext-diff", "--no-color", "--find-renames"}

// ParseNumstat parses git diff --numstat -z output. Each record is
// "added\tremoved\t" followed by the path, or, for a rename, an empty path
// and then the old and new paths as two more NUL-terminated fields.
func ParseNumstat(out []byte) ([]Numstat, error) {
	fields := bytes.Split(out, []byte{0})
	var stats []Numstat
	for i := 0; i < len(fields); i++ {
		head := fields[i]
		if len(head) == 0 {
			continue
		}
		tabs := bytes.SplitN(head, []byte{'\t'}, 3)
		if len(tabs) != 3 {
			return nil, fmt.Errorf("git diff --numstat: malformed %q", head)
		}
		n := Numstat{}
		if err := counts(&n, tabs[0], tabs[1]); err != nil {
			return nil, err
		}
		n.Path = string(tabs[2])
		if n.Path == "" { // a rename: two more NUL-terminated fields follow.
			if i+2 >= len(fields) {
				return nil, fmt.Errorf("git diff --numstat: rename without paths %q", head)
			}
			n.OrigPath, n.Path = string(fields[i+1]), string(fields[i+2])
			i += 2
		}
		stats = append(stats, n)
	}
	return stats, nil
}

// counts fills n from the added and removed columns, where "-" marks a
// binary change Git does not count in lines.
func counts(n *Numstat, added, removed []byte) error {
	if bytes.Equal(added, []byte("-")) || bytes.Equal(removed, []byte("-")) {
		n.Binary = true
		return nil
	}
	a, err := strconv.Atoi(string(added))
	if err != nil {
		return fmt.Errorf("git diff --numstat: bad added %q", added)
	}
	r, err := strconv.Atoi(string(removed))
	if err != nil {
		return fmt.Errorf("git diff --numstat: bad removed %q", removed)
	}
	n.Added, n.Removed = a, r
	return nil
}
