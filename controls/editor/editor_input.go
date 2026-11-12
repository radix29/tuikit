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
// SetReadOnly(true) lets through. It inspects Key() only: Shift and Ctrl don't
// change it, so Shift+Left passes while every mutating key is rejected.
func readOnlySafeKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight,
		tcell.KeyHome, tcell.KeyEnd, tcell.KeyPgUp, tcell.KeyPgDn, tcell.KeyCtrlA:
		return true
	}
	return false
}

// HandleKey handles keyboard input.
func (e *Editor) HandleKey(ev *tcell.EventKey) bool {
	// An open completion popup gets first refusal of list-navigation, commit and
	// dismiss keys; everything else falls through, and updateCompletion at the end
	// keeps it in sync.
	if e.completionOpen && e.handleCompletionKey(ev) {
		return true
	}
	if e.readOnly && !readOnlySafeKey(ev) {
		return false
	}
	hadSelection := e.HasSelection()

	mods := ev.Modifiers()
	ctrlHeld := mods&tcell.ModCtrl != 0
	shiftHeld := mods&tcell.ModShift != 0
	altHeld := mods&tcell.ModAlt != 0

	// typedChar marks a plainly typed character, the only key that can start a
	// completion session from closed (subject to canAutoOpenCompletion's word-start
	// gate). Everything else only re-syncs an already-open popup.
	typedChar := false
	switch ev.Key() {
	case tcell.KeyEnter, tcell.KeyBackspace, tcell.KeyBackspace2, tcell.KeyDelete,
		tcell.KeyTab, tcell.KeyCtrlZ, tcell.KeyCtrlY:
	default:
		if r := core.EvRune(ev); r != 0 && !ctrlHeld && !altHeld {
			typedChar = true
		}
	}

	isArrowKey := false
	isMovementKey := false
	switch ev.Key() {
	case tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight:
		isArrowKey = true
		isMovementKey = true
	case tcell.KeyHome, tcell.KeyEnd, tcell.KeyPgUp, tcell.KeyPgDn:
		isMovementKey = true
	}

	// Move Line (Ctrl+Shift+Up/Down) and rectangular block selection
	// (Alt+Shift+Arrow) ride on movement keys plus Shift, so they are carved out of
	// the plain "extend selection" combo below.
	moveLineCombo := (ev.Key() == tcell.KeyUp || ev.Key() == tcell.KeyDown) && ctrlHeld && shiftHeld && !altHeld
	blockCombo := isArrowKey && altHeld && shiftHeld && !e.wrapMode
	linearExtendCombo := isMovementKey && shiftHeld && !altHeld && !moveLineCombo

	extending := blockCombo || linearExtendCombo
	if extending && !e.selecting {
		e.selecting = true
		e.selBlock = blockCombo
		e.selAnchorRow, e.selAnchorCol = e.cursorRow, e.cursorCol
	}
	// dropSelection decides, after the switch, whether to clear the selection. True
	// by default; cases managing it themselves clear it.
	dropSelection := !extending

	switch ev.Key() {
	case tcell.KeyUp:
		if moveLineCombo {
			e.MoveLinesUp()
			dropSelection = false
		} else if e.wrapMode {
			e.moveVisualRows(-1)
		} else if e.cursorRow > 0 {
			e.cursorRow--
			e.cursorCol = e.colForDesired()
		}
	case tcell.KeyDown:
		if moveLineCombo {
			e.MoveLinesDown()
			dropSelection = false
		} else if e.wrapMode {
			e.moveVisualRows(1)
		} else if e.cursorRow < e.doc.Len()-1 {
			e.cursorRow++
			e.cursorCol = e.colForDesired()
		}
	case tcell.KeyLeft:
		if ctrlHeld {
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
	case tcell.KeyRight:
		if ctrlHeld {
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
	case tcell.KeyHome:
		if ctrlHeld {
			e.cursorRow = 0
		}
		e.cursorCol = 0
	case tcell.KeyEnd:
		if ctrlHeld {
			e.cursorRow = e.doc.Len() - 1
		}
		if e.cursorRow < e.doc.Len() {
			e.cursorCol = len(e.doc.Line(e.cursorRow))
		}
	case tcell.KeyCtrlA:
		e.SelectAll()
		dropSelection = false
	case tcell.KeyPgUp:
		if e.wrapMode {
			e.moveVisualRows(-e.contentH())
			break
		}
		e.cursorRow = max(0, e.cursorRow-e.contentH())
		e.cursorCol = e.colForDesired()
	case tcell.KeyPgDn:
		if e.wrapMode {
			e.moveVisualRows(e.contentH())
			break
		}
		e.cursorRow = min(e.doc.Len()-1, e.cursorRow+e.contentH())
		e.cursorCol = e.colForDesired()
	case tcell.KeyEnter:
		e.pushUndoLocal()
		// Read before deleteSelection, which drops the block along with the selection;
		// asking afterwards always answers false.
		wasBlock := e.blockEditing()
		if hadSelection {
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
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		e.pushUndoLocal()
		switch {
		case e.blockEditing():
			// Block edits keep the block armed for the next key, so they take over from the
			// plain-selection cases below rather than via deleteSelection, which drops it.
			e.blockBackspace()
			dropSelection = false
		case hadSelection:
			e.deleteSelection()
		case ctrlHeld:
			e.deleteWordLeft()
		default:
			e.backspace()
		}
	case tcell.KeyDelete:
		e.pushUndoLocal()
		switch {
		case e.blockEditing():
			e.blockDelete()
			dropSelection = false
		case hadSelection:
			e.deleteSelection()
		case ctrlHeld:
			e.deleteWordRight()
		default:
			e.deleteChar()
		}
	case tcell.KeyTab:
		if shiftHeld {
			e.DedentLines()
			dropSelection = false
			break
		}
		if e.blockEditing() {
			e.pushUndoLocal()
			for range e.indentWidth {
				e.blockInsertRune(' ')
			}
			dropSelection = false
			break
		}
		if hadSelection {
			sr, _, er, _ := e.selectionBounds()
			if sr != er {
				e.IndentLines()
				dropSelection = false
				break
			}
		}
		e.pushUndoLocal()
		if hadSelection {
			e.deleteSelection()
		}
		for range e.indentWidth {
			e.insertRune(' ')
		}
	case tcell.KeyBacktab:
		// Some terminals report Shift+Tab as this key rather than KeyTab+ModShift.
		// Backtab always implies Shift.
		e.DedentLines()
		dropSelection = false
	case tcell.KeyCtrlZ:
		e.undo()
	case tcell.KeyCtrlY:
		e.redo()
	case tcell.KeyCtrlD:
		e.DuplicateLines()
	case tcell.KeyCtrlL:
		e.DeleteLines()
	case tcell.KeyCtrlU:
		if shiftHeld {
			e.UppercaseSelection()
		} else {
			e.LowercaseSelection()
		}
		dropSelection = false
	case tcell.KeyUS:
		// Defensive: tcell v3 doesn't surface the 0x1F byte as KeyUS (see the default
		// case), but a protocol that did would still toggle comments.
		e.ToggleLineComments()
		dropSelection = false
	default:
		r := core.EvRune(ev)
		// Ctrl+/ emits 0x1F on legacy terminals, which tcell v3 decodes as KeyRune '_'
		// with ModCtrl, not KeyUS; a modern keyboard protocol reports '/'. Accept both.
		if ctrlHeld && (r == '/' || r == '_') {
			e.ToggleLineComments()
			dropSelection = false
			break
		}
		// Ctrl+Space is SSMS's IntelliSense trigger where a completion provider is
		// installed; otherwise it opens OnRightClick's Cut/Copy/Paste menu at the text
		// cursor.
		if ctrlHeld && r == ' ' {
			if e.completionProvider != nil {
				e.triggerCompletionExplicit()
			} else if e.OnRightClick != nil {
				x, y := e.cursorScreenPos()
				e.OnRightClick(x, y)
			}
			dropSelection = false
			break
		}
		if r != 0 && !ctrlHeld && !altHeld {
			e.pushUndoLocal()
			if e.blockEditing() {
				// One character into every row the block spans, leaving it armed a column
				// further right so the next keystroke follows.
				for _, r := range core.EvText(ev) {
					e.blockInsertRune(r)
				}
				dropSelection = false
				break
			}
			if hadSelection {
				e.deleteSelection()
			}
			for _, r := range core.EvText(ev) {
				e.insertRune(r)
			}
		} else {
			if dropSelection {
				e.selecting = false
				e.selBlock = false
			}
			return false
		}
	}
	if dropSelection {
		e.selecting = false
		e.selBlock = false
	}
	e.clampCursor()
	// Vertical movement preserves the goal column; every other cursor-moving key
	// re-anchors it where the cursor ended.
	switch ev.Key() {
	case tcell.KeyUp, tcell.KeyDown, tcell.KeyPgUp, tcell.KeyPgDn:
	default:
		e.desiredCol = e.cursorDisplayCol()
	}
	e.ensureCursorVisible()
	// A typed character may open the popup from closed only when the cursor sits at
	// the end of a word starting with a letter or '['. While open, every key reaching
	// here re-syncs it.
	if e.completionOpen || (typedChar && e.canAutoOpenCompletion()) {
		e.updateCompletion()
	}
	return true
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
			e.scrollColBy(-horizontalWheelChars)
		} else if e.scrollRow > 0 {
			e.scrollRow--
		}
	case tcell.WheelDown:
		if shift && !e.wrapMode {
			e.scrollColBy(horizontalWheelChars)
		} else if e.scrollRow < e.scrollRows()-1 {
			e.scrollRow++
		}
	case tcell.WheelLeft:
		if !e.wrapMode {
			e.scrollColBy(-horizontalWheelChars)
		}
	case tcell.WheelRight:
		if !e.wrapMode {
			e.scrollColBy(horizontalWheelChars)
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

// horizontalWheelChars is how many characters one horizontal wheel tick scrolls;
// meaningful only outside wrapMode, where scrollCol is a character offset.
const horizontalWheelChars = 4

// scrollColBy shifts scrollCol by delta (negative scrolls left), clamped so it
// can't pass the last character of the longest line.
func (e *Editor) scrollColBy(delta int) {
	e.scrollCol = core.Clamp(e.scrollCol+delta, 0, max(0, e.doc.maxDisplayWidth()-1))
}
