// Package prefs stores GitOwl's ordinary preferences as JSON. Credentials
// never go here; package secrets keeps them in the macOS Keychain.
package prefs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Interface languages and appearances, besides "system".
var (
	Languages   = []string{"en", "zh-Hans"}
	Appearances = []string{"light", "dark"}
)

// System follows the operating system's choice.
const System = "system"

// Auto chooses the commit message language from recent commits.
const Auto = "auto"

// DraftLanguages are the commit message languages besides Auto.
var DraftLanguages = []string{"en", "zh-Hans"}

// AI configures commit message drafts. The API key is not here: it is in
// the Keychain.
type AI struct {
	// BaseURL is the OpenAI-compatible API base, as https://api.openai.com/v1.
	BaseURL string `json:"baseURL"`
	Model   string `json:"model"`
	// Prompt is the user's commit message prompt; empty uses GitOwl's
	// default style rules.
	Prompt string `json:"prompt"`
	// Language is the draft language: "auto", "en" or "zh-Hans". It is
	// independent of the interface language.
	Language string `json:"language"`
	// ContextTokens is the model's context size in tokens, which bounds what
	// a draft request may contain.
	ContextTokens int `json:"contextTokens"`
}

// Context sizes, in tokens.
const (
	DefaultContextTokens = 8192
	MinContextTokens     = 4096
	MaxContextTokens     = 1 << 20
)

// ContextPresets are the context sizes offered without typing a number.
var ContextPresets = []int{4096, 8192, 16384, 32768, 131072}

// ValidContext reports whether n is a context size GitOwl accepts.
func ValidContext(n int) bool { return n >= MinContextTokens && n <= MaxContextTokens }

// Configured reports whether a service and model are set.
func (a AI) Configured() bool { return a.BaseURL != "" && a.Model != "" }

// Preferences are what GitOwl remembers between launches.
type Preferences struct {
	// Language is the interface language: "system", "en" or "zh-Hans".
	Language string `json:"language"`
	// Appearance is "system", "light" or "dark".
	Appearance string `json:"appearance"`
	AI         AI     `json:"ai"`
	// Recent lists the worktree roots most recently opened, most recent
	// first, at most MaxRecent. Only the list is kept: no session state.
	Recent []string `json:"recent,omitempty"`
	// Pinned lists worktree roots kept at the top of the repository
	// column after quit. Pin and Unpin change only this list.
	Pinned []string `json:"pinned,omitempty"`
	// Editor is the app files and worktrees open in; empty when none is
	// chosen.
	Editor Editor `json:"editor,omitzero"`
	// CommitIdentity shows the commit panel's identity line and the dialog
	// that sets this repository's commit name and email. On by default; it
	// is always written, so turning it off persists.
	CommitIdentity bool `json:"commitIdentity"`
}

// Editor is an application chosen to edit files.
type Editor struct {
	// Name is the app's name as shown, as Visual Studio Code.
	Name string `json:"name"`
	// Path is its .app bundle.
	Path string `json:"path"`
}

// Set reports a chosen editor.
func (e Editor) Set() bool { return e.Path != "" }

// MaxRecent is the length of the recent repository list.
const MaxRecent = 10

// AddPinned returns list with root first, once. Pin is not capped like
// Recent: the column collapses extras behind Show more.
func AddPinned(list []string, root string) []string {
	out := []string{root}
	for _, r := range list {
		if r != root {
			out = append(out, r)
		}
	}
	return cleanPinned(out)
}

// RemovePinned returns list without root.
func RemovePinned(list []string, root string) []string {
	return cleanPinned(slices.DeleteFunc(slices.Clone(list), func(r string) bool { return r == root }))
}

