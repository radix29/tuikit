package editor

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"
	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/tuikittest"
)

func newTestEditor(text string) *Editor {
	e := NewEditor(nil)
	e.SetText(text)
	return e
}

func TestEditorWordNavigation(t *testing.T) {
	e := newTestEditor("foo.bar baz")

	want := []int{3, 4, 7, 11}
	for _, w := range want {
		e.HandleKey(tuikittest.Key(tcell.KeyRight, tcell.ModCtrl))
		if e.cursorCol != w {
			t.Fatalf("Ctrl+Right: cursorCol = %d, want %d", e.cursorCol, w)
		}
	}
	wantBack := []int{8, 4, 3, 0}
	for _, w := range wantBack {
		e.HandleKey(tuikittest.Key(tcell.KeyLeft, tcell.ModCtrl))
		if e.cursorCol != w {
			t.Fatalf("Ctrl+Left: cursorCol = %d, want %d", e.cursorCol, w)
		}
	}
}

func TestEditorCtrlHomeEnd(t *testing.T) {
	e := newTestEditor("one\ntwo\nthree")
	e.cursorRow, e.cursorCol = 1, 2

	e.HandleKey(tuikittest.Key(tcell.KeyEnd, tcell.ModCtrl))
	if e.cursorRow != 2 || e.cursorCol != len("three") {
		t.Fatalf("Ctrl+End: got (%d,%d), want (2,%d)", e.cursorRow, e.cursorCol, len("three"))
	}

	e.HandleKey(tuikittest.Key(tcell.KeyHome, tcell.ModCtrl))
	if e.cursorRow != 0 || e.cursorCol != 0 {
		t.Fatalf("Ctrl+Home: got (%d,%d), want (0,0)", e.cursorRow, e.cursorCol)
	}
}

func TestEditorSelectAll(t *testing.T) {
	e := newTestEditor("abc\nde")
	e.HandleKey(tuikittest.Key(tcell.KeyCtrlA, tcell.ModNone))
	if !e.HasSelection() {
		t.Fatal("Ctrl+A: expected a selection")
	}
	if got := e.SelectedText(); got != "abc\nde" {
		t.Fatalf("Ctrl+A: SelectedText() = %q, want %q", got, "abc\nde")
	}
}

func TestEditorDuplicateLines(t *testing.T) {
	e := newTestEditor("one\ntwo\nthree")
	e.cursorRow = 1
	e.DuplicateLines()
	if got := e.Text(); got != "one\ntwo\ntwo\nthree" {
		t.Fatalf("DuplicateLines() = %q", got)
	}
	if e.cursorRow != 2 {
		t.Fatalf("cursorRow = %d, want 2", e.cursorRow)
	}
}

func TestEditorDeleteLines(t *testing.T) {
	e := newTestEditor("one\ntwo\nthree")
	e.cursorRow = 1
	e.DeleteLines()
	if got := e.Text(); got != "one\nthree" {
		t.Fatalf("DeleteLines() = %q", got)
	}

	// Deleting the only line preserves the [][]rune{{}} invariant.
	e2 := newTestEditor("only")
	e2.DeleteLines()
	if got := e2.Text(); got != "" {
		t.Fatalf("DeleteLines() on sole line = %q, want empty", got)
	}
	if e2.doc.Len() != 1 {
		t.Fatalf("expected the empty-buffer invariant to hold, got %d lines", e2.doc.Len())
	}
}

// TestEditorDuplicateDeleteLinesCollapseSelectionWhenCalledDirectly pins
// down that DuplicateLines/DeleteLines collapse an active selection
// themselves, matching their doc comments, rather than relying on
// HandleKey's post-switch dropSelection cleanup: invoked directly (the Edit
// menu's path, see menu.go) they would otherwise leave a stale anchor row,
// which for DeleteLines can point past the new buffer's end and panic on
// the next SelectedText call.
func TestEditorDuplicateDeleteLinesCollapseSelectionWhenCalledDirectly(t *testing.T) {
	e := newTestEditor("one\ntwo\nthree")
	e.selecting, e.selBlock = true, false
	e.selAnchorRow, e.selAnchorCol = 0, 0
	e.cursorRow, e.cursorCol = 2, 3 // selects all three lines

	e.DuplicateLines()
	if e.HasSelection() || e.selBlock {
		t.Fatal("DuplicateLines() called directly should collapse the selection")
	}

	e.selecting, e.selBlock = true, false
	e.selAnchorRow, e.selAnchorCol = 0, 0
	e.cursorRow, e.cursorCol = e.doc.Len()-1, 0 // select down to the last line

	e.DeleteLines()
	if e.HasSelection() || e.selBlock {
		t.Fatal("DeleteLines() called directly should collapse the selection")
	}
	if got := e.SelectedText(); got != "" {
		t.Fatalf("SelectedText() after DeleteLines() = %q, want empty (and must not panic)", got)
	}
}

func TestEditorMoveLines(t *testing.T) {
	e := newTestEditor("one\ntwo\nthree")
	e.cursorRow = 1
	e.MoveLinesUp()
	if got := e.Text(); got != "two\none\nthree" {
		t.Fatalf("MoveLinesUp() = %q", got)
	}
	if e.cursorRow != 0 {
		t.Fatalf("cursorRow = %d, want 0", e.cursorRow)
	}

	// No-op at the top of the buffer.
	e.MoveLinesUp()
	if got := e.Text(); got != "two\none\nthree" {
		t.Fatalf("MoveLinesUp() at top mutated buffer: %q", got)
	}

	e2 := newTestEditor("one\ntwo\nthree")
	e2.cursorRow = 1
	e2.MoveLinesDown()
	if got := e2.Text(); got != "one\nthree\ntwo" {
		t.Fatalf("MoveLinesDown() = %q", got)
	}
	if e2.cursorRow != 2 {
		t.Fatalf("cursorRow = %d, want 2", e2.cursorRow)
	}

	// No-op at the bottom of the buffer.
	e2.MoveLinesDown()
	if got := e2.Text(); got != "one\nthree\ntwo" {
		t.Fatalf("MoveLinesDown() at bottom mutated buffer: %q", got)
	}
}

func TestEditorTabSingleLineUnchanged(t *testing.T) {
	e := newTestEditor("abc")
	e.HandleKey(tuikittest.Key(tcell.KeyTab, tcell.ModNone))
	if got := e.Text(); got != "    abc" {
		t.Fatalf("Tab with no selection = %q, want %q", got, "    abc")
	}
}

func TestEditorIndentDedentMultiLineSelection(t *testing.T) {
	e := newTestEditor("abc\ndef")
	e.selecting, e.selBlock = true, false
	e.selAnchorRow, e.selAnchorCol = 0, 0
	e.cursorRow, e.cursorCol = 1, 3

	e.HandleKey(tuikittest.Key(tcell.KeyTab, tcell.ModNone))
	if got := e.Text(); got != "    abc\n    def" {
		t.Fatalf("Tab (indent) = %q", got)
	}
	if !e.HasSelection() {
		t.Fatal("Tab (indent): expected selection to be preserved")
	}
	if e.selAnchorCol != 4 || e.cursorCol != 7 {
		t.Fatalf("Tab (indent): selAnchorCol=%d cursorCol=%d, want 4,7", e.selAnchorCol, e.cursorCol)
	}

	e.HandleKey(tuikittest.Key(tcell.KeyTab, tcell.ModShift))
	if got := e.Text(); got != "abc\ndef" {
		t.Fatalf("Shift+Tab (dedent) = %q", got)
	}
	if e.selAnchorCol != 0 || e.cursorCol != 3 {
		t.Fatalf("Shift+Tab (dedent): selAnchorCol=%d cursorCol=%d, want 0,3", e.selAnchorCol, e.cursorCol)
	}
}

func TestEditorToggleLineComments(t *testing.T) {
	e := newTestEditor("SELECT 1\nSELECT 2")
	e.selecting, e.selBlock = true, false
	e.selAnchorRow, e.selAnchorCol = 0, 0
	e.cursorRow, e.cursorCol = 1, 8

	e.SetLineComment("--")
	e.ToggleLineComments()
	if got := e.Text(); got != "-- SELECT 1\n-- SELECT 2" {
		t.Fatalf("comment = %q", got)
	}

	e.SetLineComment("--")
	e.ToggleLineComments()
	if got := e.Text(); got != "SELECT 1\nSELECT 2" {
		t.Fatalf("uncomment = %q", got)
	}

	// Mixed state (one line already commented) comments every line,
	// including the already-commented one.
	e.SetText("-- SELECT 1\nSELECT 2")
	e.selecting, e.selBlock = true, false
	e.selAnchorRow, e.selAnchorCol = 0, 0
	e.cursorRow, e.cursorCol = 1, 8
	e.SetLineComment("--")
	e.ToggleLineComments()
	if got := e.Text(); got != "-- -- SELECT 1\n-- SELECT 2" {
		t.Fatalf("mixed-state comment = %q", got)
	}
}

// Ctrl+Shift+U reaches the editor as KeyCtrlU+ModShift only because the
// application layer folds it back there (tui.normalizeCtrlRune) — terminals
// deliver it as a Ctrl+Shift-modified rune, or, on legacy encodings, as plain
// Ctrl+U.
func TestEditorCaseConversion(t *testing.T) {
	e := newTestEditor("hello world")
	e.selecting, e.selBlock = true, false
	e.selAnchorRow, e.selAnchorCol = 0, 0
	e.cursorRow, e.cursorCol = 0, 5

	e.HandleKey(tuikittest.Key(tcell.KeyCtrlU, tcell.ModShift))
	if got := e.Text(); got != "HELLO world" {
		t.Fatalf("uppercase = %q", got)
	}
	if !e.HasSelection() {
		t.Fatal("case conversion: expected selection to be preserved")
	}

	e.HandleKey(tuikittest.Key(tcell.KeyCtrlU, tcell.ModNone))
	if got := e.Text(); got != "hello world" {
		t.Fatalf("lowercase = %q", got)
	}
}

