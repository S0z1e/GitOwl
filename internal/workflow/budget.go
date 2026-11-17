package workflow

import (
	"unicode/utf8"

	"gitowl/internal/ai"
	"gitowl/internal/prefs"
)

// Budget divides a model's context between a draft request and its reply.
// GitOwl plans the request to fit Input; it does not send the reserve to
// the service, which applies its own output limit.
type Budget struct {
	// Context is the model's context size in tokens.
	Context int
	// Output is kept free for the reply.
	Output int
	// Margin covers message wrapping and the estimate's error.
	Margin int
	// Input is the most a request may need, as EstimateTokens counts.
	Input int
}

// Reserves of every budget, in tokens.
const (
	OutputReserve = 1024
	MinMargin     = 1024
)

// NewBudget returns the budget of a context of n tokens; an n outside the
// accepted range uses the default size.
func NewBudget(n int) Budget {
	if !prefs.ValidContext(n) {
		n = prefs.DefaultContextTokens
	}
	b := Budget{Context: n, Output: OutputReserve, Margin: max(MinMargin, n/8)}
	b.Input = n - b.Output - b.Margin
	return b
}

// Token estimate parameters. The estimate is deliberately high for common
// tokenizers: English text and code average more than three bytes per
// token, and most Chinese characters take one or two.
const (
	asciiBytesPerToken = 3
	otherRuneTokens    = 2
	messageTokens      = 4
	replyPrimingTokens = 3
)

// EstimateTokens estimates what messages need of a model's context: one
// token per three ASCII bytes, rounded up, two per other character, four
// per message for its wrapping and three to prime the reply. It is
// reproducible and does not depend on the model; a model's own tokenizer
// may count fewer or, rarely, more.
func EstimateTokens(msgs []ai.Message) int {
	n := replyPrimingTokens
	for _, m := range msgs {
		n += messageTokens + estimateText(m.Role) + estimateText(m.Content)
	}
	return n
}

func estimateText(s string) int {
	ascii, other := 0, 0
	for i := 0; i < len(s); {
		if s[i] < utf8.RuneSelf {
			ascii++
			i++
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		other++
		i += size
	}
	return (ascii+asciiBytesPerToken-1)/asciiBytesPerToken + other*otherRuneTokens
}
