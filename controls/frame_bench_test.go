package controls

import (
	"strconv"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
)

// The frame benchmarks draw into a real tcell screen on a mock terminal, not
// discardScreen: what they measure is the cost per cell of reaching tcell's
// cell buffer, which a fake that swallows SetContent hides. That cost was two
// allocations per cell per frame before core's drawing went through Put —
// one in putGrapheme's []rune, one in tcell's SetContent re-packing the rune
// into a string — on every visible cell, every frame.

const frameW, frameH = 200, 50

func frameScreen(b *testing.B) tcell.Screen { return mockScreen(b, frameW, frameH) }

// mockScreen is tcell's own screen on a w x h mock terminal, for a test that
// must measure as tcell does rather than as a tuikittest.Screen does.
func mockScreen(tb testing.TB, w, h int) tcell.Screen {
	tb.Helper()
	s, err := tcell.NewTerminfoScreenFromTty(vt.NewMockTerm(vt.MockOptSize{X: vt.Col(w), Y: vt.Row(h)}))
	if err != nil {
		tb.Fatal(err)
	}
	if err := s.Init(); err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(s.Fini)
	return s
}

// BenchmarkFrameDataGrid is a full-screen results grid: header, separator,
// 46 rows of six columns, the status bar and both scrollbars.
func BenchmarkFrameDataGrid(b *testing.B) {
	rows := make([][]string, 1000)
	for i := range rows {
		n := strconv.Itoa(i)
		rows[i] = []string{n, "dbo.SomeTable_" + n, "2026-09-10 12:34:56.789", "ONLINE", "NULL", "a longer value that the column clamp truncates " + n}
	}
	g := NewDataGrid()
	g.SetRowNumbers(true)
	g.SetData([]string{"id", "name", "created", "state", "note", "detail"}, rows)
	g.SetBounds(0, 0, frameW, frameH)
	s := frameScreen(b)

	b.ReportAllocs()
	for b.Loop() {
		g.Draw(s)
	}
}