// TestEditorSelectionDeletedByBackspace pins down that selecting text then
// pressing Backspace/Delete removes it — dropping the selection before
// deleteSelection() can see it silently leaves the text in place.
func TestEditorSelectionDeletedByBackspace(t *testing.T) {
	e := newTestEditor("abcdef")
	for range 3 {
		e.HandleKey(tuikittest.Key(tcell.KeyRight, tcell.ModShift))
	}
	if !e.HasSelection() || e.SelectedText() != "abc" {
		t.Fatalf("setup: SelectedText() = %q, want %q", e.SelectedText(), "abc")
	}
	e.HandleKey(tuikittest.Key(tcell.KeyBackspace, tcell.ModNone))
	if got := e.Text(); got != "def" {
		t.Fatalf("Backspace over selection = %q, want %q", got, "def")
	}

	e2 := newTestEditor("abcdef")
	for range 3 {
		e2.HandleKey(tuikittest.Key(tcell.KeyRight, tcell.ModShift))
	}
	e2.HandleKey(tuikittest.Key(tcell.KeyDelete, tcell.ModNone))
	if got := e2.Text(); got != "def" {
		t.Fatalf("Delete over selection = %q, want %q", got, "def")
	}
}

// TestEditorPasteTextAndCaret pins insertText's joins and where it leaves the
// caret: after the pasted text, on the last pasted line.
func TestEditorPasteTextAndCaret(t *testing.T) {
	for _, tc := range []struct {
		name, doc    string
		row, col     int
		paste, want  string
		wantR, wantC int
	}{
		{"one line mid-line", "abcd", 0, 2, "XY", "abXYcd", 0, 4},
		{"one line at the end", "ab", 0, 2, "XY", "abXY", 0, 4},
		{"multi-line mid-line", "abcd\nz", 0, 1, "1\n22\n333", "a1\n22\n333bcd\nz", 2, 3},
		{"trailing newline", "abcd", 0, 2, "X\n", "abX\ncd", 1, 0},
		{"leading newline", "abcd", 0, 2, "\nX", "ab\nXcd", 1, 1},
		{"CRLF", "ab", 0, 1, "1\r\n2", "a1\n2b", 1, 1},
		{"last row of many", "a\nb\nc", 2, 1, "x\ny", "a\nb\ncx\ny", 3, 1},
		{"into an empty document", "", 0, 0, "p\nq", "p\nq", 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEditor(tc.doc)
			e.cursorRow, e.cursorCol = tc.row, tc.col
			e.Paste(tc.paste)
			if got := e.Text(); got != tc.want {
				t.Fatalf("Text() = %q, want %q", got, tc.want)
			}
			if e.cursorRow != tc.wantR || e.cursorCol != tc.wantC {
				t.Fatalf("caret = (%d,%d), want (%d,%d)", e.cursorRow, e.cursorCol, tc.wantR, tc.wantC)
			}
			e.undo()
			if got := e.Text(); got != tc.doc {
				t.Fatalf("after undo Text() = %q, want %q", got, tc.doc)
			}
		})
	}
}

func TestEditorCutCopyPasteRoundTrip(t *testing.T) {
	e := newTestEditor("hello world")
	for range 5 {
		e.HandleKey(tuikittest.Key(tcell.KeyRight, tcell.ModShift))
	}
	if got := e.SelectedText(); got != "hello" {
		t.Fatalf("SelectedText() = %q, want %q", got, "hello")
	}
	cut := e.Cut()
	if cut != "hello" {
		t.Fatalf("Cut() = %q, want %q", cut, "hello")
	}
	if got := e.Text(); got != " world" {
		t.Fatalf("after Cut(): Text() = %q, want %q", got, " world")
	}
	e.Paste("hello")
	if got := e.Text(); got != "hello world" {
		t.Fatalf("after Paste(): Text() = %q, want %q", got, "hello world")
	}
}

func TestEditorBlockSelection(t *testing.T) {
	e := newTestEditor("abcdef\nxy\nghijkl")
	e.selecting, e.selBlock = true, true
	e.selAnchorRow, e.selAnchorCol = 0, 2
	e.cursorRow, e.cursorCol = 2, 4

	if got := e.SelectedText(); got != "cd\n\nij" {
		t.Fatalf("block SelectedText() = %q, want %q", got, "cd\n\nij")
	}

	cut := e.Cut()
	if cut != "cd\n\nij" {
		t.Fatalf("block Cut() = %q, want %q", cut, "cd\n\nij")
	}
	if got := e.Text(); got != "abef\nxy\nghkl" {
		t.Fatalf("after block Cut(): Text() = %q, want %q", got, "abef\nxy\nghkl")
	}
	if e.HasSelection() || e.selBlock {
		t.Fatal("block Cut() should collapse the selection")
	}
	if e.cursorRow != 0 || e.cursorCol != 2 {
		t.Fatalf("cursor after block Cut() = (%d,%d), want (0,2)", e.cursorRow, e.cursorCol)
	}
}

func TestEditorBlockSelectionDisabledInWrapMode(t *testing.T) {
	e := newTestEditor("hello")
	e.SetWrapMode(true)
	e.HandleKey(tuikittest.Key(tcell.KeyRight, tcell.ModAlt|tcell.ModShift))
	if e.HasSelection() || e.selBlock {
		t.Fatal("Alt+Shift+Right should not enter block selection in wrap mode")
	}
}

func TestEditorEscClearsSelection(t *testing.T) {
	e := newTestEditor("abcdef")
	e.HandleKey(tuikittest.Key(tcell.KeyRight, tcell.ModShift))
	if !e.HasSelection() {
		t.Fatal("setup: expected a selection")
	}
	e.HandleKey(tuikittest.Key(tcell.KeyEsc, tcell.ModNone))
	if e.HasSelection() {
		t.Fatal("Esc should clear the selection")
	}
}

func TestEditorCommentToggleShortcut(t *testing.T) {
	// The real legacy path: Ctrl+/ emits the 0x1F byte, which tcell v3
	// decodes as KeyRune '_' with ModCtrl (Ctrl+_), not KeyUS.
	e := newTestEditor("SELECT 1")
	e.SetLineComment("--")
	e.HandleKey(tuikittest.Rune('_', tcell.ModCtrl))
	if got := e.Text(); got != "-- SELECT 1" {
		t.Fatalf("Ctrl+/ (legacy 0x1F -> Ctrl+_) = %q", got)
	}

	// Modern keyboard protocol reports Ctrl+/ as rune '/' with ModCtrl.
	e2 := newTestEditor("SELECT 1")
	e2.SetLineComment("--")
	e2.HandleKey(tuikittest.Rune('/', tcell.ModCtrl))
	if got := e2.Text(); got != "-- SELECT 1" {
		t.Fatalf("Ctrl+/ (advanced protocol) = %q", got)
	}

	// Defensive fallback: a terminal that ever surfaced KeyUS directly.
	e3 := newTestEditor("SELECT 1")
	e3.SetLineComment("--")
	e3.HandleKey(tuikittest.Key(tcell.KeyUS, tcell.ModNone))
	if got := e3.Text(); got != "-- SELECT 1" {
		t.Fatalf("Ctrl+/ (KeyUS fallback) = %q", got)
	}
}

func TestEditorRightClickFiresCallback(t *testing.T) {
	e := newTestEditor("hello world")
	e.SetBounds(0, 0, 40, 5)

	// Arm a selection; right-click must leave it untouched so the context
	// menu's Copy/Cut still act on it.
	e.selecting = true
	e.selAnchorRow, e.selAnchorCol = 0, 0
	e.cursorRow, e.cursorCol = 0, 5

	var gotX, gotY int
	fired := false
	e.OnRightClick = func(x, y int) { fired, gotX, gotY = true, x, y }

	handled := e.HandleMouse(tcell.NewEventMouse(8, 0, tcell.Button2, tcell.ModNone))
	if !handled {
		t.Fatal("Button2 in the content area should be consumed")
	}
	if !fired {
		t.Fatal("OnRightClick was not called")
	}
	if gotX != 8 || gotY != 0 {
		t.Fatalf("OnRightClick position = (%d,%d), want (8,0)", gotX, gotY)
	}
	if !e.HasSelection() || e.SelectedText() != "hello" {
		t.Fatalf("right-click disturbed the selection: %q", e.SelectedText())
	}
}

// TestEditorCtrlSpaceFiresRightClickAtCursor confirms Ctrl+Space is a
// keyboard equivalent for OnRightClick, positioned at the text cursor
// instead of a mouse click, and — since it's a menu trigger, not a typed
// character — doesn't insert anything or disturb the buffer.
func TestEditorCtrlSpaceFiresRightClickAtCursor(t *testing.T) {
	e := newTestEditor("hello world")
	e.SetBounds(0, 0, 40, 5)
	e.cursorRow, e.cursorCol = 0, 5

	var gotX, gotY int
	fired := false
	e.OnRightClick = func(x, y int) { fired, gotX, gotY = true, x, y }

	if !e.HandleKey(tcell.NewEventKey(tcell.KeyRune, " ", tcell.ModCtrl)) {
		t.Fatal("Ctrl+Space should be consumed")
	}
	if !fired {
		t.Fatal("OnRightClick was not called")
	}
	// gutterWidth() is 5 by default, so column 5 lands at screen x=10.
	if gotX != 10 || gotY != 0 {
		t.Fatalf("OnRightClick position = (%d,%d), want (10,0)", gotX, gotY)
	}
	if got := e.Text(); got != "hello world" {
		t.Fatalf("Ctrl+Space mutated the buffer: %q", got)
	}
}

