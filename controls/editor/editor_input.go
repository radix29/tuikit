package editor

import (
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/core"
)

// ---------------------------------------------------------------------------
// Keyboard and mouse input for Editor (non-wrap-mode; see editor_wrap.go for
// wrap-mode mouse handling)
// ---------------------------------------------------------------------------

// readOnlySafeKey reports whether ev is a movement or selection key
// SetReadOnly(true) lets through (traitReadOnly in editorKeys). It inspects
// Key() only: Shift and Ctrl don't change it, so Shift+Left passes while every
// mutating key is rejected.
func readOnlySafeKey(ev *tcell.EventKey) bool {
	return editorKeys[ev.Key()].traits&traitReadOnly != 0
}

// HandleKey handles keyboard input. It reports true only when the key changed
// something (text, cursor, selection, scroll, the completion popup) or fired a
// callback: Up on the first line, Down on the last, Backspace at the start or
// Ctrl+Z with nothing to undo answer false, so a host (propsheet.Form) can use
// the key for its own navigation. A key the editor doesn't bind is false too.
func (e *Editor) HandleKey(ev *tcell.EventKey) bool {
	// Ctrl+letter arrives as KeyRune+ModCtrl under advanced key reporting and
	// for Ctrl+Shift chords; the key table binds the KeyCtrlA..KeyCtrlZ form.
	ev = core.NormalizeKey(ev)
	// An open completion popup gets first refusal of list-navigation, commit and
	// dismiss keys; everything else falls through, and updateCompletion at the end
	// keeps it in sync.
	if e.completionOpen && e.handleCompletionKey(ev) {
		return true
	}
	if e.readOnly && !readOnlySafeKey(ev) {
		return false
	}
	// Detected, never predicted: comparing what the key left behind stays right
	// as the key handling below changes.
	before := e.keyState()
	bound, fired := e.applyKey(ev)
	return bound && (fired || e.keyState() != before)
}

// editorKeyState is what HandleKey compares around a key to tell whether it was
// acted on: everything a key can change that the user can see.
type editorKeyState struct {
	version                          uint64
	cursorRow, cursorCol             int
	scrollRow, scrollCol             int
	selStartRow, selStartCol         int
	selEndRow, selEndCol             int
	hasSel, selBlock, completionOpen bool
	completionSel, completionCount   int
}

func (e *Editor) keyState() editorKeyState {
	st := editorKeyState{
		version:   e.doc.Version(),
		cursorRow: e.cursorRow, cursorCol: e.cursorCol,
		scrollRow: e.scrollRow, scrollCol: e.scrollCol,
		completionOpen: e.completionOpen,
	}
	if e.completionOpen {
		st.completionSel, st.completionCount = e.completionSel, len(e.completionItems)
	}
	if e.HasSelection() {
		st.hasSel, st.selBlock = true, e.selBlock
		st.selStartRow, st.selStartCol, st.selEndRow, st.selEndCol = e.selectionBounds()
	}
	return st
}

// keyTrait is what applyKey's shared prologue and epilogue need to know about a
// key, beyond what its binding does.
type keyTrait uint8

const (
	// traitArrow keys extend a rectangular block selection under Alt+Shift
	// (unwrapped only).
	traitArrow keyTrait = 1 << iota
	// traitMotion keys extend a linear selection under Shift.
	traitMotion
	// traitReadOnly keys pass SetReadOnly(true): movement and selection only.
	traitReadOnly
	// traitGoalColumn keys keep desiredCol (vertical movement); every other key
	// re-anchors it where the cursor ended.
	traitGoalColumn
	// traitNotTyped keys never count as a typed character, whatever Str carries,
	// so they never open the completion popup from closed.
	traitNotTyped
)

// keyCtx is what applyKey works out about a key once, before running its
// binding.
type keyCtx struct {
	ev               *tcell.EventKey
	ctrl, shift, alt bool
	// hadSelection is HasSelection() before the key: the extend-selection
	// prologue can arm an anchor, which must not read as a selection to replace.
	hadSelection bool
	// moveLine is Move Line (Ctrl+Shift+Up/Down), carved out of Shift's
	// "extend selection".
	moveLine bool
}

