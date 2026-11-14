package tuikittest

import (
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
)

// NewMockScreen is tcell's own screen on a w×h mock terminal, initialised and
// finalised at the end of the test. Use it over Screen when what is under
// test is tcell's view of the cells — grapheme widths as tcell measures them,
// a control learning its size from the screen, or the per-cell cost of a real
// SetContent in a benchmark — which a Buffer would hide.
func NewMockScreen(tb testing.TB, w, h int) tcell.Screen {
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
