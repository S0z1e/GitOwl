package workflow

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"gitowl/internal/ai"
	"gitowl/internal/secrets"
)

const (
	CodeKeychain Code = "Keychain"
	// CodeDraftManifestTooLarge: the list of staged files alone needs more
	// than the input budget. Nothing is sent.
	CodeDraftManifestTooLarge Code = "DraftManifestTooLarge"
	// CodeDraftPromptTooLong: the instructions and the user's prompt alone
	// need more than the input budget. Nothing is sent.
	CodeDraftPromptTooLong Code = "DraftPromptTooLong"
)

// afterDraftContent, when set by a test, runs after a request's content is
// read and before its staged state is checked again.
var afterDraftContent func()

// AIConfig is what Application needs to ask for drafts.
type AIConfig struct {
	Client  *ai.Client
	Secrets secrets.Store
}

// SetAI connects the workflow to an AI service client and a secret store.
func (a *Application) SetAI(cfg AIConfig) { a.ai = cfg }

func (a *Application) secrets() secrets.Store {
	if a.ai.Secrets == nil {
		return &secrets.Memory{}
	}
	return a.ai.Secrets
}

// HasAPIKey reports whether an API key is saved.
func (a *Application) HasAPIKey() (bool, error) {
	k, err := a.secrets().Get(secrets.APIKeyAccount)
	if err != nil {
		return false, fail(CodeKeychain, err)
	}
	return k != "", nil
}

// SaveAPIKey saves key in the secret store; an empty key deletes it.
func (a *Application) SaveAPIKey(key string) error {
	key = strings.TrimSpace(key)
	var err error
	if key == "" {
		err = a.secrets().Delete(secrets.APIKeyAccount)
	} else {
		err = a.secrets().Set(secrets.APIKeyAccount, key)
	}
	if err != nil {
		return fail(CodeKeychain, err)
	}
	return nil
}

// DraftOptions are the user's choices for a draft.
type DraftOptions struct {
	// Prompt is the user's commit message prompt; empty uses the default
	// style rules.
	Prompt string
	// Language is "auto", "en" or "zh-Hans".
	Language string
	// InterfaceLanguage is the fallback when Auto finds no language.
	InterfaceLanguage string
	// ContextTokens is the model's context size; zero uses the default.
	ContextTokens int
}

// LanguageSource says where a draft's language came from.
type LanguageSource uint8

const (
	LanguageChosen LanguageSource = iota
	LanguageFromHistory
	LanguageFromInterface
)

// StyleSource says which style rules a draft follows.
type StyleSource uint8

const (
	StyleCustom StyleSource = iota
	StyleFromHistory
	StyleDefault
)

// DraftRequest is exactly what a draft request sends, prepared without
// touching the network so the user can review it first.
type DraftRequest struct {
	Root        string
	Fingerprint string
	// Files are the staged files, as the working tree review lists them.
	Files []Change
	// Notes describe every staged file as the request lists it, with how
	// much of its diff Diff shows.
	Notes []FileNote
	// Diff is the staged diff as sent: in full when it fits the budget,
	// otherwise shortened as Notes say.
	Diff string
	// Recent are the recent commit subjects sent as style reference: at
	// most RecentExamples, none with a custom prompt.
	Recent []string
	// Language is the draft language, "en" or "zh-Hans".
	Language       string
	LanguageSource LanguageSource
	Style          StyleSource
	// Messages are the first request, which writes the subject line.
	Messages []ai.Message
	// Budget is the context plan; Estimate is what the larger of the two
	// requests needs of it, by EstimateTokens, at most Budget.Input.
	Budget   Budget
	Estimate int
	// prompt is the user's prompt, for the description request.
	prompt string
}

// Shortened reports a request that does not show every staged change in
// full.
func (r DraftRequest) Shortened() bool {
	for _, n := range r.Notes {
		if n.Treatment != TreatFull || len(n.Reasons) > 0 {
			return true
		}
	}
	return false
}

// Size returns the request's text size in bytes.
func (r DraftRequest) Size() int {
	n := 0
	for _, m := range r.Messages {
		n += len(m.Content)
	}
	return n
}

