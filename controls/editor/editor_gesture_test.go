package editor

import (
	"strconv"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/tuikittest"
)

// gestureHeld reports whether a Button1 press is held.
func (e *Editor) gestureHeld() bool {
	_, held := e.gesture.Owner()
	return held
}

// gestureIs reports whether a press is held and z owns it.
func (e *Editor) gestureIs(z editorZone) bool {
	got, held := e.gesture.Owner()
	return held && got == z
}

func emouse(e *Editor, x, y int, b tcell.ButtonMask) bool {
	return e.HandleMouse(tcell.NewEventMouse(x, y, b, tcell.ModNone))
}

// numberedEditor is n lines "line<i>" in a 30x10 editor.
func numberedEditor(n int, wrap bool) *Editor {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = "line" + strconv.Itoa(i)
	}
	e := newTestEditor(strings.Join(lines, "\n"))
	e.SetWrapMode(wrap)
	e.SetBounds(0, 0, 30, 10)
	return e
}

// BG3: a selection dragged below the last visible line scrolls one row per
// event and extends onto the lines that scroll in, in both wrap modes; dragged
// back above the top it scrolls back.
func TestEditorSelectionDragPastTheEdgeScrolls(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		e := numberedEditor(40, wrap)
		x := e.rect.X + e.gutterWidth() + 1
		emouse(e, x, 0, tcell.Button1)
		for range 4 {
			if !emouse(e, x, e.rect.Bottom()+3, tcell.Button1) {
				t.Fatalf("wrap=%v: a held selection drag below the editor was not consumed", wrap)
			}
		}
		if row, _ := e.CursorPos(); e.scrollRow != 4 || row != 13 {
			t.Errorf("wrap=%v: after 4 events below: scrollRow=%d cursor row=%d, want 4 and 13", wrap, e.scrollRow, row)
		}
		if !e.HasSelection() || e.selAnchorRow != 0 {
			t.Errorf("wrap=%v: the selection did not stay anchored at line 0", wrap)
		}
		for range 2 {
			emouse(e, x, e.rect.Y-2, tcell.Button1)
		}
		if row, _ := e.CursorPos(); e.scrollRow != 2 || row != 2 {
			t.Errorf("wrap=%v: after 2 events above: scrollRow=%d cursor row=%d, want 2 and 2", wrap, e.scrollRow, row)
		}
		emouse(e, x, 50, tcell.ButtonNone)
		if e.gestureHeld() {
			t.Errorf("wrap=%v: a release outside did not end the drag", wrap)
		}
	}
}

// BG3: unwrapped, a selection dragged right of the editor scrolls sideways.
func TestEditorSelectionDragPastTheRightEdgeScrolls(t *testing.T) {
	e := wideTestEditor()
	x := e.rect.X + e.gutterWidth() + 1
	emouse(e, x, 1, tcell.Button1)
	for range 3 {
		emouse(e, e.rect.Right()+5, 1, tcell.Button1)
	}
	if e.scrollCol == 0 {
		t.Errorf("scrollCol = 0 after dragging right of the editor, want scrolled")
	}
	before := e.scrollCol
	emouse(e, e.rect.X-3, 1, tcell.Button1)
	if e.scrollCol >= before {
		t.Errorf("scrollCol = %d after dragging left of the editor, want below %d", e.scrollCol, before)
	}
}

// BG3: a thumb dragged off the bottom keeps control and reaches the end.
func TestEditorThumbDragPastTheEdge(t *testing.T) {
	e := numberedEditor(40, false)
	emouse(e, e.rect.Right()-1, 0, tcell.Button1)
	if !emouse(e, e.rect.Right()-6, e.rect.Bottom()+4, tcell.Button1) {
		t.Fatal("a held thumb drag below the editor was not consumed")
	}
	if want := 40 - e.contentH(); e.scrollRow != want {
		t.Errorf("scrollRow = %d, want %d (the end)", e.scrollRow, want)
	}
	if row, _ := e.CursorPos(); row != 0 || e.HasSelection() {
		t.Error("a thumb drag moved the cursor or selected text")
	}
}

