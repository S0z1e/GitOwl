package workflow

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"gitowl/internal/git"
)

// RangeCounts counts the commits local has that remote lacks (ahead) and the
// commits remote has that local lacks (behind).
func (r *Repository) RangeCounts(ctx context.Context, local, remote string) (ahead, behind int, err error) {
	out, err := r.run(ctx, "rev-list", "--left-right", "--count", local+"..."+remote, "--")
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(string(out))
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("unexpected rev-list output %q", out)
	}
	if ahead, err = strconv.Atoi(fields[0]); err != nil {
		return 0, 0, err
	}
	behind, err = strconv.Atoi(fields[1])
	return ahead, behind, err
}

// RangeCommits lists the commits reachable from to but not from from, newest
// first, at most limit of them. Pull's incoming commits are
// RangeCommits(HEAD, target) and Push's outgoing commits are
// RangeCommits(target, HEAD).
func (r *Repository) RangeCommits(ctx context.Context, from, to string, limit int) ([]CommitInfo, error) {
	args := []string{"log", "-z", "--no-color", "--no-notes", "--format=" + git.LogFormat,
		"--max-count=" + strconv.Itoa(limit), from + ".." + to, "--"}
	out, err := r.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	commits, _, err := pageOf(out, limit)
	return commits, err
}
