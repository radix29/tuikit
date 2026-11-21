package dialogs

import (
	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/core"
)

// focus_ring.go holds the operations over a hand-rolled dialog's focus ring:
// a host dialog that lays out widgets itself and drives focus with a flat
// []core.Focusable and an index, rather than delegating to propsheet.Form's
// rows. A dialog whose whole surface is that ring holds a FieldRing
// (field_ring.go), which owns the state and wires these together; one with
// tabs, a scrollable pane or a mode keeps its own state and calls them
// piecemeal.
//
// They pair with what ModalDialog already draws: DrawButtonsGated paints a
// button row with some buttons disabled, ButtonRowKey is its keyboard (and
// ButtonsKey the keyboard of a dialog that is nothing but that row).

// SetFocusIn focuses list[i], blurring every other entry, and reports the
// index that ended up focused. An out-of-range i blurs everything and answers
// cur, leaving the caller's index where it was — a dialog that rebuilds a
// shorter ring on a mode change and refocuses its old index reaches this in
// normal use.
//
// The target is never blurred on its way to being focused: a DropDown's
// Focus(false) closes its list, so a click that opened the list and then
// focused it closed it again.
func SetFocusIn(list []core.Focusable, i, cur int) int {
	for j, f := range list {
		if j != i {
			f.Focus(false)
		}
	}
	if i >= 0 && i < len(list) {
		list[i].Focus(true)
		return i
	}
	return cur
}

// IndexOfFocusable returns w's position in list, or -1. Identity comparison,
// not equality: the callers hold the very pointers they put in the list.
func IndexOfFocusable(list []core.Focusable, w core.Focusable) int {
	for i, f := range list {
		if f == w {
			return i
		}
	}
	return -1
}

// FocusableEnabled reports whether w accepts input; a widget with no enabled
// state (no core.Enabler) always does.
func FocusableEnabled(w core.Focusable) bool {
	e, ok := w.(core.Enabler)
	return !ok || e.Enabled()
}

// FocusedClipboardTarget answers core.ClipboardHost for a dialog that drives
// focus with a flat slice: list[i] when the focused widget can take part in
// Copy/Cut/Paste, and nil otherwise.
//
// The explicit nil in the miss arms is load-bearing. A typed nil widget put
// into an interface is not a nil interface, and a host resolving Ctrl+C/X/V
// tests the result against nil before calling through it
// (core.ClipboardHost: nil is a real answer).
func FocusedClipboardTarget(list []core.Focusable, i int) core.ClipboardTarget {
	if i < 0 || i >= len(list) {
		return nil
	}
	if t, ok := list[i].(core.ClipboardTarget); ok {
		return t
	}
	return nil
}

// ClipboardFieldAt is core.ClipboardFocuser's hit test for a dialog that drives
// focus with a flat slice: the index of the entry that is a text field (a
// core.ClipboardTarget) drawn at (x, y), or -1. An InputField is hit on its box
// (HitTest), as a left-click is, not on its label; anything else on its Bounds.
// Buttons, checkboxes and dropdowns are not targets, so a right-click on one is
// never taken for the clipboard. A disabled (greyed) field is skipped too, as
// the left-click and Tab paths skip it: focusing one would leave the focus ring
// on a widget that refuses every key and that Tab can never reach. Nothing is
// hit while an entry's overlay is open (a dropdown's list): the list is drawn
// over the fields below it, so the field under (x, y) is not what the user
// right-clicked (core.ClipboardFocuser). This check covers every dropdown in
// list; one drawn by the dialog outside its ring is the dialog's to check.
func ClipboardFieldAt(list []core.Focusable, x, y int) int {
	for _, f := range list {
		if o, ok := f.(core.Overlay); ok && o.OverlayActive() {
			return -1
		}
	}
	for i, f := range list {
		if _, ok := f.(core.ClipboardTarget); ok && FocusableEnabled(f) && fieldHit(f, x, y) {
			return i
		}
	}
	return -1
}

// fieldHit reports whether (x, y) is on w: its HitTest where it has one (an
// InputField's box), else its Bounds.
func fieldHit(w any, x, y int) bool {
	if h, ok := w.(interface{ HitTest(x, y int) bool }); ok {
		return h.HitTest(x, y)
	}
	if b, ok := w.(core.Bounded); ok {
		return b.Bounds().Contains(x, y)
	}
	return false
}

// NextFocus and PrevFocus are Tab and Backtab over a ring of n entries,
// wrapping at both ends. n must be positive: an empty ring divides by zero.
func NextFocus(idx, n int) int { return (idx + 1) % n }

