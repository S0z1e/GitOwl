package prefs

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPreferencesRoundTripAndFallback(t *testing.T) {
	s := Store{Path: filepath.Join(t.TempDir(), "GitOwl", "preferences.json")}
	got, err := s.Load()
	if err != nil || !got.Equal(Default()) {
		t.Fatalf("missing file: %+v %v", got, err)
	}
	want := Preferences{Language: "zh-Hans", Appearance: "dark", AI: AI{BaseURL: "https://api.example.com/v1", Model: "m", Prompt: "Use Conventional Commits", Language: "en", ContextTokens: 32768}}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	if got, err = s.Load(); err != nil || !got.Equal(want) {
		t.Fatalf("round trip %+v %v", got, err)
	}
	if data, _ := os.ReadFile(s.Path); strings.Contains(string(data), "key") {
		t.Fatalf("preferences mention a key: %s", data)
	}
	os.WriteFile(s.Path, []byte(`{"language":"fr","appearance":"sepia","ai":{"language":"fr"}}`), 0o600)
	if got, err = s.Load(); err != nil || !got.Equal(Default()) {
		t.Fatalf("unknown values %+v %v", got, err)
	}
	os.WriteFile(s.Path, []byte(`{broken`), 0o600)
	if got, err = s.Load(); err == nil || !got.Equal(Default()) {
		t.Fatalf("corrupt file %+v %v", got, err)
	}
	// A failed save leaves the previous file.
	os.WriteFile(s.Path, []byte(`{"language":"en","appearance":"light"}`), 0o600)
	bad := Store{Path: filepath.Join(s.Path, "nested.json")}
	if err := bad.Save(want); err == nil {
		t.Fatal("save into a file path succeeded")
	}
	if got, _ = s.Load(); got.Language != "en" {
		t.Fatalf("previous preferences lost: %+v", got)
	}
}

func TestContextSizeDefaultsAndRange(t *testing.T) {
	s := Store{Path: filepath.Join(t.TempDir(), "preferences.json")}
	// Preferences saved before the context size existed get the default.
	os.WriteFile(s.Path, []byte(`{"language":"en","appearance":"dark","ai":{"baseURL":"http://localhost:11434/v1","model":"llama","language":"en"}}`), 0o600)
	got, err := s.Load()
	if err != nil || got.AI.ContextTokens != DefaultContextTokens || got.AI.Model != "llama" || got.Appearance != "dark" {
		t.Fatalf("old file %+v %v", got, err)
	}
	for _, n := range []int{0, -1, 100, MinContextTokens - 1, MaxContextTokens + 1} {
		os.WriteFile(s.Path, []byte(`{"ai":{"contextTokens":`+strconv.Itoa(n)+`}}`), 0o600)
		if got, _ := s.Load(); got.AI.ContextTokens != DefaultContextTokens {
			t.Errorf("%d kept as %d", n, got.AI.ContextTokens)
		}
	}
	for _, n := range []int{MinContextTokens, 12000, MaxContextTokens} {
		p := Default()
		p.AI.ContextTokens = n
		if err := s.Save(p); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.Load(); got.AI.ContextTokens != n {
			t.Errorf("%d loaded as %d", n, got.AI.ContextTokens)
		}
	}
	os.WriteFile(s.Path, []byte(`{"ai":{"contextTokens":"8k"}}`), 0o600)
	if got, err := s.Load(); err == nil || !got.Equal(Default()) {
		t.Fatalf("non-number %+v %v", got, err)
	}
}

func TestRecentList(t *testing.T) {
	s := Store{Path: filepath.Join(t.TempDir(), "preferences.json")}
	// Saved before the list existed: empty, the rest kept.
	os.WriteFile(s.Path, []byte(`{"language":"zh-Hans","appearance":"dark"}`), 0o600)
	got, err := s.Load()
	if err != nil || got.Recent != nil || got.Language != "zh-Hans" {
		t.Fatalf("old file %+v %v", got, err)
	}
	// Invalid entries go one by one; valid ones and other settings stay.
	os.WriteFile(s.Path, []byte(`{"language":"en","appearance":"light","ai":{"model":"m"},"recent":["/a","relative","/a/","",42,"/b/../c",{"x":1}]}`), 0o600)
	got, err = s.Load()
	if err != nil || strings.Join(got.Recent, ",") != "/a,/c" || got.AI.Model != "m" || got.Appearance != "light" {
		t.Fatalf("mixed list %+v %v", got, err)
	}
	var list []string
	for i := range 12 {
		list = AddRecent(list, "/r"+strconv.Itoa(i))
	}
	list = AddRecent(list, "/r5")
	if len(list) != MaxRecent || list[0] != "/r5" || list[1] != "/r11" || list[MaxRecent-1] != "/r2" {
		t.Fatalf("list %q", list)
	}
	list = RemoveRecent(list, "/r5")
	if len(list) != MaxRecent-1 || list[0] != "/r11" {
		t.Fatalf("after remove %q", list)
	}
	p := Default()
	p.Recent = list
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Load(); !got.Equal(p) {
		t.Fatalf("round trip %+v", got)
	}
}

func TestPinnedList(t *testing.T) {
	s := Store{Path: filepath.Join(t.TempDir(), "preferences.json")}
	os.WriteFile(s.Path, []byte(`{"language":"en","appearance":"light"}`), 0o600)
	got, err := s.Load()
	if err != nil || got.Pinned != nil || got.Language != "en" {
		t.Fatalf("old file %+v %v", got, err)
	}
	os.WriteFile(s.Path, []byte(`{"language":"en","pinned":["/a","relative","/a/","",42,"/b/../c",{"x":1}]}`), 0o600)
	got, err = s.Load()
	if err != nil || strings.Join(got.Pinned, ",") != "/a,/c" || got.Language != "en" {
		t.Fatalf("mixed list %+v %v", got, err)
	}
	list := AddPinned(nil, "/r1")
	list = AddPinned(list, "/r2")
	list = AddPinned(list, "/r1")
	if len(list) != 2 || list[0] != "/r1" || list[1] != "/r2" {
		t.Fatalf("list %q", list)
	}
	list = RemovePinned(list, "/r1")
	if len(list) != 1 || list[0] != "/r2" {
		t.Fatalf("after remove %q", list)
	}
	p := Default()
	p.Pinned = list
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.Load(); !got.Equal(p) {
		t.Fatalf("round trip %+v", got)
	}
}

func TestEditorPreference(t *testing.T) {
	s := Store{Path: filepath.Join(t.TempDir(), "preferences.json")}
	p := Default()
	p.Editor = Editor{Path: "/Applications/Visual Studio Code.app"}
	p.Normalize()
	if p.Editor.Name != "Visual Studio Code" {
		t.Fatalf("name %q", p.Editor.Name)
	}
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Load(); !got.Equal(p) {
		t.Fatalf("round trip %+v", got)
	}
	for _, bad := range []string{"relative.app", "/usr/bin/vim", "/bin/sh -c x"} {
		os.WriteFile(s.Path, []byte(`{"language":"en","editor":{"name":"x","path":"`+bad+`"}}`), 0o600)
		if got, _ := s.Load(); got.Editor.Set() || got.Language != "en" {
			t.Errorf("%q kept: %+v", bad, got)
		}
	}
}
