package dialogs

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/controls/editor"
	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/theme"
	"github.com/radix29/tuikit/widgets"
)

// ---------------------------------------------------------------------------
// FindReplaceDialog — Find and Replace over an editor.Editor
// ---------------------------------------------------------------------------

// FindReplaceDialog is a Find and Replace dialog over an editor.Editor's search
// engine (SetSearch, FindNext, ReplaceCurrent, ReplaceAll). One dialog serves
// both modes: Find hides the Replace field, the "in selection only" option and
// the two Replace buttons; Replace shows them.
//
// Being modal it covers part of the editor, so the status line reports "Match 2
// of 7 — line 143, col 12" (MatchStatus) rather than relying on the hit being
// visible. Repeat searching happens with the dialog closed: the host binds F3 /
// Shift+F3 to editor.FindNext and reports MatchStatus itself, since the search,
// its options and the highlighting all live on the editor and survive closing
// the dialog. Ctrl+F3-style searches the host compiles on its own go through
// AdoptSearch, so the fields keep describing what the editor runs.
//
// The dialog knows nothing of where the editor lives: the host resolves the
// target and passes it to ShowFind/ShowReplace.
type FindReplaceDialog struct {
	ModalDialog

	fFind    *widgets.InputField
	fReplace *widgets.InputField
	cbCase   *widgets.CheckBox
	cbWord   *widgets.CheckBox
	cbRegex  *widgets.CheckBox
	cbSel    *widgets.CheckBox

	// target is the editor this showing searches, captured at Show time. The dialog
	// is modal, so the host's active editor can't change underneath it.
	target *editor.Editor

	replaceMode bool

	// rescope makes the next search compile afresh even with the options
	// unchanged: set when Replace opens on a selection that is not the current
	// match, i.e. a new selection for "In selection only" to scope to. Without it
	// a reopened dialog kept the scope of the selection it was last compiled
	// against (emptied, if an edit since touched its first or last row), and Find
	// Next reported no matches with the new selection on screen.
	rescope bool

	// ring is the focus ring over fields() and buttons(): its default button
	// is findBtnNext, so Enter from a field finds next.
	ring FieldRing

	// status is the readout under the fields: match position, replacement count, or
	// a regexp compile error. statusErr picks the colour.
	status    string
	statusErr bool

	// statusY is the row status is drawn on, computed by layout and read back by
	// Draw once the widgets above it are placed.
	statusY int
}

// Button positions. Index 0 is Find Next in both modes, and is what Enter
// presses while focus is still in the fields; the rest differ by mode.
const (
	findBtnNext = 0

	findBtnPrev  = 1 // Find mode
	findBtnClose = 2 // Find mode

	replaceBtnReplace = 1 // Replace mode
	replaceBtnAll     = 2 // Replace mode
	replaceBtnClose   = 3 // Replace mode
)

// findDialogMinW is wide enough for Replace mode's four-button row in English,
// the widest thing drawn; narrower, that row runs over the left border. Longer
// (translated) labels widen the dialog, see width.
const findDialogMinW = 62

// NewFindReplaceDialog builds the dialog. Fields and options persist across
// showings, so reopening offers the previous search back. Labels are read from
// Strings now.
func NewFindReplaceDialog() *FindReplaceDialog {
	d := &FindReplaceDialog{}
	d.InitModal(Strings.FindTitle, findDialogMinW, 13)
	// The two field labels are padded to one width so both inputs start in the
	// same column.
	lw := max(core.DisplayWidth(Strings.FindWhat), core.DisplayWidth(Strings.ReplaceWith))
	d.fFind = widgets.NewInputField(core.PadRight(Strings.FindWhat, lw), 34, false)
	d.fReplace = widgets.NewInputField(core.PadRight(Strings.ReplaceWith, lw), 34, false)
	d.cbCase = widgets.NewCheckBox(Strings.MatchCase)
	d.cbWord = widgets.NewCheckBox(Strings.MatchWholeWord)
	d.cbRegex = widgets.NewCheckBox(Strings.RegularExpression)
	// Find Next and Replace stay inside the selection too (editor's InSelection),
	// not only Replace All.
	d.cbSel = widgets.NewCheckBox(Strings.InSelectionOnly)
	d.ring = FieldRing{
		Fields:     d.fields,
		Buttons:    d.buttons,
		DefaultBtn: findBtnNext,
		Press:      d.pressButton,
		Dismiss:    d.Hide,
	}
	return d
}

