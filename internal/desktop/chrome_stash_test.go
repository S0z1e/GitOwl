package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

func (h *harness) firstStash() workflow.Stash {
	h.T.Helper()
	if len(h.u.chrome.navStashes) == 0 {
		h.T.Fatalf("no stash in %q", h.tt.Texts())
	}
	return h.u.chrome.navStashes[0]
}

func (h *harness) chooseStashMenu(item string) {
	h.T.Helper()
	st := h.firstStash()
	if err := h.tt.RightClick(st.Message); err != nil {
		h.T.Fatalf("open stash menu: %v (texts %q)", err, h.tt.Texts())
	}
	if err := h.tt.ChooseMenuItem(item); err != nil {
		h.T.Fatalf("%v (menu %q)", err, h.tt.Menu())
	}
	h.tt.Frame()
}

func TestStashDisabledWhenClean(t *testing.T) {
	h, _ := chromeRepo(t, 1280, 800)
	h.wantTip("Stash", h.u.t("errorNothingToStash"))
	h.click("Stash")
	if len(h.u.chrome.navStashes) != 0 {
		t.Fatal("clean worktree created a stash")
	}
}

func TestStashCreatesRowIncludingUntracked(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	h.openBranchFolders()
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "edit\n")
	gittest.Write(t, filepath.Join(dir, "new.txt"), "untracked\n")
	h.u.Refresh()
	h.tt.Frame()
	h.click("Stash")
	st := h.firstStash()
	h.want("Stashes", st.Message)
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); !os.IsNotExist(err) {
		t.Fatal("untracked file still in the worktree")
	}
	if !h.u.snap.Clean() {
		t.Fatal("worktree not clean after Stash")
	}
}

func TestStashClickOpensReadOnlyDetail(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	h.openBranchFolders()
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "edit\n")
	h.u.Refresh()
	h.tt.Frame()
	h.click("Stash")
	st := h.firstStash()
	h.click(st.Message)
	h.want(st.Message, "Unified", "Split", "Wrap lines", "Apply stash", "Pop stash", "seed.txt")
	if _, ok := h.tt.Find("Stage seed.txt"); ok {
		t.Fatal("stash detail drew a staging checkbox")
	}
	if h.tt.HasText("Generate Draft") || h.tt.HasText("Stage Hunk") || h.tt.HasText("Unstage Hunk") {
		t.Fatalf("stash detail kept working-tree chrome: %q", h.tt.Texts())
	}
	h.click(st.Message)
	h.press(ui.KeyEnter)
	if !h.u.snap.Clean() {
		t.Fatal("double click or Enter applied the stash")
	}
	if len(h.u.chrome.navStashes) != 1 {
		t.Fatal("Enter removed the stash")
	}
}

func TestStashApplyKeepsEntry(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	h.openBranchFolders()
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "edit\n")
	h.u.Refresh()
	h.tt.Frame()
	h.click("Stash")
	st := h.firstStash()
	h.click(st.Message)
	h.click("Apply stash")
	if got := h.Git(dir, "show", ":seed.txt"); !strings.Contains(got, "edit") && h.Git(dir, "diff", "HEAD", "--", "seed.txt") == "" {
		if b, err := os.ReadFile(filepath.Join(dir, "seed.txt")); err != nil || !strings.Contains(string(b), "edit") {
			t.Fatalf("apply did not restore seed.txt: %q", b)
		}
	}
	if len(h.u.chrome.navStashes) != 1 {
		t.Fatal("Apply removed the stash")
	}
}

func TestStashPopRemoves(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	h.openBranchFolders()
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "edit\n")
	h.u.Refresh()
	h.tt.Frame()
	h.click("Stash")
	st := h.firstStash()
	h.click(st.Message)
	h.click("Pop stash")
	if b, err := os.ReadFile(filepath.Join(dir, "seed.txt")); err != nil || !strings.Contains(string(b), "edit") {
		t.Fatalf("pop did not restore seed.txt: %v %q", err, b)
	}
	if len(h.u.chrome.navStashes) != 0 {
		t.Fatalf("Pop kept stashes %+v", h.u.chrome.navStashes)
	}
	if h.u.stv.open {
		t.Fatal("Pop left the stash detail open")
	}
}

