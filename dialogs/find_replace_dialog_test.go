package dialogs

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/controls/editor"
	"github.com/radix29/tuikit/tuikittest"
)

// newFindTest returns a FindReplaceDialog on a screen and an editor holding
// text for it to search.
func newFindTest(t *testing.T, text string) (*FindReplaceDialog, *editor.Editor) {
	t.Helper()
	e := editor.NewEditor(nil)
	e.SetBounds(0, 0, 80, 24)
	e.SetText(text)
	return withScreen(NewFindReplaceDialog(), tuikittest.NewScreen(100, 40)), e
}

func findKey(k tcell.Key) *tcell.EventKey { return tcell.NewEventKey(k, "", tcell.ModNone) }

func TestFindDialogWithNoTargetStaysClosed(t *testing.T) {
	d, _ := newFindTest(t, "")
	d.ShowFind(nil)
	d.ShowReplace(nil)
	if d.Visible() {
		t.Fatal("the dialog opened with no editor to search")
	}
}

func TestFindDialogSeedsFromSingleLineSelection(t *testing.T) {
	d, e := newFindTest(t, "select Total from dbo.Orders\nwhere Total > 0")
	e.SelectAll() // multi-line
	d.ShowFind(e)
	if got := d.fFind.Value(); got != "" {
		t.Fatalf("Find what = %q after a multi-line selection, want it left alone", got)
	}
	d.Hide()

	// A single-line selection does seed the field.
	e.SetText("select Total from dbo.Orders")
	if err := e.SetSearch(editor.SearchOptions{Query: "Total"}); err != nil {
		t.Fatal(err)
	}
	e.FindNext(1) // selects "Total"
	d.ShowFind(e)
	if got := d.fFind.Value(); got != "Total" {
		t.Fatalf("Find what = %q, want the selected word", got)
	}
}

func TestFindDialogEnterFindsFromAField(t *testing.T) {
	d, e := newFindTest(t, "a bb a bb a")
	d.ShowFind(e)
	d.fFind.SetValue("bb")

	// Focus is on the Find what field; Enter presses Find Next.
	d.HandleKey(findKey(tcell.KeyEnter))
	if got := e.SelectedText(); got != "bb" {
		t.Fatalf("selection = %q after Enter, want the first match", got)
	}
	if msg, _ := d.Status(); !strings.Contains(msg, "Match 1 of 2") {
		t.Fatalf("status = %q, want a match-position readout", msg)
	}

	d.HandleKey(findKey(tcell.KeyEnter))
	if msg, _ := d.Status(); !strings.Contains(msg, "Match 2 of 2") {
		t.Fatalf("status = %q after a second Enter, want match 2", msg)
	}
}

func TestFindDialogTabReachesButtons(t *testing.T) {
	d, e := newFindTest(t, "x")
	d.ShowFind(e)

	// While focus is in the fields, Enter presses Find Next (button 0).
	if d.ring.BtnFocus != findBtnNext {
		t.Fatalf("btnFocus = %d with a field focused, want Find Next", d.ring.BtnFocus)
	}
	// Four fields, then the button row as one stop.
	for range 4 {
		d.HandleKey(findKey(tcell.KeyTab))
	}
	if !d.ring.OnButtons() || d.ring.BtnFocus != findBtnNext {
		t.Fatalf("after 4 Tabs: focusIdx=%d btnFocus=%d, want the row on Find Next", d.ring.FocusIdx, d.ring.BtnFocus)
	}
	// Right walks the row; Tab leaves it, wrapping to the first field.
	d.HandleKey(findKey(tcell.KeyRight))
	if d.ring.BtnFocus != findBtnPrev {
		t.Fatalf("btnFocus = %d after Right, want %d (Find Previous)", d.ring.BtnFocus, findBtnPrev)
	}
	d.HandleKey(findKey(tcell.KeyTab))
	if d.ring.FocusIdx != 0 || d.ring.BtnFocus != findBtnNext {
		t.Fatalf("after Tab off the row: focusIdx=%d btnFocus=%d, want 0 and Find Next", d.ring.FocusIdx, d.ring.BtnFocus)
	}
	// Backtab from the first field wraps onto the row; Up leaves the row to
	// the last field and keeps moving back, like Backtab.
	d.HandleKey(findKey(tcell.KeyBacktab))
	if !d.ring.OnButtons() {
		t.Fatalf("focusIdx = %d after Backtab from the first field, want the button row", d.ring.FocusIdx)
	}
	d.HandleKey(findKey(tcell.KeyUp))
	if d.ring.FocusIdx != 3 {
		t.Fatalf("focusIdx = %d after Up from the row, want 3 (the last field)", d.ring.FocusIdx)
	}
	d.HandleKey(findKey(tcell.KeyUp))
	if d.ring.FocusIdx != 2 {
		t.Fatalf("focusIdx = %d after another Up, want 2", d.ring.FocusIdx)
	}
	d.HandleKey(findKey(tcell.KeyDown))
	if d.ring.FocusIdx != 3 {
		t.Fatalf("focusIdx = %d after Down, want 3", d.ring.FocusIdx)
	}
}

