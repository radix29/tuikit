package editor

import (
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/tuikittest"
)

func TestEditorArrowCollapsesSelection(t *testing.T) {
	sel := func() *Editor {
		e := newTestEditor("hello")
		for range 5 {
			e.HandleKey(tuikittest.Key(tcell.KeyRight, tcell.ModShift))
		}
		return e
	}
	e := sel()
	e.HandleKey(tuikittest.Key(tcell.KeyLeft, tcell.ModNone))
	if e.cursorCol != 0 || e.HasSelection() {
		t.Fatalf("Left: col %d sel %v", e.cursorCol, e.HasSelection())
	}
	e = sel()
	e.cursorCol = 0 // reversed: anchor 0.. caret moved to 0 after anchor set at 0
	e.selAnchorCol, e.cursorCol = 5, 2
	e.HandleKey(tuikittest.Key(tcell.KeyRight, tcell.ModNone))
	if e.cursorCol != 5 || e.HasSelection() {
		t.Fatalf("Right: col %d", e.cursorCol)
	}
	e = sel()
	e.HandleKey(tuikittest.Key(tcell.KeyLeft, tcell.ModShift))
	if !e.HasSelection() || e.cursorCol != 4 {
		t.Fatalf("Shift+Left: col %d", e.cursorCol)
	}
	// block selection: rows 0-1, columns 1..3
	e = newTestEditor("abcde\nabcde")
	e.selecting, e.selBlock = true, true
	e.selAnchorRow, e.selAnchorCol, e.cursorRow, e.cursorCol = 0, 1, 1, 3
	e.HandleKey(tuikittest.Key(tcell.KeyLeft, tcell.ModNone))
	if e.cursorCol != 1 || e.cursorRow != 1 || e.HasSelection() {
		t.Fatalf("block Left: %d,%d", e.cursorRow, e.cursorCol)
	}
	// empty selection moves one step
	e = newTestEditor("abc")
	e.cursorCol = 2
	e.HandleKey(tuikittest.Key(tcell.KeyLeft, tcell.ModNone))
	if e.cursorCol != 1 {
		t.Fatalf("plain Left: %d", e.cursorCol)
	}
}