// PrevFocus is Backtab over a ring of n entries; see NextFocus.
func PrevFocus(idx, n int) int { return (idx - 1 + n) % n }

// ScrollToShow returns the scroll offset that brings row sel into a viewport
// dataH rows tall, moving by the least that does it — up when sel is above the
// top, down when it is past the bottom, unchanged when it is already in view.
//
// A viewport with no rows has nothing to scroll into, so the offset is left
// alone. Without the guard the "past the bottom" arm is unconditionally true at
// dataH == 0 and answers sel+1 — one row past the selection it was asked to
// reveal, which draws as an empty pane rather than a short one.
func ScrollToShow(sel, scroll, dataH int) int {
	if dataH <= 0 {
		return scroll
	}
	if sel < scroll {
		return sel
	}
	if sel >= scroll+dataH {
		return sel - dataH + 1
	}
	return scroll
}

// ButtonRowKey is the keyboard of a dialog's button row while it holds focus:
// Left/Right move along the row, past a disabled button; Enter fires the
// focused one unless it is disabled; Escape hides; Tab and Down leave it through leave(+1), Backtab
// and Up through leave(-1), as FieldRingKey moves between fields. Everything
// else is swallowed — a letter typed on the row must not edit the field focus
// came from.
//
// The row is one stop in the Tab ring, crossed with the arrows, as a
// propsheet.Form's button zone is. disabled is the same slice the dialog
// hands DrawButtonsGated; it may be nil or shorter than the row.
func ButtonRowKey(ev *tcell.EventKey, btnFocus *int, n int, disabled []bool, fire, hide func(), leave func(dir int)) bool {
	switch ev.Key() {
	case tcell.KeyLeft:
		*btnFocus = StepButton(*btnFocus, -1, n, disabled)
	case tcell.KeyRight:
		*btnFocus = StepButton(*btnFocus, +1, n, disabled)
	case tcell.KeyEnter:
		if !buttonDisabled(disabled, *btnFocus) {
			fire()
		}
	case tcell.KeyEscape:
		hide()
	case tcell.KeyTab, tcell.KeyDown:
		leave(+1)
	case tcell.KeyBacktab, tcell.KeyUp:
		leave(-1)
	}
	return true
}

// ButtonsKey is the whole keyboard of a dialog that is only a button row — no
// fields to Tab back to, so ButtonRowKey's leave has nowhere to go: Tab and
// Right step forward, Backtab and Left back, wrapping at both ends and past a
// disabled button (StepButtonWrap); Enter fires the focused one; Escape hides.
// Everything else is swallowed: the dialog is modal.
//
// *btnFocus is clamped into the row first, on every key. A row can shrink
// under the dialog between a draw and the keypress (a progress view's Cancel
// disappears the moment its task finishes), and an index recorded against the
// longer row would have fire index past the shorter. With n <= 0 only Escape
// acts. disabled may be nil or shorter than the row.
func ButtonsKey(ev *tcell.EventKey, btnFocus *int, n int, disabled []bool, fire, hide func()) bool {
	if ev.Key() == tcell.KeyEscape {
		hide()
		return true
	}
	if n <= 0 {
		return true
	}
	*btnFocus = core.Clamp(*btnFocus, 0, n-1)
	switch ev.Key() {
	case tcell.KeyTab, tcell.KeyRight:
		*btnFocus = StepButtonWrap(*btnFocus, +1, n, disabled)
	case tcell.KeyBacktab, tcell.KeyLeft:
		*btnFocus = StepButtonWrap(*btnFocus, -1, n, disabled)
	case tcell.KeyEnter:
		if !buttonDisabled(disabled, *btnFocus) {
			fire()
		}
	}
	return true
}

// buttonDisabled reports whether button i is disabled; disabled may be nil or
// shorter than the row, and a missing flag means enabled.
func buttonDisabled(disabled []bool, i int) bool {
	return i >= 0 && i < len(disabled) && disabled[i]
}

