package editor

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/tuikittest"
)

// pressEnter puts the cursor at row/col and sends a plain Enter.
func pressEnter(e *Editor, row, col int) {
	e.cursorRow, e.cursorCol = row, col
	e.HandleKey(tuikittest.Key(tcell.KeyEnter, tcell.ModNone))
}

func TestAutoIndentAtEndOfIndentedLine(t *testing.T) {
	e := newTestEditor("SELECT 1\n    AND x = 1")
	pressEnter(e, 1, len("    AND x = 1"))

	if got, want := e.Text(), "SELECT 1\n    AND x = 1\n    "; got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
	if r, c := e.CursorPos(); r != 2 || c != 4 {
		t.Fatalf("cursor = (%d,%d), want (2,4) — after the copied indent", r, c)
	}
}

func TestAutoIndentSplitsMidLineWithoutDoubleIndenting(t *testing.T) {
	e := newTestEditor("    AND x = 1")
	pressEnter(e, 0, len("    AND "))

	if got, want := e.Text(), "    AND \n    x = 1"; got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
	if r, c := e.CursorPos(); r != 1 || c != 4 {
		t.Fatalf("cursor = (%d,%d), want (1,4)", r, c)
	}
}

func TestAutoIndentInsideLeadingWhitespaceClampsToCursor(t *testing.T) {
	e := newTestEditor("    AND x = 1")
	pressEnter(e, 0, 2) // inside the 4-space indent

	if got, want := e.Text(), "  \n    AND x = 1"; got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
	if r, c := e.CursorPos(); r != 1 || c != 2 {
		t.Fatalf("cursor = (%d,%d), want (1,2) — only the indent left of the cursor is copied", r, c)
	}
}

func TestAutoIndentLeavesUnindentedLineAlone(t *testing.T) {
	e := newTestEditor("SELECT 1")
	pressEnter(e, 0, len("SELECT 1"))

	if got, want := e.Text(), "SELECT 1\n"; got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
	if r, c := e.CursorPos(); r != 1 || c != 0 {
		t.Fatalf("cursor = (%d,%d), want (1,0)", r, c)
	}
}

func TestAutoIndentAfterDeletingSelection(t *testing.T) {
	e := newTestEditor("    one\n    two")
	// Select from row 0 col 7 through row 1 col 7, then Enter.
	e.selecting = true
	e.selAnchorRow, e.selAnchorCol = 0, 7
	e.cursorRow, e.cursorCol = 1, 7
	e.HandleKey(tuikittest.Key(tcell.KeyEnter, tcell.ModNone))

	if got, want := e.Text(), "    one\n    "; got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
}

func TestAutoIndentIsOneUndoStep(t *testing.T) {
	const before = "SELECT 1\n    AND x = 1"
	e := newTestEditor(before)
	pressEnter(e, 1, len("    AND x = 1"))
	if e.Text() == before {
		t.Fatal("Enter did nothing")
	}

	e.HandleKey(tuikittest.Key(tcell.KeyCtrlZ, tcell.ModNone))
	if got := e.Text(); got != before {
		t.Fatalf("after one Ctrl+Z: Text() = %q, want %q", got, before)
	}
}

func TestPasteOfIndentedScriptDoesNotStaircase(t *testing.T) {
	e := newTestEditor("")
	script := "SELECT 1\n    FROM t\n        WHERE x = 1"
	e.Paste(script)

	if got := e.Text(); got != script {
		t.Fatalf("Text() = %q, want %q — auto-indent must live in the key handler, not insertNewline", got, script)
	}
}

func TestAutoIndentSkippedInBlockEditing(t *testing.T) {
	e := newTestEditor("    aaaa\n    bbbb")
	blockSelect(e, 0, 6, 1, 6)
	e.HandleKey(tuikittest.Key(tcell.KeyEnter, tcell.ModNone))

	if got, want := e.Text(), "    aa\naa\n    bbbb"; got != want {
		t.Fatalf("Text() = %q, want %q — the block path keeps its plain split", got, want)
	}
}

func TestSetIndentWidthDrivesTabIndentAndDedent(t *testing.T) {
	e := NewEditor(nil)
	e.SetIndentWidth(2)
	e.SetText("SELECT 1")
	if got := e.IndentWidth(); got != 2 {
		t.Fatalf("IndentWidth() = %d, want 2", got)
	}

	e.cursorRow, e.cursorCol = 0, 0
	e.HandleKey(tuikittest.Key(tcell.KeyTab, tcell.ModNone))
	if got, want := e.Text(), "  SELECT 1"; got != want {
		t.Fatalf("Tab: Text() = %q, want %q", got, want)
	}

	e.IndentLines()
	if got, want := e.Text(), "    SELECT 1"; got != want {
		t.Fatalf("IndentLines: Text() = %q, want %q", got, want)
	}

	e.DedentLines()
	if got, want := e.Text(), "  SELECT 1"; got != want {
		t.Fatalf("DedentLines: Text() = %q, want %q", got, want)
	}

	e.HandleKey(tuikittest.Key(tcell.KeyBacktab, tcell.ModNone))
	if got, want := e.Text(), "SELECT 1"; got != want {
		t.Fatalf("Shift+Tab: Text() = %q, want %q", got, want)
	}
}

