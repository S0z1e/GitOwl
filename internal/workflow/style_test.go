package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func stageFile(t *testing.T, e *env, dir string) {
	write(t, filepath.Join(dir, "change.txt"), "staged "+t.Name()+"\n")
	e.git(dir, "add", "change.txt")
}

func history(t *testing.T, e *env, dir string, messages ...string) {
	for i, m := range messages {
		write(t, filepath.Join(dir, "h.txt"), strings.Repeat("x", i+1)+"\n")
		e.commit(dir, m)
	}
}

func TestDraftFollowsChineseHistory(t *testing.T) {
	e, f, _, cfg := aiEnv(t)
	dir := e.repo()
	history(t, e, dir, "feat: 初始化项目", "fix(解析): 修复空行处理", "docs: 更新使用说明")
	stageFile(t, e, dir)
	r, _ := e.open(dir)
	req, err := r.PrepareDraft(context.Background(), DraftOptions{Language: "auto", InterfaceLanguage: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if req.Language != "zh-Hans" || req.LanguageSource != LanguageFromHistory || req.Style != StyleFromHistory || len(req.Recent) != 3 {
		t.Fatalf("%s %d %d %v", req.Language, req.LanguageSource, req.Style, req.Recent)
	}
	if _, err := r.GenerateDraft(context.Background(), req, cfg); err != nil {
		t.Fatal(err)
	}
	sent := f.first()
	if !strings.Contains(sent.Messages[0].Content, "Simplified Chinese") || !strings.Contains(sent.Messages[0].Content, "Match the style") {
		t.Fatalf("system:\n%s", sent.Messages[0].Content)
	}
	user := sent.Messages[1].Content
	if strings.Index(user, "docs: 更新使用说明") > strings.Index(user, "feat: 初始化项目") || !strings.Contains(user, "- fix(解析): 修复空行处理\n") {
		t.Fatalf("history not sent newest first:\n%s", user)
	}
}

func TestDraftFollowsEnglishHistoryAndSkipsMerges(t *testing.T) {
	e, _, _, _ := aiEnv(t)
	dir := e.repo()
	history(t, e, dir, "Add parser", "Fix empty lines")
	e.git(dir, "switch", "-q", "-c", "side")
	history(t, e, dir, "Side work")
	e.git(dir, "switch", "-q", "main")
	e.git(dir, "merge", "-q", "--no-ff", "-m", "合并分支 side", "side")
	stageFile(t, e, dir)
	r, _ := e.open(dir)
	req, err := r.PrepareDraft(context.Background(), DraftOptions{Language: "auto", InterfaceLanguage: "zh-Hans"})
	if err != nil {
		t.Fatal(err)
	}
	if req.Language != "en" || req.LanguageSource != LanguageFromHistory {
		t.Fatalf("%s %d", req.Language, req.LanguageSource)
	}
	for _, m := range req.Recent {
		if strings.Contains(m, "合并") {
			t.Fatalf("merge message sampled: %v", req.Recent)
		}
	}
	if len(req.Recent) != 3 {
		t.Fatalf("recent %v", req.Recent)
	}
}

func TestDraftWithoutHistoryUsesDefaultsAndInterfaceLanguage(t *testing.T) {
	e, _, _, _ := aiEnv(t)
	dir := e.repo()
	stageFile(t, e, dir)
	r, _ := e.open(dir)
	req, err := r.PrepareDraft(context.Background(), DraftOptions{Language: "auto", InterfaceLanguage: "zh-Hans"})
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Recent) != 0 || req.Style != StyleDefault || req.Language != "zh-Hans" || req.LanguageSource != LanguageFromInterface {
		t.Fatalf("%+v", req)
	}
	sys := req.Messages[0].Content
	if !strings.Contains(sys, "imperative verb") || strings.Contains(req.Messages[1].Content, "Recent commit") {
		t.Fatalf("defaults:\n%s", sys)
	}
	// A custom prompt still takes precedence without history.
	req, _ = r.PrepareDraft(context.Background(), DraftOptions{Prompt: "Start with an emoji.", Language: "auto", InterfaceLanguage: "en"})
	if req.Style != StyleCustom || strings.Contains(req.Messages[0].Content, "imperative verb") {
		t.Fatalf("custom without history:\n%s", req.Messages[0].Content)
	}
}

func TestDraftUnrecognizedLanguageFallsBack(t *testing.T) {
	e, _, _, _ := aiEnv(t)
	dir := e.repo()
	history(t, e, dir, "1.2.3", "🚀", "v2")
	stageFile(t, e, dir)
	r, _ := e.open(dir)
	req, _ := r.PrepareDraft(context.Background(), DraftOptions{Language: "auto", InterfaceLanguage: "zh-Hans"})
	// "v2" has a Latin letter: one English message against none.
	if req.Language != "en" {
		t.Fatalf("v2 counted: %s", req.Language)
	}
	dir2 := e.repo()
	history(t, e, dir2, "1.2.3", "🚀")
	stageFile(t, e, dir2)
	r2, _ := e.open(dir2)
	req, _ = r2.PrepareDraft(context.Background(), DraftOptions{Language: "auto", InterfaceLanguage: "zh-Hans"})
	if req.Language != "zh-Hans" || req.LanguageSource != LanguageFromInterface {
		t.Fatalf("no letters: %s %d", req.Language, req.LanguageSource)
	}
}

func TestCustomPromptAndManualLanguageOverrideHistory(t *testing.T) {
	e, f, _, cfg := aiEnv(t)
	dir := e.repo()
	history(t, e, dir, "修复问题", "增加功能")
	stageFile(t, e, dir)
	r, _ := e.open(dir)
	req, _ := r.PrepareDraft(context.Background(), DraftOptions{Prompt: "Use Conventional Commits.", Language: "en", InterfaceLanguage: "zh-Hans"})
	if req.Style != StyleCustom || req.Language != "en" || req.LanguageSource != LanguageChosen || len(req.Recent) != 0 {
		t.Fatalf("%+v", req)
	}
	r.GenerateDraft(context.Background(), req, cfg)
	if user := f.last().Messages[1].Content; strings.Contains(user, "修复问题") || strings.Contains(user, "Recent commit") {
		t.Fatalf("history sent with a custom prompt:\n%s", user)
	}
	// With a custom prompt the language still follows history, locally.
	req, _ = r.PrepareDraft(context.Background(), DraftOptions{Prompt: "Use Conventional Commits.", Language: "auto", InterfaceLanguage: "en"})
	if req.Language != "zh-Hans" || req.LanguageSource != LanguageFromHistory || len(req.Recent) != 0 {
		t.Fatalf("custom prompt language %s %d %v", req.Language, req.LanguageSource, req.Recent)
	}
	sys := f.last().Messages[0].Content
	if !strings.Contains(sys, "Use Conventional Commits.") || strings.Contains(sys, "Match the style") || !strings.Contains(sys, "in English") {
		t.Fatalf("system:\n%s", sys)
	}
}

func TestOnlyRecentSubjectsAreSent(t *testing.T) {
	e, _, _, _ := aiEnv(t)
	dir := e.repo()
	long := strings.Repeat("长", 800)
	var msgs []string
	for i := range 25 {
		msgs = append(msgs, "Commit "+itoa(i)+"\n\nBODY-"+itoa(i)+" "+strings.Repeat("y", 50))
	}
	msgs = append(msgs, long+"\n\nlong body")
	history(t, e, dir, msgs...)
	stageFile(t, e, dir)
	r, _ := e.open(dir)
	req, err := r.PrepareDraft(context.Background(), DraftOptions{Language: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Recent) != RecentExamples {
		t.Fatalf("recent %v", req.Recent)
	}
	if len([]rune(req.Recent[0])) != RecentSubjectChars+1 || !strings.HasSuffix(req.Recent[0], "…") {
		t.Fatalf("long subject not cut: %d", len([]rune(req.Recent[0])))
	}
	if req.Recent[1] != "Commit 24" || req.Recent[2] != "Commit 23" {
		t.Fatalf("not newest first: %q", req.Recent)
	}
	all := req.Messages[0].Content + req.Messages[1].Content
	if strings.Contains(all, "BODY-") || strings.Contains(all, "Commit 22") || strings.Contains(all, "long body") {
		t.Fatalf("more than the subjects sent:\n%s", all)
	}
	// Language detection still reads more of the history, locally: the long
	// Chinese subject is one message against many English ones.
	if req.Language != "en" || req.LanguageSource != LanguageFromHistory {
		t.Fatalf("language %s %d", req.Language, req.LanguageSource)
	}
}

func TestSwitchedRepositoryDoesNotReuseHistory(t *testing.T) {
	e, _, _, _ := aiEnv(t)
	a, b := e.repo(), e.repo()
	history(t, e, a, "Only in a")
	stageFile(t, e, b)
	ra, _ := e.open(a)
	rb, _ := e.open(b)
	if _, err := ra.PrepareDraft(context.Background(), DraftOptions{}); CodeOf(err) != CodeRepositoryClosed {
		t.Fatalf("closed repository prepared: %v", err)
	}
	req, _ := rb.PrepareDraft(context.Background(), DraftOptions{})
	if len(req.Recent) != 0 || strings.Contains(req.Messages[1].Content, "Only in a") {
		t.Fatalf("a's history in b: %v", req.Recent)
	}
}