// Draft is a commit message draft from the service.
type Draft struct {
	Text string
	// Fingerprint is the staged content the draft was asked for.
	Fingerprint string
	// Stale is set when the staged content changed while the draft was
	// being written.
	Stale bool
}

// PrepareDraft reads the staged changes and builds the request for a
// draft, without touching the network, so the user can inspect it.
// Unstaged and untracked changes are never part of it.
func (r *Repository) PrepareDraft(ctx context.Context, opts DraftOptions) (DraftRequest, error) {
	if !r.Current() {
		return DraftRequest{}, &Error{Code: CodeRepositoryClosed}
	}
	return r.prepare(ctx, opts)
}

// Draft prepares a request from the staged changes as they are now and
// sends it, once. Nothing is sent when nothing is staged, the request
// cannot be prepared, or the staged content changes while it is read. It
// never stages or commits.
func (r *Repository) Draft(ctx context.Context, opts DraftOptions, cfg ai.Config) (DraftRequest, Draft, error) {
	req, err := r.PrepareDraft(ctx, opts)
	if err != nil {
		return DraftRequest{}, Draft{}, err
	}
	d, err := r.GenerateDraft(ctx, req, cfg)
	return req, d, err
}

// prepare builds a request whose content all comes from one staged state:
// the fingerprint is read before and after the content, and a change in
// between stops the preparation.
func (r *Repository) prepare(ctx context.Context, opts DraftOptions) (DraftRequest, error) {
	snap, err := r.read(ctx)
	if err != nil {
		return DraftRequest{}, err
	}
	if len(snap.Staged) == 0 {
		return DraftRequest{}, &Error{Code: CodeNothingStaged}
	}
	notes, err := r.stagedFiles(ctx)
	if err != nil {
		return DraftRequest{}, err
	}
	if err := r.classify(ctx, notes); err != nil {
		return DraftRequest{}, err
	}
	// Recent messages decide the language here; only a few subjects are
	// sent, and none with a custom prompt.
	recent, err := r.recentMessages(ctx, snap)
	if err != nil {
		return DraftRequest{}, err
	}
	req := DraftRequest{Root: r.root, Fingerprint: snap.StagedFingerprint, Files: snap.Staged, Budget: NewBudget(opts.ContextTokens), prompt: opts.Prompt}
	req.Language, req.LanguageSource = draftLanguage(opts, recent)
	custom := strings.TrimSpace(opts.Prompt) != ""
	var examples []string
	if !custom {
		examples = subjects(recent, RecentExamples)
	}
	// The instructions and the user's whole prompt must fit on their own;
	// GitOwl never cuts the prompt.
	probe := DraftRequest{Language: req.Language, Style: styleOf(custom, nil)}
	if est := requestEstimate(probe, opts.Prompt); est > req.Budget.Input {
		return DraftRequest{}, &Error{Code: CodeDraftPromptTooLong, Detail: budgetDetail(est, req.Budget)}
	}
	// So must the list of every staged file, with the longest notes it can
	// have; style examples give way to it.
	worst := DraftRequest{Language: req.Language, Notes: worstNotes(notes)}
	fixed := 0
	for k := len(examples); ; k-- {
		if k < 0 {
			return DraftRequest{}, &Error{Code: CodeDraftManifestTooLarge, Detail: budgetDetail(fixed, req.Budget)}
		}
		worst.Recent, worst.Style = examples[:k], styleOf(custom, examples[:k])
		if fixed = requestEstimate(worst, opts.Prompt); fixed <= req.Budget.Input {
			req.Recent, req.Style = worst.Recent, worst.Style
			break
		}
	}
	// The diff gets the rest, read in bounded parts and shortened to fit.
	patches, err := r.readPatches(ctx, notes, req.Budget.Input)
	if err != nil {
		return DraftRequest{}, err
	}
	room := req.Budget.Input - fixed
	for {
		req.Notes = cloneNotes(notes)
		req.Diff = fitDiff(req.Notes, patches, room)
		req.Messages = buildMessages(req, opts.Prompt)
		req.Estimate = requestEstimate(req, opts.Prompt)
		if req.Estimate <= req.Budget.Input || room <= 0 {
			break
		}
		room -= req.Estimate - req.Budget.Input + 8
	}
	if afterDraftContent != nil {
		afterDraftContent()
	}
	if fp, err := r.stagedFingerprint(ctx, snap.Head); err != nil {
		return DraftRequest{}, err
	} else if fp != snap.StagedFingerprint {
		return DraftRequest{}, &Error{Code: CodeStateChanged}
	}
	return req, nil
}