// Buttons answer by position, so a host translating Strings changes nothing
// but the text: Find Previous still steps back, Close still closes.
func TestFindDialogButtonsDispatchByPositionNotText(t *testing.T) {
	saved := Strings
	t.Cleanup(func() { Strings = saved })
	Strings.FindPrevious, Strings.Close = "Précédent", "Fermer"

	d, e := newFindTest(t, "a bb a bb a")
	d.ShowFind(e)
	d.fFind.SetValue("bb")
	d.pressButton(findBtnNext)
	d.pressButton(findBtnNext)
	d.pressButton(findBtnPrev)
	if msg, _ := d.Status(); !strings.Contains(msg, "Match 1 of 2") {
		t.Fatalf("status = %q after Next, Next, Previous, want match 1", msg)
	}
	d.pressButton(findBtnClose)
	if d.Visible() {
		t.Fatal("the Close position did not close the dialog")
	}
}

func TestFindDialogEscapeKeepsTheSearch(t *testing.T) {
	d, e := newFindTest(t, "a bb a bb a")
	d.ShowFind(e)
	d.fFind.SetValue("bb")
	d.HandleKey(findKey(tcell.KeyEnter))

	d.HandleKey(findKey(tcell.KeyEscape))
	if d.Visible() {
		t.Fatal("Escape did not close the dialog")
	}
	if !e.HasSearch() {
		t.Fatal("Escape dropped the search — the host's F3 would have nothing to repeat")
	}
	// What the host's F3 does with the dialog closed, and the readout it shows.
	e.FindNext(1)
	if got := MatchStatus(e); !strings.Contains(got, "Match 2 of 2") {
		t.Fatalf("MatchStatus = %q after F3, want match 2", got)
	}
}

func TestFindDialogReplaceAllReportsCount(t *testing.T) {
	d, e := newFindTest(t, "a a\na")
	d.ShowReplace(e)
	d.fFind.SetValue("a")
	d.fReplace.SetValue("z")
	d.pressButton(replaceBtnAll)

	if got, want := e.Text(), "z z\nz"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	if msg, _ := d.Status(); !strings.Contains(msg, "Replaced 3") {
		t.Fatalf("status = %q, want a replacement count", msg)
	}
}

// Find mode has no "in selection only" option, so a search in Find mode must
// not silently inherit a ticked checkbox from a previous Replace showing — the
// option is compiled from replaceMode && cbSel.
func TestFindModeIgnoresTheSelectionOnlyOption(t *testing.T) {
	d, e := newFindTest(t, "a a")
	d.cbSel.SetChecked(true)
	d.ShowFind(e)
	d.fFind.SetValue("a")
	d.doFind(1)

	if e.SearchOpts().InSelection {
		t.Fatal("Find mode compiled an in-selection search from the hidden checkbox")
	}
	// Nor does that hidden checkbox count as a changed option, which would
	// recompile on every Find Next and never step past the first match.
	if d.optionsChanged() {
		t.Fatal("the hidden checkbox reads as a change against the compiled search")
	}
}

// Reopening Replace on a new selection scopes "In selection only" to that
// selection, though the options are the ones already compiled: unchanged options
// skip the recompile, so the dialog kept the previous selection's scope, and with
// Find Next scoped it reported no matches with the new selection on screen.
func TestReplaceReopenedOnANewSelectionRescopes(t *testing.T) {
	d, e := newFindTest(t, "foo foo\nfoo end")
	e.SelectRange(0, 0, 0, 7)
	d.ShowReplace(e)
	d.fFind.SetValue("foo")
	d.fReplace.SetValue("Z")
	d.cbSel.SetChecked(true)
	d.doFind(1)
	d.Hide()

	e.SelectRange(1, 0, 1, 7)
	d.ShowReplace(e)
	d.fFind.SetValue("foo") // ShowReplace seeded it from the selection
	d.doReplaceAll()
	if got, want := e.Text(), "foo foo\nZ end"; got != want {
		t.Fatalf("text = %q, want %q (status %q)", got, want, d.status)
	}

	// Reopened on the current match (F3 left it selected), the search carries on.
	e.SelectRange(0, 0, 0, 7)
	d.ShowReplace(e)
	d.fFind.SetValue("foo")
	d.doFind(1)
	d.Hide()
	d.ShowReplace(e)
	d.fFind.SetValue("foo")
	if d.rescope {
		t.Fatal("reopening on the current match asked for a fresh scope")
	}
}