// TestEditorSetTextResetsSelectionAndUndo pins down that SetText clears any
// active selection and the undo/redo stacks, which refer to the document
// being replaced: a stale selection anchor past the new (shorter) buffer's
// end makes SelectedText panic, and Undo could otherwise restore text from
// the document that existed before SetText — both reachable via
// connect_dialog.go's fExtraProps.SetText when applying a saved
// connection.
func TestEditorSetTextResetsSelectionAndUndo(t *testing.T) {
	e := newTestEditor("a long original line")
	e.HandleKey(tuikittest.Key(tcell.KeyRight, tcell.ModShift))
	e.HandleKey(tuikittest.Key(tcell.KeyRight, tcell.ModShift))
	if !e.HasSelection() {
		t.Fatal("setup: expected a selection")
	}
	e.pushUndoSpan(0, e.doc.Len()) // simulate prior edit history on the old document

	e.SetText("hi") // shorter than the old selection anchor's column

	if e.HasSelection() {
		t.Fatal("SetText should have cleared the active selection")
	}
	if got := e.SelectedText(); got != "" {
		t.Fatalf("SelectedText() after SetText = %q, want empty (and must not panic)", got)
	}
	if len(e.undoStack) != 0 || len(e.redoStack) != 0 {
		t.Fatalf("SetText should have cleared undo/redo history, got undo=%d redo=%d", len(e.undoStack), len(e.redoStack))
	}
	e.undo() // no-op: must not restore the pre-SetText document
	if got := e.Text(); got != "hi" {
		t.Fatalf("undo() after SetText = %q, want %q (undo history must not cross SetText)", got, "hi")
	}
}

func TestEditorWordDelete(t *testing.T) {
	e := newTestEditor("foo bar baz")
	e.cursorCol = len("foo bar baz")
	e.HandleKey(tuikittest.Key(tcell.KeyBackspace, tcell.ModCtrl))
	if got := e.Text(); got != "foo bar " {
		t.Fatalf("Ctrl+Backspace = %q, want %q", got, "foo bar ")
	}

	e2 := newTestEditor("foo bar baz")
	e2.HandleKey(tuikittest.Key(tcell.KeyDelete, tcell.ModCtrl))
	if got := e2.Text(); got != " bar baz" {
		t.Fatalf("Ctrl+Delete = %q, want %q", got, " bar baz")
	}
}

// TestEditorUndoStackCapped covers the bounded undo stack: pushUndo drops
// the oldest snapshot once maxUndoSteps is exceeded rather than growing
// without limit. Typing maxUndoSteps+5 characters pushes maxUndoSteps+5
// snapshots, but only the newest maxUndoSteps survive; undoing all of them
// rewinds to 5 characters short of empty.
func TestEditorUndoStackCapped(t *testing.T) {
	e := newTestEditor("")
	for range maxUndoSteps + 5 {
		e.HandleKey(tuikittest.Rune('x', tcell.ModNone))
	}
	if len(e.undoStack) != maxUndoSteps {
		t.Fatalf("undoStack len = %d, want %d (capped)", len(e.undoStack), maxUndoSteps)
	}
	for range maxUndoSteps {
		e.undo()
	}
	if len(e.undoStack) != 0 {
		t.Fatalf("undoStack should be empty after undoing every retained step, got %d", len(e.undoStack))
	}
	if got := e.Text(); got != "xxxxx" {
		t.Fatalf("Text() after exhausting undo = %q, want %q (the 5 oldest snapshots were evicted, so undo can't reach empty)", got, "xxxxx")
	}
}

// TestEditorUndoStackCappedByBytes covers the second undo cap: a document big
// enough that maxUndoBytes binds before maxUndoSteps does. ReplaceText is the
// operation used because it rewrites the whole document, so its step copies the
// whole buffer — a keystroke's or a line action's step covers a few lines and
// would never reach either cap (see TestEditorKeystrokeStepIsProportionalToTheEdit
// and TestEditorLineActionsKeepUndoHistoryOnALargeScript). Ten rewrites of a
// ~13 MB document may retain at most four steps, not ten.
func TestEditorUndoStackCappedByBytes(t *testing.T) {
	line := strings.Repeat("x", 200)
	lines := make([]string, 16000)
	for i := range lines {
		lines[i] = line
	}
	e := newTestEditor(strings.Join(lines, "\n"))
	for range 10 {
		e.ReplaceText("y" + e.Text())
	}
	if e.undoBytes > maxUndoBytes {
		t.Fatalf("undoBytes = %d, want <= %d", e.undoBytes, maxUndoBytes)
	}
	if len(e.undoStack) == 0 || len(e.undoStack) >= 10 {
		t.Fatalf("undoStack len = %d, want a byte-capped count between 1 and 9", len(e.undoStack))
	}
	// The retained steps are the newest ones, so undo still walks back one
	// character at a time from the newest edit.
	want := len(e.undoStack)
	for range want {
		e.undo()
	}
	if got := e.Text(); !strings.HasPrefix(got, strings.Repeat("y", 10-want)+line) {
		t.Fatalf("first line after undoing every retained step = %q…, want %d leading y", got[:20], 10-want)
	}
	if e.undoBytes != 0 {
		t.Fatalf("undoBytes = %d after emptying the stack, want 0", e.undoBytes)
	}
}

// TestEditorUndoKeepsNewestStepOverBudget pins that a single step larger than
// maxUndoBytes is kept anyway — one undo has to work on any document.
func TestEditorUndoKeepsNewestStepOverBudget(t *testing.T) {
	e := newTestEditor(strings.Repeat(strings.Repeat("x", 1000)+"\n", 20000))
	e.ReplaceText("y" + e.Text())
	if len(e.undoStack) != 1 {
		t.Fatalf("undoStack len = %d, want 1 (newest step kept regardless of size)", len(e.undoStack))
	}
	if e.undoBytes <= maxUndoBytes {
		t.Fatalf("undoBytes = %d, want a single step over the %d budget", e.undoBytes, maxUndoBytes)
	}
	e.undo()
	if strings.HasPrefix(e.Text(), "y") {
		t.Fatal("undo did not restore the pre-edit document")
	}
}

// An undo step costs what the edit costs, not what the document costs. This
// is the whole point of steps being deltas: pushUndo runs on every keystroke,
// and copying a 20,000-line buffer to record a one-character insert measured
// 3.2 ms and 5 MB per character.
//
// The bound is stated in lines rather than seconds so it can't go green on a
// faster machine. A keystroke may cover the cursor's row and one either side
// (see editSpan); ten rows leaves room to widen that without a false alarm,
// and still fails by three orders of magnitude if a step ever goes back to
// covering the buffer.
func TestEditorKeystrokeStepIsProportionalToTheEdit(t *testing.T) {
	e := newTestEditor(strings.Repeat("SELECT col1, col2 FROM dbo.T WHERE x = 1\n", 20000))
	e.cursorRow = 10000

	for _, k := range []tcell.EventKey{
		*tuikittest.Rune('y', tcell.ModNone),
		*tuikittest.Key(tcell.KeyEnter, tcell.ModNone),
		*tuikittest.Key(tcell.KeyBackspace, tcell.ModNone),
		*tuikittest.Key(tcell.KeyDelete, tcell.ModNone),
		*tuikittest.Key(tcell.KeyTab, tcell.ModNone),
	} {
		before := len(e.undoStack)
		e.HandleKey(&k)
		if len(e.undoStack) != before+1 {
			t.Fatalf("key %v pushed %d steps, want 1", k.Key(), len(e.undoStack)-before)
		}
		if got := len(e.undoStack[before].old); got > 10 {
			t.Errorf("key %v recorded %d lines, want a step proportional to the edit", k.Key(), got)
		}
	}
}