// keyOutcome is what a binding tells applyKey's shared epilogue.
type keyOutcome int

const (
	// keyActed: the selection survives only while Shift (or Alt+Shift) extends it.
	keyActed keyOutcome = iota
	// keyKeptSelection: the binding managed the selection itself (block edits,
	// line commands, Select All).
	keyKeptSelection
	// keyFired: the key went to a callback (Ctrl+Space), which changes nothing
	// HandleKey can compare; the selection is kept.
	keyFired
	// keyUnbound: no binding. The selection is dropped as for any key, and nothing
	// else runs.
	keyUnbound
	// keyDeclined: a binding switched off (Ctrl+/ with no comment prefix set).
	// Nothing changes at all.
	keyDeclined
)

// keyBinding is one row of editorKeys.
type keyBinding struct {
	run    func(e *Editor, k keyCtx) keyOutcome
	traits keyTrait
}

// editorKeys binds every non-rune key the editor acts on. A key absent here
// (KeyRune, and any other key) goes to keyText: typing, Ctrl+/ and Ctrl+Space
// arrive as runes on most terminals.
var editorKeys = map[tcell.Key]keyBinding{
	tcell.KeyUp: {func(e *Editor, k keyCtx) keyOutcome { return e.keyVertical(k, -1) },
		traitArrow | traitMotion | traitReadOnly | traitGoalColumn},
	tcell.KeyDown: {func(e *Editor, k keyCtx) keyOutcome { return e.keyVertical(k, 1) },
		traitArrow | traitMotion | traitReadOnly | traitGoalColumn},
	tcell.KeyLeft:  {(*Editor).keyLeft, traitArrow | traitMotion | traitReadOnly},
	tcell.KeyRight: {(*Editor).keyRight, traitArrow | traitMotion | traitReadOnly},
	tcell.KeyHome:  {(*Editor).keyHome, traitMotion | traitReadOnly},
	tcell.KeyEnd:   {(*Editor).keyEnd, traitMotion | traitReadOnly},
	tcell.KeyPgUp:  {(*Editor).keyPgUp, traitMotion | traitReadOnly | traitGoalColumn},
	tcell.KeyPgDn:  {(*Editor).keyPgDn, traitMotion | traitReadOnly | traitGoalColumn},
	tcell.KeyCtrlA: {func(e *Editor, _ keyCtx) keyOutcome { e.SelectAll(); return keyKeptSelection },
		traitReadOnly},

	tcell.KeyEnter:      {(*Editor).keyEnter, traitNotTyped},
	tcell.KeyBackspace:  {(*Editor).keyBackspace, traitNotTyped},
	tcell.KeyBackspace2: {(*Editor).keyBackspace, traitNotTyped},
	tcell.KeyDelete:     {(*Editor).keyDelete, traitNotTyped},
	tcell.KeyTab:        {(*Editor).keyTab, traitNotTyped},
	// Some terminals report Shift+Tab as Backtab rather than KeyTab+ModShift.
	// Backtab always implies Shift.
	tcell.KeyBacktab: {func(e *Editor, _ keyCtx) keyOutcome { e.DedentLines(); return keyKeptSelection }, 0},

	tcell.KeyCtrlZ: {func(e *Editor, _ keyCtx) keyOutcome { e.undo(); return keyActed }, traitNotTyped},
	tcell.KeyCtrlY: {func(e *Editor, _ keyCtx) keyOutcome { e.redo(); return keyActed }, traitNotTyped},
	tcell.KeyCtrlD: {func(e *Editor, _ keyCtx) keyOutcome { e.DuplicateLines(); return keyActed }, 0},
	tcell.KeyCtrlL: {func(e *Editor, _ keyCtx) keyOutcome { e.DeleteLines(); return keyActed }, 0},
	tcell.KeyCtrlU: {(*Editor).keyCase, 0},
	// Defensive: tcell v3 doesn't surface the 0x1F byte as KeyUS (see keyText),
	// but a protocol that did would still toggle comments.
	tcell.KeyUS: {func(e *Editor, _ keyCtx) keyOutcome { return e.keyToggleComment() }, 0},
}