func TestStashDropConfirms(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	h.openBranchFolders()
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "edit\n")
	h.u.Refresh()
	h.tt.Frame()
	h.click("Stash")
	st := h.firstStash()
	h.chooseStashMenu("Drop stash")
	h.want("Drop stash "+st.Message+"?", "Cancel")
	h.click("Cancel")
	if len(h.u.chrome.navStashes) != 1 {
		t.Fatal("cancel dropped the stash")
	}
	h.chooseStashMenu("Drop stash")
	h.click("Drop stash")
	if len(h.u.chrome.navStashes) != 0 {
		t.Fatal("confirm did not drop the stash")
	}
	if b, err := os.ReadFile(filepath.Join(dir, "seed.txt")); err != nil || strings.Contains(string(b), "edit") {
		t.Fatalf("drop applied the stash: %v %q", err, b)
	}
}

func TestStashMenuGroupsInDesignOrder(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	h.openBranchFolders()
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "edit\n")
	h.u.Refresh()
	h.tt.Frame()
	h.click("Stash")
	st := h.firstStash()
	if err := h.tt.RightClick(st.Message); err != nil {
		t.Fatal(err)
	}
	got := h.tt.Menu()
	want := []string{
		st.Message,
		"-",
		"Apply stash",
		"Pop stash",
		"-",
		"New branch from stash",
		"-",
		"Compare",
		"Compare to working dir",
		"Save as patch",
		"-",
		"Rename message",
		"Drop stash",
		"-",
		"Copy stash name",
	}
	if len(got) != len(want) {
		t.Fatalf("menu %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("menu %q", got)
		}
	}
}

func TestStashNewBranchReusesDialog(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	h.openBranchFolders()
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "edit\n")
	h.u.Refresh()
	h.tt.Frame()
	h.click("Stash")
	st := h.firstStash()
	h.chooseStashMenu("New branch from stash")
	h.want("New Branch", "Starts from this stash: "+st.Message+".", "Switch to the new branch", "Create Branch")
	h.click("Branch name")
	h.tt.Type("from-stash")
	h.tt.Frame()
	h.click("Create Branch")
	if got := strings.TrimSpace(h.Git(dir, "branch", "--show-current")); got != "from-stash" {
		t.Fatalf("branch %q", got)
	}
}

func TestStashRenameReusesNameField(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	h.openBranchFolders()
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "edit\n")
	h.u.Refresh()
	h.tt.Frame()
	h.click("Stash")
	st := h.firstStash()
	h.chooseStashMenu("Rename message")
	h.want("Rename message", "Stash message", "Rename", "Renames this stash message.")
	if h.u.br.form.name != st.Message {
		t.Fatalf("rename field %q, want the current message %q", h.u.br.form.name, st.Message)
	}
	h.u.br.form.name = "parked work"
	h.tt.Frame()
	h.click("Rename")
	if h.firstStash().Message != "parked work" {
		t.Fatalf("renamed to %q", h.firstStash().Message)
	}
	h.want("parked work")
}

func TestStashCompareLeavesFromNav(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	h.openBranchFolders()
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "edit\n")
	h.u.Refresh()
	h.tt.Frame()
	h.click("Stash")
	st := h.firstStash()
	h.chooseStashMenu("Compare to working dir")
	h.want("Compare "+st.Message+" and working directory", "Unified", "Split")
	if h.tt.HasText("Generate Draft") {
		t.Fatal("compare kept the commit box")
	}
	h.click("Current changes")
	if h.u.cmp.open || h.u.stv.open {
		t.Fatal("Current changes did not leave compare")
	}
}

func TestStashSaveAsPatchAndCopyName(t *testing.T) {
	var saved string
	g := gittest.New(t)
	u := New(Options{
		Workflow:  workflow.New(g.Runner),
		Preferred: []string{"en-US"},
		Go:        func(fn func()) { fn() },
		ChooseSaveFile: func(title, suggested string) (string, error) {
			saved = filepath.Join(t.TempDir(), suggested)
			return saved, nil
		},
	})
	h := &harness{Env: g, u: u, tt: ui.NewTester(chromeWindow(u), 1280, 800)}
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "s\n")
	h.Commit(dir, "Initial")
	h.open(dir)
	h.openBranchFolders()
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "edit\n")
	h.u.Refresh()
	h.tt.Frame()
	h.click("Stash")
	st := h.firstStash()
	h.chooseStashMenu("Save as patch")
	body, err := os.ReadFile(saved)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "edit") {
		t.Fatalf("patch %q", body)
	}
	h.chooseStashMenu("Copy stash name")
	if got := h.tt.Clipboard(); got != st.Message {
		t.Fatalf("clipboard %q, want %q", got, st.Message)
	}
}