// FieldRingKey is the keyboard of a hand-rolled dialog while focus is on one
// of its fields; ButtonRowKey is the button row's. It is propsheet.Form's
// order: focused gets every key first, so an open dropdown's list takes the
// arrows, Enter and Escape, a radio box its arrows, a checkbox Enter and
// Space, a closed dropdown Enter (which opens it). What focused declines falls
// to the ring: Tab and Down move(+1), Backtab and Up move(-1), Enter fires the
// default button, Escape hides. Everything else is swallowed, as on the button
// row: the dialog is modal.
//
// focused may be nil or a widget with no core.KeyHandler. A key the dialog
// owns ahead of its fields (Find's F3) is the dialog's to test first.
func FieldRingKey(ev *tcell.EventKey, focused core.Focusable, fire, hide func(), move func(dir int)) bool {
	if kh, ok := focused.(core.KeyHandler); ok && kh.HandleKey(ev) {
		return true
	}
	switch ev.Key() {
	case tcell.KeyTab, tcell.KeyDown:
		move(+1)
	case tcell.KeyBacktab, tcell.KeyUp:
		move(-1)
	case tcell.KeyEnter:
		fire()
	case tcell.KeyEscape:
		hide()
	}
	return true
}

// RingStep is the stop dir away from cur in a ring of fields followed by the
// button row, which is the one stop at index len(fields). It wraps at both
// ends and passes over disabled fields (FocusableEnabled); the button row is
// always a stop, so a ring whose fields are all disabled lands there. A cur
// outside 0..len(fields) is treated as the button row.
//
// It pairs with FieldRingKey's move and ButtonRowKey's leave: a dialog keeps
// its focused index and its button-row index (reset to the default button on
// leaving the row, so Enter from a field presses the default) as its own
// fields.
func RingStep(fields []core.Focusable, cur, dir int) int {
	n := len(fields) + 1
	i := cur
	if i < 0 || i >= n {
		i = len(fields)
	}
	for range n {
		i = (i + dir + n) % n
		if i == len(fields) || FocusableEnabled(fields[i]) {
			return i
		}
	}
	return len(fields)
}

// StepButton is the next enabled button from i in dir, or i at either end of
// the row. disabled may be nil or shorter than the row.
func StepButton(i, dir, n int, disabled []bool) int {
	for j := i + dir; j >= 0 && j < n; j += dir {
		if j >= len(disabled) || !disabled[j] {
			return j
		}
	}
	return i
}

// StepButtonWrap is StepButton on a row that wraps: the next enabled button
// from i in dir, going round the end, or i when no other button is enabled.
// It is ButtonsKey's step; ButtonRowKey keeps StepButton, because there the
// ends of the row are where Tab and Backtab take over. disabled may be nil or
// shorter than the row.
func StepButtonWrap(i, dir, n int, disabled []bool) int {
	for k := 1; k < n; k++ {
		j := ((i+k*dir)%n + n) % n
		if j >= len(disabled) || !disabled[j] {
			return j
		}
	}
	return i
}

// ForwardReleaseToFocusedField hands a release to the dialog's focused field so
// a text-selection drag in it ends cleanly wherever the pointer was when the
// button came up, and reports whether the event was that release (true:
// HandleMouse is done with it). ARCHITECTURE.md § Mouse gestures: a host that
// returns early from HandleMouse must still forward ButtonNone to a
// latch-bearing child.
//
// Call it below ConsumeOutsideClick and FieldGesture.Release. The live case is
// a field that keeps its own latch outside FieldGesture (an editor.Editor), so
// nothing above here ends a selection drag started in one.
//
// Only a text field — a core.ClipboardTarget that is a core.MouseHandler
// (InputField, Editor, DataGrid) — is forwarded to: checkboxes, radio groups
// and drop-downs are not targets, already got the release above
// ConsumeOutsideClick, and a second one would toggle them. An InputField
// always has the FieldGesture too, so FieldGesture.Release has already ended
// its drag, and FieldGesture.Clear covers a dialog dismissed mid-drag; the
// forward is harmless for it and stays for a dialog that hit-tests a field
// itself.
//
// It deliberately calls neither FieldGesture.Release nor .Replay, nor answers
// the non-Button1 question: their placement differs per dialog (one reads the
// wheel between the two), so a helper fixing an order would be wrong for
// dialogs with a mode or scrollable pane.
func ForwardReleaseToFocusedField(ev *tcell.EventMouse, focused any) bool {
	if ev.Buttons() != tcell.ButtonNone {
		return false
	}
	if _, ok := focused.(core.ClipboardTarget); ok {
		if m, ok := focused.(core.MouseHandler); ok {
			m.HandleMouse(ev)
		}
	}
	return true
}