// Every editing path pushes its own undo step, and a step that claims a
// narrower span than its edit actually touched does not fail loudly — it
// restores a document that was never typed. So each one is round-tripped
// here: apply, undo, and require both the text and the cursor back exactly.
// Redo then has to reproduce the edit, since undo builds redo's step from the
// span it just restored.
func TestEditorUndoRestoresEveryEditPath(t *testing.T) {
	const doc = "SELECT one\n    SELECT two\n-- SELECT three\nSELECT four\nSELECT five"

	// selectRows puts a linear selection over rows [sr, er] to make the
	// multi-row and replace-selection branches reachable.
	selectRows := func(e *Editor, sr, er int) {
		e.selecting, e.selBlock = true, false
		e.selAnchorRow, e.selAnchorCol = sr, 0
		e.cursorRow, e.cursorCol = er, len(e.doc.Line(er))
	}

	for _, tc := range []struct {
		name string
		do   func(*Editor)
	}{
		{"type a rune", func(e *Editor) { e.HandleKey(tuikittest.Rune('Z', tcell.ModNone)) }},
		{"type over a selection", func(e *Editor) {
			selectRows(e, 1, 3)
			e.HandleKey(tuikittest.Rune('Z', tcell.ModNone))
		}},
		{"enter", func(e *Editor) { e.cursorCol = 3; e.HandleKey(tuikittest.Key(tcell.KeyEnter, tcell.ModNone)) }},
		{"enter over a selection", func(e *Editor) {
			selectRows(e, 0, 2)
			e.HandleKey(tuikittest.Key(tcell.KeyEnter, tcell.ModNone))
		}},
		{"backspace mid-line", func(e *Editor) {
			e.cursorRow, e.cursorCol = 2, 5
			e.HandleKey(tuikittest.Key(tcell.KeyBackspace, tcell.ModNone))
		}},
		{"backspace joining the line above", func(e *Editor) {
			e.cursorRow, e.cursorCol = 2, 0
			e.HandleKey(tuikittest.Key(tcell.KeyBackspace, tcell.ModNone))
		}},
		{"backspace over a selection", func(e *Editor) {
			selectRows(e, 1, 4)
			e.HandleKey(tuikittest.Key(tcell.KeyBackspace, tcell.ModNone))
		}},
		{"ctrl+backspace", func(e *Editor) {
			e.cursorRow, e.cursorCol = 1, len(e.doc.Line(1))
			e.HandleKey(tuikittest.Key(tcell.KeyBackspace, tcell.ModCtrl))
		}},
		{"delete mid-line", func(e *Editor) {
			e.cursorRow, e.cursorCol = 2, 4
			e.HandleKey(tuikittest.Key(tcell.KeyDelete, tcell.ModNone))
		}},
		{"delete joining the line below", func(e *Editor) {
			e.cursorRow, e.cursorCol = 1, len(e.doc.Line(1))
			e.HandleKey(tuikittest.Key(tcell.KeyDelete, tcell.ModNone))
		}},
		{"ctrl+delete", func(e *Editor) {
			e.cursorRow, e.cursorCol = 1, 4
			e.HandleKey(tuikittest.Key(tcell.KeyDelete, tcell.ModCtrl))
		}},
		{"tab", func(e *Editor) { e.cursorCol = 3; e.HandleKey(tuikittest.Key(tcell.KeyTab, tcell.ModNone)) }},
		{"tab over a one-row selection", func(e *Editor) {
			selectRows(e, 2, 2)
			e.HandleKey(tuikittest.Key(tcell.KeyTab, tcell.ModNone))
		}},
		{"tab over a multi-row selection indents", func(e *Editor) {
			selectRows(e, 1, 3)
			e.HandleKey(tuikittest.Key(tcell.KeyTab, tcell.ModNone))
		}},
		{"shift+tab dedents", func(e *Editor) { e.cursorRow = 1; e.HandleKey(tuikittest.Key(tcell.KeyBacktab, tcell.ModNone)) }},
		{"paste multi-line", func(e *Editor) { e.cursorRow, e.cursorCol = 1, 2; e.Paste("a\nb\nc") }},
		{"paste over a selection", func(e *Editor) { selectRows(e, 0, 3); e.Paste("x\ny") }},
		{"cut", func(e *Editor) { selectRows(e, 1, 2); e.Cut() }},
		{"duplicate lines", func(e *Editor) { e.cursorRow = 2; e.DuplicateLines() }},
		{"delete lines", func(e *Editor) { selectRows(e, 1, 2); e.DeleteLines() }},
		{"move lines up", func(e *Editor) { e.cursorRow = 3; e.MoveLinesUp() }},
		{"move lines down", func(e *Editor) { e.cursorRow = 1; e.MoveLinesDown() }},
		{"toggle comments", func(e *Editor) { selectRows(e, 0, 4); e.SetLineComment("--"); e.ToggleLineComments() }},
		{"uppercase selection", func(e *Editor) { selectRows(e, 1, 3); e.UppercaseSelection() }},
		{"lowercase a block selection", func(e *Editor) {
			e.selecting, e.selBlock = true, true
			e.selAnchorRow, e.selAnchorCol = 3, 6
			e.cursorRow, e.cursorCol = 1, 0
			e.LowercaseSelection()
		}},
		{"duplicate a selection ending at column 0", func(e *Editor) {
			selectRows(e, 1, 2)
			e.cursorRow, e.cursorCol = 3, 0
			e.DuplicateLines()
		}},
		{"duplicate the last line", func(e *Editor) { e.cursorRow = 4; e.DuplicateLines() }},
		{"delete every line", func(e *Editor) { selectRows(e, 0, 4); e.DeleteLines() }},
		{"delete the last line", func(e *Editor) { e.cursorRow = 4; e.DeleteLines() }},
		{"move a selection up to the top", func(e *Editor) { selectRows(e, 1, 3); e.MoveLinesUp() }},
		{"move a selection down to the bottom", func(e *Editor) {
			selectRows(e, 2, 3)
			e.cursorRow, e.cursorCol = 4, 0 // column-0 end: row 4 is not moved
			e.MoveLinesDown()
		}},
		{"indent lines", func(e *Editor) { selectRows(e, 2, 4); e.IndentLines() }},
		{"dedent lines", func(e *Editor) { selectRows(e, 0, 2); e.DedentLines() }},
		{"cut a block", func(e *Editor) {
			e.selecting, e.selBlock = true, true
			e.selAnchorRow, e.selAnchorCol = 0, 1
			e.cursorRow, e.cursorCol = 3, 4
			e.Cut()
		}},
		{"block paste", func(e *Editor) {
			e.blockClip = "ab\ncd"
			e.cursorRow, e.cursorCol = 1, 3
			e.Paste("ab\ncd")
		}},
		{"block paste over a selection", func(e *Editor) {
			e.blockClip = "ab\ncd\nef"
			selectRows(e, 1, 2)
			e.Paste("ab\ncd\nef")
		}},
		{"block paste running past the end", func(e *Editor) {
			e.blockClip = "1\n2\n3\n4"
			e.cursorRow, e.cursorCol = 3, 2
			e.Paste("1\n2\n3\n4")
		}},
		{"replace all in a selection", func(e *Editor) {
			selectRows(e, 1, 3)
			e.SetSearch(SearchOptions{Query: "SELECT", Replace: "x", InSelection: true})
			e.ReplaceAll()
		}},
		{"block insert", func(e *Editor) {
			e.selecting, e.selBlock = true, true
			e.selAnchorRow, e.selAnchorCol = 1, 2
			e.cursorRow, e.cursorCol = 3, 2
			e.HandleKey(tuikittest.Rune('#', tcell.ModNone))
		}},
		{"block delete", func(e *Editor) {
			e.selecting, e.selBlock = true, true
			e.selAnchorRow, e.selAnchorCol = 1, 1
			e.cursorRow, e.cursorCol = 3, 4
			e.HandleKey(tuikittest.Key(tcell.KeyDelete, tcell.ModNone))
		}},
		{"replace current", func(e *Editor) {
			e.SetSearch(SearchOptions{Query: "two", Replace: "II"})
			e.FindNext(1)
			e.ReplaceCurrent()
		}},
		{"replace all", func(e *Editor) {
			e.SetSearch(SearchOptions{Query: "SELECT", Replace: "select"})
			e.ReplaceAll()
		}},
		{"replace text", func(e *Editor) { e.cursorRow, e.cursorCol = 3, 4; e.ReplaceText("SELECT 1\n\nGO") }},
		{"replace selected lines", func(e *Editor) {
			e.selecting, e.selAnchorRow, e.selAnchorCol = true, 1, 6
			e.cursorRow, e.cursorCol = 3, 2
			e.ReplaceSelectedLines("a\nb\nc\nd\ne")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEditor(doc)
			tc.do(e)
			after, afterRow, afterCol := e.Text(), e.cursorRow, e.cursorCol
			if after == doc {
				t.Fatalf("setup: the operation changed nothing, so undo proves nothing")
			}

			e.undo()
			if got := e.Text(); got != doc {
				t.Fatalf("undo left\n%q\nwant\n%q", got, doc)
			}
			if e.cursorRow >= e.doc.Len() || e.cursorCol > len(e.doc.Line(e.cursorRow)) {
				t.Fatalf("undo left the cursor at (%d,%d), outside the restored document", e.cursorRow, e.cursorCol)
			}

			e.redo()
			if got := e.Text(); got != after {
				t.Fatalf("redo left\n%q\nwant\n%q", got, after)
			}
			if e.cursorRow != afterRow || e.cursorCol != afterCol {
				t.Errorf("redo put the cursor at (%d,%d), want (%d,%d)", e.cursorRow, e.cursorCol, afterRow, afterCol)
			}
		})
	}
}

// TestEditorLineActionsKeepUndoHistoryOnALargeScript pins that the line
// actions record only the rows they touch: a whole-document step per action
// on a 20,000-line script is ~5 MB, so a few of them evicted every earlier
// step past maxUndoBytes.
func TestEditorLineActionsKeepUndoHistoryOnALargeScript(t *testing.T) {
	doc := strings.Repeat("SELECT name, object_id FROM sys.objects WHERE type = 'U'\n", 20000)
	e := newTestEditor(doc)
	e.cursorRow = 10000
	for range 50 {
		e.HandleKey(tuikittest.Rune('x', tcell.ModNone))
	}
	for range 10 {
		e.IndentLines()
		e.DedentLines()
		e.SetLineComment("--")
		e.ToggleLineComments()
		e.DuplicateLines()
		e.MoveLinesDown()
		e.MoveLinesUp()
		e.DeleteLines()
	}
	e.SelectRange(100, 0, 110, 3)
	e.UppercaseSelection()
	e.Cut()
	e.Paste("a\nb")
	e.SetSearch(SearchOptions{Query: "WHERE type = 'U'x", Replace: "-"})
	e.ReplaceAll()

	e.finalizeStep()
	if e.undoBytes > 1<<20 {
		t.Errorf("undo stack holds %d bytes; line actions should record only their rows", e.undoBytes)
	}
	for len(e.undoStack) > 0 {
		e.undo()
	}
	if e.Text() != doc {
		t.Error("undoing every step did not restore the original script: early steps were evicted")
	}
}