// keyBindingFor is ev's row in editorKeys, or keyText for a key absent there.
func keyBindingFor(ev *tcell.EventKey) keyBinding {
	if b, ok := editorKeys[ev.Key()]; ok {
		return b
	}
	return keyBinding{run: (*Editor).keyText}
}

// applyKey acts on a key HandleKey let through. bound is false for a key the
// editor has no binding for; fired is true when it handed the key to a callback
// (Ctrl+Space), which changes nothing HandleKey can compare.
func (e *Editor) applyKey(ev *tcell.EventKey) (bound, fired bool) {
	mods := ev.Modifiers()
	k := keyCtx{
		ev:           ev,
		ctrl:         mods&tcell.ModCtrl != 0,
		shift:        mods&tcell.ModShift != 0,
		alt:          mods&tcell.ModAlt != 0,
		hadSelection: e.HasSelection(),
	}
	b := keyBindingFor(ev)

	// typedChar marks a plainly typed character, the only key that can start a
	// completion session from closed (subject to canAutoOpenCompletion's word-start
	// gate). Everything else only re-syncs an already-open popup.
	typedChar := b.traits&traitNotTyped == 0 && core.EvRune(ev) != 0 && !k.ctrl && !k.alt

	// Move Line (Ctrl+Shift+Up/Down) and rectangular block selection
	// (Alt+Shift+Arrow) ride on movement keys plus Shift, so they are carved out of
	// the plain "extend selection" combo below.
	k.moveLine = (ev.Key() == tcell.KeyUp || ev.Key() == tcell.KeyDown) && k.ctrl && k.shift && !k.alt
	blockCombo := b.traits&traitArrow != 0 && k.alt && k.shift && !e.wrapMode
	linearExtendCombo := b.traits&traitMotion != 0 && k.shift && !k.alt && !k.moveLine

	extending := blockCombo || linearExtendCombo
	if extending && !e.selecting {
		e.selecting = true
		e.selBlock = blockCombo
		e.selAnchorRow, e.selAnchorCol = e.cursorRow, e.cursorCol
	}

	out := b.run(e, k)
	if out == keyDeclined {
		return false, false
	}
	if (out == keyActed || out == keyUnbound) && !extending {
		e.selecting = false
		e.selBlock = false
	}
	if out == keyUnbound {
		return false, false
	}
	e.clampCursor()
	if b.traits&traitGoalColumn == 0 {
		e.desiredCol = e.cursorDisplayCol()
	}
	e.ensureCursorVisible()
	// A typed character may open the popup from closed only when
	// canAutoOpenCompletion allows it: the host's SetCompletionTrigger, else a word
	// starting with a letter or '_' ending at the cursor. While open, every key
	// reaching here re-syncs it.
	if e.completionOpen || (typedChar && e.canAutoOpenCompletion()) {
		e.updateCompletion()
	}
	return true, out == keyFired
}

// ---------------------------------------------------------------------------
// Key bindings: movement
// ---------------------------------------------------------------------------

// keyVertical is Up (dir -1) and Down (dir 1), and Move Line under Ctrl+Shift.
func (e *Editor) keyVertical(k keyCtx, dir int) keyOutcome {
	switch {
	case k.moveLine:
		if dir < 0 {
			e.MoveLinesUp()
		} else {
			e.MoveLinesDown()
		}
		return keyKeptSelection
	case e.wrapMode:
		e.moveVisualRows(dir)
	case dir < 0 && e.cursorRow > 0, dir > 0 && e.cursorRow < e.doc.Len()-1:
		e.cursorRow += dir
		e.cursorCol = e.colForDesired()
	}
	return keyActed
}

