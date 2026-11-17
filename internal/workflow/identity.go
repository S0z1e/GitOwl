package workflow

import (
	"context"
	"strings"
)

const (
	// CodeInvalidIdentity: the name or email holds a character Git would
	// drop from an identity, so it is refused rather than changed.
	CodeInvalidIdentity Code = "InvalidIdentity"
	// CodeIdentityNameMissing: an identity needs a name; the email may be
	// empty, as Git accepts an empty user.email.
	CodeIdentityNameMissing Code = "IdentityNameMissing"
)

// Identity is the name and email Git records on commits made in a
// repository.
type Identity struct {
	Name, Email string
	// Local reports whether the repository sets its own identity. When it
	// does not, the values come from Git's global or system configuration.
	Local bool
}

// Set reports whether Git has a name to record. The email may be empty.
func (i Identity) Set() bool { return i.Name != "" }

// Identity reads the name and email Git uses for commits in this repository.
func (r *Repository) Identity(ctx context.Context) (Identity, error) {
	if !r.Current() {
		return Identity{}, &Error{Code: CodeRepositoryClosed}
	}
	id := Identity{Name: r.config(ctx, "user.name"), Email: r.config(ctx, "user.email")}
	id.Local = r.localConfig(ctx, "user.name") != "" || r.localConfig(ctx, "user.email") != ""
	return id, nil
}

// SetIdentity records name and email as the commit identity. Without global,
// the values go into this repository's own Git configuration and Git's global
// settings do not change. With global, they go into the global settings for
// every repository, and this repository's own values are removed so the new
// identity applies here too. Empty name and email remove the identity from
// the chosen scope. It returns the identity as Git now reads it.
func (r *Repository) SetIdentity(ctx context.Context, name, email string, global bool) (Identity, error) {
	defer r.lockRepository()()
	if !r.Current() {
		return Identity{}, &Error{Code: CodeRepositoryClosed}
	}
	name, email = strings.TrimSpace(name), strings.TrimSpace(email)
	scope := "--local"
	if global {
		scope = "--global"
	}
	switch {
	case name == "" && email == "":
		if err := r.unsetIdentity(ctx, scope); err != nil {
			return Identity{}, err
		}
	case name == "":
		return Identity{}, &Error{Code: CodeIdentityNameMissing}
	case !validIdentity(name) || !validIdentity(email):
		return Identity{}, &Error{Code: CodeInvalidIdentity}
	default:
		// The "--" keeps a value that starts with a dash from being read as
		// an option. An empty email is stored as an empty value, which Git
		// accepts for commits.
		if _, err := r.run(ctx, "config", scope, "--", "user.name", name); err != nil {
			return Identity{}, err
		}
		if _, err := r.run(ctx, "config", scope, "--", "user.email", email); err != nil {
			return Identity{}, err
		}
		if global {
			if err := r.unsetIdentity(ctx, "--local"); err != nil {
				return Identity{}, err
			}
		}
	}
	return r.Identity(ctx)
}

// unsetIdentity removes user.name and user.email from scope, which is
// "--local" or "--global". Exit status 5 means the key was not set there.
func (r *Repository) unsetIdentity(ctx context.Context, scope string) error {
	for _, key := range []string{"user.name", "user.email"} {
		if _, _, err := r.app.git.RunStatus(ctx, r.root, nil, []int{5}, "config", scope, "--unset", key); err != nil {
			return gitFailure(err)
		}
	}
	return nil
}

// validIdentity reports whether s can be recorded as a Git name or email.
// Git removes angle brackets and line breaks from an identity.
func validIdentity(s string) bool {
	return !strings.ContainsAny(s, "<>\r\n\x00")
}

// localConfig returns the repository's own value of key, "" when it has none.
func (r *Repository) localConfig(ctx context.Context, key string) string {
	out, _, err := r.app.git.RunStatus(ctx, r.root, nil, []int{1}, "config", "--local", "--get", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
