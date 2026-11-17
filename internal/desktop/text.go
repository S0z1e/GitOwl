package desktop

import (
	"github.com/egoist/mygo/i18n"

	"gitowl/internal/locales"
)

// Supported interface languages besides following the system.
var interfaceLanguages = []string{"en", "zh-Hans"}

// ResolveLanguage returns the catalog language to show: the saved choice, or
// the best match for the system's preferred languages, falling back to
// English when none is supported.
func ResolveLanguage(choice string, preferred []string) string {
	for _, l := range interfaceLanguages {
		if choice == l {
			return l
		}
	}
	if tag := i18n.Match(preferred, locales.Catalog.Languages()); tag != "" {
		return tag
	}
	return locales.Catalog.Source()
}

func (u *UI) language() string { return ResolveLanguage(u.prefLanguage(), u.preferred) }

// t formats the message key in the interface language. kv lists placeholder
// names and values in pairs.
func (u *UI) t(key string, kv ...any) string {
	var args map[string]any
	if len(kv) > 1 {
		args = make(map[string]any, len(kv)/2)
		for i := 0; i+1 < len(kv); i += 2 {
			args[kv[i].(string)] = kv[i+1]
		}
	}
	return locales.Catalog.Format(u.language(), key, args)
}