// TestEditorShiftClickExtendsFromCursorWhenNoSelection pins down that
// Shift+Click with no active selection yet extends from the cursor's
// current position, rather than re-anchoring at the click point the way a
// plain click does.
func TestEditorShiftClickExtendsFromCursorWhenNoSelection(t *testing.T) {
	e := newTestEditor("hello world")
	e.SetBounds(0, 0, 40, 5)
	e.cursorRow, e.cursorCol = 0, 0

	e.HandleMouse(tcell.NewEventMouse(e.gutterWidth()+5, 0, tcell.Button1, tcell.ModShift))

	if !e.HasSelection() {
		t.Fatal("Shift+Click should have created a selection")
	}
	if got := e.SelectedText(); got != "hello" {
		t.Fatalf("SelectedText() = %q, want %q (cursor's pre-click position to the click)", got, "hello")
	}
}

// TestEditorShiftClickExtendsExistingSelection pins down that Shift+Click
// with a selection already active keeps the existing anchor and only moves
// the cursor, rather than collapsing to a fresh selection anchored at the
// click point.
func TestEditorShiftClickExtendsExistingSelection(t *testing.T) {
	e := newTestEditor("hello world")
	e.SetBounds(0, 0, 40, 5)
	e.cursorRow, e.cursorCol = 0, 0
	for range 3 {
		e.HandleKey(tuikittest.Key(tcell.KeyRight, tcell.ModShift))
	}
	if got := e.SelectedText(); got != "hel" {
		t.Fatalf("setup: SelectedText() = %q, want %q", got, "hel")
	}

	e.HandleMouse(tcell.NewEventMouse(e.gutterWidth()+8, 0, tcell.Button1, tcell.ModShift))

	if got := e.SelectedText(); got != "hello wo" {
		t.Fatalf("SelectedText() = %q, want %q (extended from the original anchor, not the click)", got, "hello wo")
	}
}

// TestEditorPlainClickStillReanchors confirms the Shift+Click change left
// plain-click behavior untouched: it still collapses any prior selection
// and re-anchors at the click point.
func TestEditorPlainClickStillReanchors(t *testing.T) {
	e := newTestEditor("hello world")
	e.SetBounds(0, 0, 40, 5)
	e.selecting = true
	e.selAnchorRow, e.selAnchorCol = 0, 0
	e.cursorRow, e.cursorCol = 0, 3

	e.HandleMouse(tcell.NewEventMouse(e.gutterWidth()+6, 0, tcell.Button1, tcell.ModNone))

	if e.HasSelection() {
		t.Fatal("a plain (non-Shift) click should collapse any prior selection, not extend it")
	}
	if e.cursorCol != 6 {
		t.Fatalf("cursorCol after click = %d, want 6", e.cursorCol)
	}
}

// TestEditorShiftClickExtendsSelectionInWrapMode confirms the same
// Shift+Click extension applies in wrap mode, where the press derives its
// position through wrappedPosAt.
func TestEditorShiftClickExtendsSelectionInWrapMode(t *testing.T) {
	e := newTestEditor("hello world")
	e.SetWrapMode(true)
	e.SetBounds(0, 0, 40, 5)
	e.cursorRow, e.cursorCol = 0, 0

	e.HandleMouse(tcell.NewEventMouse(e.gutterWidth()+5, 0, tcell.Button1, tcell.ModShift))

	if got := e.SelectedText(); got != "hello" {
		t.Fatalf("SelectedText() = %q, want %q", got, "hello")
	}
}

// TestEditorReadOnlyRejectsMutatingKeys confirms SetReadOnly(true) blocks
// every mutating key (typed runes, Enter, Backspace, Tab-indent, Ctrl+D
// duplicate line, undo) without erroring, and that the same keys work
// normally again once read-only is turned back off.
func TestEditorReadOnlyRejectsMutatingKeys(t *testing.T) {
	e := newTestEditor("hello")
	e.SetReadOnly(true)
	e.cursorCol = 5

	mutating := []*tcell.EventKey{
		tuikittest.Rune('!', tcell.ModNone),
		tuikittest.Key(tcell.KeyEnter, tcell.ModNone),
		tuikittest.Key(tcell.KeyBackspace, tcell.ModNone),
		tuikittest.Key(tcell.KeyDelete, tcell.ModNone),
		tuikittest.Key(tcell.KeyTab, tcell.ModNone),
		tuikittest.Key(tcell.KeyCtrlD, tcell.ModNone),
		tuikittest.Key(tcell.KeyCtrlZ, tcell.ModNone),
	}
	for _, ev := range mutating {
		if e.HandleKey(ev) {
			t.Errorf("HandleKey(%v) returned true while read-only, want false (rejected)", ev.Key())
		}
	}
	if got := e.Text(); got != "hello" {
		t.Fatalf("text mutated while read-only: got %q, want unchanged %q", got, "hello")
	}

	// Movement and selection still work.
	e.cursorCol = 0
	if !e.HandleKey(tuikittest.Key(tcell.KeyRight, tcell.ModNone)) || e.cursorCol != 1 {
		t.Error("plain movement should still work while read-only")
	}
	if !e.HandleKey(tuikittest.Key(tcell.KeyRight, tcell.ModShift)) || !e.HasSelection() {
		t.Error("Shift+movement (selection) should still work while read-only")
	}
	e.selecting = false
	if !e.HandleKey(tuikittest.Key(tcell.KeyCtrlA, tcell.ModNone)) || !e.HasSelection() {
		t.Error("Ctrl+A (Select All) should still work while read-only")
	}

	// Turning read-only back off restores normal editing.
	e.SetReadOnly(false)
	e.selecting = false
	e.cursorRow, e.cursorCol = 0, 5
	e.HandleKey(tuikittest.Rune('!', tcell.ModNone))
	if got := e.Text(); got != "hello!" {
		t.Fatalf("text after SetReadOnly(false) = %q, want %q", got, "hello!")
	}
}

// TestEditorVerticalMovePreservesDesiredColumn confirms Up/Down use a
// Notepad++/Scintilla-style "goal column": passing through a line too short
// to hold the starting column clamps the cursor for that line only, and the
// original column is restored the moment a long-enough line is reached
// again, rather than staying stuck at the short line's clamped column.
func TestEditorVerticalMovePreservesDesiredColumn(t *testing.T) {
	e := newTestEditor("SELECT column_one\nshort\nFROM some_long_table_name")
	e.cursorRow, e.cursorCol = 0, 10
	e.desiredCol = 10

	e.HandleKey(tuikittest.Key(tcell.KeyDown, tcell.ModNone))
	if e.cursorRow != 1 || e.cursorCol != len("short") {
		t.Fatalf("Down onto a shorter line: got (%d,%d), want (1,%d)", e.cursorRow, e.cursorCol, len("short"))
	}

	e.HandleKey(tuikittest.Key(tcell.KeyDown, tcell.ModNone))
	if e.cursorRow != 2 || e.cursorCol != 10 {
		t.Fatalf("Down onto a longer line: got (%d,%d), want (2,10) — the goal column should be restored", e.cursorRow, e.cursorCol)
	}

	e.HandleKey(tuikittest.Key(tcell.KeyUp, tcell.ModNone))
	if e.cursorRow != 1 || e.cursorCol != len("short") {
		t.Fatalf("Up back onto the shorter line: got (%d,%d), want (1,%d)", e.cursorRow, e.cursorCol, len("short"))
	}
	e.HandleKey(tuikittest.Key(tcell.KeyUp, tcell.ModNone))
	if e.cursorRow != 0 || e.cursorCol != 10 {
		t.Fatalf("Up back onto the original line: got (%d,%d), want (0,10)", e.cursorRow, e.cursorCol)
	}
}

// TestEditorHorizontalMoveResetsDesiredColumn confirms any non-vertical
// cursor movement (Left/Right, typing, a click, …) re-anchors the goal
// column to wherever the cursor actually is, so a later Up/Down aims for
// that new position instead of a stale one left over from an earlier
// vertical run.
func TestEditorHorizontalMoveResetsDesiredColumn(t *testing.T) {
	e := newTestEditor("one two\nshort\nthree four")
	e.cursorRow, e.cursorCol = 0, 7
	e.desiredCol = 7

	e.HandleKey(tuikittest.Key(tcell.KeyLeft, tcell.ModNone))
	if e.desiredCol != 6 {
		t.Fatalf("desiredCol after Left = %d, want 6", e.desiredCol)
	}

	e.HandleKey(tuikittest.Key(tcell.KeyDown, tcell.ModNone)) // onto "short" (len 5): clamps
	e.HandleKey(tuikittest.Key(tcell.KeyDown, tcell.ModNone)) // onto "three four": should land at 6, not the original 7
	if e.cursorRow != 2 || e.cursorCol != 6 {
		t.Fatalf("cursor after two Downs = (%d,%d), want (2,6)", e.cursorRow, e.cursorCol)
	}
}

