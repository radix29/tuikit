package editor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/radix29/tuikit/theme"
	"github.com/radix29/tuikit/tuikittest"
)

// ---------------------------------------------------------------------------
// Version-keyed caches: prefix states and wrap flattening
// ---------------------------------------------------------------------------
//
// Cross-line highlighters and Editor's wrap flattening do their O(document) work
// once per *edit* rather than once per Draw. That is only correct while
// Document.Version() cannot go stale, so these tests attack the cache from the
// side a stale version would show up on: change the text, ask again, and
// require the answer to reflect the new text rather than the old.

// TestLineStateCacheInvalidatesOnEdit is the load-bearing one. Closing an
// open block comment must uncolour every line after it on the very next call —
// a cache that kept the pre-edit prefix states would keep drawing the rest of
// the document as comment text until something else happened to rebuild it.
func TestLineStateCacheInvalidatesOnEdit(t *testing.T) {
	e := NewEditor(nil)
	e.SetBounds(0, 0, 60, 10)
	e.SetText("SELECT 1 /* open\nSELECT 2\nSELECT 3")
	hl := toyHighlighter(&theme.Default)
	doc := e.Document()

	// Warm the cache: lines 1 and 2 are inside the unterminated comment.
	for idx := range 3 {
		hl(doc, idx)
	}
	if toyLineState(doc, 2) == 0 {
		t.Fatal("line 2 before the edit is not inside the comment")
	}

	// Close the comment on line 0.
	e.cursorRow, e.cursorCol = 0, len(doc.Line(0))
	e.insertRune('*')
	e.insertRune('/')

	got := hl(doc, 2)
	want := toyHighlighter(&theme.Default)(docOf(doc.Lines()), 2) // fresh: full replay
	if !reflect.DeepEqual(got, want) {
		t.Errorf("line 2 after closing the comment = %v, want %v — the cache served pre-edit state", got, want)
	}
	if toyLineState(doc, 2) != 0 {
		t.Error("line 2 is still inside a comment after the comment was closed")
	}
}

// TestLineStateCacheInvalidatesOnLineCountChange covers the shape change a
// plain rune edit doesn't: inserting and deleting lines moves every later
// line's state to a different index, so a cache sized to the old document
// would answer for the wrong line.
func TestLineStateCacheInvalidatesOnLineCountChange(t *testing.T) {
	e := NewEditor(nil)
	e.SetBounds(0, 0, 60, 10)
	e.SetText("SELECT 1\n/* open\nstill inside\n*/ SELECT 2")
	doc := e.Document()

	for idx := range doc.Len() {
		toyLineState(doc, idx)
	}

	// Push the whole comment down a line.
	e.cursorRow, e.cursorCol = 0, 0
	e.insertNewline()

	for idx := range doc.Len() {
		if got, want := toyLineState(doc, idx), toyFullReplay(doc.Lines(), idx); got != want {
			t.Errorf("line %d (%q) after inserting a line = %d, want %d",
				idx, string(doc.Line(idx)), got, want)
		}
	}
}

// TestHighlighterCacheDistinguishesTwoDocuments: two Documents number their
// versions independently from zero, so XMLHighlighter, whose cache lives in its
// closure, handed a second one at the same version must not answer with the
// first one's states. The cache is keyed on the pointer as well as the number
// for exactly this.
func TestHighlighterCacheDistinguishesTwoDocuments(t *testing.T) {
	inComment := docOf([][]rune{
		[]rune("<!-- open"),
		[]rune("still inside"),
	})
	plain := docOf([][]rune{
		[]rune("<a/>"),
		[]rune("<b/>"),
	})
	if inComment.Version() != plain.Version() {
		t.Fatalf("test needs two documents at the same version, got %d and %d",
			inComment.Version(), plain.Version())
	}

	hl := XMLHighlighter(&theme.Default)
	hl(inComment, 0)
	hl(inComment, 1)

	got := hl(plain, 1)
	want := XMLHighlighter(&theme.Default)(docOf(plain.Lines()), 1)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("second document's line 1 = %v, want %v — the first document's states leaked", got, want)
	}
}