// A double-click on a completion item commits it: the resends while the button
// is held belong to that click, so they neither commit again nor fall to the
// text underneath once the popup has closed.
func TestEditorCompletionClickResendsAreSwallowed(t *testing.T) {
	e := newTestEditor("")
	e.SetBounds(0, 0, 40, 20)
	e.SetCompletionProvider(testCompletionProvider("ca", "cb"))
	e.HandleKey(tuikittest.Rune('c', tcell.ModNone))
	if !e.completionOpen {
		t.Fatal("completion popup should be open")
	}
	rect := e.completionRect()
	y := rect.Y + e.completionSel - e.completionScroll
	emouse(e, rect.X+1, y, tcell.Button1)
	emouse(e, rect.X+1, y, tcell.ButtonNone)
	emouse(e, rect.X+1, y, tcell.Button1)
	if e.completionOpen {
		t.Fatal("a double-click on an item should commit it")
	}
	text := e.Text()
	_, col := e.CursorPos()
	emouse(e, rect.X+1, y, tcell.Button1)
	emouse(e, rect.X+2, y+1, tcell.Button1)
	if _, c := e.CursorPos(); e.Text() != text || c != col || e.HasSelection() {
		t.Errorf("resends after the commit moved the cursor to %d or changed the text to %q", c, e.Text())
	}
}

// BG10: a click selects and a double-click commits, on every row alike. The
// first row is preselected, so committing on a click on the selected row made
// one click commit there and only select anywhere else.
func TestEditorCompletionClickSelectsDoubleClickCommits(t *testing.T) {
	for _, row := range []int{0, 1} {
		e := newTestEditor("")
		e.SetBounds(0, 0, 40, 20)
		e.SetCompletionProvider(testCompletionProvider("ca", "cb"))
		e.HandleKey(tuikittest.Rune('c', tcell.ModNone))
		if !e.completionOpen || e.completionSel != 0 {
			t.Fatalf("popup open=%v sel=%d, want open with row 0 selected", e.completionOpen, e.completionSel)
		}
		rect := e.completionRect()
		y := rect.Y + row - e.completionScroll
		emouse(e, rect.X+1, y, tcell.Button1)
		emouse(e, rect.X+1, y, tcell.ButtonNone)
		if !e.completionOpen || e.completionSel != row {
			t.Fatalf("row %d: one click: open=%v sel=%d, want still open with row %d selected", row, e.completionOpen, e.completionSel, row)
		}
		emouse(e, rect.X+1, y, tcell.Button1)
		emouse(e, rect.X+1, y, tcell.ButtonNone)
		if e.completionOpen {
			t.Fatalf("row %d: a double-click did not commit", row)
		}
		if want := []string{"ca", "cb"}[row]; e.Text() != want {
			t.Errorf("row %d: text = %q, want %q", row, e.Text(), want)
		}
	}
}

// Clicks on two different rows never pair into a double-click.
func TestEditorCompletionClicksOnTwoRowsDoNotCommit(t *testing.T) {
	e := newTestEditor("")
	e.SetBounds(0, 0, 40, 20)
	e.SetCompletionProvider(testCompletionProvider("ca", "cb"))
	e.HandleKey(tuikittest.Rune('c', tcell.ModNone))
	rect := e.completionRect()
	for _, y := range []int{rect.Y, rect.Y + 1} {
		emouse(e, rect.X+1, y, tcell.Button1)
		emouse(e, rect.X+1, y, tcell.ButtonNone)
	}
	if !e.completionOpen || e.completionSel != 1 {
		t.Errorf("open=%v sel=%d, want still open with row 1 selected", e.completionOpen, e.completionSel)
	}
}