// RingMouse is the mouse of a hand-rolled dialog that drives focus with a
// flat ring (FieldRingKey and ButtonRowKey are its keyboard). ring holds the
// dialog's fields in Tab order and the button row is the stop at len(ring);
// focus is the focused stop. setFocus(i) moves focus to stop i, len(ring)
// included; press(btn) fires button btn, after setFocus(len(ring)). It reports
// whether ev was spoken for: every event the dialog sees is, except a
// non-Button1 event inside the box (the wheel), which is left to the host.
//
// The members are routed by type. One that is a core.DragCanceller and a
// core.MouseHandler (InputField, ListBox, Editor) starts a drag, so a press on
// it (HitTest, else Bounds; never a disabled one) goes through drag.Claim.
// Any other core.MouseHandler (CheckBox, RadioBox, DropDown, Button) takes its
// press itself.
//
// The order is the one ARCHITECTURE.md § Mouse gestures and goSSMS's
// docs/ui-rules.md § Mouse list as shipped bugs, each step for a reason:
//
//   - TrackPress first, since an open list below sees events before
//     ConsumeOutsideClick: a list pick's resends, dragged onto the button row,
//     must not press a button.
//   - On a release, every self-routing member that is not a text field gets
//     it and drag.Release runs,
//     both above ConsumeOutsideClick: a release outside the box would
//     otherwise leave a latch set and the widget's next press swallowed.
//   - An open overlay (a DropDown's list) gets first refusal of everything but
//     a release: its list can overhang the box, and it covers the fields
//     below it.
//   - ConsumeOutsideClick, then ForwardReleaseToFocusedField: a release ends
//     here, a focused Editor's selection drag included.
//   - drag.Replay before any hit test, so a selection drag over the button
//     row does not press a button and does not freeze at the field's rect.
//   - The button row, then the drag-starting members, then the rest.
//
// A dialog with tabs, a scrollable pane or a mode (goSSMS's Connect, Backup,
// Restore) is not this shape and keeps its own HandleMouse.
func RingMouse(ev *tcell.EventMouse, d *ModalDialog, drag *FieldGesture,
	ring []core.Focusable, focus int, buttons []string,
	setFocus func(i int), press func(btn int)) bool {
	d.TrackPress(ev)
	if ev.Buttons() == tcell.ButtonNone {
		for _, f := range ring {
			// A text field is skipped: ForwardReleaseToFocusedField below
			// gives it its release, and a second could act on it.
			if _, text := f.(core.ClipboardTarget); text {
				continue
			}
			if m, ok := ringSelfRouting(f); ok {
				m.HandleMouse(ev)
			}
		}
		drag.Release(ev)
	}
	if ev.Buttons() != tcell.ButtonNone {
		for i, f := range ring {
			o, ok := f.(core.Overlay)
			if !ok || !o.OverlayActive() {
				continue
			}
			if m, ok := f.(core.MouseHandler); ok && m.HandleMouse(ev) {
				setFocus(i)
				return true
			}
			// One list is open at a time; a click outside it has closed it
			// and falls through to whatever it landed on.
			break
		}
	}
	if d.ConsumeOutsideClick(ev) {
		return true
	}
	var focused core.Focusable
	if focus >= 0 && focus < len(ring) {
		focused = ring[focus]
	}
	if ForwardReleaseToFocusedField(ev, focused) {
		return true
	}
	if ev.Buttons() != tcell.Button1 {
		return false
	}
	if drag.Replay(ev) {
		return true
	}
	if i := d.ButtonClicked(ev, buttons); i >= 0 {
		setFocus(len(ring))
		press(i)
		return true
	}
	mx, my := ev.Position()
	for i, f := range ring {
		if m, ok := ringDragStarter(f); ok && FocusableEnabled(f) && fieldHit(f, mx, my) {
			setFocus(i)
			drag.Claim(m, ev)
			return true
		}
	}
	for i, f := range ring {
		if m, ok := ringSelfRouting(f); ok && m.HandleMouse(ev) {
			setFocus(i)
			return true
		}
	}
	return true
}

// ringDragStarter is a RingMouse member whose press starts a drag the dialog
// must hold in its FieldGesture: one FieldGesture.Clear can cancel.
func ringDragStarter(f core.Focusable) (core.MouseHandler, bool) {
	if _, ok := f.(core.DragCanceller); !ok {
		return nil, false
	}
	m, ok := f.(core.MouseHandler)
	return m, ok
}

// ringSelfRouting is a RingMouse member that takes a press itself. A drag
// starter is excluded: its press goes through the FieldGesture, and its own
// HandleMouse would honour a latch left from a previous showing.
func ringSelfRouting(f core.Focusable) (core.MouseHandler, bool) {
	if _, ok := f.(core.DragCanceller); ok {
		return nil, false
	}
	m, ok := f.(core.MouseHandler)
	return m, ok
}
