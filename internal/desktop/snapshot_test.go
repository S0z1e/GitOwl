package desktop

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// near reports whether got is within tol of want, for coordinates whose
// exact value depends on text width.
func near(got, want, tol float32) bool {
	d := got - want
	if d < 0 {
		d = -d
	}
	return d <= tol
}

func shellNear(t *testing.T, what string, got, want, tol float32) {
	t.Helper()
	if !near(got, want, tol) {
		t.Fatalf("%s = %.1f, want %.1f ±%.1f", what, got, want, tol)
	}
}

// snapshot writes the tester's last frame to $GITOWL_SNAPSHOT_DIR, when set,
// for reviewing layouts without a desktop session.
func snapshot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("GITOWL_SNAPSHOT_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second) // let dialogs finish their entrance
	tt.Frame()
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