// TestEditorShiftDownSelectsToEndOfLineThenToSameColumn locks in the exact
// selection shape Shift+Down (or Shift+Up) is expected to produce — the
// same "ragged" multi-line highlight Notepad++/Scintilla renders: from the
// cursor to the end of the starting line, then from the start of the next
// line up to the same column the movement began at.
func TestEditorShiftDownSelectsToEndOfLineThenToSameColumn(t *testing.T) {
	e := newTestEditor("SELECT *\nFROM patients")
	e.cursorRow, e.cursorCol = 0, 3
	e.desiredCol = 3

	e.HandleKey(tuikittest.Key(tcell.KeyDown, tcell.ModShift))
	if !e.HasSelection() {
		t.Fatal("Shift+Down should start a selection")
	}
	if e.cursorRow != 1 || e.cursorCol != 3 {
		t.Fatalf("cursor after Shift+Down = (%d,%d), want (1,3)", e.cursorRow, e.cursorCol)
	}

	if start, end, ok := e.selectionRangeForLine(0); !ok || start != 3 || end != len("SELECT *")+1 {
		t.Errorf("line 0 selection range = (%d,%d,%v), want (3,%d,true) — selected from the cursor to end of line",
			start, end, ok, len("SELECT *")+1)
	}
	if start, end, ok := e.selectionRangeForLine(1); !ok || start != 0 || end != 3 {
		t.Errorf("line 1 selection range = (%d,%d,%v), want (0,3,true) — selected from the start of the line to the same column",
			start, end, ok)
	}
}

// TestEditorHorizontalWheelScroll confirms WheelLeft/WheelRight, and
// Shift+WheelUp/WheelDown as the common desktop-convention alias for them,
// all move scrollCol, clamped so the last character of the longest line
// stays reachable — mirrors DataGrid's identical convention
// (TestDataGridHorizontalWheelScroll).
func TestEditorHorizontalWheelScroll(t *testing.T) {
	e := newTestEditor("0123456789abcdefghijklmnopqrstuvwxyz")
	e.SetBounds(0, 0, 10, 5)

	e.HandleMouse(tcell.NewEventMouse(6, 2, tcell.WheelRight, tcell.ModNone))
	if e.scrollCol != core.WheelCells {
		t.Fatalf("scrollCol after WheelRight = %d, want %d", e.scrollCol, core.WheelCells)
	}
	// Enough further ticks to overshoot the line's end regardless of step size.
	for range len(e.doc.Line(0)) {
		e.HandleMouse(tcell.NewEventMouse(6, 2, tcell.WheelRight, tcell.ModNone))
	}
	want := len(e.doc.Line(0)) - 1
	if e.scrollCol != want {
		t.Fatalf("scrollCol after repeated WheelRight = %d, want %d (clamped to the last character)", e.scrollCol, want)
	}
	e.HandleMouse(tcell.NewEventMouse(6, 2, tcell.WheelLeft, tcell.ModNone))
	if e.scrollCol != want-core.WheelCells {
		t.Fatalf("scrollCol after WheelLeft = %d, want %d", e.scrollCol, want-core.WheelCells)
	}

	e.scrollCol = 0
	e.HandleMouse(tcell.NewEventMouse(6, 2, tcell.WheelDown, tcell.ModShift))
	if e.scrollCol != core.WheelCells {
		t.Fatalf("scrollCol after Shift+WheelDown = %d, want %d", e.scrollCol, core.WheelCells)
	}
	e.HandleMouse(tcell.NewEventMouse(6, 2, tcell.WheelUp, tcell.ModShift))
	if e.scrollCol != 0 {
		t.Fatalf("scrollCol after Shift+WheelUp = %d, want 0", e.scrollCol)
	}

	// Plain WheelUp/WheelDown (no Shift) still scroll rows, not columns.
	e.HandleMouse(tcell.NewEventMouse(6, 2, tcell.WheelDown, tcell.ModNone))
	if e.scrollCol != 0 {
		t.Fatalf("plain WheelDown must not touch scrollCol, got %d", e.scrollCol)
	}
}

// ---------------------------------------------------------------------------
// Horizontal scrollbar
// ---------------------------------------------------------------------------

func TestEditorHScrollbarOnlyWhenALineOverflows(t *testing.T) {
	e := newTestEditor("short\nalso short")
	e.SetBounds(0, 0, 40, 6)
	if e.hScrollbarVisible() {
		t.Fatal("bar drawn even though every line fits")
	}
	if got := e.contentH(); got != 6 {
		t.Fatalf("contentH with no bar = %d, want the full height 6", got)
	}

	e.SetText("short\n" + strings.Repeat("x", 200))
	if !e.hScrollbarVisible() {
		t.Fatal("no bar for a buffer whose longest line is far wider than the editor")
	}
	if got := e.contentH(); got != 5 {
		t.Fatalf("contentH with a bar = %d, want 5 — the bottom row belongs to the bar", got)
	}
}

// Word wrap has no horizontal scrolling at all, so it must never give up a
// row to a bar it would then never draw.
func TestEditorHScrollbarNeverInWrapMode(t *testing.T) {
	e := newTestEditor(strings.Repeat("x", 200))
	e.SetBounds(0, 0, 40, 6)
	e.SetWrapMode(true)
	if e.hScrollbarVisible() {
		t.Fatal("bar reported in wrap mode")
	}
	if got := e.contentH(); got != 6 {
		t.Fatalf("contentH in wrap mode = %d, want the full height 6", got)
	}
}

func TestEditorHScrollbarDragScrollsSideways(t *testing.T) {
	e := newTestEditor(strings.Repeat("x", 200))
	e.SetBounds(0, 0, 40, 6)
	x, y, w, _, _, ok := e.hScrollbar()
	if !ok {
		t.Fatal("expected a horizontal scrollbar")
	}

	// Press two-thirds along the track, then release.
	e.HandleMouse(tcell.NewEventMouse(x+w*2/3, y, tcell.Button1, tcell.ModNone))
	if e.scrollCol == 0 {
		t.Fatal("dragging the thumb two-thirds along left scrollCol at 0")
	}
	scrolled := e.scrollCol
	e.HandleMouse(tcell.NewEventMouse(x+w*2/3, y, tcell.ButtonNone, tcell.ModNone))
	if e.gestureHeld() {
		t.Error("thumb drag still held after the release")
	}

	// Back to the far left.
	e.HandleMouse(tcell.NewEventMouse(x, y, tcell.Button1, tcell.ModNone))
	if e.scrollCol != 0 {
		t.Fatalf("scrollCol after dragging to the far left = %d, want 0 (was %d)", e.scrollCol, scrolled)
	}
	e.HandleMouse(tcell.NewEventMouse(x, y, tcell.ButtonNone, tcell.ModNone))
}

// wideTestEditor is 20 long lines in a 40x6 rect — wide enough and tall
// enough that both scrollbars are showing.
func wideTestEditor() *Editor {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = strings.Repeat("x", 200)
	}
	e := newTestEditor(strings.Join(lines, "\n"))
	e.SetBounds(0, 0, 40, 6)
	return e
}

// The bar's row is not a line of text: nothing may put the cursor on the
// line that would otherwise have been drawn there. SetCursorFromScreen is
// the reachable path — the app layer calls it with the drop coordinates of
// an Object Explorer drag-and-drop, which can land anywhere in the rect.
func TestEditorCursorFromScreenSkipsScrollbarRow(t *testing.T) {
	e := wideTestEditor()
	_, y, _, _, _, ok := e.hScrollbar()
	if !ok {
		t.Fatal("expected a horizontal scrollbar")
	}
	if y != 5 {
		t.Fatalf("bar row = %d, want the rect's bottom row 5", y)
	}

	e.SetCursorFromScreen(e.rect.X+e.gutterWidth(), y)
	if e.cursorRow > 4 {
		t.Errorf("cursorRow after dropping on the scrollbar row = %d, want at most 4 — row 5 isn't a text row", e.cursorRow)
	}
}

// A selection dragged downward off the last text line reaches the bar's
// row, and the track spans the whole content width — the gesture has to
// stay with the text, not be taken over by the bar: a gesture belongs to
// whatever claimed its press, until the release.
func TestEditorSelectionDragIsNotStolenByHScrollbar(t *testing.T) {
	e := wideTestEditor()
	_, barY, _, _, _, _ := e.hScrollbar()
	contentX := e.rect.X + e.gutterWidth()

	// Press on the first text line, then drag down across the bar's row.
	e.HandleMouse(tcell.NewEventMouse(contentX+3, 0, tcell.Button1, tcell.ModNone))
	before := e.scrollCol
	e.HandleMouse(tcell.NewEventMouse(contentX+20, barY, tcell.Button1, tcell.ModNone))

	if !e.gestureIs(editorText) {
		t.Error("the scrollbar took over a text-selection drag that merely crossed its row")
	}
	if e.scrollCol != before {
		t.Errorf("scrollCol moved from %d to %d during a text-selection drag", before, e.scrollCol)
	}
	if !e.selecting {
		t.Error("the text selection was dropped when the drag reached the scrollbar row")
	}
	e.HandleMouse(tcell.NewEventMouse(contentX+20, barY, tcell.ButtonNone, tcell.ModNone))
}

// SetText is the reset-for-reuse path, so no mouse latch may survive it — a
// latch left set makes the next press in the reused editor read as the
// continuation of a gesture that ended in the previous document. Enumerated
// rather than spot-checked because the bug was one field missing from the
// list.
func TestEditorSetTextClearsEveryMouseLatch(t *testing.T) {
	latches := func(e *Editor) map[string]*bool {
		return map[string]*bool{
			"selecting": &e.selecting,
			"selBlock":  &e.selBlock,
		}
	}
	e := wideTestEditor()
	for _, p := range latches(e) {
		*p = true
	}
	e.HandleMouse(tcell.NewEventMouse(e.rect.X+e.gutterWidth()+1, 0, tcell.Button1, tcell.ModNone))
	e.SetText("SELECT 1")
	for name, p := range latches(e) {
		if *p {
			t.Errorf("%s survived SetText", name)
		}
	}
	if e.gestureHeld() {
		t.Error("the held gesture survived SetText")
	}
}

