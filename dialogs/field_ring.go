package dialogs

import (
	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/core"
)

// FieldRing is the state and input handling of a flat-ring dialog: a dialog
// that lays out its own widgets, drives focus with a []core.Focusable in Tab
// order followed by its button row (one stop, crossed with the arrows), and
// has no tabs, scrollable pane or mode of its own. It bundles what such a
// dialog otherwise repeats around the focus_ring.go operations: the focused
// stop, the button the row would press, the text-selection gesture, and the
// keyboard (FieldRingKey/ButtonRowKey), mouse (RingMouse) and clipboard
// (ClipboardFieldAt/FocusedClipboardTarget) wiring over them.
//
// A dialog holds one as a field, sets the callbacks once at construction, and
// forwards HandleKey, HandleMouse and the two clipboard methods to it. Only the
// field list, the button labels, what a press does and how the dialog closes
// differ between dialogs; drawing stays the dialog's (it reads BtnFocus and
// OnButtons). A dialog with tabs, a scrollable pane or a mode keeps its own
// handling and uses the free functions piecemeal.
type FieldRing struct {
	// Fields is the ring's widgets in Tab order. It is called on every use, so a
	// dialog whose fields change with its mode or its rows answers the current
	// ones.
	Fields func() []core.Focusable
	// Buttons is the button row's labels, as the dialog draws them.
	Buttons func() []string
	// Disabled is the button row's disabled flags, as the dialog hands
	// DrawButtonsGated; nil, or a nil or short answer, means all enabled.
	Disabled func() []bool
	// DefaultBtn is the button Enter presses while focus is in the fields:
	// BtnFocus returns to it whenever focus leaves the button row.
	DefaultBtn int
	// Press fires button btn. BtnFocus is already btn when it runs.
	Press func(btn int)
	// Dismiss closes the dialog on Escape — usually its Hide, or a wrapper
	// that also drops a pending callback.
	Dismiss func()

	// FocusIdx is the focused stop: an index into Fields, or len(Fields())
	// for the button row.
	FocusIdx int
	// BtnFocus is the button the row would press.
	BtnFocus int
	// Drag is the text-selection gesture a press in one of the fields starts;
	// see FieldGesture for the ordering its three calls depend on.
	Drag FieldGesture
}

// Reset readies the ring for a new showing: the first field focused, BtnFocus
// on the default button, and any gesture latch cleared — a dialog dismissed
// mid-drag would otherwise reopen still routing every click to that field.
func (r *FieldRing) Reset() {
	r.BtnFocus = r.DefaultBtn
	r.Drag.Clear()
	r.SetFocus(0)
}

// OnButtons reports whether the button row holds focus.
func (r *FieldRing) OnButtons() bool { return r.FocusIdx == len(r.Fields()) }

// Focused is the focused field, or nil on the button row.
func (r *FieldRing) Focused() core.Focusable {
	fields := r.Fields()
	if r.FocusIdx < 0 || r.FocusIdx >= len(fields) {
		return nil
	}
	return fields[r.FocusIdx]
}

// SetFocus moves focus to stop i — a field, or the button row at
// len(Fields()) — through SetFocusIn, so the target is never blurred on its
// way to focus (a DropDown's Focus(false) closes its list). It does not touch
// BtnFocus: a click on a field leaves the row's choice where it was, as Tab
// does not (Step).
func (r *FieldRing) SetFocus(i int) {
	r.FocusIdx = SetFocusIn(r.Fields(), i, i)
}

// Step moves focus one stop in dir (RingStep: wrapping, past disabled
// fields). Landing on a field resets BtnFocus to DefaultBtn, so Enter from a
// field presses the default.
func (r *FieldRing) Step(dir int) {
	r.SetFocus(RingStep(r.Fields(), r.FocusIdx, dir))
	if !r.OnButtons() {
		r.BtnFocus = r.DefaultBtn
		return
	}
	r.snapBtnFocus()
}

// disabled is the row's disabled flags, nil when the dialog sets none.
func (r *FieldRing) disabled() []bool {
	if r.Disabled == nil {
		return nil
	}
	return r.Disabled()
}

// snapBtnFocus moves BtnFocus off a disabled button to the nearest enabled
// one — after it first, then before it — so the focus highlight never sits on
// a greyed button. It stays put when every button is disabled.
func (r *FieldRing) snapBtnFocus() {
	d := r.disabled()
	if !buttonDisabled(d, r.BtnFocus) {
		return
	}
	n := len(r.Buttons())
	for _, dir := range [...]int{+1, -1} {
		if j := StepButton(r.BtnFocus, dir, n, d); j != r.BtnFocus {
			r.BtnFocus = j
			return
		}
	}
}

// press fires the focused button, unless it is disabled.
func (r *FieldRing) press() {
	if buttonDisabled(r.disabled(), r.BtnFocus) {
		return
	}
	r.Press(r.BtnFocus)
}

// Key is the dialog's keyboard: ButtonRowKey on the button row, FieldRingKey
// on a field (the focused widget first, so an open dropdown takes the arrows,
// Enter and Escape). It always reports true — the dialog is modal. A key the
// dialog owns ahead of the ring (Find's F3) is the dialog's to test first.
func (r *FieldRing) Key(ev *tcell.EventKey) bool {
	if r.OnButtons() {
		return ButtonRowKey(ev, &r.BtnFocus, len(r.Buttons()), r.disabled(), r.press, r.Dismiss, r.Step)
	}
	return FieldRingKey(ev, r.Focused(), r.press, r.Dismiss, r.Step)
}

// Mouse is the dialog's mouse: RingMouse over the ring, d being the dialog's
// own ModalDialog. A clicked button takes BtnFocus before Press runs.
func (r *FieldRing) Mouse(ev *tcell.EventMouse, d *ModalDialog) bool {
	return RingMouse(ev, d, &r.Drag, r.Fields(), r.FocusIdx, r.Buttons(),
		r.SetFocus,
		func(btn int) {
			if buttonDisabled(r.disabled(), btn) {
				return
			}
			r.BtnFocus = btn
			r.Press(btn)
		})
}

// FocusClipboardTargetAt implements core.ClipboardFocuser: a right-click on
// one of the ring's text fields (ClipboardFieldAt — enabled, under no open
// list) focuses it.
func (r *FieldRing) FocusClipboardTargetAt(x, y int) bool {
	i := ClipboardFieldAt(r.Fields(), x, y)
	if i < 0 {
		return false
	}
	r.SetFocus(i)
	return true
}

// FocusedClipboardTarget implements core.ClipboardHost: the focused field when
// it is a text field, nil otherwise (the button row, a checkbox, a dropdown).
func (r *FieldRing) FocusedClipboardTarget() core.ClipboardTarget {
	return FocusedClipboardTarget(r.Fields(), r.FocusIdx)
}
