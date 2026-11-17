package workflow

import (
	"bytes"
	"context"
	"path"
	"strconv"
	"strings"
)

// Treatment is how much of a staged file's diff a draft request shows.
type Treatment uint8

const (
	// TreatFull shows the diff as Git does, with three lines of context.
	TreatFull Treatment = iota
	// TreatLessContext shows every change with fewer unchanged lines.
	TreatLessContext
	// TreatShortened leaves out some changed lines to fit.
	TreatShortened
	// TreatSummary gives statistics only, because of the file's kind.
	TreatSummary
	// TreatOmitted gives statistics only, because there was no room.
	TreatOmitted
)

// Why a file's diff is not shown in full. Reasons double as message keys,
// prefixed with "note".
const (
	ReasonLock      = "Lock"
	ReasonGenerated = "Generated"
	ReasonBinary    = "Binary"
	ReasonSubmodule = "Submodule"
	ReasonDeleted   = "Deleted"
	ReasonNoText    = "NoText"
	ReasonContext   = "Context"
	ReasonBudget    = "Budget"
	ReasonLongLines = "LongLines"
	ReasonReadLimit = "ReadLimit"
	ReasonManyFiles = "ManyFiles"
)

// reasonText is how the request tells the model about a reason.
var reasonText = map[string]string{
	ReasonLock:      "lock file, statistics only",
	ReasonGenerated: "generated file, statistics only",
	ReasonBinary:    "binary, no text diff",
	ReasonSubmodule: "submodule, no text diff",
	ReasonDeleted:   "deleted, content not shown",
	ReasonNoText:    "no text lines changed",
	ReasonContext:   "fewer unchanged lines shown",
	ReasonBudget:    "some changes left out to fit",
	ReasonLongLines: "long lines cut",
	ReasonReadLimit: "only the start of the diff was read",
	ReasonManyFiles: "content not read: too many files",
}

// FileNote is one staged file as a draft request describes it.
type FileNote struct {
	Path, OrigPath string
	// Status is Git's letter: A, M, D, R, T.
	Status         string
	Added, Deleted int
	Binary         bool
	Treatment      Treatment
	// Reasons explain a treatment other than TreatFull.
	Reasons []string

	oldMode, newMode string
}

// line is the file's entry in the request's file list.
func (n FileNote) line() string {
	s := n.Status + " " + n.Path
	if n.OrigPath != "" {
		s += " (from " + n.OrigPath + ")"
	}
	if !n.Binary {
		s += " +" + strconv.Itoa(n.Added) + " -" + strconv.Itoa(n.Deleted)
	}
	if n.oldMode != n.newMode && n.Status != "A" && n.Status != "D" {
		s += " mode " + n.oldMode + "→" + n.newMode
	}
	var notes []string
	for _, r := range n.Reasons {
		notes = append(notes, reasonText[r])
	}
	if len(notes) > 0 {
		s += "; " + strings.Join(notes, "; ")
	}
	return s
}

// lockFiles are dependency lock files, recognized by name.
var lockFiles = map[string]bool{
	"package-lock.json": true, "npm-shrinkwrap.json": true, "yarn.lock": true,
	"pnpm-lock.yaml": true, "bun.lockb": true, "bun.lock": true, "deno.lock": true,
	"go.sum": true, "go.work.sum": true, "Cargo.lock": true, "Gemfile.lock": true,
	"Podfile.lock": true, "poetry.lock": true, "Pipfile.lock": true, "uv.lock": true,
	"composer.lock": true, "mix.lock": true, "pubspec.lock": true,
	"Package.resolved": true, "flake.lock": true, "gradle.lockfile": true,
	"packages.lock.json": true, "Cartfile.resolved": true,
}

// minified reports names of minified bundles and source maps.
func minified(p string) bool {
	for _, s := range []string{".min.js", ".min.css", ".js.map", ".css.map", ".min.mjs"} {
		if strings.HasSuffix(p, s) {
			return true
		}
	}
	return false
}

// generatedHeader is the line Go and other tools put in generated files.
const generatedHeader = `^// Code generated .* DO NOT EDIT\.$`

// maxHeaderChecks bounds the files searched for generatedHeader.
const maxHeaderChecks = 1000

