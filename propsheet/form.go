package propsheet

import (
	"slices"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/theme"
)

// band records the on-screen line range a row occupied on the last Draw, so
// HandleMouse can map a click back to a row without re-deriving layout.
type band struct {
	row  int
	y, h int
}

// Form lays out and drives a vertical stack of Rows: focus cycling (Tab/Backtab,
// plus Up/Down for rows that don't consume them), scrolling, and the aggregate
// Dirty/Revert/Validate/CopyText operations PropertySheet needs. Scroll is in
// row-index units, not display lines: a row taller than one line scrolls in and
// out whole. A row too tall for the space left is shrunk into it (Shrinkable) or
// left for the next scroll position (see drawHeight).
type Form struct {
	rows        []Row
	focus       int // -1 = no focusable row
	scroll      int // index of the first row considered for drawing
	formFocused bool
	rect        core.Rect
	bands       []band

	// readOnly makes every row but a Browsable one unfocusable and refuses to route
	// a click into one. See SetReadOnly.
	readOnly bool

	// gesture is what the held Button1 press began on: a row's index, formBar or
	// formInert. Every event until the release goes there, wherever the pointer
	// drifts: a thumb drag keeps following the pointer over a focused GridRow, and
	// a selection drag in one row's grid or field never reaches the row it drifts
	// onto as a fresh press.
	gesture core.Gesture[int]

	// applyConfirm is asked before this form's edits are applied for real; see
	// SetApplyConfirm.
	applyConfirm func() string

	// commit is the form's commit hook — see SetCommit.
	commit func()

	// heights is every row's Height at width heightsW, measured once per Draw: the
	// scrollbar, scroll clamp and layout each walk the rows, and a Note or Hint
	// row's Height wraps its whole text. measured is false outside Draw, where a
	// handler may have changed the content, so the row is asked directly.
	heights  []int
	heightsW int
	measured bool
}

// NewForm creates a Form from an initial set of rows; order is tab order and
// draw order.
func NewForm(rows ...Row) *Form {
	f := &Form{focus: -1}
	f.Add(rows...)
	return f
}

// Add appends more rows.
func (f *Form) Add(rows ...Row) {
	f.rows = append(f.rows, rows...)
	f.applyReadOnlyDraw(rows)
}

// Prepend inserts rows ahead of the existing ones, for a caveat that must be read
// before the page it applies to (an appended Note sits below a grid, and being
// unfocusable Tab never brings it into view). Only safe before the form is
// interacted with: focus is an index into rows.
func (f *Form) Prepend(rows ...Row) {
	f.rows = append(append([]Row{}, rows...), f.rows...)
	f.applyReadOnlyDraw(rows)
}

// SetReadOnly makes the form uneditable: no row can take focus or receive a
// press, except a Browsable row, which keeps both for looking at and which
// ReadOnlyDrawer makes inert: a grid browses, a flat text, check, select or radio
// row takes focus only so Ctrl+C can copy it, a read-only editor scrolls and
// selects. Wheel scrolling and PgUp/PgDn still work, and every ReadOnlyDrawer row
// switches to flat rendering.
//
// This presents a Properties page whose reads succeed but writes would be
// refused. Nothing can become dirty, so Apply and Script Changes have nothing to
// send. Dirty enforces it outright: a browsing grid's OnSelectRow may run a
// commit-the-detail-rows-back handler, and a lossy round trip there (a size shown
// in MB, stored in KB) must not turn a look into a write.
func (f *Form) SetReadOnly(v bool) {
	f.readOnly = v
	if v {
		f.focus = -1
	}
	f.applyReadOnlyDraw(f.rows)
}

// applyReadOnlyDraw pushes the read-only state into the given rows. Called from
// Add and Prepend too: rows built after the gate decided would draw editable.
func (f *Form) applyReadOnlyDraw(rows []Row) {
	for _, row := range rows {
		if rd, ok := row.(ReadOnlyDrawer); ok {
			rd.SetDrawReadOnly(f.readOnly)
		}
	}
}

