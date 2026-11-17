package desktop

import (
	"time"
)

// choose opens the select labeled label and picks item in its popup.
func (h *harness) choose(label, item string) {
	h.T.Helper()
	h.click(label)
	h.tt.Advance(time.Second)
	h.tt.Frame()
	if err := h.tt.Click(item); err != nil {
		h.T.Fatalf("choose %q in %q: %v (%q)", item, label, err, h.tt.Texts())
	}
	h.tt.Frame()
}