// stagedFiles lists every staged change with its statistics, as git diff
// --cached reports them, in Git's order.
func (r *Repository) stagedFiles(ctx context.Context) ([]FileNote, error) {
	raw, err := r.run(ctx, append([]string{"diff", "--cached", "--raw", "-z", "--no-abbrev"}, diffOptions...)...)
	if err != nil {
		return nil, err
	}
	num, err := r.run(ctx, append([]string{"diff", "--cached", "--numstat", "-z"}, diffOptions...)...)
	if err != nil {
		return nil, err
	}
	var notes []FileNote
	f := strings.Split(string(raw), "\x00")
	for i := 0; i < len(f); i++ {
		if !strings.HasPrefix(f[i], ":") {
			continue
		}
		parts := strings.Fields(f[i][1:])
		if len(parts) < 5 || i+1 >= len(f) {
			break
		}
		n := FileNote{Status: parts[4][:1], oldMode: parts[0], newMode: parts[1]}
		if n.Status == "R" || n.Status == "C" {
			if i+2 >= len(f) {
				break
			}
			n.OrigPath, n.Path = f[i+1], f[i+2]
			i += 2
		} else {
			n.Path = f[i+1]
			i++
		}
		notes = append(notes, n)
	}
	index := map[string]int{}
	for i, n := range notes {
		index[n.Path] = i
	}
	f = strings.Split(string(num), "\x00")
	for i := 0; i < len(f); i++ {
		parts := strings.SplitN(f[i], "\t", 3)
		if len(parts) != 3 {
			continue
		}
		p := parts[2]
		if p == "" && i+2 < len(f) {
			p = f[i+2]
			i += 2
		}
		j, ok := index[p]
		if !ok {
			continue
		}
		if parts[0] == "-" {
			notes[j].Binary = true
			continue
		}
		notes[j].Added, _ = strconv.Atoi(parts[0])
		notes[j].Deleted, _ = strconv.Atoi(parts[1])
	}
	return notes, nil
}

// classify marks the files whose diff a request summarizes by kind:
// submodules, binary files, deletions, lock files and generated files. A
// file is generated when its staged .gitattributes say linguist-generated,
// its name is a minified bundle, or its staged content has a "Code
// generated … DO NOT EDIT." line. Anything else is an ordinary file.
func (r *Repository) classify(ctx context.Context, notes []FileNote) error {
	var candidates []int
	for i := range notes {
		n := &notes[i]
		switch {
		case n.oldMode == "160000" || n.newMode == "160000":
			n.summarize(ReasonSubmodule)
		case n.Binary:
			n.summarize(ReasonBinary)
		case n.Status == "D":
			n.summarize(ReasonDeleted)
		case lockFiles[path.Base(n.Path)]:
			n.summarize(ReasonLock)
		case minified(n.Path):
			n.summarize(ReasonGenerated)
		case n.Added == 0 && n.Deleted == 0:
			n.summarize(ReasonNoText)
		default:
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	var list bytes.Buffer
	for _, i := range candidates {
		list.WriteString(notes[i].Path)
		list.WriteByte(0)
	}
	out, err := r.app.git.Run(ctx, r.root, &list, "check-attr", "--cached", "-z", "--stdin", "linguist-generated")
	if err != nil {
		return gitFailure(err)
	}
	generated := map[string]bool{}
	f := strings.Split(string(out), "\x00")
	for i := 0; i+2 < len(f); i += 3 {
		if f[i+2] == "set" || f[i+2] == "true" {
			generated[f[i]] = true
		}
	}
	if len(candidates) <= maxHeaderChecks {
		args := []string{"--literal-pathspecs", "grep", "--cached", "-l", "-z", "-I", "-E", generatedHeader, "--"}
		for _, i := range candidates {
			args = append(args, notes[i].Path)
		}
		out, _, err := r.app.git.RunStatus(ctx, r.root, nil, []int{1}, args...)
		if err != nil {
			return gitFailure(err)
		}
		for _, p := range strings.Split(string(out), "\x00") {
			generated[p] = p != ""
		}
	}
	for _, i := range candidates {
		if generated[notes[i].Path] {
			notes[i].summarize(ReasonGenerated)
		}
	}
	return nil
}

func (n *FileNote) summarize(reason string) {
	n.Treatment, n.Reasons = TreatSummary, []string{reason}
}