func (e *Editor) keyLeft(k keyCtx) keyOutcome {
	if k.ctrl {
		if e.cursorCol > 0 {
			e.cursorCol = core.WordBoundaryLeft(e.doc.Line(e.cursorRow), e.cursorCol)
		} else if e.cursorRow > 0 && !e.selBlock {
			// Column selection never crosses lines via Left/Right; only Up/Down changes a
			// block's row range. Applies to the word-jump above as well as the plain move.
			e.cursorRow--
			e.cursorCol = len(e.doc.Line(e.cursorRow))
		}
	} else if e.cursorCol > 0 {
		e.cursorCol = core.PrevGrapheme(e.doc.Line(e.cursorRow), e.cursorCol)
	} else if e.cursorRow > 0 && !e.selBlock {
		e.cursorRow--
		e.cursorCol = len(e.doc.Line(e.cursorRow))
	}
	return keyActed
}

func (e *Editor) keyRight(k keyCtx) keyOutcome {
	if k.ctrl {
		if e.cursorRow < e.doc.Len() && e.cursorCol < len(e.doc.Line(e.cursorRow)) {
			e.cursorCol = core.WordBoundaryRight(e.doc.Line(e.cursorRow), e.cursorCol)
		} else if e.cursorRow < e.doc.Len()-1 && !e.selBlock {
			e.cursorRow++
			e.cursorCol = 0
		}
	} else if e.cursorRow < e.doc.Len() && e.cursorCol < len(e.doc.Line(e.cursorRow)) {
		e.cursorCol = core.NextGrapheme(e.doc.Line(e.cursorRow), e.cursorCol)
	} else if e.cursorRow < e.doc.Len()-1 && !e.selBlock {
		e.cursorRow++
		e.cursorCol = 0
	}
	return keyActed
}

func (e *Editor) keyHome(k keyCtx) keyOutcome {
	if k.ctrl {
		e.cursorRow = 0
	}
	e.cursorCol = 0
	return keyActed
}

func (e *Editor) keyEnd(k keyCtx) keyOutcome {
	if k.ctrl {
		e.cursorRow = e.doc.Len() - 1
	}
	if e.cursorRow < e.doc.Len() {
		e.cursorCol = len(e.doc.Line(e.cursorRow))
	}
	return keyActed
}

func (e *Editor) keyPgUp(keyCtx) keyOutcome {
	if e.wrapMode {
		e.moveVisualRows(-e.contentH())
		return keyActed
	}
	e.cursorRow = max(0, e.cursorRow-e.contentH())
	e.cursorCol = e.colForDesired()
	return keyActed
}

func (e *Editor) keyPgDn(keyCtx) keyOutcome {
	if e.wrapMode {
		e.moveVisualRows(e.contentH())
		return keyActed
	}
	e.cursorRow = min(e.doc.Len()-1, e.cursorRow+e.contentH())
	e.cursorCol = e.colForDesired()
	return keyActed
}

// ---------------------------------------------------------------------------
// Key bindings: editing
// ---------------------------------------------------------------------------

func (e *Editor) keyEnter(k keyCtx) keyOutcome {
	e.pushUndoLocal()
	// Read before deleteSelection, which drops the block along with the selection;
	// asking afterwards always answers false.
	wasBlock := e.blockEditing()
	if k.hadSelection {
		e.deleteSelection()
	}
	// Auto-indent lives here, in the key handler, never in insertNewline: Paste
	// and blockPaste drive insertNewline once per pasted line, so indenting there
	// would re-indent each line on top of the indentation it carries (paste
	// becomes a staircase). Block editing keeps its plain split. The indent is
	// measured before the split, and the spaces go in under the pushUndoLocal
	// above, so one Ctrl+Z undoes the whole Enter.
	indent := 0
	if !wasBlock {
		indent = e.leadingIndentForNewLine() + e.smartIndentBonus()
	}
	e.insertNewline()
	for range indent {
		e.insertRune(' ')
	}
	return keyActed
}

func (e *Editor) keyBackspace(k keyCtx) keyOutcome {
	return e.keyErase(k, (*Editor).blockBackspace, (*Editor).deleteWordLeft, (*Editor).backspace)
}

func (e *Editor) keyDelete(k keyCtx) keyOutcome {
	return e.keyErase(k, (*Editor).blockDelete, (*Editor).deleteWordRight, (*Editor).deleteChar)
}

