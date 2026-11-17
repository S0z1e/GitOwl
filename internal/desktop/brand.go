package desktop

import (
	_ "embed"

	"github.com/egoist/mygo/ui"
)

// owlPNG is GitOwl's brand mark: the owl of the application icon, at 256
// pixels, which the app draws at its own size.
//
//go:embed owl.png
var owlPNG []byte

// owlBitmap is owlPNG decoded once at start.
var owlBitmap = mustDecodeBitmap(owlPNG)

// OwlImage returns GitOwl's brand mark.
func OwlImage() *ui.Bitmap { return owlBitmap }

func mustDecodeBitmap(data []byte) *ui.Bitmap {
	b, err := ui.DecodeBitmap(data)
	if err != nil {
		panic("desktop: brand mark: " + err.Error())
	}
	return b
}