// worstNotes are notes with the longest file list entries a request can
// end up with.
func worstNotes(notes []FileNote) []FileNote {
	out := cloneNotes(notes)
	for i := range out {
		if out[i].Treatment == TreatFull {
			out[i].Treatment = TreatShortened
			out[i].Reasons = []string{ReasonBudget, ReasonReadLimit, ReasonLongLines}
		}
	}
	return out
}

func cloneNotes(notes []FileNote) []FileNote {
	out := append([]FileNote{}, notes...)
	for i := range out {
		out[i].Reasons = append([]string(nil), out[i].Reasons...)
	}
	return out
}

func styleOf(custom bool, examples []string) StyleSource {
	switch {
	case custom:
		return StyleCustom
	case len(examples) > 0:
		return StyleFromHistory
	}
	return StyleDefault
}

func budgetDetail(est int, b Budget) string {
	return "≈" + strconv.Itoa(est) + " / " + strconv.Itoa(b.Input) + " tokens (" + strconv.Itoa(b.Context) + " context)"
}

// maxSubjectChars is the longest subject a draft asks for.
const maxSubjectChars = 72

// placeholderSubject is a subject as long as one can be, for budgeting the
// description request before the subject is known. It is Chinese, which
// costs the most tokens per character.
var placeholderSubject = strings.Repeat("中", maxSubjectChars)

// requestEstimate is the larger estimate of the two requests, which the
// budget must cover.
func requestEstimate(req DraftRequest, prompt string) int {
	return max(EstimateTokens(buildMessages(req, prompt)), EstimateTokens(describeMessages(req, prompt, placeholderSubject)))
}

// buildMessages is the first request: the subject line of the message.
func buildMessages(req DraftRequest, prompt string) []ai.Message {
	return []ai.Message{{Role: "system", Content: subjectInstructions(req, prompt)}, {Role: "user", Content: changesText(req)}}
}

// describeMessages is the second request: the description of the message
// whose subject the first request wrote, from the same staged changes.
func describeMessages(req DraftRequest, prompt, subject string) []ai.Message {
	user := changesText(req) + "\n\nCommit subject, already written:\n" + subject
	return []ai.Message{{Role: "system", Content: descriptionInstructions(req, prompt)}, {Role: "user", Content: user}}
}

// DescriptionMessages is the second request for a draft whose subject line
// is subject.
func (r DraftRequest) DescriptionMessages(subject string) []ai.Message {
	return describeMessages(r, r.prompt, subject)
}

func subjectInstructions(req DraftRequest, prompt string) string {
	var sys strings.Builder
	sys.WriteString("You write the subject line of a Git commit message for the staged changes the user sends. ")
	sys.WriteString("Reply with the subject line only: no body, no explanation, no quotes, no code fences.\n\n")
	switch req.Style {
	case StyleCustom:
		sys.WriteString("Follow the user's instructions below. They take precedence over any default style.\n")
		sys.WriteString("<instructions>\n" + strings.TrimSpace(prompt) + "\n</instructions>\n")
	case StyleFromHistory:
		sys.WriteString("Match the style of the repository's recent commit subjects that the user includes: their format, prefixes, capitalization and length.\n")
	default:
		sys.WriteString("Write a concise subject line under 72 characters that starts with an imperative verb, such as \"Add\", \"Fix\" or \"Update\".\n")
	}
	sys.WriteString(shortenedNote(req))
	sys.WriteString("\n")
	sys.WriteString(languageNote(req, "subject line"))
	return sys.String()
}