// ReadOnly reports whether SetReadOnly is in force.
func (f *Form) ReadOnly() bool { return f.readOnly }

// focusableAt reports whether row i can take focus right now: its own answer,
// unless the form is read-only, when only a Browsable row can.
func (f *Form) focusableAt(i int) bool {
	return f.rows[i].Focusable() && (!f.readOnly || browsable(f.rows[i]))
}

// browsable reports whether row may be focused and clicked on a read-only form.
func browsable(row Row) bool {
	b, ok := row.(Browsable)
	return ok && b.Browsable()
}

// Rows returns the form's rows in order, for a caller that needs to find one
// after the form is built.
func (f *Form) Rows() []Row { return f.rows }

// SetBounds positions the form's content area. The same rect again changes
// nothing; a new one keeps the scroll inside the content and never moves focus.
// Draw clamps again, as rows can grow or shrink between frames. An unscrolled
// form skips the clamp: it measures every row, a Note's a whole word-wrap, and
// scroll 0 is always in range.
func (f *Form) SetBounds(x, y, w, h int) {
	r := core.Rect{X: x, Y: y, W: w, H: h}
	if r == f.rect {
		return
	}
	f.rect = r
	if f.scroll > 0 {
		f.scroll = core.Clamp(f.scroll, 0, f.maxScroll(f.contentWidth()))
	}
}

// Bounds returns the content area SetBounds last gave the form.
func (f *Form) Bounds() core.Rect { return f.rect }

// Focus sets whether the form as a whole holds focus, which drives whether the
// current row draws highlighted. The first focusable row is focused the first
// time this is called with true.
func (f *Form) Focus(v bool) {
	f.formFocused = v
	if v && f.focus < 0 {
		f.FocusFirst()
	}
}

// Focused reports whether the form as a whole holds focus (Focus); FocusedRow
// is which row has it within the form.
func (f *Form) Focused() bool { return f.formFocused }

// FocusFirst focuses the first focusable row, if any.
func (f *Form) FocusFirst() bool { f.focus = -1; return f.FocusNext() }

// FocusLast focuses the last focusable row, if any.
func (f *Form) FocusLast() bool { f.focus = len(f.rows); return f.FocusPrev() }

// FocusNext moves focus to the next focusable row, returning false and leaving
// focus unchanged if there isn't one (PropertySheet then moves to the button
// zone).
func (f *Form) FocusNext() bool {
	for i := f.focus + 1; i < len(f.rows); i++ {
		if f.focusableAt(i) {
			f.setFocus(i)
			return true
		}
	}
	return false
}

// FocusPrev is FocusNext's mirror image.
func (f *Form) FocusPrev() bool {
	for i := f.focus - 1; i >= 0; i-- {
		if f.focusableAt(i) {
			f.setFocus(i)
			return true
		}
	}
	return false
}

func (f *Form) setFocus(i int) {
	f.focus = i
	f.ensureVisible(i)
}

// FocusedRow returns the currently focused row, or nil.
func (f *Form) FocusedRow() Row {
	if f.focus < 0 || f.focus >= len(f.rows) {
		return nil
	}
	return f.rows[f.focus]
}

// contentWidth returns the width available to rows, reserving a column for the
// scrollbar when the form doesn't fit.
func (f *Form) contentWidth() int {
	if f.totalHeight(f.rect.W) > f.rect.H {
		return max(0, f.rect.W-1)
	}
	return f.rect.W
}

func (f *Form) totalHeight(w int) int {
	total := 0
	for i := range f.rows {
		total += f.rowHeight(i, w)
	}
	return total
}

// rowHeight is row i's Height at w, from the frame's measurement when Draw
// has taken one at that width.
func (f *Form) rowHeight(i, w int) int {
	if f.measured && w == f.heightsW {
		return f.heights[i]
	}
	return f.rows[i].Height(w)
}

// measure records every row's height at w for the rest of the frame.
func (f *Form) measure(w int) {
	f.heights = f.heights[:0]
	for _, row := range f.rows {
		f.heights = append(f.heights, row.Height(w))
	}
	f.heightsW, f.measured = w, true
}

