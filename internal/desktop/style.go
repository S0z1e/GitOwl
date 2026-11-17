package desktop

import "github.com/egoist/mygo/ui"

// rowHeight is the height of a file, branch or commit-file row, in DIPs.
const rowHeight = 34

// twoLineRowHeight is the height of a file row that puts its directory
// under its name, in a narrow window.
const twoLineRowHeight = 44

// Type roles used by the remaining Diff cards. All text is in the theme's
// font at one of these sizes; views pick a role rather than a size.

func caption(c *ui.Context, s string) ui.Element {
	t := c.Theme()
	return ui.Text(c, s).FontSize(t.Rem(12.0 / 13)).TextColor(t.TextMuted)
}

func codeText(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).Font("monospace").FontSize(c.Theme().Rem(12.5 / 13))
}

func kitPlaceholder(c *ui.Context, title, body string) {
	tk := kitDesignTokens(c.Theme().Dark)
	ui.Column(c).Fill().Center().Padding(24).Gap(8).Children(func() {
		if title != "" {
			ui.Text(c, title).Bold().TextColor(tk.text)
		}
		if body != "" {
			ui.Text(c, body).Wrap().TextColor(tk.muted)
		}
	})
}
