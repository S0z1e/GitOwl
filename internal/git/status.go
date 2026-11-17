package git

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Kind is how a file changed.
type Kind uint8

const (
	Modified Kind = iota
	Added
	Deleted
	Renamed
	Copied
	TypeChanged
	Untracked
	Conflicted
)

// Entry is one path in git status.
type Entry struct {
	// Path is the current path; OrigPath the source of a rename or copy.
	Path, OrigPath string
	// Index and Worktree are the status letters, '.' when unchanged.
	Index, Worktree byte
	Untracked       bool
	Conflicted      bool
	// Submodule is set for gitlink entries.
	Submodule bool
	// HeadHash and IndexHash are the object names in HEAD and the index.
	HeadHash, IndexHash string
}

// IndexKind returns how the staged side changed.
func (e Entry) IndexKind() Kind { return kindOf(e.Index) }

// WorktreeKind returns how the unstaged side changed.
func (e Entry) WorktreeKind() Kind { return kindOf(e.Worktree) }

func kindOf(b byte) Kind {
	switch b {
	case 'A':
		return Added
	case 'D':
		return Deleted
	case 'R':
		return Renamed
	case 'C':
		return Copied
	case 'T':
		return TypeChanged
	}
	return Modified
}

// Status is the parsed output of git status --porcelain=v2 --branch.
type Status struct {
	// Head is the commit checked out; empty in a repository without commits.
	Head string
	// Branch is the checked-out branch, empty when HEAD is detached.
	Branch   string
	Detached bool
	// Upstream is the branch's configured upstream, if any, with the counts
	// of commits ahead and behind it as last known locally.
	Upstream       string
	Ahead, Behind  int
	HasAheadBehind bool
	Entries        []Entry
}

// Unborn reports a repository without commits.
func (s Status) Unborn() bool { return s.Head == "" }

// StatusArgs are the arguments ParseStatus expects.
var StatusArgs = []string{"status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all", "--ignore-submodules=none"}

// ParseStatus parses NUL-separated porcelain v2 output.
func ParseStatus(out []byte) (Status, error) {
	var s Status
	fields := bytes.Split(out, []byte{0})
	for i := 0; i < len(fields); i++ {
		line := string(fields[i])
		if line == "" {
			continue
		}
		switch line[0] {
		case '#':
			parseHeader(&s, line)
		case '?':
			s.Entries = append(s.Entries, Entry{Path: line[2:], Index: '.', Worktree: '?', Untracked: true})
		case '!':
		case '1', '2', 'u':
			// Fields: 1 XY sub mH mI mW hH hI path
			//         2 XY sub mH mI mW hH hI Xscore path NUL origPath
			//         u XY sub m1 m2 m3 mW h1 h2 h3 path
			n := map[byte]int{'1': 9, '2': 10, 'u': 11}[line[0]]
			parts := strings.SplitN(line, " ", n)
			if len(parts) != n || len(parts[1]) != 2 {
				return s, fmt.Errorf("git status: malformed entry %q", line)
			}
			e := Entry{Path: parts[n-1], Index: parts[1][0], Worktree: parts[1][1], Submodule: parts[2][0] == 'S'}
			if line[0] != 'u' {
				e.HeadHash, e.IndexHash = parts[6], parts[7]
			}
			switch line[0] {
			case '2':
				i++
				if i >= len(fields) {
					return s, fmt.Errorf("git status: rename without source %q", line)
				}
				e.OrigPath = string(fields[i])
			case 'u':
				e.Conflicted = true
			}
			s.Entries = append(s.Entries, e)
		default:
			return s, fmt.Errorf("git status: unknown entry %q", line)
		}
	}
	return s, nil
}

func parseHeader(s *Status, line string) {
	key, value, _ := strings.Cut(strings.TrimPrefix(line, "# "), " ")
	switch key {
	case "branch.oid":
		if value != "(initial)" {
			s.Head = value
		}
	case "branch.head":
		if value == "(detached)" {
			s.Detached = true
		} else {
			s.Branch = value
		}
	case "branch.upstream":
		s.Upstream = value
	case "branch.ab":
		a, b, ok := strings.Cut(value, " ")
		if ok {
			s.Ahead, _ = strconv.Atoi(strings.TrimPrefix(a, "+"))
			s.Behind, _ = strconv.Atoi(strings.TrimPrefix(b, "-"))
			s.HasAheadBehind = true
		}
	}
}
