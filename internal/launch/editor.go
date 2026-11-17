package launch

import (
	"os"
	"path/filepath"

	"gitowl/internal/prefs"
)

// editorCandidates are the editors GitOwl looks for while none is chosen,
// most preferred first. Each is an app's name without the .app suffix.
var editorCandidates = []string{"Visual Studio Code", "Cursor", "Zed", "Sublime Text"}

// FindEditor returns the first of the candidate editors installed in one of
// dirs, and whether one was found. The result is a preference, as the
// editor the user would otherwise choose.
func FindEditor(dirs ...string) (prefs.Editor, bool) {
	for _, name := range editorCandidates {
		for _, dir := range dirs {
			path := filepath.Join(dir, name+".app")
			if st, err := os.Stat(path); err == nil && st.IsDir() {
				return prefs.Editor{Name: name, Path: path}, true
			}
		}
	}
	return prefs.Editor{}, false
}