// The scrollbar measures the form in content lines (the unit of totalHeight and
// f.rect.H) while f.scroll is a row index; the helpers below convert. Sizing the
// thumb in row-index units leaves the bar empty and undraggable on a page of few
// tall rows.

// scrollLines returns how many content lines sit above the current scroll
// position, i.e. the scrollbar offset for f.scroll.
func (f *Form) scrollLines(w int) int {
	lines := 0
	for i := 0; i < f.scroll && i < len(f.rows); i++ {
		lines += f.rowHeight(i, w)
	}
	return lines
}

// maxScroll returns the largest scroll index worth reaching: the one that first
// brings the last row into view. Past it, content only leaves the top and the
// thumb can't reach the bottom of its track.
func (f *Form) maxScroll(w int) int {
	used := 0
	for i := range slices.Backward(f.rows) {
		used += f.rowHeight(i, w)
		if used > f.rect.H {
			return min(i+1, max(0, len(f.rows)-1))
		}
	}
	return 0
}

// ensureVisible scrolls just enough that row idx's start is in view (not its
// whole height, which a GridRow taller than the form could never satisfy).
func (f *Form) ensureVisible(idx int) {
	if idx < f.scroll {
		f.scroll = idx
		return
	}
	w := f.contentWidth()
	for f.scroll < idx && !f.rowFits(idx, w) {
		f.scroll++
	}
}

// rowFits reports whether row idx is drawn at the current scroll position,
// given rows are laid out top-down starting at f.scroll.
func (f *Form) rowFits(idx, w int) bool {
	y := f.rect.Y
	bottom := f.rect.Y + f.rect.H
	for i := f.scroll; i <= idx; i++ {
		h, ok := f.drawHeight(i, w, bottom-y, i == f.scroll)
		if !ok {
			return false
		}
		if i == idx {
			return true
		}
		y += h
	}
	return false
}

// drawHeight returns the height row i draws at when avail lines remain, and
// whether it is drawn. A row that doesn't fit is clamped into the space left if
// Shrinkable, else left for the scrollbar to bring into view; the first row on
// screen always draws in whatever space there is rather than leaving a blank
// page.
func (f *Form) drawHeight(i, w, avail int, first bool) (int, bool) {
	h := f.rowHeight(i, w)
	if avail <= 0 {
		return h, false
	}
	if avail >= h {
		return h, true
	}
	if sh, ok := f.rows[i].(Shrinkable); ok && (first || avail >= sh.MinDrawHeight()) {
		return avail, true
	}
	return h, first
}

// Draw renders every row that fits below f.scroll, then rebuilds the
// click-routing bands used by HandleMouse.
func (f *Form) Draw(s tcell.Screen) {
	core.FillRect(s, f.rect, ' ', theme.StyleDialog())
	defer func() { f.measured = false }()
	f.measure(f.rect.W)
	w := f.contentWidth()
	if w != f.rect.W {
		f.measure(w)
	}
	f.bands = f.bands[:0]
	// A row that asked to be seen since the last frame is scrolled to here, after
	// the handler ran, whichever path (key, click, button) ran it.
	for i, row := range f.rows {
		if r, ok := row.(Revealer); ok && r.TakeReveal() {
			f.ensureVisible(i)
		}
	}
	// Nothing else re-clamps f.scroll when the form gets taller (a resize), leaving
	// the page starting mid-list with dead space below and the thumb pinned to the
	// bottom.
	f.scroll = core.Clamp(f.scroll, 0, f.maxScroll(w))

	// A row taller than the space left is drawn shrunk into it where it can be
	// (drawHeight), so nothing renders past the bottom edge into the hint line and
	// button row. A row that can't shrink that far ends the pass.
	y := f.rect.Y
	bottom := f.rect.Y + f.rect.H
	for i := f.scroll; i < len(f.rows) && y < bottom; i++ {
		row := f.rows[i]
		h, ok := f.drawHeight(i, w, bottom-y, i == f.scroll)
		if !ok {
			break
		}
		if sh, isShrinkable := row.(Shrinkable); isShrinkable {
			sh.SetDrawHeight(h)
		}
		row.Layout(f.rect.X, y, w)
		row.Draw(s, f.formFocused && i == f.focus)
		f.bands = append(f.bands, band{row: i, y: y, h: h})
		y += h
	}

	if total := f.totalHeight(w); total > f.rect.H {
		sbStyle := theme.StyleDialogLine()
		sbThumb := theme.StyleScrollThumb()
		offset := f.scrollLines(w)
		if f.scroll >= f.maxScroll(w) {
			// Parked on the last row: put the thumb at the bottom of its track.
			offset = total - f.rect.H
		}
		core.DrawScrollbar(s, f.rect.Right()-1, f.rect.Y, f.rect.H, total, f.rect.H, offset, sbStyle, sbThumb)
	}
}

