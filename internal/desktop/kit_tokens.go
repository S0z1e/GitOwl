package desktop

import "github.com/egoist/mygo/ui"

// kitTokens are the surfaces of the custom-draw design system. They replace the older terracotta set for new chrome. Roles match the
// HTML custom properties: bg, rail, nav, surf, bd, sel, tx, ac, add, del.
type kitTokens struct {
	bg     ui.Color
	rail   ui.Color
	nav    ui.Color
	surf   ui.Color
	fill   ui.Color
	hd     ui.Color // card header (.cd .h)
	sel    ui.Color
	border ui.Color
	bd2    ui.Color
	bd3    ui.Color // hairline between rows of one panel
	cxb    ui.Color // unchecked checkbox border
	text   ui.Color
	muted  ui.Color
	dis    ui.Color
	accent ui.Color
	acx    ui.Color
	onAc   ui.Color
	add    ui.Color
	del    ui.Color
	addbg  ui.Color // added diff line
	delbg  ui.Color // removed diff line
	addwd  ui.Color // added words inside a changed line
	delwd  ui.Color // removed words inside a changed line
	mbg    ui.Color // M status badge
	mfg    ui.Color
	ubg    ui.Color // U status badge
	ufg    ui.Color
	lane2  ui.Color // odd History graph lanes (--l2)
	// groups is the palette a History branch group and an author take a
	// colour from. Only the first two are in the design; the rest are added
	// for the many branches and people of a real repository.
	groups [graphGroupCount]ui.Color
	refbg  ui.Color // ref pill fill
	refbd  ui.Color // ref pill border
	warn   ui.Color
}

var kitLight = kitTokens{
	bg:     ui.Hex("#FFFFFF"),
	rail:   ui.Hex("#F3F4F6"),
	nav:    ui.Hex("#F9FAFB"),
	surf:   ui.Hex("#FFFFFF"),
	fill:   ui.Hex("#ECEEF2"),
	hd:     ui.Hex("#F5F6F8"),
	sel:    ui.Hex("#E8EBF0"),
	border: ui.Hex("#E5E7EB"),
	bd2:    ui.Hex("#D1D5DB"),
	bd3:    ui.Hex("#ECEEF1"),
	cxb:    ui.Hex("#8A919E"),
	text:   ui.Hex("#111827"),
	muted:  ui.Hex("#4B5563"),
	dis:    ui.Hex("#9CA3AF"),
	accent: ui.Hex("#2563EB"),
	acx:    ui.Hex("#1D4ED8"),
	onAc:   ui.Hex("#FFFFFF"),
	add:    ui.Hex("#166534"),
	del:    ui.Hex("#991B1B"),
	addbg:  ui.Hex("#DCFCE7"),
	delbg:  ui.Hex("#FEE2E2"),
	addwd:  ui.Hex("#22C55E").Alpha(.35),
	delwd:  ui.Hex("#EF4444").Alpha(.28),
	mbg:    ui.Hex("#DBEAFE"),
	mfg:    ui.Hex("#1D4ED8"),
	ubg:    ui.Hex("#DCFCE7"),
	ufg:    ui.Hex("#166534"),
	lane2:  ui.Hex("#7C3AED"),
	groups: groupPaletteLight,
	refbg:  ui.Hex("#EFF6FF"),
	refbd:  ui.Hex("#BFDBFE"),
	warn:   ui.Hex("#92400E"),
}

var kitDark = kitTokens{
	bg:     ui.Hex("#111318"),
	rail:   ui.Hex("#0B0D10"),
	nav:    ui.Hex("#14171C"),
	surf:   ui.Hex("#1A1D23"),
	fill:   ui.Hex("#1E222A"),
	hd:     ui.Hex("#161A20"),
	sel:    ui.Hex("#232832"),
	border: ui.Hex("#262A32"),
	bd2:    ui.Hex("#343944"),
	bd3:    ui.Hex("#20242B"),
	cxb:    ui.Hex("#707889"),
	text:   ui.Hex("#E6E8EC"),
	muted:  ui.Hex("#A0A7B4"),
	dis:    ui.Hex("#5F6672"),
	accent: ui.Hex("#60A5FA"),
	acx:    ui.Hex("#8AB4FF"),
	onAc:   ui.Hex("#111318"),
	add:    ui.Hex("#4ADE80"),
	del:    ui.Hex("#F87171"),
	addbg:  ui.Hex("#22C55E").Alpha(.14),
	delbg:  ui.Hex("#F87171").Alpha(.14),
	addwd:  ui.Hex("#22C55E").Alpha(.32),
	delwd:  ui.Hex("#F87171").Alpha(.30),
	mbg:    ui.Hex("#608EFF").Alpha(.18),
	mfg:    ui.Hex("#9DBFFF"),
	ubg:    ui.Hex("#22C55E").Alpha(.16),
	ufg:    ui.Hex("#4ADE80"),
	lane2:  ui.Hex("#A78BFA"),
	groups: groupPaletteDark,
	refbg:  ui.Hex("#608EFF").Alpha(.14),
	refbd:  ui.Hex("#608EFF").Alpha(.38),
	warn:   ui.Hex("#FBBF24"),
}

// groupPaletteLight and groupPaletteDark are the colours of branch groups and
// authors in the two themes. Slot 0 and 1 are the design's lane blue and
// purple. The rest are proposals that need the design's sign-off.
var groupPaletteLight = [graphGroupCount]ui.Color{
	ui.Hex("#2563EB"), ui.Hex("#7C3AED"), ui.Hex("#0D9488"), ui.Hex("#EA580C"),
	ui.Hex("#DB2777"), ui.Hex("#16A34A"), ui.Hex("#CA8A04"), ui.Hex("#0891B2"),
}

var groupPaletteDark = [graphGroupCount]ui.Color{
	ui.Hex("#60A5FA"), ui.Hex("#A78BFA"), ui.Hex("#2DD4BF"), ui.Hex("#FB923C"),
	ui.Hex("#F472B6"), ui.Hex("#4ADE80"), ui.Hex("#FACC15"), ui.Hex("#22D3EE"),
}

// groupColor is the palette colour of group i.
func (t kitTokens) groupColor(i int) ui.Color { return t.groups[i] }

func kitDesignTokens(dark bool) kitTokens {
	if dark {
		return kitDark
	}
	return kitLight
}

// KitTheme maps the custom-draw design system onto MyGo's theme. It does
// not use Onevra. Focus is the opaque accent: custom drawing has no
// vibrancy, so the ring is a solid color.
func KitTheme(c *ui.Context) *ui.Theme {
	dark := false
	if cur := c.Theme(); cur != nil {
		dark = cur.Dark
	}
	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
	}
	tk := kitDesignTokens(dark)
	t.Background = tk.bg
	t.Surface = tk.surf
	t.SurfaceHover = tk.fill
	t.SurfacePressed = tk.sel
	t.Border = tk.border
	t.Text = tk.text
	t.TextMuted = tk.muted
	t.Accent = tk.accent
	t.AccentHover = tk.acx
	t.AccentPressed = tk.acx.Mix(tk.text, .18)
	t.AccentText = tk.onAc
	t.Danger = tk.del
	t.Success = tk.add
	t.Warning = tk.warn
	t.Focus = tk.accent
	t.Selection = tk.sel
	// A transparent thumb hides every scroll bar. Wheel and trackpad
	// scrolling still work.
	t.Scrollbar = ui.Transparent
	t.Radius = 12
	t.Font = ""
	return t
}

const (
	kitToolbarH = 34
	kitPrimaryH = 40
	kitFieldH   = 34
	kitIconSize = 22
)
