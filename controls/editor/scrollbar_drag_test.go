package editor

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/tuikittest"
)

// TestEditorScrollbarDragScrolls mirrors controls' DataGrid scrollbar-drag test for Editor in
// plain (non-wrap) mode.
func TestEditorScrollbarDragScrolls(t *testing.T) {
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "line"
	}
	e := newTestEditor(strings.Join(lines, "\n"))
	e.SetBounds(0, 0, 30, 10)
	sbX := e.rect.Right() - 1

	if !e.HandleMouse(tcell.NewEventMouse(sbX, e.rect.Y+e.rect.H-1, tcell.Button1, tcell.ModNone)) {
		t.Fatal("HandleMouse on scrollbar column should be handled")
	}
	if !e.gestureIs(editorVBar) {
		t.Fatal("pressing on the scrollbar should start a thumb drag")
	}
	if e.scrollRow == 0 {
		t.Error("scrollRow should have jumped forward when clicking near the bottom of the track")
	}
	if e.cursorRow != 0 {
		t.Errorf("cursorRow = %d, clicking the scrollbar must not move the text cursor", e.cursorRow)
	}

	e.HandleMouse(tcell.NewEventMouse(sbX, e.rect.Y, tcell.ButtonNone, tcell.ModNone))
	if e.gestureHeld() {
		t.Error("the thumb drag should end on release")
	}
}

// TestEditorCompletionPopupScrollbarDragScrolls confirms dragging the
// IntelliSense popup's own scrollbar scrolls its candidate list instead of
// being read as a click that commits whatever candidate sits under it.
func TestEditorCompletionPopupScrollbarDragScrolls(t *testing.T) {
	e := newTestEditor("")
	e.SetBounds(0, 0, 40, 20)
	candidates := make([]string, 15)
	for i := range candidates {
		candidates[i] = "c" + string(rune('a'+i))
	}
	e.SetCompletionProvider(testCompletionProvider(candidates...))
	e.HandleKey(tuikittest.Rune('c', tcell.ModNone))
	if !e.completionOpen {
		t.Fatal("completion popup should be open after typing a matching prefix")
	}
	if len(e.completionItems) <= maxCompletionRows {
		t.Fatalf("test needs more candidates than maxCompletionRows (%d) to exercise scrolling", maxCompletionRows)
	}

	rect := e.completionRect()
	sbX := rect.Right() - 1
	selBefore := e.completionSel

	if !e.HandleMouse(tcell.NewEventMouse(sbX, rect.Y+rect.H-1, tcell.Button1, tcell.ModNone)) {
		t.Fatal("HandleMouse on the popup's scrollbar column should be handled")
	}
	if !e.gestureIs(editorPopupBar) {
		t.Fatal("pressing on the popup's scrollbar should start a thumb drag")
	}
	if e.completionScroll == 0 {
		t.Error("completionScroll should have jumped forward when clicking near the bottom of the track")
	}
	if e.completionSel != selBefore {
		t.Errorf("completionSel changed to %d, clicking the scrollbar must not commit/select a candidate", e.completionSel)
	}
	if !e.completionOpen {
		t.Error("dragging the scrollbar must not close the popup")
	}

	e.HandleMouse(tcell.NewEventMouse(sbX, rect.Y, tcell.ButtonNone, tcell.ModNone))
	if e.gestureHeld() {
		t.Error("the popup's thumb drag should end on release")
	}
}

// TestEditorScrollbarDragScrollsWrapMode confirms the same works in
// word-wrap mode, where the scrollbar's total is visual rows, not logical
// lines.
func TestEditorScrollbarDragScrollsWrapMode(t *testing.T) {
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "line"
	}
	e := newTestEditor(strings.Join(lines, "\n"))
	e.SetWrapMode(true)
	e.SetBounds(0, 0, 30, 10)
	sbX := e.rect.Right() - 1

	if !e.HandleMouse(tcell.NewEventMouse(sbX, e.rect.Y+e.rect.H-1, tcell.Button1, tcell.ModNone)) {
		t.Fatal("HandleMouse on scrollbar column should be handled in wrap mode")
	}
	if !e.gestureIs(editorVBar) {
		t.Fatal("pressing on the scrollbar should start a thumb drag")
	}
	if e.scrollRow == 0 {
		t.Error("scrollRow should have jumped forward when clicking near the bottom of the track")
	}
	if e.cursorRow != 0 {
		t.Errorf("cursorRow = %d, clicking the scrollbar must not move the text cursor", e.cursorRow)
	}
}
