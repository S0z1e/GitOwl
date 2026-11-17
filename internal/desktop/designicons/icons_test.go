package designicons

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var pathData = regexp.MustCompile(`\sd="([^"]+)"`)

// TestEveryIconParses checks that each icon parses and has a size.
func TestEveryIconParses(t *testing.T) {
	for _, ic := range all() {
		if ic.svg == nil {
			t.Fatalf("icon %q has no parsed SVG", ic.src)
		}
		if w, h := ic.svg.Size(); w <= 0 || h <= 0 {
			t.Errorf("icon %q has size %g×%g", ic.src, w, h)
		}
	}
}

// TestPathsComeFromTheDesign checks that every path of every icon is a path
// the design boards draw, as listed in testdata/design-paths.txt.
func TestPathsComeFromTheDesign(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "design-paths.txt"))
	if err != nil {
		t.Fatal(err)
	}
	design := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			design[line] = true
		}
	}
	for _, ic := range all() {
		for _, m := range pathData.FindAllStringSubmatch(ic.src, -1) {
			if !design[m[1]] {
				t.Errorf("path %q is not drawn by the design", m[1])
			}
		}
	}
}