// keyErase is Backspace and Delete, which differ only in direction.
func (e *Editor) keyErase(k keyCtx, block, word, char func(*Editor)) keyOutcome {
	e.pushUndoLocal()
	switch {
	case e.blockEditing():
		// Block edits keep the block armed for the next key, so they take over from the
		// plain-selection cases below rather than via deleteSelection, which drops it.
		block(e)
		return keyKeptSelection
	case k.hadSelection:
		e.deleteSelection()
	case k.ctrl:
		word(e)
	default:
		char(e)
	}
	return keyActed
}

func (e *Editor) keyTab(k keyCtx) keyOutcome {
	if k.shift {
		e.DedentLines()
		return keyKeptSelection
	}
	if e.blockEditing() {
		e.pushUndoLocal()
		for range e.indentWidth {
			e.blockInsertRune(' ')
		}
		return keyKeptSelection
	}
	if k.hadSelection {
		if sr, _, er, _ := e.selectionBounds(); sr != er {
			e.IndentLines()
			return keyKeptSelection
		}
	}
	e.pushUndoLocal()
	if k.hadSelection {
		e.deleteSelection()
	}
	for range e.indentWidth {
		e.insertRune(' ')
	}
	return keyActed
}

// keyCase is Ctrl+U (lowercase) and Ctrl+Shift+U (uppercase).
func (e *Editor) keyCase(k keyCtx) keyOutcome {
	if k.shift {
		e.UppercaseSelection()
	} else {
		e.LowercaseSelection()
	}
	return keyKeptSelection
}

// keyToggleComment is Ctrl+/, unbound while no language prefix is set.
func (e *Editor) keyToggleComment() keyOutcome {
	if len(e.lineComment) == 0 {
		return keyDeclined
	}
	e.ToggleLineComments()
	return keyKeptSelection
}

// keyText is every key absent from editorKeys: a typed character, Ctrl+/ and
// Ctrl+Space (both arrive as runes with ModCtrl), or an unbound key.
func (e *Editor) keyText(k keyCtx) keyOutcome {
	r := core.EvRune(k.ev)
	switch {
	case k.ctrl && (r == '/' || r == '_'):
		// Ctrl+/ emits 0x1F on legacy terminals, which tcell v3 decodes as KeyRune '_'
		// with ModCtrl, not KeyUS; a modern keyboard protocol reports '/'. Accept both.
		return e.keyToggleComment()
	case k.ctrl && r == ' ':
		// Ctrl+Space is SSMS's IntelliSense trigger where a completion provider is
		// installed; otherwise it opens OnRightClick's Cut/Copy/Paste menu at the text
		// cursor.
		switch {
		case e.completionProvider != nil:
			e.triggerCompletionExplicit()
			return keyFired
		case e.OnRightClick != nil:
			x, y := e.cursorScreenPos()
			e.OnRightClick(x, y)
			return keyFired
		}
		return keyKeptSelection
	case r == 0 || k.ctrl || k.alt:
		return keyUnbound
	}
	e.pushUndoLocal()
	if e.blockEditing() {
		// One character into every row the block spans, leaving it armed a column
		// further right so the next keystroke follows.
		for _, r := range core.EvText(k.ev) {
			e.blockInsertRune(r)
		}
		return keyKeptSelection
	}
	if k.hadSelection {
		e.deleteSelection()
	}
	for _, r := range core.EvText(k.ev) {
		e.insertRune(r)
	}
	return keyActed
}

// editorZone is the part of the editor a Button1 press began in, which owns
// every event of the gesture until the release (Editor.gesture).
type editorZone int

const (
	// editorText is a text selection drag: the press placed the cursor and armed
	// the anchor, the drag moves the cursor.
	editorText editorZone = iota
	// editorVBar and editorHBar drag the scrollbar thumbs.
	editorVBar
	editorHBar
	// editorPopupItem is a press on a completion item, acted on by the press: the
	// resends while held must not commit again.
	editorPopupItem
	// editorPopupBar drags the completion popup's scrollbar thumb.
	editorPopupBar
)

