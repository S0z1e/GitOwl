package desktop

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/ai"
	"gitowl/internal/prefs"
	"gitowl/internal/workflow"
)

// aiState is the commit message draft flow of the open repository.
type aiState struct {
	// seq numbers draft requests; only the latest applies.
	seq uint64
	// generating prepares and sends a request; cancel stops it.
	generating bool
	cancel     context.CancelFunc
	err        *notice
	// pending is a draft waiting for the user to use or discard it.
	pending *workflow.Draft
	// draftedByAI marks the current message as an accepted AI draft, so
	// the editor footer can say "Drafted by AI". Editing the fields by
	// hand clears it (answerHint / recompose).
	draftedByAI bool
	// applied is the draft text last put in the message.
	applied string
	// last is the request the latest draft was written from.
	last *workflow.DraftRequest

	// inspecting prepares a request to show without sending it; inspect is
	// that request, shown while inspectOpen.
	inspecting  bool
	inspectSeq  uint64
	inspect     *workflow.DraftRequest
	inspectOpen bool
	inspectText string
	hasKey      bool
}

// aiForm holds the AI settings being edited.
type aiForm struct {
	baseURL, model, prompt string
	// contextCustom shows the field for a context size outside the
	// presets; contextText is what it holds and contextErr why it was not
	// saved.
	contextCustom     bool
	contextText       string
	contextErr        bool
	key               string
	hasKey, keyLoaded bool
	keyBusy           bool
	keyErr            string
}

func (u *UI) aiConfig() ai.Config {
	return ai.Config{BaseURL: u.prefs.AI.BaseURL, Model: u.prefs.AI.Model, Timeout: u.opts.AITimeout}
}

func (u *UI) resetAI() {
	if u.ai.cancel != nil {
		u.ai.cancel()
	}
	u.ai = aiState{seq: u.ai.seq + 1, inspectSeq: u.ai.inspectSeq + 1}
}

func (u *UI) draftOptionsNow() workflow.DraftOptions {
	return workflow.DraftOptions{Prompt: u.prefs.AI.Prompt, Language: u.prefs.AI.Language, InterfaceLanguage: u.language(), ContextTokens: u.prefs.AI.ContextTokens}
}

// generateDraft prepares a request from the staged changes as they are now
// and sends it, in one step. Choosing Generate is the decision to send.
func (u *UI) generateDraft() {
	if u.repo == nil || u.ai.generating || u.draftBlock() == "busy" {
		return
	}
	u.ai.err, u.ai.inspectOpen = nil, false
	if len(u.snap.Staged) == 0 {
		u.ai.err = &notice{code: workflow.CodeNothingStaged}
		return
	}
	u.ai.seq++
	seq, repo, cfg, opts := u.ai.seq, u.repo, u.aiConfig(), u.draftOptionsNow()
	ctx, cancel := context.WithCancel(context.Background())
	u.ai.generating, u.ai.cancel = true, cancel
	u.background(func() func() {
		req, d, err := repo.Draft(ctx, opts, cfg)
		cancel()
		return func() {
			if repo != u.repo || seq != u.ai.seq {
				return
			}
			u.ai.generating, u.ai.cancel = false, nil
			if req.Root != "" {
				// The request that was sent, or would have been.
				u.ai.last = &req
			}
			if err != nil {
				u.ai.err = noticeOf(err)
				return
			}
			u.receiveDraft(d)
		}
	})
}

// inspectRequest prepares the request Generate would send now and shows
// it. Nothing is sent.
func (u *UI) inspectRequest() {
	if u.repo == nil || u.ai.inspecting {
		return
	}
	u.ai.err = nil
	if len(u.snap.Staged) == 0 {
		u.ai.err = &notice{code: workflow.CodeNothingStaged}
		return
	}
	u.ai.inspectSeq++
	seq, repo, opts := u.ai.inspectSeq, u.repo, u.draftOptionsNow()
	u.ai.inspecting = true
	u.background(func() func() {
		hasKey, kerr := u.wf.HasAPIKey()
		req, err := repo.PrepareDraft(context.Background(), opts)
		if err == nil {
			err = kerr
		}
		return func() {
			if repo != u.repo || seq != u.ai.inspectSeq {
				return
			}
			u.ai.inspecting = false
			if err != nil {
				u.ai.err = noticeOf(err)
				return
			}
			u.ai.inspect, u.ai.hasKey, u.ai.inspectOpen = &req, hasKey, true
			u.ai.inspectText = requestText(req)
		}
	})
}

func requestText(req workflow.DraftRequest) string {
	first := messagesText(req.Messages)
	second := messagesText(req.DescriptionMessages("<subject line from the first reply>"))
	return first + "\n\n--- Second request, sent after the first reply, with its subject line ---\n\n" + second
}

