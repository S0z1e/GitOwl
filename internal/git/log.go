package git

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// LogEntry is one commit of git log.
type LogEntry struct {
	Hash    string
	Parents []string
	Author  string
	Email   string
	When    time.Time
	Subject string
}

// LogFormat is the --format ParseLog expects, with -z between commits.
const LogFormat = "%H%x1f%P%x1f%an%x1f%ae%x1f%at%x1f%s"

// ParseLog parses git log -z --format=LogFormat.
func ParseLog(out []byte) ([]LogEntry, error) {
	var entries []LogEntry
	for _, rec := range bytes.Split(out, []byte{0}) {
		rec = bytes.TrimLeft(rec, "\n")
		if len(rec) == 0 {
			continue
		}
		f := strings.SplitN(string(rec), "\x1f", 6)
		if len(f) != 6 {
			return nil, fmt.Errorf("git log: malformed record %q", rec)
		}
		sec, err := strconv.ParseInt(f[4], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("git log: bad time %q", f[4])
		}
		e := LogEntry{Hash: f[0], Author: f[2], Email: f[3], When: time.Unix(sec, 0), Subject: f[5]}
		if f[1] != "" {
			e.Parents = strings.Fields(f[1])
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// RawChange is one file of git diff --raw -z.
type RawChange struct {
	Path, OrigPath   string
	Status           byte
	OldMode, NewMode string
}

// Submodule reports a gitlink on either side.
func (c RawChange) Submodule() bool { return c.OldMode == "160000" || c.NewMode == "160000" }

// ParseRaw parses git diff --raw -z output.
func ParseRaw(out []byte) ([]RawChange, error) {
	fields := bytes.Split(out, []byte{0})
	var changes []RawChange
	for i := 0; i < len(fields); i++ {
		head := string(fields[i])
		if head == "" {
			continue
		}
		if head[0] != ':' {
			return nil, fmt.Errorf("git diff --raw: unexpected %q", head)
		}
		p := strings.Fields(head[1:])
		if len(p) != 5 || p[4] == "" {
			return nil, fmt.Errorf("git diff --raw: malformed %q", head)
		}
		c := RawChange{OldMode: p[0], NewMode: p[1], Status: p[4][0]}
		i++
		if i >= len(fields) {
			return nil, fmt.Errorf("git diff --raw: missing path")
		}
		c.Path = string(fields[i])
		if c.Status == 'R' || c.Status == 'C' {
			i++
			if i >= len(fields) {
				return nil, fmt.Errorf("git diff --raw: missing rename target")
			}
			c.OrigPath, c.Path = c.Path, string(fields[i])
		}
		changes = append(changes, c)
	}
	return changes, nil
}