// pressText is a fresh Button1 press on the text at (row, col), shared by both
// wrap modes, which differ only in how they derive (row, col) (a clamp plus
// runeColAtScreenX against a physical row, versus a visual-line lookup plus
// RuneIndexAtColumn against a segment).
//
// It repositions the cursor. Without Shift, it arms a new anchor here;
// HasSelection() stays false until the drag moves off this point. With Shift,
// it keeps the active anchor (or the pre-click cursor) and moves only the
// cursor, the usual click-to-extend. Alt on the press picks block vs. linear
// selection for the whole drag, best-effort since terminals vary in reporting
// it, and never in wrap mode, which breaks the fixed rune columns a block
// selection assumes, so the press clears it there rather than leaving what the
// previous mode armed.
func (e *Editor) pressText(row, col int, ev *tcell.EventMouse) {
	// A second unmodified press on the same spot selects the word under it. The
	// resends while held are the gesture's (dragText), so they extend from the
	// word. pressIsDouble runs for every fresh press, modified or not, so a Shift-
	// or Alt-clicked press still counts as "the previous press"; the modifier test
	// lives inside it, not in an && here that would discard the press before
	// consulting it.
	if e.pressIsDouble(row, col, ev.When(), ev.Modifiers()) {
		if e.OnDoubleClick == nil || !e.OnDoubleClick(row) {
			e.selectWordAt(row, col)
		}
		return
	}
	if ev.Modifiers()&tcell.ModShift != 0 {
		if !e.selecting {
			e.selAnchorRow, e.selAnchorCol = e.cursorRow, e.cursorCol
		}
	} else {
		e.selAnchorRow, e.selAnchorCol = row, col
	}
	e.selecting = true
	e.selBlock = !e.wrapMode && ev.Modifiers()&tcell.ModAlt != 0
	e.cursorRow, e.cursorCol = row, col
	e.desiredCol = core.ColumnOfRune(e.doc.Line(row), col)
}

// dragText continues a text selection at (mx, my): the cursor moves, anchor
// and mode stay as the press set them. Past an edge the pointer is clamped to
// the row (and, unwrapped, the column) just outside the view, and
// ensureCursorVisible scrolls one toward it per motion event (tuikit has no
// timer to scroll while the pointer rests). The horizontal bar's row counts as
// below the text.
func (e *Editor) dragText(mx, my int) {
	h := e.contentH()
	if h <= 0 || e.doc.Len() == 0 {
		return
	}
	contentX := e.rect.X + e.gutterWidth()
	cy := core.Clamp(my, e.rect.Y-1, e.rect.Y+h)
	outside := cy != my || cy < e.rect.Y || cy >= e.rect.Y+h
	var row, col int
	if e.wrapMode {
		row, col = e.wrappedPosAt(e.buildVisualLines(e.wrapWidth()), mx, cy, contentX)
	} else {
		w := e.rect.W - e.gutterWidth()
		dx := core.Clamp(mx-contentX, -1, max(0, w))
		outside = outside || dx < 0 || dx >= w
		row = core.Clamp(e.scrollRow+(cy-e.rect.Y), 0, e.doc.Len()-1)
		col = e.runeColAtScreenX(row, dx)
	}
	e.cursorRow, e.cursorCol = row, col
	e.desiredCol = core.ColumnOfRune(e.doc.Line(row), col)
	if outside {
		e.ensureCursorVisible()
	}
}

