package workflow

import (
	"context"
	"strings"
	"unicode"
)

// Sampling limits for the recent commit messages read to choose the draft
// language. They stay on this computer.
const (
	RecentCommits      = 20
	RecentMessageChars = 500
	RecentTotalChars   = 4000
)

// Limits of the commit subjects sent as style reference.
const (
	RecentExamples     = 3
	RecentSubjectChars = 200
)

// subjects returns the first lines of the first n messages, cut at
// RecentSubjectChars.
func subjects(msgs []string, n int) []string {
	var out []string
	for _, m := range msgs[:min(n, len(msgs))] {
		line, _, _ := strings.Cut(m, "\n")
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, cut(line, RecentSubjectChars))
		}
	}
	return out
}

// recentMessages returns up to RecentCommits non-merge commit messages
// reachable from HEAD, newest first, each cut at RecentMessageChars and
// RecentTotalChars in all. A repository without commits has none.
func (r *Repository) recentMessages(ctx context.Context, snap Snapshot) ([]string, error) {
	if snap.Unborn() {
		return nil, nil
	}
	out, err := r.run(ctx, "log", "-z", "--no-merges", "--no-color", "--format=%B",
		"--max-count="+itoa(RecentCommits), "HEAD", "--")
	if err != nil {
		return nil, err
	}
	var msgs []string
	total := 0
	for _, m := range strings.Split(string(out), "\x00") {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		m = cut(m, RecentMessageChars)
		if total+len([]rune(m)) > RecentTotalChars {
			break
		}
		total += len([]rune(m))
		msgs = append(msgs, m)
	}
	return msgs, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}

// messageLanguage classifies one message: "zh-Hans" when at least 30 % of
// its letters are CJK, "en" when it has Latin letters otherwise, "" when it
// has neither.
func messageLanguage(m string) string {
	cjk, latin := 0, 0
	for _, r := range m {
		switch {
		case unicode.Is(unicode.Han, r):
			cjk++
		case r < unicode.MaxASCII && unicode.IsLetter(r):
			latin++
		}
	}
	switch {
	case cjk > 0 && cjk*10 >= (cjk+latin)*3:
		return "zh-Hans"
	case latin > 0:
		return "en"
	}
	return ""
}

// draftLanguage resolves the draft language: the chosen one, the majority
// language of recent messages, or the interface language when history has
// no clear language.
func draftLanguage(opts DraftOptions, recent []string) (string, LanguageSource) {
	switch opts.Language {
	case "en", "zh-Hans":
		return opts.Language, LanguageChosen
	}
	zh, en := 0, 0
	for _, m := range recent {
		switch messageLanguage(m) {
		case "zh-Hans":
			zh++
		case "en":
			en++
		}
	}
	switch {
	case zh > en:
		return "zh-Hans", LanguageFromHistory
	case en > zh:
		return "en", LanguageFromHistory
	}
	if opts.InterfaceLanguage == "zh-Hans" {
		return "zh-Hans", LanguageFromInterface
	}
	return "en", LanguageFromInterface
}
