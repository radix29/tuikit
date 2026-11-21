package editor

import (
	"strings"
	"testing"

	"github.com/radix29/tuikit/tuikittest"
)

// TestReplaceTextIsOneUndoStep pins the property Format Document relies on and
// SetText lacks: the previous text and cursor come back with a single undo, and
// the history from before the replace survives behind it.
func TestReplaceTextIsOneUndoStep(t *testing.T) {
	e := newTestEditor("select 1")
	e.cursorCol = len(e.doc.Line(0))
	e.HandleKey(tuikittest.Rune('0', 0)) // a step from before the replace
	before := e.Text()
	e.cursorRow, e.cursorCol = 0, 3

	if !e.ReplaceText("SELECT\n    10;") {
		t.Fatal("ReplaceText reported no change")
	}
	if got := e.Text(); got != "SELECT\n    10;" {
		t.Fatalf("Text() = %q", got)
	}
	e.undo()
	if got := e.Text(); got != before {
		t.Fatalf("undo left %q, want %q", got, before)
	}
	if e.cursorRow != 0 || e.cursorCol != 3 {
		t.Errorf("undo put the cursor at (%d,%d), want (0,3)", e.cursorRow, e.cursorCol)
	}
	e.undo()
	if got := e.Text(); got != "select 1" {
		t.Errorf("the step before the replace was lost: second undo left %q", got)
	}
}

// TestReplaceTextClampsCursorAndDropsSelection: the cursor keeps its row and
// column where the new text still has them and is clamped where it doesn't, and a
// selection collapses (its anchor could otherwise point past the new end).
func TestReplaceTextClampsCursorAndDropsSelection(t *testing.T) {
	e := newTestEditor("one\ntwo\nthree\nfour")
	e.selecting, e.selAnchorRow, e.selAnchorCol = true, 0, 0
	e.cursorRow, e.cursorCol = 3, 4

	e.ReplaceText("a\nbb")
	if e.HasSelection() {
		t.Error("selection survived ReplaceText")
	}
	if e.cursorRow != 1 || e.cursorCol != 2 {
		t.Errorf("cursor at (%d,%d), want clamped to (1,2)", e.cursorRow, e.cursorCol)
	}
}

// TestReplaceTextNormalisesLikeSetText: CRLF and lone CR fold and tabs expand, so the
// document a replace leaves is the one SetText would have built.
func TestReplaceTextNormalisesLikeSetText(t *testing.T) {
	for _, src := range []string{"a\r\n\tb", "a\r\tb\rc", "a\r\r\nb"} {
		want := newTestEditor(src).Text()
		e := newTestEditor("x")
		e.ReplaceText(src)
		if got := e.Text(); got != want {
			t.Errorf("%q: Text() = %q, want %q (as SetText)", src, got, want)
		}
	}
}

// TestReplaceTextNoOps: unchanged text and a read-only editor push no undo step
// and leave the version alone, so an unchanged format doesn't mark the document
// modified.
func TestReplaceTextNoOps(t *testing.T) {
	e := newTestEditor("same\ntext")
	v := e.doc.Version()
	if e.ReplaceText("same\r\ntext") {
		t.Error("ReplaceText reported a change for equal text")
	}
	if len(e.undoStack) != 0 || e.doc.Version() != v {
		t.Error("equal text pushed an undo step or bumped the version")
	}

	e.SetReadOnly(true)
	if e.ReplaceText("other") || e.Text() != "same\ntext" {
		t.Error("ReplaceText changed a read-only editor")
	}
	if e.ReplaceSelectedLines("x") {
		t.Error("ReplaceSelectedLines changed a read-only editor")
	}
}

// TestSelectedLinesText covers the line span a selection expands to: partial
// first and last rows widen to whole lines, a selection ending at column 0 of a
// row leaves that row out (affectedLineRange), and no selection or a block
// selection has no span.
func TestSelectedLinesText(t *testing.T) {
	e := newTestEditor("r0\nr1 abc\nr2\nr3 xyz\nr4")
	if _, ok := e.SelectedLinesText(); ok {
		t.Error("no selection reported ok")
	}

	e.selecting, e.selAnchorRow, e.selAnchorCol = true, 1, 3
	e.cursorRow, e.cursorCol = 3, 2
	if got, ok := e.SelectedLinesText(); !ok || got != "r1 abc\nr2\nr3 xyz" {
		t.Errorf("partial rows: got %q, %v", got, ok)
	}

	e.cursorRow, e.cursorCol = 4, 0
	if got, _ := e.SelectedLinesText(); got != "r1 abc\nr2\nr3 xyz" {
		t.Errorf("column-0 end: got %q, want row 4 left out", got)
	}

	e.selBlock = true
	if _, ok := e.SelectedLinesText(); ok {
		t.Error("block selection reported ok")
	}
	if e.ReplaceSelectedLines("x") {
		t.Error("ReplaceSelectedLines acted on a block selection")
	}
}