// HandleMouse handles mouse events.
func (e *Editor) HandleMouse(ev *tcell.EventMouse) bool {
	// A release ends the gesture wherever it lands, so a drag ending outside the
	// editor leaves nothing latched.
	if ev.Buttons() == tcell.ButtonNone {
		_, held := e.gesture.Owner()
		e.gesture.Release()
		return held
	}
	// Ahead of the popup and the bounds test: a held drag keeps its zone wherever
	// the pointer drifts, so a selection dragged past the last line never lands on
	// the horizontal bar, and one dragged off the editor keeps scrolling.
	if e.gesture.Route(ev, e.routeDrag) {
		return true
	}
	// As with HandleKey's completionOpen check: the popup floats independently of the
	// editor's rect, so it gets first refusal before position-based routing can
	// misread a click meant for it.
	if e.completionOpen && e.handleCompletionMouse(ev) {
		return true
	}

	mx, my := ev.Position()
	if !e.rect.Contains(mx, my) {
		return false
	}
	// The wheel scrolls from the gutter too, and is consumed even at a limit
	// (core.MouseHandler), in both wrap modes.
	if e.handleWheel(ev) {
		return true
	}
	contentX := e.rect.X + e.gutterWidth()
	if mx < contentX {
		return false
	}
	// Right-click (Button2 is Secondary in tcell v3): hand off to the app layer for a
	// context menu, in both wrap modes, without disturbing cursor or selection.
	if ev.Buttons() == tcell.Button2 {
		if e.OnRightClick != nil {
			e.OnRightClick(mx, my)
		}
		return true
	}

	if ev.Buttons() != tcell.Button1 {
		return false
	}
	// Scrollbar presses outrank text-click handling: the vertical bar is drawn over
	// the rightmost content column and would read as a click positioning the cursor
	// at that line's end. total mirrors what drawScrollbar passed to
	// core.DrawScrollbar (visual rows in wrap mode, logical lines otherwise).
	// wrappedPosAt reuses vls below rather than recomputing the same O(document)
	// slice.
	if x, y, w, total, _, ok := e.hScrollbar(); ok && core.OnScrollbarH(mx, my, x, y, w, total) {
		e.dragHScrollbar(mx)
		e.gesture.Arm(ev, editorHBar)
		return true
	}
	var vls []visualLine
	total := e.doc.Len()
	if e.wrapMode {
		vls = e.buildVisualLines(e.wrapWidth())
		total = len(vls)
	}
	if core.OnScrollbar(mx, my, e.rect.Right()-1, e.rect.Y, e.contentH(), total) {
		e.dragVScrollbar(my)
		e.gesture.Arm(ev, editorVBar)
		return true
	}
	var row, col int
	if e.wrapMode {
		row, col = e.wrappedPosAt(vls, mx, my, contentX)
	} else {
		row = core.Clamp(e.scrollRow+min(my-e.rect.Y, e.contentH()-1), 0, e.doc.Len()-1)
		col = e.runeColAtScreenX(row, mx-contentX)
	}
	e.pressText(row, col, ev)
	e.gesture.Arm(ev, editorText)
	return true
}

// routeDrag delivers a held Button1 to the zone its press began in.
func (e *Editor) routeDrag(ev *tcell.EventMouse, z editorZone) {
	mx, my := ev.Position()
	switch z {
	case editorText:
		e.dragText(mx, my)
	case editorVBar:
		e.dragVScrollbar(my)
	case editorHBar:
		e.dragHScrollbar(mx)
	case editorPopupBar:
		e.dragCompletionScrollbar(my)
	}
}

// dragVScrollbar points the view at the track position of screen row my. The
// track counts what scrollRow counts: visual rows wrapped, logical lines
// otherwise.
func (e *Editor) dragVScrollbar(my int) {
	if h, total := e.contentH(), e.scrollRows(); h > 0 && total > h {
		e.scrollRow = core.ScrollOffsetForDrag(my-e.rect.Y, h, total, h)
	}
}

// dragHScrollbar points the view at the track position of screen column mx.
// Unlike DataGrid's the plain core math serves: the track's width is the
// visible count, both in rune columns.
func (e *Editor) dragHScrollbar(mx int) {
	if x, _, w, total, _, ok := e.hScrollbar(); ok {
		e.scrollCol = core.ScrollOffsetForDrag(mx-x, w, total, w)
	}
}

