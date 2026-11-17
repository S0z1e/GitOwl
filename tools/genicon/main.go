// Command genicon renders GitOwl's application icon, assets/icon.png, from
// the design's owl (designicons.Owl, the Landing board's mark):
//
//	go run ./tools/genicon
//
// The icon is a 1024×1024 PNG laid out on Apple's macOS icon grid: an 824-px
// rounded square in the app's blue accent (kit tokens accent #2563EB, darkening
// to the accent text colour #1D4ED8) with transparent margins. MyGo derives the
// .icns from it when packaging.
package main

import (
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/desktop/designicons"
)

const size = 1024

func main() {
	out := filepath.Join("assets", "icon.png")
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	img := ui.Render(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Box(c).Fill().Center().Children(func() {
			ui.Box(c).Size(824, 824).Radius(185).Center().
				LinearGradient(ui.LinearGradient{From: ui.Hex("#2563eb"), To: ui.Hex("#1d4ed8"), Angle: 180, Oklab: true}).
				Shadow(0, 10, 28, 0, ui.RGBA(0, 0, 0, .28)).
				Children(func() {
					ui.Icon(c, designicons.Owl()).Size(560, 560).TextColor(ui.Hex("#fff8f2"))
				})
		})
	}, size, size, 1)
	if err := write(out, img); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s", out)
}

func write(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
