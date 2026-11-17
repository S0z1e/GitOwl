package git

import "testing"

func TestRedact(t *testing.T) {
	for in, want := range map[string]string{
		"fatal: unable to access 'https://alice:t0ken@github.com/x.git/': 403": "fatal: unable to access 'https://***@github.com/x.git/': 403",
		"https://ghp_abc@github.com/x.git":                                     "https://***@github.com/x.git",
		"ssh://git@example.com/x.git":                                          "ssh://git@example.com/x.git",
		"ssh://git:pw@example.com/x.git":                                       "ssh://***@example.com/x.git",
		"git@github.com:x/y.git":                                               "git@github.com:x/y.git",
		"no url here":                                                          "no url here",
	} {
		if got := Redact(in); got != want {
			t.Errorf("Redact(%q) = %q, want %q", in, got, want)
		}
	}
}