// cleanPinned drops entries that are not absolute paths and repeats.
func cleanPinned(list []string) []string {
	var out []string
	for _, r := range list {
		if r == "" || !filepath.IsAbs(r) || strings.ContainsRune(r, 0) {
			continue
		}
		r = filepath.Clean(r)
		if slices.Contains(out, r) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// Equal reports whether p and q are the same preferences.
func (p Preferences) Equal(q Preferences) bool {
	return p.Language == q.Language && p.Appearance == q.Appearance && p.AI == q.AI && p.Editor == q.Editor && p.CommitIdentity == q.CommitIdentity && slices.Equal(p.Recent, q.Recent) && slices.Equal(p.Pinned, q.Pinned)
}

// AddRecent returns list with root first, once, cut to MaxRecent.
func AddRecent(list []string, root string) []string {
	out := []string{root}
	for _, r := range list {
		if r != root {
			out = append(out, r)
		}
	}
	return cleanRecent(out)
}

// RemoveRecent returns list without root.
func RemoveRecent(list []string, root string) []string {
	return cleanRecent(slices.DeleteFunc(slices.Clone(list), func(r string) bool { return r == root }))
}

// cleanRecent drops entries that are not absolute paths and repeats, each
// on its own, and keeps at most MaxRecent; nil when none are left.
func cleanRecent(list []string) []string {
	var out []string
	for _, r := range list {
		if r == "" || !filepath.IsAbs(r) || strings.ContainsRune(r, 0) {
			continue
		}
		r = filepath.Clean(r)
		if slices.Contains(out, r) {
			continue
		}
		out = append(out, r)
		if len(out) == MaxRecent {
			break
		}
	}
	return out
}

// Default returns the preferences of a first launch.
func Default() Preferences {
	return Preferences{Language: System, Appearance: System, AI: AI{Language: Auto, ContextTokens: DefaultContextTokens}, CommitIdentity: true}
}

// Normalize replaces unknown values, as those of a newer version, with
// defaults.
func (p *Preferences) Normalize() {
	d := Default()
	if p.Language != System && !slices.Contains(Languages, p.Language) {
		p.Language = d.Language
	}
	if p.Appearance != System && !slices.Contains(Appearances, p.Appearance) {
		p.Appearance = d.Appearance
	}
	if p.AI.Language != Auto && !slices.Contains(DraftLanguages, p.AI.Language) {
		p.AI.Language = Auto
	}
	if !ValidContext(p.AI.ContextTokens) {
		// Missing in preferences saved before it existed, or out of range.
		p.AI.ContextTokens = DefaultContextTokens
	}
	p.AI.BaseURL, p.AI.Model = strings.TrimSpace(p.AI.BaseURL), strings.TrimSpace(p.AI.Model)
	p.Recent = cleanRecent(p.Recent)
	p.Pinned = cleanPinned(p.Pinned)
	if p.Editor.Path != "" && (!filepath.IsAbs(p.Editor.Path) || !strings.HasSuffix(filepath.Clean(p.Editor.Path), ".app")) {
		p.Editor = Editor{}
	}
	if p.Editor.Path != "" {
		p.Editor.Path = filepath.Clean(p.Editor.Path)
		if strings.TrimSpace(p.Editor.Name) == "" {
			p.Editor.Name = strings.TrimSuffix(filepath.Base(p.Editor.Path), ".app")
		}
	}
}

// Store reads and writes preferences in one file.
type Store struct{ Path string }

// DefaultPath is ~/Library/Application Support/GitOwl/preferences.json on
// macOS.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "GitOwl", "preferences.json"), nil
}

// Load returns the saved preferences, the defaults when none are saved, and
// the defaults with an error when the file cannot be read.
func (s Store) Load() (Preferences, error) {
	p := Default()
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		// A recent list of another shape, as from a newer version, does not
		// cost the other preferences.
		var loose struct {
			Language       string            `json:"language"`
			Appearance     string            `json:"appearance"`
			AI             AI                `json:"ai"`
			Editor         Editor            `json:"editor"`
			CommitIdentity *bool             `json:"commitIdentity"`
			Recent         []json.RawMessage `json:"recent"`
			Pinned         []json.RawMessage `json:"pinned"`
		}
		if json.Unmarshal(data, &loose) != nil {
			return Default(), err
		}
		p = Preferences{Language: loose.Language, Appearance: loose.Appearance, AI: loose.AI, Editor: loose.Editor, CommitIdentity: true}
		if loose.CommitIdentity != nil {
			p.CommitIdentity = *loose.CommitIdentity
		}
		for _, raw := range loose.Recent {
			var s string
			if json.Unmarshal(raw, &s) == nil {
				p.Recent = append(p.Recent, s)
			}
		}
		for _, raw := range loose.Pinned {
			var s string
			if json.Unmarshal(raw, &s) == nil {
				p.Pinned = append(p.Pinned, s)
			}
		}
	}
	p.Normalize()
	return p, nil
}

// Save replaces the file atomically, so an interrupted save keeps the
// previous preferences.
func (s Store) Save(p Preferences) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".preferences-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.Path)
}
