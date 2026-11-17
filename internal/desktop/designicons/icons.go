// Package designicons is GitOwl's one icon set. Every icon is the SVG the
// design file draws, copied from the design's own markup with two mechanical
// edits: the design's sc-camel-view-box attribute is written as the standard
// viewBox, and an xmlns is added so each source is a self-contained
// document. The set uses the design's 16 × 16 grid with a 1.6 stroke, round
// caps and joins, except StageCheck, which the design draws on a 24 × 24 grid.
//
// MyGo draws an icon in the text color it is given and ignores the colors
// inside its SVG, so the sources carry no colors.
package designicons

import "github.com/egoist/mygo/ui"

// icon pairs a source with its parsed form. The test reads sources to check
// them against the design file.
type icon struct {
	src string
	svg *ui.SVG
}

func mk(inner string) icon {
	src := `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">` + inner + `</svg>`
	return icon{src: src, svg: ui.MustParseSVG([]byte(src))}
}

var (
	fetch    = mk(`<path d="M8 2v8M4.5 6.5L8 10l3.5-3.5M3 13h10"/>`)
	pull     = mk(`<path d="M8 3v9M4.5 8.5L8 12l3.5-3.5"/>`)
	push     = mk(`<path d="M8 13V4M4.5 7.5L8 4l3.5 3.5"/>`)
	stash    = mk(`<rect x="2.5" y="3" width="11" height="3" rx="1"/><path d="M3.5 6v6.5h9V6M6.5 9h3"/>`)
	editor   = mk(`<path d="M6 5L3 8l3 3M10 5l3 3-3 3"/>`)
	terminal = mk(`<rect x="2" y="3" width="12" height="10" rx="2"/><path d="M5 7l2 1.5L5 10M8.5 10h2.5"/>`)
	finder   = mk(`<path d="M2 4.5A1.5 1.5 0 0 1 3.5 3h3L8 4.5h4.5A1.5 1.5 0 0 1 14 6v5.5a1.5 1.5 0 0 1-1.5 1.5h-9A1.5 1.5 0 0 1 2 11.5z"/>`)
	folder   = mk(`<path d="M2 4.5A1.5 1.5 0 0 1 3.5 3h3L8 4.5h4.5A1.5 1.5 0 0 1 14 6v5.5a1.5 1.5 0 0 1-1.5 1.5h-9A1.5 1.5 0 0 1 2 11.5z"/>`)
	sidebar  = mk(`<rect x="2" y="3" width="12" height="10" rx="2.5"/><path d="M6 3v10"/>`)
	changes  = mk(`<path d="M4 3h6l2.5 2.5V13H4z"/><path d="M6.5 8.5h3M8 7v3"/>`)
	commits  = mk(`<path d="M8 2v4M8 10v4"/><circle cx="8" cy="8" r="2"/>`)
	branch   = mk(`<path d="M5 5.2v5.6M11.5 7.7c0 2.5-6.5 1.5-6.5 3.1"/><circle cx="5" cy="3.5" r="1.7"/><circle cx="5" cy="12.5" r="1.7"/><circle cx="11.5" cy="6" r="1.7"/>`)
	settings = mk(`<path d="M3 5h10M3 11h10"/><circle cx="6" cy="5" r="1.6"/><circle cx="10.5" cy="11" r="1.6"/>`)
	search   = mk(`<circle cx="7" cy="7" r="4.5"/><path d="M10.5 10.5L14 14"/>`)
	pin      = mk(`<path d="M5.5 2.5h5l-.7 4.2L12 9H4l2.2-2.3z"/><path d="M8 9v4.5"/>`)
	pinFill  = mk(`<path fill="currentColor" d="M5.5 2.5h5l-.7 4.2L12 9H4l2.2-2.3z"/><path d="M8 9v4.5"/>`)
	closeX   = mk(`<path d="M4 4l8 8M12 4l-8 8"/>`)
	check    = mk(`<path d="M3.5 8.5l3 3 6-7"/>`)
	sortIcon = mk(`<path d="M5 3v10M2.5 10.5L5 13l2.5-2.5M11 13V3M8.5 5.5L11 3l2.5 2.5"/>`)
	chevronR = mk(`<path d="M6 4l4 4-4 4"/>`)
	chevronD = mk(`<path d="M4 6l4 4 4-4"/>`)
	copyIcon = mk(`<rect x="5.5" y="5.5" width="8" height="8" rx="1.6"/><path d="M10.5 5.5V4A1.5 1.5 0 0 0 9 2.5H4A1.5 1.5 0 0 0 2.5 4v5A1.5 1.5 0 0 0 4 10.5h1.5"/>`)
	sparkle  = mk(`<path d="M8 2l1.3 3.7L13 7l-3.7 1.3L8 12 6.7 8.3 3 7l3.7-1.3z"/>`)
	minus    = mk(`<path d="M3 8h10"/>`)
	plus     = mk(`<path d="M8 3v10M3 8h10"/>`)
	cloud    = mk(`<path d="M4.5 12.5a3 3 0 0 1-.3-6 4 4 0 0 1 7.6 1A2.5 2.5 0 0 1 11.5 12.5z"/>`)
	tag      = mk(`<path d="M2.5 8.5V3h5.5l6 6-5.5 5.5z"/><circle cx="5.5" cy="5.5" r=".8"/>`)

	// owl is GitOwl's mark on the welcome screen and in the application icon.
	// The design draws it on a 24-unit grid, so it keeps that grid.
	owl = icon{src: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M5 4l3 3h8l3-3v9a7 7 0 0 1-14 0z"/><circle cx="9.5" cy="12" r="1.7"/><circle cx="14.5" cy="12" r="1.7"/><path d="M12 15v1.5"/></svg>`}

	// stageCheck is the check drawn inside a staged file's box. The design
	// draws it on a 24-unit grid with a heavier stroke, so it keeps that.
	stageCheck = icon{
		src: `<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3.4" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12.5l4.5 4.5L19 7.5"/></svg>`,
	}
)

func init() {
	stageCheck.svg = ui.MustParseSVG([]byte(stageCheck.src))
	owl.svg = ui.MustParseSVG([]byte(owl.src))
}

// Owl is GitOwl's mark: the welcome screen's owl and the application icon.
func Owl() *ui.SVG { return owl.svg }

// Fetch is the toolbar's download arrow.
func Fetch() *ui.SVG { return fetch.svg }

// Pull is the toolbar's down arrow.
func Pull() *ui.SVG { return pull.svg }

// Push is the toolbar's up arrow.
func Push() *ui.SVG { return push.svg }

// Stash is the archive box with its lid.
func Stash() *ui.SVG { return stash.svg }

// Editor is the code brackets that open the editor.
func Editor() *ui.SVG { return editor.svg }

// Terminal is the window with a prompt.
func Terminal() *ui.SVG { return terminal.svg }

// Finder is the folder that reveals the worktree.
func Finder() *ui.SVG { return finder.svg }

// Folder is the repository and worktree glyph.
func Folder() *ui.SVG { return folder.svg }

// Sidebar is the panel toggle.
func Sidebar() *ui.SVG { return sidebar.svg }

// Changes is the document of Current changes.
func Changes() *ui.SVG { return changes.svg }

// Commits is the commit node of All commits.
func Commits() *ui.SVG { return commits.svg }

// Branch is the branch glyph.
func Branch() *ui.SVG { return branch.svg }

// Settings is the sliders glyph.
func Settings() *ui.SVG { return settings.svg }

// Search is the magnifier of the filter fields.
func Search() *ui.SVG { return search.svg }

// Pin is the pin outline.
func Pin() *ui.SVG { return pin.svg }

// PinFilled is the pin when it is pinned.
func PinFilled() *ui.SVG { return pinFill.svg }

// Close is the ✕ that clears a field or removes a row.
func Close() *ui.SVG { return closeX.svg }

// Check is the check mark that marks the current item.
func Check() *ui.SVG { return check.svg }

// StageCheck is the check inside a staged file's box.
func StageCheck() *ui.SVG { return stageCheck.svg }

// ChevronRight is the collapsed disclosure chevron.
func ChevronRight() *ui.SVG { return chevronR.svg }

// ChevronDown is the expanded disclosure and menu chevron.
func ChevronDown() *ui.SVG { return chevronD.svg }

// Copy is the copy glyph.
func Copy() *ui.SVG { return copyIcon.svg }

// Sort is the up-and-down arrows of the History sort chooser.
func Sort() *ui.SVG { return sortIcon.svg }

// Sparkle marks the AI Generate Draft action.
func Sparkle() *ui.SVG { return sparkle.svg }

// Minus is the horizontal bar, for unstaging a chunk.
func Minus() *ui.SVG { return minus.svg }

// Plus is the cross's upright and bar, for staging a chunk.
func Plus() *ui.SVG { return plus.svg }

// Cloud marks a remote.
func Cloud() *ui.SVG { return cloud.svg }

// Tag marks a tag.
func Tag() *ui.SVG { return tag.svg }

// all lists every icon of the set, for the test.
func all() []icon {
	return []icon{fetch, pull, push, stash, editor, terminal, finder, folder, sidebar,
		changes, commits, branch, settings, search, pin, pinFill, closeX, check, sortIcon,
		chevronR, chevronD, copyIcon, sparkle, minus, plus, cloud, tag, stageCheck, owl}
}