func messagesText(msgs []ai.Message) string {
	var b strings.Builder
	for i, m := range msgs {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("[" + m.Role + "]\n" + m.Content)
	}
	return b.String()
}

// receiveDraft holds a reply as a suggestion for the user to accept or
// dismiss. Issue 09: a reply never fills the message by itself — not even
// a blank one — so Generate only ever offers a draft to review.
func (u *UI) receiveDraft(d workflow.Draft) {
	d.Stale = d.Stale || d.Fingerprint != u.snap.StagedFingerprint
	if u.chrome.shell && !d.Stale && u.chromeDraftAppliesDirectly() {
		u.useDraft(d)
		return
	}
	u.ai.pending = &d
}

// useDraft accepts a suggestion into the message: it fills Summary and
// Description, marks the message as drafted by AI and clears the pending
// suggestion. It replaces whatever was there, so callers gate it behind an
// explicit accept when the user had written their own words.
func (u *UI) useDraft(d workflow.Draft) {
	u.cm.message, u.ai.applied, u.ai.pending, u.cm.hint = d.Text, d.Text, nil, nil
	u.ai.draftedByAI = true
}

func (u *UI) cancelDraft() {
	if u.ai.cancel != nil {
		u.ai.cancel()
	}
}

// openAISettings shows the AI page of Settings.
func (u *UI) openAISettings() {
	u.set.state.Category = "ai"
	u.OpenSettings()
}

func (u *UI) languageText(lang string) string {
	if lang == prefs.Auto {
		return u.t("draftLanguageAuto")
	}
	return u.optionText(lang)
}

// draftBlock returns the message key explaining why a draft cannot be
// asked for now, or "".
func (u *UI) draftBlock() string {
	switch {
	case u.repo == nil || u.cm.committing || u.ai.generating || u.wt.bulk != "":
		return "busy"
	case len(u.snap.Staged) == 0:
		return "draftNeedsStaged"
	}
	return ""
}

func (u *UI) suggestHint() string {
	switch {
	case u.ai.pending != nil:
		return u.t("aiSuggestTabEsc")
	case u.ai.draftedByAI && strings.TrimSpace(u.cm.message) != "":
		return u.t("aiSuggestDrafted")
	default:
		return u.t("aiSuggestUses", "count", len(u.snap.Staged))
	}
}

func (u *UI) suggestStale(d *workflow.Draft) bool {
	return d.Stale || d.Fingerprint != u.snap.StagedFingerprint
}

func suggestSummary(d *workflow.Draft) string { s, _ := splitMessage(d.Text); return s }
func suggestBody(d *workflow.Draft) string    { _, b := splitMessage(d.Text); return b }

func needsSettings(code workflow.Code) bool {
	switch code {
	case "AINotConfigured", "AIInvalidURL", "AIUnauthorized", "AITooLarge", workflow.CodeDraftManifestTooLarge, workflow.CodeDraftPromptTooLong:
		return true
	}
	return false
}

// retryable reports a failure that sending again unchanged might not
// repeat.
func retryable(code workflow.Code) bool {
	switch code {
	case workflow.CodeNothingStaged, workflow.CodeDraftManifestTooLarge, workflow.CodeDraftPromptTooLong:
		return false
	}
	return true
}

// shortenedText summarizes how a request was fitted, or "" when it shows
// every staged change in full.
func (u *UI) shortenedText(req *workflow.DraftRequest) string {
	if !req.Shortened() {
		return ""
	}
	summary, shortened := 0, 0
	for _, n := range req.Notes {
		switch n.Treatment {
		case workflow.TreatSummary, workflow.TreatOmitted:
			summary++
		default:
			if len(n.Reasons) > 0 {
				shortened++
			}
		}
	}
	return u.t("draftShortened", "summary", summary, "shortened", shortened)
}

// noteText explains one file's treatment, or "" for a file shown in full.
func (u *UI) noteText(n workflow.FileNote) string {
	var parts []string
	for _, r := range n.Reasons {
		parts = append(parts, u.t("note"+r))
	}
	return strings.Join(parts, "; ")
}

// estimateText says how much of the input budget req needs.
func (u *UI) estimateText(req *workflow.DraftRequest) string {
	return u.t("draftEstimate", "estimate", req.Estimate, "limit", req.Budget.Input, "context", req.Budget.Context)
}

// choiceWith is choice with its own option names.
func (u *UI) choiceWith(c *ui.Context, label, value string, choices []string, text func(string) string, set func(string)) {
	u.kitChoice(c, label, value, choices, text, set)
}