// -- wrap-mode highlighting -------------------------------------------------

// markerHighlighter colours exactly the rune range [start,end) of every line
// with a style nothing else in the editor uses, so a test can find it.
func markerHighlighter(start, end int, st tcell.Style) Highlighter {
	return func(doc *Document, idx int) []ColorRun {
		return []ColorRun{{Start: start, Len: end - start, Style: st}}
	}
}

// TestEditorWrapModeAppliesHighlighter pins that wrap mode and a Highlighter
// compose. drawWrapped originally never called the highlighter at all, so a
// wrapped editor rendered as plain text — silently, since setting both is not
// an error.
func TestEditorWrapModeAppliesHighlighter(t *testing.T) {
	marker := tcell.StyleDefault.Foreground(color.Fuchsia).Underline(true)

	// Columns 30-34 of the logical line fall on the *second* visual row once
	// the line wraps, which is the case a per-visual-row implementation that
	// forgot to offset by vl.start would get wrong.
	e := NewEditor(markerHighlighter(30, 35, marker))
	e.SetWrapMode(true)
	e.SetGutterVisible(false)
	e.SetText("aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd")
	e.SetBounds(0, 0, 22, 6)

	s := tuikittest.NewScreen(40, 10)
	e.Draw(s)

	found := false
	for y := range 10 {
		for x := range 40 {
			if s.StyleAt(x, y) == marker {
				found = true
				if y == 0 {
					t.Errorf("highlighted cell at (%d,%d) is on the first visual row; columns 30-34 wrap onto a later one", x, y)
				}
			}
		}
	}
	if !found {
		t.Error("no cell carried the highlighter's style — wrap mode ignored the Highlighter")
	}
}

// TestEditorWrapModeSelectionBeatsHighlighter: selection styling is applied
// after the highlighter in non-wrap mode, and has to stay that way in wrap
// mode, or selecting highlighted text stops looking selected.
func TestEditorWrapModeSelectionBeatsHighlighter(t *testing.T) {
	marker := tcell.StyleDefault.Foreground(color.Fuchsia).Underline(true)

	e := NewEditor(markerHighlighter(0, 5, marker))
	e.SetWrapMode(true)
	e.SetGutterVisible(false)
	e.SetText("aaaaaaaaaa bbbbbbbbbb")
	e.SetBounds(0, 0, 22, 6)
	e.SelectAll()

	s := tuikittest.NewScreen(40, 10)
	e.Draw(s)

	if s.StyleAt(0, 0) == marker {
		t.Error("column 0 kept the highlighter style inside a selection; selection must win")
	}
}

func TestStyleAt(t *testing.T) {
	def := tcell.StyleDefault
	a := tcell.StyleDefault.Foreground(color.Red)
	b := tcell.StyleDefault.Foreground(color.Blue)
	runs := []ColorRun{{Start: 2, Len: 3, Style: a}, {Start: 4, Len: 2, Style: b}}

	cases := []struct {
		ci   int
		want tcell.Style
	}{
		{0, def},
		{2, a},
		{3, a},
		{4, b}, // overlapping runs: the later one wins, as drawHighlighted's map does
		{5, b},
		{6, def},
	}
	for _, tc := range cases {
		if got := styleAt(runs, tc.ci, def); got != tc.want {
			t.Errorf("styleAt(ci=%d) = %v, want %v", tc.ci, got, tc.want)
		}
	}
}

// clickAt sends one Button1 press followed by its release at (x, y) — the
// pair the editor pairs up to recognise a double-click.
func clickAt(e *Editor, x, y int) {
	e.HandleMouse(tcell.NewEventMouse(x, y, tcell.Button1, tcell.ModNone))
	e.HandleMouse(tcell.NewEventMouse(x, y, tcell.ButtonNone, tcell.ModNone))
}

func TestEditorDoubleClickSelectsWord(t *testing.T) {
	e := newTestEditor("select foo from bar")
	e.SetGutterVisible(false)
	e.SetBounds(0, 0, 40, 5)

	clickAt(e, 8, 0) // inside "foo"
	if e.HasSelection() {
		t.Fatal("a single click should not select anything")
	}
	clickAt(e, 8, 0)

	if got := e.SelectedText(); got != "foo" {
		t.Fatalf("double-click SelectedText() = %q, want %q", got, "foo")
	}
}

func TestEditorDoubleClickOnWhitespaceSelectsNothing(t *testing.T) {
	e := newTestEditor("ab   cd")
	e.SetGutterVisible(false)
	e.SetBounds(0, 0, 40, 5)

	clickAt(e, 3, 0)
	clickAt(e, 3, 0)

	if e.HasSelection() {
		t.Fatalf("double-click between words selected %q, want nothing", e.SelectedText())
	}
}

// clickAtMod is clickAt with a modifier held on the press.
func clickAtMod(e *Editor, x, y int, mod tcell.ModMask) {
	e.HandleMouse(tcell.NewEventMouse(x, y, tcell.Button1, mod))
	e.HandleMouse(tcell.NewEventMouse(x, y, tcell.ButtonNone, tcell.ModNone))
}

// A modified press is not itself a double-click, but it is still "the
// previous press" for the one after it. Testing the modifier outside
// pressIsDouble breaks that: the true branch discards the press before the
// modifier is consulted, so the click after a Shift-click has nothing to
// pair with.
func TestEditorClickAfterShiftClickStillSelectsWord(t *testing.T) {
	e := newTestEditor("select foo from bar")
	e.SetGutterVisible(false)
	e.SetBounds(0, 0, 40, 5)

	clickAt(e, 8, 0)
	clickAtMod(e, 8, 0, tcell.ModShift)
	if got := e.SelectedText(); got == "foo" {
		t.Fatal("a Shift-click selected the word; it should extend the selection instead")
	}
	clickAt(e, 8, 0)

	if got := e.SelectedText(); got != "foo" {
		t.Errorf("the click after a Shift-click selected %q, want %q", got, "foo")
	}
}

// Two clicks far apart on the *same* spot are two separate clicks, not a
// double-click.
func TestEditorSlowSecondClickDoesNotSelectWord(t *testing.T) {
	e := newTestEditor("select foo")
	e.SetGutterVisible(false)
	e.SetBounds(0, 0, 40, 5)

	t0 := time.Now()
	for _, at := range []time.Time{t0, t0.Add(2 * core.DoubleClickInterval)} {
		press := tcell.NewEventMouse(8, 0, tcell.Button1, tcell.ModNone)
		press.SetEventTime(at)
		e.HandleMouse(press)
		e.HandleMouse(tcell.NewEventMouse(8, 0, tcell.ButtonNone, tcell.ModNone))
	}

	if e.HasSelection() {
		t.Fatalf("slow second click selected %q, want nothing", e.SelectedText())
	}
}

// TestEditorRedoStackBound pins what maxUndoSteps' comment claims about the
// redo stack, both halves of it: the count bound holds, and the byte bound
// does not.
//
// The second half is the one worth a test. applyStep's inverse carries the
// lines being replaced — the document as it is now — not the lines the popped
// step carried, so on a document that grew over its history every inverse is
// bigger than the step that produced it and the redo stack ends up holding
// more than the undo stack ever did. The comment this replaced claimed redo
// "can never grow past what undo popped", which is the reading anyone
// re-deriving the bound from the code will reach for; this fails on it.
func TestEditorRedoStackBound(t *testing.T) {
	chunk := strings.Repeat(strings.Repeat("z", 100)+"\n", 50)
	e := newTestEditor("seed")
	for range 10 {
		e.cursorRow, e.cursorCol = 0, 0
		e.Paste(chunk) // each one grows the buffer
	}
	e.finalizeStep()
	undoSteps, undoBytes := len(e.undoStack), e.undoBytes

	redoBytes := func() int {
		n := 0
		for _, st := range e.redoStack {
			n += st.bytes
		}
		return n
	}
	peak := 0
	for len(e.undoStack) > 0 {
		e.undo()
		if b := redoBytes(); b > peak {
			peak = b
		}
	}

	if len(e.redoStack) != undoSteps {
		t.Fatalf("redo holds %d steps after undoing %d; undo pushes exactly one per pop", len(e.redoStack), undoSteps)
	}
	if len(e.redoStack) > maxUndoSteps {
		t.Errorf("redo holds %d steps, more than maxUndoSteps (%d)", len(e.redoStack), maxUndoSteps)
	}
	if peak <= undoBytes {
		t.Errorf("peak redo bytes %d <= undo bytes %d; the growing-document case is meant to invert them, "+
			"so either this setup stopped growing the buffer or applyStep changed which lines the inverse carries",
			peak, undoBytes)
	}
}

// TestEditorGutterWidensPastLine9999: a fixed five-column gutter drew line
// 10,000 one column left of the editor, over whatever bordered it (T32). The
// gutter grows to the widest number, in both the plain and the wrapped paths.
func TestEditorGutterWidensPastLine9999(t *testing.T) {
	text := strings.Repeat("x\n", 10000) + "last"
	for _, wrap := range []bool{false, true} {
		e := NewEditor(nil)
		e.SetText(text)
		e.SetWrapMode(wrap)
		e.SetBounds(10, 0, 30, 3)
		e.scrollRow = 9999
		s := tuikittest.NewScreen(40, 3)
		s.Fill('~', tcell.StyleDefault) // so a drawn space shows too
		e.Draw(s)

		for y := range 3 {
			if got := s.Text(0, y, 10); got != "~~~~~~~~~~" {
				t.Fatalf("wrap=%v: drew %q on row %d, left of the editor at 10", wrap, got, y)
			}
		}
		if got, want := e.gutterWidth(), 7; got != want {
			t.Errorf("wrap=%v: gutterWidth = %d, want %d (five digits and a column each side)", wrap, got, want)
		}
		if got := s.Text(10, 0, 7); got != " 10000 " {
			t.Errorf("wrap=%v: gutter row 0 = %q, want %q", wrap, got, " 10000 ")
		}
	}
}