// DrawOverlay draws every row's popup overlay (open dropdowns), if any.
// Call after Draw, once every other element in the dialog has been drawn.
func (f *Form) DrawOverlay(s tcell.Screen) {
	for _, row := range f.rows {
		if od, ok := row.(core.Overlay); ok {
			od.DrawOverlay(s)
		}
	}
}

// OverlayActive reports whether the focused row has an open overlay
// (SelectRow's dropdown, GridRow's "Show Value" popup). A host laying the form
// out beside other position-routed elements must give it first refusal of every
// click while true, the contract of DataGrid.OverlayActive.
func (f *Form) OverlayActive() bool {
	row := f.FocusedRow()
	if row == nil {
		return false
	}
	oa, ok := row.(core.Overlay)
	return ok && oa.OverlayActive()
}

// HandleKey forwards to the focused row first; if it doesn't consume the
// event, Tab/Backtab/Up/Down move focus and PgUp/PgDn scroll.
func (f *Form) HandleKey(ev *tcell.EventKey) bool {
	if row := f.FocusedRow(); row != nil {
		if kh, ok := row.(core.KeyHandler); ok && kh.HandleKey(ev) {
			return true
		}
	}
	switch ev.Key() {
	case tcell.KeyTab, tcell.KeyDown:
		return f.FocusNext()
	case tcell.KeyBacktab, tcell.KeyUp:
		return f.FocusPrev()
	case tcell.KeyPgDn:
		if f.OverlayActive() {
			// Scrolling the row list from under an open dropdown leaves its list at a stale
			// position (DrawOverlay draws every row's overlay every frame). Swallow the key.
			return true
		}
		f.scroll = min(f.maxScroll(f.contentWidth()), f.scroll+max(1, f.rect.H/2))
		return true
	case tcell.KeyPgUp:
		if f.OverlayActive() {
			return true
		}
		f.scroll = max(0, f.scroll-max(1, f.rect.H/2))
		return true
	}
	return false
}