func (u *UI) sourceText(req *workflow.DraftRequest) (string, string) {
	lang := u.optionText(req.Language)
	switch req.LanguageSource {
	case workflow.LanguageChosen:
		lang = u.t("draftLangChosen", "language", lang)
	case workflow.LanguageFromHistory:
		lang = u.t("draftLangHistory", "language", lang)
	default:
		lang = u.t("draftLangInterface", "language", lang)
	}
	style := u.t("draftStyleDefault")
	switch req.Style {
	case workflow.StyleCustom:
		style = u.t("draftStyleCustom")
	case workflow.StyleFromHistory:
		style = u.t("draftStyleHistory", "count", len(req.Recent))
	}
	return lang, style
}

func (u *UI) requestDialog(c *ui.Context) {
	if !u.ai.inspectOpen {
		return
	}
	req := u.ai.inspect
	title := u.t("previewTitle")
	u.chromeFormDialog(c, title, func(tk kitTokens) {
		ui.Text(c, title).FontWeight(600).FontSize(15).TextColor(tk.text)
		if req == nil {
			return
		}
		ui.Scroll(c).FillWidth().MaxHeight(360).Children(func() {
			ui.Text(c, u.ai.inspectText).Font("monospace").FontSize(12).TextColor(tk.text).Wrap().Selectable()
		})
		u.chromeFormButtons(c, tk, u.t("draftGenerate"), func() {
			u.ai.inspectOpen, u.ai.inspect = false, nil
		}, func() {
			u.ai.inspectOpen, u.ai.inspect = false, nil
			u.generateDraft()
		}, false)
	}, func() { u.ai.inspectOpen, u.ai.inspect = false, nil })
}

func aiKind(err error) ai.Kind {
	if e, ok := err.(*ai.Error); ok {
		return e.Kind
	}
	return ai.InvalidURL
}

func orText(s, empty string) string {
	if strings.TrimSpace(s) == "" {
		return empty
	}
	return s
}

// loadKeyStatus reads whether an API key is saved.
func (u *UI) loadKeyStatus() {
	if u.wf == nil || u.form.keyBusy {
		return
	}
	u.form.keyBusy = true
	u.background(func() func() {
		ok, err := u.wf.HasAPIKey()
		return func() {
			u.form.keyBusy, u.form.keyLoaded = false, true
			u.form.hasKey = ok
			u.form.keyErr = ""
			if err != nil {
				u.form.keyErr = err.Error()
			}
		}
	})
}

func (u *UI) saveKey(key string) {
	if u.wf == nil || u.form.keyBusy {
		return
	}
	u.form.keyBusy, u.form.keyErr = true, ""
	u.background(func() func() {
		err := u.wf.SaveAPIKey(key)
		return func() {
			u.form.keyBusy = false
			if err != nil {
				u.form.keyErr = err.Error()
				return
			}
			u.form.key, u.form.hasKey, u.form.keyLoaded = "", strings.TrimSpace(key) != "", true
		}
	})
}