func TestSetIndentWidthDrivesTabExpansion(t *testing.T) {
	e := NewEditor(nil)
	e.SetIndentWidth(2)
	e.SetText("\tSELECT 1")
	if got, want := e.Text(), "  SELECT 1"; got != want {
		t.Fatalf("SetText: Text() = %q, want %q", got, want)
	}
}

func TestAutoIndentCopiesWhateverIsThereRegardlessOfWidth(t *testing.T) {
	e := NewEditor(nil)
	e.SetIndentWidth(2)
	e.SetText("      deep") // 6 spaces, not a multiple of the width
	pressEnter(e, 0, len("      deep"))

	if got, want := e.Text(), "      deep\n      "; got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
}

func TestSetIndentWidthRejectsOutOfRange(t *testing.T) {
	e := NewEditor(nil)
	for _, n := range []int{0, -1, MaxIndentWidth + 1} {
		e.SetIndentWidth(n)
		if got := e.IndentWidth(); got != DefaultIndentWidth {
			t.Fatalf("SetIndentWidth(%d): IndentWidth() = %d, want the default %d", n, got, DefaultIndentWidth)
		}
	}
}

func TestSetDefaultIndentWidthSeedsNewEditors(t *testing.T) {
	t.Cleanup(func() { defaultIndentWidth = DefaultIndentWidth })

	SetDefaultIndentWidth(2)
	if got := NewEditor(nil).IndentWidth(); got != 2 {
		t.Fatalf("new editor IndentWidth() = %d, want 2", got)
	}
	SetDefaultIndentWidth(0)
	if got := NewEditor(nil).IndentWidth(); got != 2 {
		t.Fatalf("out-of-range SetDefaultIndentWidth changed the default: got %d, want 2", got)
	}
}

func TestAutoIndentAcceptsCompletionWithoutExtraIndent(t *testing.T) {
	e := newTestEditor("")
	e.SetCompletionProvider(testCompletionProvider("SELECT"))
	e.HandleKey(tuikittest.Key(tcell.KeyTab, tcell.ModNone)) // one indent level
	typeString(e, "SEL")
	if !e.OverlayActive() {
		t.Fatal("expected the completion popup open after typing \"SEL\"")
	}

	e.HandleKey(tuikittest.Key(tcell.KeyEnter, tcell.ModNone))
	if got, want := e.Text(), "    SELECT"; got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
	if got := e.Text(); strings.Contains(got, "\n") {
		t.Fatalf("Enter committed the completion but also split the line: Text() = %q", got)
	}
}

// TestIndentRuleGetsTheTrimmedLeftAndAddsLevels: Enter hands the rule the text
// left of the cursor with trailing blanks trimmed, and indents by the levels it
// returns times the indent width, on top of the copied indentation. An empty
// left side never reaches the rule, and a negative answer adds nothing.
func TestIndentRuleGetsTheTrimmedLeftAndAddsLevels(t *testing.T) {
	var asked []string
	levels := 2
	e := NewEditor(nil)
	e.SetIndentRule(func(left string) int { asked = append(asked, left); return levels })
	e.SetText("  open:   tail")
	pressEnter(e, 0, len("  open:   "))
	if got, want := e.Text(), "  open:   \n          tail"; got != want {
		t.Fatalf("Text() = %q, want %q (2 copied + 2 levels of 4)", got, want)
	}
	if len(asked) != 1 || asked[0] != "  open:" {
		t.Fatalf("rule asked %q, want [\"  open:\"]", asked)
	}

	asked = nil
	e.SetText("   ")
	pressEnter(e, 0, 3)
	if len(asked) != 0 {
		t.Errorf("rule asked %q for a blank left side, want no call", asked)
	}

	levels = -3
	e.SetText("x")
	pressEnter(e, 0, 1)
	if got, want := e.Text(), "x\n"; got != want {
		t.Errorf("negative levels: Text() = %q, want %q", got, want)
	}
}

func TestSmartIndentUsesTheConfiguredWidth(t *testing.T) {
	e := NewEditor(nil)
	e.SetIndentRule(func(string) int { return 1 })
	e.SetIndentWidth(2)
	e.SetText("  SELECT")
	pressEnter(e, 0, len("  SELECT"))

	if got, want := e.Text(), "  SELECT\n    "; got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
}

// Off by default: a plain multi-line text box has no indent rule.
func TestSmartIndentOffByDefault(t *testing.T) {
	e := newTestEditor("SELECT")
	pressEnter(e, 0, len("SELECT"))

	if got, want := e.Text(), "SELECT\n"; got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
}