// A composed key inserts every rune it carries (T66): a ZWJ family emoji is
// five runes in one KeyRune event, and the editor kept only the first.
func TestEditorInsertsAComposedKeyWhole(t *testing.T) {
	const family = "\U0001F468\u200d\U0001F469\u200d\U0001F467"
	e := NewEditor(nil)
	e.SetBounds(0, 0, 40, 5)
	e.Focus(true)
	e.SetText("ab")
	e.cursorCol = 1
	e.HandleKey(tcell.NewEventKey(tcell.KeyRune, family, tcell.ModNone))
	e.HandleKey(tcell.NewEventKey(tcell.KeyRune, "x", tcell.ModNone))
	if got, want := e.Text(), "a"+family+"xb"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
}

// TestEditorLineCommentPrefix pins that the comment prefix is the host's:
// none set makes Ctrl+/ a no-op the editor does not claim, and any other
// prefix is inserted and stripped the way "--" is.
func TestEditorLineCommentPrefix(t *testing.T) {
	e := newTestEditor("SELECT 1")
	e.ToggleLineComments()
	if got := e.Text(); got != "SELECT 1" {
		t.Fatalf("no prefix: ToggleLineComments changed the text to %q", got)
	}
	if e.HandleKey(tuikittest.Rune('/', tcell.ModCtrl)) {
		t.Fatal("no prefix: Ctrl+/ was claimed though it did nothing")
	}

	for _, prefix := range []string{"//", "#", "REM"} {
		e := newTestEditor("  x\n\n  y")
		e.SetLineComment(prefix)
		e.SelectAll()
		if !e.HandleKey(tuikittest.Rune('/', tcell.ModCtrl)) {
			t.Fatalf("%q: Ctrl+/ was not claimed", prefix)
		}
		want := "  " + prefix + " x\n" + prefix + " \n  " + prefix + " y"
		if got := e.Text(); got != want {
			t.Fatalf("%q: comment = %q, want %q", prefix, got, want)
		}
		e.SelectAll()
		e.ToggleLineComments()
		if got := e.Text(); got != "  x\n\n  y" {
			t.Fatalf("%q: uncomment = %q", prefix, got)
		}
	}
}

// TestTextAndSelectedTextRoundTripNonASCII pins Text and SelectedText's
// rune-by-rune builders (sized by rune count, exact only for ASCII) on text
// whose bytes outnumber its runes: an accent, a CJK glyph, an emoji, and an
// empty line between.
func TestTextAndSelectedTextRoundTripNonASCII(t *testing.T) {
	const text = "SELECT 'café'\n\n-- 漢字 🙂\nGO"
	e := NewEditor(nil)
	e.SetText(text)
	if got := e.Text(); got != text {
		t.Fatalf("Text() = %q, want %q", got, text)
	}
	e.SelectRange(0, 8, 2, 5)
	if got, want := e.SelectedText(), "café'\n\n-- 漢字"; got != want {
		t.Errorf("SelectedText() = %q, want %q", got, want)
	}
	e.SelectRange(2, 3, 2, 5)
	if got, want := e.SelectedText(), "漢字"; got != want {
		t.Errorf("one-line SelectedText() = %q, want %q", got, want)
	}
}

// A click arms the selection with its anchor at the click point (so
// Shift+click and Shift+arrow extend from there). Paste moved the cursor
// without disarming it, so the pasted text became the selection and the next
// key typed replaced it — every Ctrl+V, bracketed paste and drop into the
// editor after a click.
func TestEditorPasteAfterClickLeavesNoSelection(t *testing.T) {
	const line = "select 1 from t"
	for col := 0; col <= len(line); col++ {
		e := newTestEditor(line)
		e.SetGutterVisible(false)
		e.SetBounds(0, 0, 40, 5)
		clickAt(e, col, 0)
		e.Paste("XYZ ")
		if e.HasSelection() {
			t.Fatalf("click at %d then Paste: selection %q, want none", col, e.SelectedText())
		}
		e.HandleKey(tcell.NewEventKey(tcell.KeyRune, "q", tcell.ModNone))
		if want := line[:col] + "XYZ q" + line[col:]; e.Text() != want {
			t.Fatalf("click at %d, Paste, type q: text %q, want %q", col, e.Text(), want)
		}
	}
}

// The armed anchor a click leaves is what Shift+Right extends from; the paste
// fix must disarm at the insertion, not at the release.
func TestEditorShiftRightAfterClickExtendsFromClick(t *testing.T) {
	e := newTestEditor("select 1 from t")
	e.SetGutterVisible(false)
	e.SetBounds(0, 0, 40, 5)
	clickAt(e, 9, 0)
	e.HandleKey(tuikittest.Key(tcell.KeyRight, tcell.ModShift))
	if got := e.SelectedText(); got != "f" {
		t.Fatalf("click then Shift+Right: SelectedText() = %q, want %q", got, "f")
	}
}

// An undo or redo that removes rows must not leave a selection anchored on one of
// them: the Edit menu's Undo()/Redo() skip the key path's drop, and the next read
// of the selection indexed past the document.
func TestUndoDropsSelectionItCanNoLongerHold(t *testing.T) {
	readSelection := func(t *testing.T, e *Editor) {
		t.Helper()
		if e.HasSelection() {
			t.Fatalf("HasSelection() = true after the step")
		}
		_ = e.SelectedText()
		_ = e.Cut()
		e.HandleKey(tuikittest.Rune('x'))
	}
	setup := func(text string) *Editor {
		e := newTestEditor(text)
		e.SetBounds(0, 0, 40, 10)
		e.Focus(true)
		return e
	}

	t.Run("undo", func(t *testing.T) {
		e := setup("abc")
		e.DuplicateLines()
		e.HandleKey(tuikittest.Key(tcell.KeyLeft, tcell.ModShift))
		e.Undo()
		readSelection(t, e)
	})
	t.Run("redo", func(t *testing.T) {
		e := setup("abc\ndef")
		e.HandleKey(tuikittest.Key(tcell.KeyDown))
		e.DeleteLines() // 1 row
		e.Undo()        // 2 rows
		e.HandleKey(tuikittest.Key(tcell.KeyEnd, tcell.ModCtrl))
		e.HandleKey(tuikittest.Key(tcell.KeyLeft, tcell.ModShift))
		if !e.HasSelection() || e.cursorRow != 1 {
			t.Fatalf("setup: selection %v on row %d, want one on row 1", e.HasSelection(), e.cursorRow)
		}
		e.Redo() // 1 row again
		readSelection(t, e)
	})
	t.Run("block", func(t *testing.T) {
		e := setup("abc")
		e.DuplicateLines()
		e.HandleKey(tuikittest.Key(tcell.KeyLeft, tcell.ModShift|tcell.ModAlt))
		if !e.selBlock {
			t.Fatalf("setup: Alt+Shift+Left made no block selection")
		}
		e.Undo()
		if e.selBlock {
			t.Fatalf("selBlock still set after Undo")
		}
		readSelection(t, e)
	})
}

// Undo() from the Edit menu must scroll the caret back into view, as Ctrl+Z does,
// when the step it reverses lies far from the viewport.
func TestUndoAPIScrollsCaretIntoView(t *testing.T) {
	e := newTestEditor(strings.Repeat("line\n", 60))
	e.SetBounds(0, 0, 40, 5)
	e.Focus(true)
	e.HandleKey(tuikittest.Rune('x')) // edit on row 0
	e.HandleKey(tuikittest.Key(tcell.KeyEnd, tcell.ModCtrl))
	if row, _ := e.ScrollPos(); row == 0 {
		t.Fatalf("setup: Ctrl+End did not scroll")
	}
	e.Undo()
	if row, _ := e.ScrollPos(); e.cursorRow != 0 || row != 0 {
		t.Fatalf("after Undo: cursor row %d, scroll row %d; want both 0", e.cursorRow, row)
	}
	e.Redo()
	e.HandleKey(tuikittest.Key(tcell.KeyEnd, tcell.ModCtrl))
	e.Redo() // nothing to redo: no step, no scroll needed — must not panic
}

// TestLoneCRIsALineBreak: a classic Mac (CR-only) file loads as lines, as SQL
// Server and the host's lexer read it. Before normalizeEOL only "\r\n" folded,
// and "select 1 --a\rselect 2\rGO" loaded as one line, so the host's batch
// splitter never saw the GO. Paste folds the same way.
func TestLoneCRIsALineBreak(t *testing.T) {
	e := newTestEditor("select 1 --a\rselect 2\rGO\rselect 3")
	if got, want := e.doc.Len(), 4; got != want {
		t.Fatalf("SetText: %d lines, want %d", got, want)
	}
	if got, want := e.Text(), "select 1 --a\nselect 2\nGO\nselect 3"; got != want {
		t.Errorf("SetText: Text() = %q, want %q", got, want)
	}

	e = newTestEditor("")
	e.Paste("a\rb\r\nc")
	if got, want := e.Text(), "a\nb\nc"; got != want {
		t.Errorf("Paste: Text() = %q, want %q", got, want)
	}
}