// TestLineStateAtSharesOneCachePerKey: a highlighter and a host's line-oriented
// rewrite that ask LineStateAt with the same key read one cache on the
// Document, so drawing a script leaves the rewrite nothing to lex — two caches
// would cost two replays of the whole script after every edit. A different key
// gets its own cache, and an editor nobody asks never builds one.
func TestLineStateAtSharesOneCachePerKey(t *testing.T) {
	e := NewEditor(toyHighlighter(&theme.Default))
	e.SetBounds(0, 0, 60, 10)
	e.SetText("SELECT 1 /* open\nstill inside\n*/ SELECT 2")
	e.Draw(tuikittest.NewScreen(60, 10))

	steps := 0
	counting := func(line []rune, in int) int { steps++; return toyStep(line, in) }
	if got := LineStateAt(e.doc, toyKey{}, 1, 0, counting); got != 1 {
		t.Errorf("line 1 with the highlighter's key = %d, want 1 (inside the comment line 0 opens)", got)
	}
	if steps != 0 {
		t.Errorf("asking with the highlighter's key after a Draw lexed %d lines, want 0", steps)
	}

	type otherKey struct{}
	if got := LineStateAt(e.doc, otherKey{}, 1, 0, counting); got != 1 || steps == 0 {
		t.Errorf("another key: state %d after %d steps, want 1 from its own replay", got, steps)
	}

	x := NewEditor(nil)
	x.SetBounds(0, 0, 60, 10)
	x.SetText("SELECT 1 /* open\nstill inside")
	x.Draw(tuikittest.NewScreen(60, 10))
	if x.doc.lineStates != nil {
		t.Error("drawing a plain-text editor built a line-state cache")
	}
}

// TestXMLHighlighterCacheInvalidatesOnEdit mirrors the LineStateAt case for the
// built-in highlighter with its own cache, whose cross-line state is a comment
// or CDATA section.
func TestXMLHighlighterCacheInvalidatesOnEdit(t *testing.T) {
	e := NewEditor(nil)
	e.SetBounds(0, 0, 60, 10)
	e.SetText("<a><!-- open\nstill inside\n<b/>")
	hl := XMLHighlighter(&theme.Default)
	doc := e.Document()

	for idx := range 3 {
		hl(doc, idx)
	}

	e.cursorRow, e.cursorCol = 0, len(doc.Line(0))
	for _, r := range "-->" {
		e.insertRune(r)
	}

	got := hl(doc, 2)
	want := XMLHighlighter(&theme.Default)(docOf(doc.Lines()), 2)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("line 2 after closing the comment = %v, want %v — the cache served pre-edit state", got, want)
	}
}

// TestBuildVisualLinesCacheHitsAndInvalidates pins both halves of the wrap
// memo: repeated calls at the same width and version do no work, and any of
// an edit or a width change rebuilds. A stale hit renders the previous
// document's wrap segments — old text under a new cursor.
func TestBuildVisualLinesCacheHitsAndInvalidates(t *testing.T) {
	e := NewEditor(nil)
	e.SetWrapMode(true)
	e.SetGutterVisible(false)
	// The content width must be the same 10 the test asks for below, or
	// ensureCursorVisible's own buildVisualLines call rebuilds at a different
	// width and masks a stale hit rather than exposing it.
	e.SetBounds(0, 0, 10, 6)
	e.SetText("aaaa bbbb cccc dddd")

	first := e.buildVisualLines(10)
	if !e.vlCacheValid {
		t.Fatal("buildVisualLines did not mark its cache valid")
	}
	firstLen := len(first)

	// Same width, untouched document: must be the identical flattening.
	if got := e.buildVisualLines(10); !reflect.DeepEqual(got, first) {
		t.Errorf("second call at the same width = %v, want the cached %v", got, first)
	}

	// A different width must not be answered from the cache.
	narrow := e.buildVisualLines(5)
	if len(narrow) <= firstLen {
		t.Errorf("wrapping at width 5 gave %d visual rows, want more than the %d at width 10 — "+
			"the cache answered for the wrong width", len(narrow), firstLen)
	}

	// An edit must not be answered from the cache either.
	e.buildVisualLines(10)
	e.cursorRow, e.cursorCol = 0, 0
	e.Paste("eeee ffff gggg ")
	afterEdit := e.buildVisualLines(10)
	want := referenceVisualLines(e.doc.Lines(), 10)
	if !reflect.DeepEqual(afterEdit, want) {
		t.Errorf("after an edit = %v, want %v — the cache served the pre-edit document", afterEdit, want)
	}
}