// ShowFind opens the dialog in Find mode against target. ShowReplace opens it
// with the Replace field, the "in selection only" option and the Replace
// buttons shown. A nil target leaves the dialog closed.
func (d *FindReplaceDialog) ShowFind(target *editor.Editor)    { d.show(target, false) }
func (d *FindReplaceDialog) ShowReplace(target *editor.Editor) { d.show(target, true) }

func (d *FindReplaceDialog) show(target *editor.Editor, replace bool) {
	if target == nil {
		return
	}
	d.target = target
	d.replaceMode = replace
	if replace {
		d.SetTitle(Strings.ReplaceTitle)
	} else {
		d.SetTitle(Strings.FindTitle)
	}
	d.SetSize(d.width(), d.height())
	d.status, d.statusErr = "", false

	// Seed from a single-line selection, as every other editor's Find does. A
	// multi-line selection is left alone: that is what "in selection only" is for.
	if d.target.HasSelection() {
		if sel := d.target.SelectedText(); sel != "" && !strings.Contains(sel, "\n") {
			d.fFind.SetValue(sel)
		}
	}
	if cur, _ := d.target.MatchPosition(); replace && cur == 0 && d.target.HasSelection() {
		d.rescope = true
	}
	d.ring.Reset()
	d.Show()
}

// AdoptSearch points the dialog's fields at opts, so what it shows is what the
// editor searches for: a host that compiles a search itself (a find-word-at-
// cursor key) calls it after editor.SetSearch. Every field of opts is written,
// including zero ones: a partial sync leaves the next Find Next seeing a false
// difference, recompiling, and restarting from the cursor.
func (d *FindReplaceDialog) AdoptSearch(opts editor.SearchOptions) {
	d.fFind.SetValue(opts.Query)
	d.fReplace.SetValue(opts.Replace)
	d.cbCase.SetChecked(opts.MatchCase)
	d.cbWord.SetChecked(opts.WholeWord)
	d.cbRegex.SetChecked(opts.Regexp)
	d.cbSel.SetChecked(opts.InSelection)
}

// Fields returns what the dialog's fields and options hold, as SearchOptions.
// InSelection is the checkbox as ticked, whatever the mode; a Find-mode search
// compiles it as false. A host compiling its own search reads Replace from here
// so the user's typed replacement survives.
func (d *FindReplaceDialog) Fields() editor.SearchOptions {
	return editor.SearchOptions{
		Query:       d.fFind.Value(),
		Replace:     d.fReplace.Value(),
		MatchCase:   d.cbCase.Checked(),
		WholeWord:   d.cbWord.Checked(),
		Regexp:      d.cbRegex.Checked(),
		InSelection: d.cbSel.Checked(),
	}
}

// Status returns the line under the fields and whether it reports an error.
func (d *FindReplaceDialog) Status() (msg string, isErr bool) { return d.status, d.statusErr }

// MatchStatus is the readout for e's current match — "Match 2 of 7 — line 143,
// col 12", or a bare count with none current. The dialog's status line and a
// host's repeat-search status (F3 with the dialog closed) both use it, so the
// two agree.
func MatchStatus(e *editor.Editor) string {
	i, n := e.MatchPosition()
	line, col, ok := e.CurrentMatchPos()
	if !ok {
		return fmt.Sprintf(Strings.MatchCount, n)
	}
	return fmt.Sprintf(Strings.MatchAt, i, n, line, col)
}

// width is the dialog's width for the current mode: the floor, or the button
// row plus border and margins when translated labels make it wider.
func (d *FindReplaceDialog) width() int {
	return max(findDialogMinW, d.ButtonRowWidth(d.buttons())+6)
}

// height is the dialog's total height for the current mode; Replace adds the
// Replace field and the "in selection only" option.
func (d *FindReplaceDialog) height() int {
	if d.replaceMode {
		return 15
	}
	return 13
}