func TestFindDialogReportsAnInvalidRegex(t *testing.T) {
	d, e := newFindTest(t, "abc")
	d.ShowFind(e)
	d.fFind.SetValue("(unclosed")
	d.cbRegex.SetChecked(true)
	d.doFind(1)

	if msg, isErr := d.Status(); !strings.HasPrefix(msg, "Invalid regex") || !isErr {
		t.Fatalf("status = %q (err=%v), want an invalid-regex error", msg, isErr)
	}
	if e.HasSearch() {
		t.Fatal("an invalid pattern left a search active")
	}
}

func TestFindDialogEmptyTermIsRefused(t *testing.T) {
	d, e := newFindTest(t, "abc")
	d.ShowFind(e)
	d.fFind.SetValue("")
	d.doFind(1)

	if e.HasSearch() {
		t.Fatal("an empty term started a search")
	}
	if msg, isErr := d.Status(); !isErr {
		t.Fatalf("status = %q, want a notice that nothing was entered", msg)
	}
}

// A host compiling its own search (Ctrl+F3) has to leave the dialog describing
// that search and no other. A partial sync — Find what / whole word / regexp
// only — left Match case (and, after a Replace showing, "in selection only")
// saying whatever the user last chose, so the dialog claimed a search the
// editor was not running, and optionsChanged saw a difference that wasn't real.
func TestAdoptSearchSyncsEveryField(t *testing.T) {
	d, e := newFindTest(t, "Total from dbo.Orders Total")

	// Leave every option set the way a previous search would have.
	d.replaceMode = true
	d.cbCase.SetChecked(true)
	d.cbRegex.SetChecked(true)
	d.cbSel.SetChecked(true)
	d.fReplace.SetValue("Amount")

	// What a host's Ctrl+F3 does: compile, keeping the dialog's replacement.
	opts := editor.SearchOptions{Query: "Total", Replace: d.Fields().Replace, WholeWord: true}
	if err := e.SetSearch(opts); err != nil {
		t.Fatal(err)
	}
	d.AdoptSearch(opts)
	e.FindNext(1)

	if got := d.Fields(); got != e.SearchOpts() {
		t.Fatalf("dialog fields = %+v, editor search = %+v", got, e.SearchOpts())
	}
	if got := d.Fields().Replace; got != "Amount" {
		t.Errorf("Replace with = %q, want the user's text kept", got)
	}
	// The sequence that matters: Ctrl+F3, then open the dialog, then Find Next.
	// optionsChanged short-circuits until a showing captures a target, so
	// opening it is what makes the comparison real — and it must find nothing
	// to recompile, or Find Next restarts from the cursor instead of stepping on.
	d.ShowFind(e)
	if d.optionsChanged() {
		t.Error("optionsChanged after AdoptSearch: a Find Next would recompile and restart from the cursor")
	}
}

func TestMatchStatusWithNoCurrentMatchIsACount(t *testing.T) {
	_, e := newFindTest(t, "a a a")
	if err := e.SetSearch(editor.SearchOptions{Query: "a"}); err != nil {
		t.Fatal(err)
	}
	if got := MatchStatus(e); got != "3 match(es)" {
		t.Fatalf("MatchStatus = %q before any FindNext, want a bare count", got)
	}
	e.FindNext(1)
	if got := MatchStatus(e); got != "Match 1 of 3 — line 1, col 1" {
		t.Fatalf("MatchStatus = %q, want the first match's position", got)
	}
}