func descriptionInstructions(req DraftRequest, prompt string) string {
	var sys strings.Builder
	sys.WriteString("You write the description of a Git commit message for the staged changes the user sends. The subject line is already written and comes after the changes. ")
	sys.WriteString("Reply with the description only: no subject, no quotes, no code fences. Write a few short sentences or bullet points that say what changed and why. Do not repeat the subject.\n\n")
	if req.Style == StyleCustom {
		sys.WriteString("Follow the user's instructions below where they apply to the description. They take precedence over any default style.\n")
		sys.WriteString("<instructions>\n" + strings.TrimSpace(prompt) + "\n</instructions>\n")
	}
	sys.WriteString(shortenedNote(req))
	sys.WriteString("\n")
	sys.WriteString(languageNote(req, "description"))
	return sys.String()
}

// shortenedNote tells the model what a shortened request leaves out, or "".
func shortenedNote(req DraftRequest) string {
	if !req.Shortened() {
		return ""
	}
	return "\nParts of the staged changes are not shown in full; the file list says which and why. Describe only what the file list and diff show, without inventing details of what was left out. A broader summary is fine for a large change.\n"
}

// languageNote names the language of the part being written, the subject
// line or the description.
func languageNote(req DraftRequest, part string) string {
	if req.Language == "zh-Hans" {
		return "Write the " + part + " in Simplified Chinese (简体中文). Keep code identifiers, file names and commands as they are."
	}
	return "Write the " + part + " in English."
}

// changesText is the staged changes both requests send.
func changesText(req DraftRequest) string {
	var user strings.Builder
	if len(req.Recent) > 0 {
		user.WriteString("Recent commit subjects in this repository, newest first, for style reference only:\n")
		for _, m := range req.Recent {
			user.WriteString("- " + m + "\n")
		}
		user.WriteString("\n")
	}
	user.WriteString("Staged files (status, path, lines added and removed):\n")
	for _, n := range req.Notes {
		user.WriteString(n.line() + "\n")
	}
	if req.Shortened() {
		user.WriteString("\nStaged diff (git diff --cached), shortened as the file list says:\n")
	} else {
		user.WriteString("\nStaged diff (git diff --cached):\n")
	}
	user.WriteString(req.Diff)
	return user.String()
}

// GenerateDraft sends req and returns the draft. It sends two requests, the
// subject line and then the description for it, and never stages or
// commits. The draft is Stale when the staged content differs from req's
// once the reply arrives; a closed repository's reply is an error.
func (r *Repository) GenerateDraft(ctx context.Context, req DraftRequest, cfg ai.Config) (Draft, error) {
	if !r.Current() {
		return Draft{}, &Error{Code: CodeRepositoryClosed}
	}
	key, err := r.app.secrets().Get(secrets.APIKeyAccount)
	if err != nil {
		return Draft{}, fail(CodeKeychain, err)
	}
	client := r.app.ai.Client
	if client == nil {
		client = &ai.Client{}
	}
	text, err := client.Complete(ctx, cfg, key, req.Messages)
	if err != nil {
		return Draft{}, aiFailure(err)
	}
	if !r.Current() {
		return Draft{}, &Error{Code: CodeRepositoryClosed}
	}
	// The subject is the reply's first line; anything after it is left to
	// the description request.
	subject, _, _ := strings.Cut(text, "\n")
	subject = strings.TrimSpace(subject)
	body, err := client.Complete(ctx, cfg, key, req.DescriptionMessages(subject))
	if err != nil {
		return Draft{}, aiFailure(err)
	}
	text = subject
	if body = strings.TrimSpace(body); body != "" {
		text += "\n\n" + body
	}
	if !r.Current() {
		return Draft{}, &Error{Code: CodeRepositoryClosed}
	}
	d := Draft{Text: text, Fingerprint: req.Fingerprint}
	if snap, err := r.read(ctx); err != nil || snap.StagedFingerprint != req.Fingerprint {
		d.Stale = true
	}
	return d, nil
}

func aiFailure(err error) error {
	var e *ai.Error
	if !errors.As(err, &e) {
		return fail(Code("AI"+string(ai.Network)), err)
	}
	if e.Kind == ai.Cancelled {
		return &Error{Code: CodeCancelled, Err: err}
	}
	detail := e.Detail
	if e.Status != 0 {
		detail = strings.TrimSpace("HTTP " + strconv.Itoa(e.Status) + " " + detail)
	}
	return &Error{Code: Code("AI" + string(e.Kind)), Detail: detail, Err: err}
}
