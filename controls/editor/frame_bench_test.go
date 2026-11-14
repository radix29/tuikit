package editor

import (
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/theme"
	"github.com/radix29/tuikit/tuikittest"
)

// Like controls' frame benchmarks (see frame_bench_test.go there for why),
// this draws into a real tcell screen on a mock terminal, not a fake that
// swallows SetContent.

const frameW, frameH = 200, 50

func frameScreen(b *testing.B) tcell.Screen { return tuikittest.NewMockScreen(b, frameW, frameH) }

// BenchmarkFrameEditor is a full-screen query editor over a highlighted
// script, scrolled into the middle so the gutter and scrollbar draw too.
func BenchmarkFrameEditor(b *testing.B) {
	e := NewEditor(toyHighlighter(&theme.Default))
	e.SetText(benchScript(2000))
	e.SetBounds(0, 0, frameW, frameH)
	e.scrollRow = 1000
	s := frameScreen(b)

	b.ReportAllocs()
	for b.Loop() {
		e.Draw(s)
	}
}