// HandleMouse gives the row under the pointer first refusal on wheel scroll (a
// GridRow's DataGrid has its own rows), then scrolls the form. For other buttons
// the focused row gets first refusal, so a click on its open dropdown overlay
// (below its band) still reaches it, then the row whose band contains the click.
func (f *Form) HandleMouse(ev *tcell.EventMouse) bool {
	switch ev.Buttons() {
	case tcell.WheelUp, tcell.WheelDown:
		if f.OverlayActive() {
			// An open overlay gets first refusal of the wheel too: a dropdown's list scrolls
			// and lies over other rows, and handing the wheel to the row under the pointer
			// scrolled a grid hidden beneath the list. Swallowed either way, as with
			// PgUp/PgDn above.
			if mh, ok := f.FocusedRow().(core.MouseHandler); ok {
				mh.HandleMouse(ev)
			}
			return true
		}
		if row, ok := f.rowAt(ev); ok {
			if mh, isHandler := row.(core.MouseHandler); isHandler && mh.HandleMouse(ev) {
				return true
			}
		}
		if ev.Buttons() == tcell.WheelUp {
			f.scroll = max(0, f.scroll-core.WheelRows)
		} else {
			f.scroll = min(f.maxScroll(f.contentWidth()), f.scroll+core.WheelRows)
		}
		return true
	}
	// A release (or plain motion) ends the gesture and goes to the rows whose
	// latches it lets go: the focused row, scrolled off or not, and the row the
	// gesture began on, which a press on an unfocusable row leaves unfocused. It
	// never falls to the click-routing below: tcell delivers motion continuously,
	// and band-matching it shifted focus to the row under the pointer, closing an
	// open DropDown on the way to its list.
	if ev.Buttons() == tcell.ButtonNone {
		owner, held := f.gesture.Owner()
		f.gesture.Release()
		claimed := f.rowMouse(f.focus, ev)
		if held && owner != f.focus && f.rowMouse(owner, ev) {
			claimed = true
		}
		return claimed
	}
	// Ahead of everything else: the gesture began on the form's bar or one row, so
	// every event until release belongs there wherever the pointer drifted.
	if f.gesture.Route(ev, f.routeDrag) {
		return true
	}
	// A press goes to the focused row first only while that row is on screen or has
	// its overlay open. Scrolled out of view it keeps its last-drawn rect, and a
	// DataGrid claims any press inside it: a grid scrolled off-screen would swallow
	// clicks on whatever row now sits there.
	if f.FocusedRow() != nil && (f.OverlayActive() || f.onScreen(f.focus)) && f.rowMouse(f.focus, ev) {
		f.gesture.Arm(ev, f.focus)
		return true
	}
	mx, my := ev.Position()
	w := f.contentWidth()
	if total := f.totalHeight(w); ev.Buttons() == tcell.Button1 && core.OnScrollbar(mx, my, f.rect.Right()-1, f.rect.Y, f.rect.H, total) {
		f.dragScrollbar(my, w)
		f.gesture.Arm(ev, formBar)
		return true
	}
	if mx < f.rect.X || mx >= f.rect.Right() {
		return false
	}
	for _, b := range f.bands {
		if my < b.y || my >= b.y+b.h {
			continue
		}
		row := f.rows[b.row]
		if f.readOnly && !browsable(row) {
			// Claimed, not routed: falling through would put the press wherever the sheet
			// draws underneath.
			f.gesture.Arm(ev, formInert)
			return true
		}
		if f.rowMouse(b.row, ev) {
			if f.focusableAt(b.row) {
				f.setFocus(b.row)
			}
			f.gesture.Arm(ev, b.row)
			return true
		}
		if f.focusableAt(b.row) {
			f.setFocus(b.row)
			f.gesture.Arm(ev, b.row)
			return true
		}
		return false
	}
	return false
}

// Form gesture zones besides a row index; see Form.gesture.
const (
	// formBar is a press on the form's scrollbar.
	formBar = -1
	// formInert is a press that acted on nothing (a read-only form's row): its
	// drag is swallowed.
	formInert = -2
)

// rowMouse offers ev to row i, if it exists and handles the mouse, and reports
// whether the row claimed it.
func (f *Form) rowMouse(i int, ev *tcell.EventMouse) bool {
	if i < 0 || i >= len(f.rows) {
		return false
	}
	mh, ok := f.rows[i].(core.MouseHandler)
	return ok && mh.HandleMouse(ev)
}

// routeDrag delivers a held Button1 to what the press began on.
func (f *Form) routeDrag(ev *tcell.EventMouse, z int) {
	switch z {
	case formBar:
		_, my := ev.Position()
		f.dragScrollbar(my, f.contentWidth())
	case formInert:
	default:
		f.rowMouse(z, ev)
	}
}

// dragScrollbar points the form at the track position of screen row my. Form
// can't use core.HandleScrollbarDrag's math: that takes one h as both track
// length and visible count, while Form's scrollbar measures lines (f.rect.H,
// f.totalHeight) and f.scroll counts rows.
func (f *Form) dragScrollbar(my, w int) {
	total := f.totalHeight(w)
	if f.rect.H <= 0 || total <= f.rect.H {
		return
	}
	line := core.ScrollOffsetForDrag(my-f.rect.Y, f.rect.H, total, f.rect.H)
	f.scroll = min(f.rowFromLine(line, w), f.maxScroll(w))
}