// handleWheel scrolls for a wheel event and reports whether ev was one. The
// vertical step counts logical lines unwrapped and visual rows wrapped, which is
// what scrollRow counts in each mode.
func (e *Editor) handleWheel(ev *tcell.EventMouse) bool {
	shift := ev.Modifiers()&tcell.ModShift != 0
	switch ev.Buttons() {
	case tcell.WheelUp:
		// Shift+wheel is the desktop convention for horizontal scroll, and some terminals
		// report it that way rather than as WheelLeft/WheelRight; honour both. As in
		// DataGrid. Wrapped text has no horizontal scroll.
		if shift && !e.wrapMode {
			e.scrollColBy(-core.WheelCells)
		} else {
			e.scrollRow = max(0, e.scrollRow-core.WheelRows)
		}
	case tcell.WheelDown:
		if shift && !e.wrapMode {
			e.scrollColBy(core.WheelCells)
		} else {
			e.scrollRow = max(e.scrollRow, min(e.scrollRows()-1, e.scrollRow+core.WheelRows))
		}
	case tcell.WheelLeft:
		if !e.wrapMode {
			e.scrollColBy(-core.WheelCells)
		}
	case tcell.WheelRight:
		if !e.wrapMode {
			e.scrollColBy(core.WheelCells)
		}
	default:
		return false
	}
	return true
}

// scrollRows is the extent scrollRow moves over: visual rows wrapped, logical
// lines otherwise.
func (e *Editor) scrollRows() int {
	if e.wrapMode {
		return len(e.buildVisualLines(e.wrapWidth()))
	}
	return e.doc.Len()
}

// pressIsDouble reports whether an unmodified press at (row, col) follows a
// previous one at the same position closely enough to be a double-click, and
// records this press for the next call. mod is taken here, not tested by the
// caller: a modified press must still be recorded, or it leaves no "previous
// press" for the next.
func (e *Editor) pressIsDouble(row, col int, at time.Time, mod tcell.ModMask) bool {
	if mod != tcell.ModNone {
		e.clicks.Record([2]int{row, col}, at)
		return false
	}
	return e.clicks.Press([2]int{row, col}, at)
}

// CancelMouseDrag ends a held gesture (a text selection, a scrollbar or popup
// drag) without a release (core.DragCanceller). The selection made so far
// stays; the next press starts afresh.
func (e *Editor) CancelMouseDrag() { e.gesture.Release() }

// SetCursorFromScreen moves the cursor to the document position under (x, y) and
// clears any selection: HandleMouse's fresh-click targeting, exposed for callers
// placing the cursor without a synthetic mouse event.
func (e *Editor) SetCursorFromScreen(x, y int) {
	contentX := e.rect.X + e.gutterWidth()
	var row, col int
	if e.wrapMode {
		row, col = e.wrappedPosAt(e.buildVisualLines(e.wrapWidth()), x, y, contentX)
	} else {
		row = core.Clamp(e.scrollRow+min(y-e.rect.Y, e.contentH()-1), 0, e.doc.Len()-1)
		col = e.runeColAtScreenX(row, x-contentX)
	}
	e.cursorRow, e.cursorCol = row, col
	e.selecting, e.selBlock = false, false
	e.gesture.Release()
	e.desiredCol = core.ColumnOfRune(e.doc.Line(row), col)
	e.ensureCursorVisible()
}

// runeColAtScreenX converts an x offset within the content area into a rune index
// on the row, clamped to the line's end. dx is a terminal-column offset and the
// result a rune index: this is where a wide character earlier on the line is
// accounted for.
func (e *Editor) runeColAtScreenX(row, dx int) int {
	if row < 0 || row >= e.doc.Len() {
		return 0
	}
	line := e.doc.Line(row)
	return min(core.RuneIndexAtColumn(line, max(0, e.scrollCol+dx)), len(line))
}

// colForDesired maps desiredCol (a display column, see Editor.desiredCol) onto a
// rune index on the cursor's row, clamped to its end: the goal-column half of
// vertical caret movement.
func (e *Editor) colForDesired() int {
	line := e.cursorLine()
	return min(core.RuneIndexAtColumn(line, e.desiredCol), len(line))
}

// scrollColBy shifts scrollCol by delta (negative scrolls left), clamped so it
// can't pass the last character of the longest line.
func (e *Editor) scrollColBy(delta int) {
	e.scrollCol = core.Clamp(e.scrollCol+delta, 0, max(0, e.doc.maxDisplayWidth()-1))
}