// buttons returns this mode's button row labels (positions: findBtn*,
// replaceBtn*).
func (d *FindReplaceDialog) buttons() []string {
	if d.replaceMode {
		return []string{Strings.FindNext, Strings.ReplaceButton, Strings.ReplaceAll, Strings.Close}
	}
	return []string{Strings.FindNext, Strings.FindPrevious, Strings.Close}
}

// fields returns the focusable widgets, in Tab order, for the current mode.
func (d *FindReplaceDialog) fields() []core.Focusable {
	if d.replaceMode {
		return []core.Focusable{d.fFind, d.fReplace, d.cbCase, d.cbWord, d.cbRegex, d.cbSel}
	}
	return []core.Focusable{d.fFind, d.cbCase, d.cbWord, d.cbRegex}
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

// applyOptions compiles the current field values onto the target editor. Reports
// false, with the status line set, when the pattern is empty or the regular
// expression doesn't compile, so callers can bail out.
func (d *FindReplaceDialog) applyOptions() bool {
	if d.target == nil {
		return false
	}
	opts := d.compiled()
	if opts.Query == "" {
		d.setStatus(Strings.EnterSearchTerm, true)
		return false
	}
	err := d.target.SetSearch(opts)
	d.rescope = false
	if err != nil {
		// regexp's message is prefixed with "error parsing regexp: ", which costs a
		// third of the status width and says nothing the label doesn't.
		msg := strings.TrimPrefix(err.Error(), "error parsing regexp: ")
		d.setStatus(fmt.Sprintf(Strings.InvalidRegex, msg), true)
		return false
	}
	return true
}

// compiled is the search the fields describe in the current mode: Fields with
// InSelection dropped in Find mode, which has no such option on screen — a
// checkbox ticked in an earlier Replace showing must not scope a Find.
func (d *FindReplaceDialog) compiled() editor.SearchOptions {
	o := d.Fields()
	o.InSelection = d.replaceMode && o.InSelection
	return o
}

// optionsChanged reports whether the target's compiled search no longer matches
// the fields (or rescope asks for a fresh compile), so a Find Next that changed
// nothing doesn't recompile and throw the current match away.
func (d *FindReplaceDialog) optionsChanged() bool {
	if d.target == nil || !d.target.HasSearch() || d.rescope {
		return true
	}
	a, b := d.target.SearchOpts(), d.compiled()
	a.Replace, b.Replace = "", ""
	return a != b
}

// ensureSearch compiles the fields onto the editor only when they differ from
// the active search: recompiling resets the current-match position, so Find
// Next would restart from the cursor instead of stepping on.
func (d *FindReplaceDialog) ensureSearch() bool {
	if !d.optionsChanged() {
		// Replace is not part of what matches: pushing it alone keeps the
		// current match, so Replace after editing "Replace with" replaces the
		// selected hit instead of stepping past it.
		d.target.SetReplacement(d.Fields().Replace)
		return true
	}
	return d.applyOptions()
}

func (d *FindReplaceDialog) doFind(dir int) {
	if !d.ensureSearch() {
		return
	}
	if !d.target.FindNext(dir) {
		d.setStatus(fmt.Sprintf(Strings.NoMatches, d.fFind.Value()), true)
		return
	}
	d.setStatus(MatchStatus(d.target), false)
}

func (d *FindReplaceDialog) doReplace() {
	if !d.ensureSearch() {
		return
	}
	if d.target.ReplaceCurrent() {
		d.setStatus(MatchStatus(d.target), false)
		return
	}
	// Nothing was replaced because no match was current: find one (the first
	// Replace press finds, the second replaces).
	d.doFind(1)
}

func (d *FindReplaceDialog) doReplaceAll() {
	if !d.ensureSearch() {
		return
	}
	n := d.target.ReplaceAll()
	switch {
	case n > 0:
		d.setStatus(fmt.Sprintf(Strings.Replaced, n), false)
	case d.cbSel.Checked():
		d.setStatus(Strings.NoMatchesInSelection, true)
	default:
		d.setStatus(fmt.Sprintf(Strings.NoMatches, d.fFind.Value()), true)
	}
}

func (d *FindReplaceDialog) setStatus(msg string, isErr bool) {
	d.status, d.statusErr = msg, isErr
}

// ---------------------------------------------------------------------------
// Drawing
// ---------------------------------------------------------------------------

func (d *FindReplaceDialog) Draw(s tcell.Screen) {
	if !d.Visible() {
		return
	}
	d.DrawBase(s)
	d.layout()

	d.fFind.Draw(s)
	if d.replaceMode {
		d.fReplace.Draw(s)
		d.cbSel.Draw(s)
	}
	d.cbCase.Draw(s)
	d.cbWord.Draw(s)
	d.cbRegex.Draw(s)

	st := theme.StyleDialogDim()
	if d.statusErr {
		st = theme.StyleDialogWarning()
	}
	inner := d.InnerRect()
	core.DrawTextClipped(s, inner.X+1, d.statusY, inner.W-2, st, d.status)

	d.DrawSeparator(s)
	d.DrawButtons(s, d.buttons(), d.ring.BtnFocus)
}

// layout positions the fields for the current mode and records statusY for Draw,
// which needs it after the widgets have been placed.
func (d *FindReplaceDialog) layout() {
	inner := d.InnerRect()
	lx := inner.X + 1
	row := inner.Y + 1

	d.fFind.SetBounds(lx, row)
	row++
	if d.replaceMode {
		d.fReplace.SetBounds(lx, row)
		row++
	}
	row++ // blank row between the fields and the options
	d.cbCase.SetBounds(lx, row)
	row++
	d.cbWord.SetBounds(lx, row)
	row++
	d.cbRegex.SetBounds(lx, row)
	row++
	if d.replaceMode {
		d.cbSel.SetBounds(lx, row)
		row++
	}
	d.statusY = row + 1
}

// ---------------------------------------------------------------------------
// Input
// ---------------------------------------------------------------------------

// HandleKey routes keyboard events: F3 / Shift+F3 step through matches without
// closing the dialog, then the focus ring (FieldRing.Key). Escape closes the dialog but leaves the
// search active on the editor, so the host's F3 keeps working.
func (d *FindReplaceDialog) HandleKey(ev *tcell.EventKey) bool {
	if !d.Visible() {
		return false
	}
	if ev.Key() == tcell.KeyF3 {
		dir := 1
		if ev.Modifiers()&tcell.ModShift != 0 {
			dir = -1
		}
		d.doFind(dir)
		return true
	}
	return d.ring.Key(ev)
}

// pressButton runs the action of buttons()[i] for the current mode, by
// position (Strings may be translated).
func (d *FindReplaceDialog) pressButton(i int) {
	if i == findBtnNext {
		d.doFind(1)
		return
	}
	if d.replaceMode {
		switch i {
		case replaceBtnReplace:
			d.doReplace()
		case replaceBtnAll:
			d.doReplaceAll()
		case replaceBtnClose:
			d.Hide()
		}
		return
	}
	switch i {
	case findBtnPrev:
		d.doFind(-1)
	case findBtnClose:
		d.Hide()
	}
}

// HandleMouse routes mouse events (FieldRing.Mouse); ModalDialog blocks clicks
// outside the dialog's bounds.
func (d *FindReplaceDialog) HandleMouse(ev *tcell.EventMouse) bool {
	if !d.Visible() {
		return false
	}
	return d.ring.Mouse(ev, &d.ModalDialog)
}

// FocusClipboardTargetAt implements core.ClipboardFocuser: a right-click on the
// Find or Replace field focuses it.
func (d *FindReplaceDialog) FocusClipboardTargetAt(x, y int) bool {
	return d.ring.FocusClipboardTargetAt(x, y)
}

// FocusedClipboardTarget implements core.ClipboardHost: whichever of the Find
// what / Replace with fields has focus, nil on the checkboxes. Without it
// Ctrl+X cuts the *editor's* selection behind the dialog.
func (d *FindReplaceDialog) FocusedClipboardTarget() core.ClipboardTarget {
	return d.ring.FocusedClipboardTarget()
}