// rowFromLine maps a drag's line offset to the first row starting at or after
// it. Rounding down to the row containing the line left the bar's last cell, offset total-H, on the
// row before maxScroll whenever the rows' starts don't land exactly on total-H,
// so a dragged thumb could not reach the bottom of most pages while the wheel
// could.
func (f *Form) rowFromLine(n, w int) int {
	y := 0
	for i := range f.rows {
		if y >= n {
			return i
		}
		y += f.rowHeight(i, w)
	}
	return max(0, len(f.rows)-1)
}

// onScreen reports whether row i has a band — some of it drawn — in the last
// Draw.
func (f *Form) onScreen(i int) bool {
	return slices.ContainsFunc(f.bands, func(b band) bool { return b.row == i })
}

// rowAt returns the row whose band contains ev's position, if any.
func (f *Form) rowAt(ev *tcell.EventMouse) (Row, bool) {
	mx, my := ev.Position()
	if mx < f.rect.X || mx >= f.rect.Right() {
		return nil, false
	}
	for _, b := range f.bands {
		if my < b.y || my >= b.y+b.h {
			continue
		}
		return f.rows[b.row], true
	}
	return nil, false
}

// Dirty reports whether any row's value differs from its loaded baseline. A
// read-only form is never dirty, whatever its rows say — see SetReadOnly.
func (f *Form) Dirty() bool {
	if f.readOnly {
		return false
	}
	for _, row := range f.rows {
		if e, ok := row.(Editable); ok && e.Dirty() {
			return true
		}
	}
	return false
}

// Revert restores every row to its loaded baseline.
func (f *Form) Revert() {
	for _, row := range f.rows {
		if e, ok := row.(Editable); ok {
			e.Revert()
		}
	}
}

// Validate runs every dirty row's validator, stopping at the first error.
func (f *Form) Validate() error {
	for _, row := range f.rows {
		e, ok := row.(Editable)
		if !ok || !e.Dirty() {
			continue
		}
		if err := e.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// SetApplyConfirm registers fn to be asked, while the form is dirty, before its
// edits are applied for real: a non-empty answer is a warning the user must
// accept first, for an edit whose write has a consequence beyond the value (such
// as disconnecting other sessions). fn answers "" when none of the edits it
// cares about is pending. The sheet only collects the answers
// (PropertySheet.ApplyConfirmations); asking, and skipping the question for a
// write that is only scripted, is the host's.
func (f *Form) SetApplyConfirm(fn func() string) { f.applyConfirm = fn }

// ApplyConfirm is the warning SetApplyConfirm's function gives for the form's
// current edits, or "" when there is none to give.
func (f *Form) ApplyConfirm() string {
	if f.applyConfirm == nil || !f.Dirty() {
		return ""
	}
	return f.applyConfirm()
}

// SetCommit registers fn as the form's commit hook: it copies what the form's
// widgets show into the model the rows and apply read, for a form whose editor
// fields stand in for one selected item of a list (grid-plus-detail page) and are
// otherwise copied back only when the selection moves.
//
// The host calls it, through PropertySheet.Commit, on the UI goroutine before
// reading Dirty, Validate or ApplyConfirm for an Apply, OK or Script Changes, not
// inside the apply, which runs on another goroutine while the UI goroutine may
// still be drawing the same widgets. fn must be safe to call at any time: with
// nothing selected, and on a form that is not dirty.
func (f *Form) SetCommit(fn func()) { f.commit = fn }

// Commit runs the form's commit hook, if it has one.
func (f *Form) Commit() {
	if f.commit != nil {
		f.commit()
	}
}

// CopyText returns the focused row's copyable value, if any.
func (f *Form) CopyText() (string, bool) {
	row := f.FocusedRow()
	if row == nil {
		return "", false
	}
	c, ok := row.(Copyable)
	if !ok {
		return "", false
	}
	return c.CopyText(), true
}