// extraSettings adds the AI page to Settings, drawn with kit fields.
func (u *UI) extraSettings() []settingsCategory {
	f := &u.form
	if !f.keyLoaded && !f.keyBusy {
		u.loadKeyStatus()
	}
	text := func(id, title, desc, placeholder string, value *string, multiline bool, apply func(*prefs.AI, string)) settingsItem {
		return settingsItem{ID: id, Title: title, Description: desc, Stacked: multiline, Build: func(c *ui.Context) {
			ui.Box(c).FillWidth().MinWidth(240).Key("ai:" + id).Children(func() {
				var box ui.Element
				if multiline {
					box = ui.TextArea(c, value).FillWidth().Label(title).Placeholder(placeholder)
				} else {
					box = ui.TextInput(c, value).FillWidth().Height(kitFieldH).Label(title).Placeholder(placeholder)
				}
				if box.Changed() {
					p := u.prefs
					apply(&p.AI, *value)
					u.setPreferences(p)
				}
			})
		}}
	}
	items := []settingsItem{
		text("baseURL", u.t("aiBaseURL"), u.t("aiBaseURLHint"), "https://api.openai.com/v1", &f.baseURL, false, func(a *prefs.AI, v string) { a.BaseURL = v }),
		text("model", u.t("aiModel"), "", "gpt-4o-mini", &f.model, false, func(a *prefs.AI, v string) { a.Model = v }),
		{ID: "key", Title: u.t("aiKey"), Description: u.t("aiKeyHint"), Stacked: true, Build: func(c *ui.Context) {
			tk := kitDesignTokens(c.Theme().Dark)
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				status := u.t("aiKeyNotSet")
				switch {
				case f.hasKey:
					status = u.t("aiKeySaved")
				case ai.KeyOptional(u.prefs.AI.BaseURL):
					status = u.t("aiKeyNotNeeded")
				}
				ui.Text(c, status).FontSize(13).TextColor(tk.muted)
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Grow(1).MinWidth(0).Key("ai:key").Children(func() {
						ui.TextInput(c, &f.key).FillWidth().Height(kitFieldH).Password().Label(u.t("aiKey")).Placeholder("sk-…")
					})
					saveOff := f.keyBusy || strings.TrimSpace(f.key) == ""
					if kitSecondaryButton(c, u.t("aiKeySave"), tk, saveOff).Clicked() && !saveOff {
						u.saveKey(f.key)
					}
					if f.hasKey {
						if kitSecondaryButton(c, u.t("aiKeyRemove"), tk, f.keyBusy).Clicked() && !f.keyBusy {
							u.saveKey("")
						}
					}
				})
				if f.keyErr != "" {
					ui.Text(c, u.t("errorKeychain")).Wrap().TextColor(tk.del)
					ui.Text(c, f.keyErr).Wrap().FontSize(12).TextColor(tk.muted)
				}
			})
		}},
		{ID: "context", Title: u.t("aiContext"), Description: u.t("aiContextHint", "input", workflow.NewBudget(u.prefs.AI.ContextTokens).Input),
			Keywords: []string{"context", "tokens", "上下文", "budget"}, Build: u.contextSetting},
		text("prompt", u.t("aiPrompt"), u.t("aiPromptHint"), u.t("aiPromptPlaceholder"), &f.prompt, true, func(a *prefs.AI, v string) { a.Prompt = v }),
		{ID: "language", Title: u.t("draftLanguage"), Description: u.t("draftLanguageHint"), Build: func(c *ui.Context) {
			u.choiceWith(c, u.t("draftLanguage"), u.prefs.AI.Language, append([]string{prefs.Auto}, prefs.DraftLanguages...), u.languageText, func(v string) {
				p := u.prefs
				p.AI.Language = v
				u.setPreferences(p)
			})
		}},
	}
	return []settingsCategory{{
		ID: "ai", Title: u.t("settingsAI"), Description: u.t("aiPrivacy"),
		Groups: []settingsGroup{{ID: "ai", Items: items}},
	}}
}

// customContext is the select value for a context size typed by the user.
const customContext = "custom"

// contextSetting chooses the model context size: a preset, or a number
// typed in the field that Custom shows.
func (u *UI) contextSetting(c *ui.Context) {
	f := &u.form
	tk := kitDesignTokens(c.Theme().Dark)
	current := u.prefs.AI.ContextTokens
	value := strconv.Itoa(current)
	if f.contextCustom || !slices.Contains(prefs.ContextPresets, current) {
		value = customContext
	}
	choices := []string{}
	for _, n := range prefs.ContextPresets {
		choices = append(choices, strconv.Itoa(n))
	}
	choices = append(choices, customContext)
	label := func(v string) string {
		if v == customContext {
			return u.t("aiContextCustom")
		}
		n, _ := strconv.Atoi(v)
		return u.t("aiContextPreset", "k", n/1024, "tokens", n)
	}
	ui.Column(c).Gap(8).MinWidth(200).Children(func() {
		u.choiceWith(c, u.t("aiContext"), value, choices, label, func(v string) {
			f.contextErr = false
			if v == customContext {
				f.contextCustom, f.contextText = true, strconv.Itoa(u.prefs.AI.ContextTokens)
				return
			}
			n, _ := strconv.Atoi(v)
			f.contextCustom = false
			p := u.prefs
			p.AI.ContextTokens = n
			u.setPreferences(p)
		})
		if value != customContext {
			return
		}
		if !f.contextCustom {
			f.contextCustom, f.contextText = true, strconv.Itoa(current)
		}
		ui.Box(c).FillWidth().Key("ai:contextTokens").Children(func() {
			box := ui.TextInput(c, &f.contextText).FillWidth().Height(kitFieldH).Label(u.t("aiContextTokens")).Placeholder("8192")
			if box.Changed() {
				n, err := strconv.Atoi(strings.TrimSpace(f.contextText))
				f.contextErr = err != nil || !prefs.ValidContext(n)
				if !f.contextErr {
					p := u.prefs
					p.AI.ContextTokens = n
					u.setPreferences(p)
				}
			}
		})
		if f.contextErr {
			ui.Text(c, u.t("aiContextInvalid", "min", prefs.MinContextTokens, "max", prefs.MaxContextTokens)).Wrap().TextColor(tk.del)
		}
	})
}