// A latch must not survive into the next showing: a dialog dismissed mid-drag
// would reopen routing every click to that field.
func TestFindDialogShowClearsTheDragLatch(t *testing.T) {
	d, e := newFindTest(t, "select 1")
	d.ShowFind(e)
	d.layout()
	// Arm the latch the way a user does — a press inside the field — rather than
	// by assignment, so the test also pins that a press arms one at all.
	ix, y := d.fFind.InputX(), d.fFind.Bounds().Y
	d.HandleMouse(tcell.NewEventMouse(ix+1, y, tcell.Button1, tcell.ModNone))
	if d.ring.Drag.Field() == nil {
		t.Fatal("the press did not arm a latch — test premise is wrong")
	}

	d.ShowFind(e)
	if d.ring.Drag.Field() != nil {
		t.Error("Show left a drag latch armed from the previous showing")
	}
}

// A selection drag begun in Find what keeps extending after the pointer leaves
// the field — over the Replace field and the options below, neither of which
// may take the gesture — and ends at a release outside the dialog.
func TestFindDialogDragOutOfAFieldKeepsExtending(t *testing.T) {
	d, e := newFindTest(t, "select 1")
	d.ShowReplace(e) // two fields, so there is somewhere below to drag to
	d.layout()
	d.fFind.SetValue("abcdefgh")

	ix, y := d.fFind.InputX(), d.fFind.Bounds().Y
	if !d.HandleMouse(tcell.NewEventMouse(ix+1, y, tcell.Button1, tcell.ModNone)) {
		t.Fatal("the press inside the field was refused — test premise is wrong")
	}
	if d.fFind.HitTest(ix+6, y+3) {
		t.Fatal("the drag point is still inside the field — test premise is wrong")
	}
	d.HandleMouse(tcell.NewEventMouse(ix+6, y+3, tcell.Button1, tcell.ModNone))
	if got := d.fFind.SelectedText(); got != "abcde" {
		t.Errorf("SelectedText() = %q, want %q — the drag stopped at the box edge", got, "abcde")
	}
	if d.ring.FocusIdx != 0 {
		t.Errorf("focusIdx = %d, want 0 — a widget below stole a gesture fFind owned", d.ring.FocusIdx)
	}

	d.HandleMouse(tcell.NewEventMouse(0, 0, tcell.ButtonNone, tcell.ModNone))
	if d.ring.Drag.Field() != nil {
		t.Fatal("a release outside the dialog left the gesture latched")
	}
	if d.fFind.HandleMouse(tcell.NewEventMouse(ix+6, y+3, tcell.Button1, tcell.ModNone)) {
		t.Error("the field is still latched — its next off-rect press was accepted")
	}
}

// The Find field has focus on Show, so Ctrl+X cuts its text, not the editor's
// selection behind the dialog.
func TestFindDialogClipboardTargetIsTheFocusedField(t *testing.T) {
	d, e := newFindTest(t, "select 1")
	d.ShowFind(e)
	if got := d.FocusedClipboardTarget(); got != d.fFind {
		t.Fatalf("FocusedClipboardTarget = %v, want the Find what field", got)
	}
	d.HandleKey(findKey(tcell.KeyTab)) // onto Match case
	if got := d.FocusedClipboardTarget(); got != nil {
		t.Fatalf("FocusedClipboardTarget = %v on a checkbox, want nil", got)
	}
}

func TestFindDialogReplaceAfterEditingReplacementHitsSelectedMatch(t *testing.T) {
	d, e := newFindTest(t, "aa aa aa")
	d.ShowReplace(e)
	d.fFind.SetValue("aa")
	d.fReplace.SetValue("X")
	d.doFind(1) // selects the first "aa"
	d.fReplace.SetValue("Y")
	d.doReplace()
	if got := e.Text(); got != "Y aa aa" {
		t.Fatalf("text = %q, want the selected match replaced with the new text", got)
	}
}

func TestFindDialogReplaceAllUsesEditedReplacement(t *testing.T) {
	d, e := newFindTest(t, "aa aa")
	d.ShowReplace(e)
	d.fFind.SetValue("aa")
	d.fReplace.SetValue("X")
	d.doFind(1)
	d.fReplace.SetValue("Z")
	d.doReplaceAll()
	if got := e.Text(); got != "Z Z" {
		t.Fatalf("text = %q, want %q", got, "Z Z")
	}
}

func TestFindDialogQueryChangeStillRecompiles(t *testing.T) {
	d, e := newFindTest(t, "aa bb")
	d.ShowReplace(e)
	d.fFind.SetValue("aa")
	d.doFind(1)
	d.fFind.SetValue("bb")
	if !d.optionsChanged() {
		t.Fatal("a changed query must recompile")
	}
	_ = e
}