// TestBuildVisualLinesCacheSurvivesAReadOnlyViewer is the case the memo was
// added for: DataGrid's full-cell viewer is read-only, so the flattening of a
// large value must be computed once and reused for every redraw. Asserted via
// the version, which is what the cache keys on and what a stray mutation
// would move.
func TestBuildVisualLinesCacheSurvivesAReadOnlyViewer(t *testing.T) {
	e := NewEditor(nil)
	e.SetWrapMode(true)
	e.SetReadOnly(true)
	e.SetGutterVisible(false)
	e.SetBounds(0, 0, 20, 8)
	e.SetText("one long logical line that wraps across a good many visual rows in a narrow box")

	e.buildVisualLines(18)
	version := e.doc.Version()

	s := tuikittest.NewScreen(20, 8)
	for range 5 {
		e.Draw(s)
	}
	if e.doc.Version() != version {
		t.Errorf("Version() moved from %d to %d across read-only Draws — "+
			"the flattening would be rebuilt on every one", version, e.doc.Version())
	}
	if !e.vlCacheValid || e.vlCacheVersion != version {
		t.Error("the wrap cache was invalidated by a Draw that changed nothing")
	}
}

// TestPrefixStatesIncrementalReplayMatchesFullReplay is the differential test
// for the resume-and-converge path in prefixStates.replay. That path trusts
// two claims: a single-line edit cannot change the state any earlier line
// begins in, and once a recomputed state matches the stored one the rest of
// the document is already correct. Both are true only if the step function is
// a pure left fold, so this hammers it with edits that open, close and nest
// block comments and checks every line against a replay that assumes nothing.
func TestPrefixStatesIncrementalReplayMatchesFullReplay(t *testing.T) {
	e := NewEditor(nil)
	e.SetBounds(0, 0, 80, 20)
	e.SetText(strings.Join([]string{
		"SELECT 1",
		"/* opened here",
		"still inside",
		"*/ SELECT 2",
		"SELECT 3 -- with /* in a line comment",
		"SELECT '/*' AS in_a_string",
		"SELECT 4",
		"/* another",
		"SELECT 5",
	}, "\n"))
	doc := e.Document()

	var cache prefixStates[int]
	// Warm it, then edit one line at a time and re-check the whole document.
	// Each edit is a single setLine, which is the case the resume path takes.
	edits := []struct {
		row  int
		text string
	}{
		{4, "SELECT 3 /* now a real comment"},     // opens one: must propagate down
		{4, "SELECT 3 -- back to a line comment"}, // closes it again
		{1, "SELECT 1b"},                          // removes the first opener
		{7, "*/ SELECT 6"},                        // a stray close
		{2, "/* reopened"},                        // opens mid-document
		{8, "SELECT 5 */"},                        // closes at the bottom
		{0, "SELECT 0 /*"},                        // line 0: dirtyFrom 0, full replay
		{2, "/* nested"},                          // inside a comment: one level deeper
		{3, "*/ still inside the outer one"},      // closes the inner level only
		{3, "*/ */ SELECT 2"},                     // closes both
	}
	for i := range doc.Len() {
		cache.at(doc, i, 0, toyStep)
	}
	for n, ed := range edits {
		e.doc.setLine(ed.row, []rune(ed.text))
		for i := range doc.Len() {
			got := cache.at(doc, i, 0, toyStep)
			want := toyFullReplay(doc.Lines(), i)
			if got != want {
				t.Fatalf("after edit %d (row %d -> %q): line %d starting comment depth = %v, want %v",
					n, ed.row, ed.text, i, got, want)
			}
		}
	}
}