// TestReplaceSelectedLines: only the selected lines change, the replacement's
// lines end up selected, and one undo restores the original.
func TestReplaceSelectedLines(t *testing.T) {
	const doc = "keep0\nold1\nold2\nkeep3"
	e := newTestEditor(doc)
	e.selecting, e.selAnchorRow, e.selAnchorCol = true, 1, 2
	e.cursorRow, e.cursorCol = 2, 1

	if !e.ReplaceSelectedLines("new1\nnew2\nnew3") {
		t.Fatal("ReplaceSelectedLines reported no change")
	}
	if got := e.Text(); got != "keep0\nnew1\nnew2\nnew3\nkeep3" {
		t.Fatalf("Text() = %q", got)
	}
	if got := e.SelectedText(); got != "new1\nnew2\nnew3" {
		t.Errorf("selection after replace = %q, want the replacement's lines", got)
	}
	e.undo()
	if got := e.Text(); got != doc {
		t.Errorf("undo left %q, want %q", got, doc)
	}
}

// TestReplaceWithLineBreakSplitsRows: a line break in Replace splits the row like
// Enter, so the row count and Text() agree, a round trip through SetText keeps
// the count, and one undo restores both.
func TestReplaceWithLineBreakSplitsRows(t *testing.T) {
	for _, repl := range []string{"X\nY", "X\r\nY", "X\rY"} {
		e := newTestEditor("a b\nc a")
		setSearch(t, e, SearchOptions{Query: "a", Replace: repl})
		if got := e.ReplaceAll(); got != 2 {
			t.Fatalf("%q: ReplaceAll = %d, want 2", repl, got)
		}
		if got, want := e.Text(), "X\nY b\nc X\nY"; got != want {
			t.Fatalf("%q: text = %q, want %q", repl, got, want)
		}
		if n := e.doc.Len(); n != 4 {
			t.Fatalf("%q: %d rows, want 4", repl, n)
		}
		for i := range e.doc.Len() {
			if strings.ContainsAny(string(e.doc.Line(i)), "\r\n") {
				t.Fatalf("%q: row %d holds a line break", repl, i)
			}
		}
		// The leftmost replacement is made last, so the cursor ends after its "Y".
		if e.cursorRow != 1 || e.cursorCol != 1 {
			t.Errorf("%q: cursor (%d,%d), want (1,1)", repl, e.cursorRow, e.cursorCol)
		}
		e.undo()
		if got, want := e.Text(), "a b\nc a"; got != want || e.doc.Len() != 2 {
			t.Fatalf("%q: undo left %q (%d rows)", repl, got, e.doc.Len())
		}
	}
}

func TestReplaceCurrentWithLineBreak(t *testing.T) {
	e := newTestEditor("one a two\nlast")
	setSearch(t, e, SearchOptions{Query: "a", Replace: "X\nY"})
	e.FindNext(1)
	if !e.ReplaceCurrent() {
		t.Fatal("ReplaceCurrent replaced nothing")
	}
	if got, want := e.Text(), "one X\nY two\nlast"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	e.undo()
	if got := e.Text(); got != "one a two\nlast" || e.doc.Len() != 2 {
		t.Fatalf("undo left %q", got)
	}
}

// TestReplaceWithLineBreakInSelection: the InSelection scope follows a
// multi-row replacement, so the matches after it are still replaced.
func TestReplaceWithLineBreakInSelection(t *testing.T) {
	e := newTestEditor("a a\na a\nz")
	e.SelectRange(0, 0, 1, 3)
	setSearch(t, e, SearchOptions{Query: "a", Replace: "X\nY", InSelection: true})
	if got := e.ReplaceAll(); got != 4 {
		t.Fatalf("ReplaceAll = %d, want 4", got)
	}
	if got, want := e.Text(), "X\nY X\nY\nX\nY X\nY\nz"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	e.undo()
	if got := e.Text(); got != "a a\na a\nz" {
		t.Fatalf("undo left %q", got)
	}
}