// TestPrefixStatesResumeHandlesLineCountChanges: the resume path is only
// valid when the line count is unchanged. Inserting or deleting lines shifts
// every later state to a different index, so those must fall back to a full
// replay however few versions behind the cache is.
func TestPrefixStatesResumeHandlesLineCountChanges(t *testing.T) {
	e := NewEditor(nil)
	e.SetBounds(0, 0, 80, 20)
	e.SetText("SELECT 1\n/* open\ninside\n*/ SELECT 2\nSELECT 3")
	doc := e.Document()

	var cache prefixStates[int]
	for i := range doc.Len() {
		cache.at(doc, i, 0, toyStep)
	}

	// One structural mutation, so the version advances by exactly 1 — the
	// same distance a resumable setLine leaves.
	e.cursorRow, e.cursorCol = 0, 0
	e.insertNewline()

	for i := range doc.Len() {
		got := cache.at(doc, i, 0, toyStep)
		want := toyFullReplay(doc.Lines(), i)
		if got != want {
			t.Errorf("after inserting a line: line %d (%q) starts-in-comment = %v, want %v",
				i, string(doc.Line(i)), got, want)
		}
	}
}

// TestPrefixStatesIncrementalReplayAfterReplaceRange covers the resume path
// for the *other* mutation that now leaves a non-zero dirtyFrom. replaceRange
// (undo, redo) used to reset it to 0 and force a full replay; it now names its
// span's first line, which makes the resume branch reachable for a splice that
// happens to keep the line count — and only the length test in
// prefixStates.at keeps it off the ones that don't.
//
// Every case is checked against toyFullReplay over the whole document,
// so a resume that trusts a stale earlier state shows up as a wrong colour
// rather than as a passing round trip.
func TestPrefixStatesIncrementalReplayAfterReplaceRange(t *testing.T) {
	e := NewEditor(nil)
	e.SetBounds(0, 0, 80, 20)
	e.SetText(strings.Join([]string{
		"SELECT 1",
		"/* opened here",
		"still inside",
		"*/ SELECT 2",
		"SELECT 3",
		"SELECT 4",
	}, "\n"))
	doc := e.Document()

	var cache prefixStates[int]
	splices := []struct {
		name   string
		row, n int
		with   []string
	}{
		// Same line count: the resume branch, from a line that is not 0.
		{"a one-line span opens a comment", 4, 1, []string{"SELECT 3 /* now open"}},
		{"a one-line span closes it", 5, 1, []string{"SELECT 4 */"}},
		{"a two-line span, same length", 1, 2, []string{"SELECT 1b", "SELECT 1c"}},
		// Line count changes: must fall back to a full replay however few
		// versions behind the cache is.
		{"the span grows", 2, 1, []string{"/* reopened", "inside", "*/ done"}},
		{"the span shrinks", 1, 3, []string{"SELECT 2 /* left open"}},
		{"a span at line 0", 0, 1, []string{"SELECT 0 */"}},
	}
	for i := range doc.Len() {
		cache.at(doc, i, 0, toyStep)
	}
	for n, sp := range splices {
		with := make([][]rune, len(sp.with))
		for i, s := range sp.with {
			with[i] = []rune(s)
		}
		e.doc.replaceRange(sp.row, sp.n, with)
		for i := range doc.Len() {
			got := cache.at(doc, i, 0, toyStep)
			want := toyFullReplay(doc.Lines(), i)
			if got != want {
				t.Fatalf("after splice %d (%s): line %d starting comment depth = %v, want %v",
					n, sp.name, i, got, want)
			}
		}
	}
}
