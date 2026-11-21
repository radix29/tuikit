package dialogs

import (
	"slices"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/tuikittest"
	"github.com/radix29/tuikit/widgets"
)

// fieldRingRig is a dialog on a FieldRing: a text field, a checkbox and a
// second text field over a three-button row whose default is the middle one,
// so a reset to the default is told apart from a reset to 0.
type fieldRingRig struct {
	ModalDialog
	ring      FieldRing
	name      *widgets.InputField
	cb        *widgets.CheckBox
	note      *widgets.InputField
	pressed   []int
	dismissed int
	disabled  []bool
}

var fieldRingButtons = []string{"Clear", "OK", "Cancel"}

func newFieldRingRig(t *testing.T) *fieldRingRig {
	t.Helper()
	d := &fieldRingRig{
		name: widgets.NewInputField("Name:", 10, false),
		cb:   widgets.NewCheckBox("Case"),
		note: widgets.NewInputField("Note:", 10, false),
	}
	d.ring = FieldRing{
		Fields:     func() []core.Focusable { return []core.Focusable{d.name, d.cb, d.note} },
		Buttons:    func() []string { return fieldRingButtons },
		Disabled:   func() []bool { return d.disabled },
		DefaultBtn: 1,
		Press:      func(btn int) { d.pressed = append(d.pressed, btn) },
		Dismiss:    func() { d.dismissed++ },
	}
	d.InitModal("Ring", 50, 9)
	d.SetScreen(tuikittest.NewScreen(80, 24))
	d.Show()
	in := d.InnerRect()
	d.name.SetBounds(in.X+1, in.Y)
	d.cb.SetBounds(in.X+1, in.Y+1)
	d.note.SetBounds(in.X+1, in.Y+2)
	d.ring.Reset()
	return d
}

func (d *fieldRingRig) focusedWidgets() []core.Focusable {
	var out []core.Focusable
	for _, f := range d.ring.Fields() {
		if f.Focused() {
			out = append(out, f)
		}
	}
	return out
}

func TestFieldRingTabWrapsThroughTheButtonRow(t *testing.T) {
	d := newFieldRingRig(t)
	if d.ring.FocusIdx != 0 || d.ring.BtnFocus != 1 || !d.name.Focused() {
		t.Fatalf("after Reset: focus %d btn %d, want 0 and the default", d.ring.FocusIdx, d.ring.BtnFocus)
	}
	var stops []int
	for range 5 {
		d.ring.Key(key(tcell.KeyTab))
		stops = append(stops, d.ring.FocusIdx)
	}
	if want := []int{1, 2, 3, 0, 1}; !slices.Equal(stops, want) {
		t.Errorf("Tab stops = %v, want %v", stops, want)
	}
	d.ring.SetFocus(0)
	d.ring.Key(key(tcell.KeyBacktab))
	if !d.ring.OnButtons() || d.ring.Focused() != nil || len(d.focusedWidgets()) != 0 {
		t.Errorf("Backtab from the first field: focus %d, focused widgets %v, want the button row and none",
			d.ring.FocusIdx, d.focusedWidgets())
	}
}

func TestFieldRingLeavingTheRowResetsTheDefaultButton(t *testing.T) {
	d := newFieldRingRig(t)
	d.ring.SetFocus(3)
	d.ring.Key(key(tcell.KeyRight))
	if d.ring.BtnFocus != 2 {
		t.Fatalf("Right on the row: btn %d, want 2", d.ring.BtnFocus)
	}
	d.ring.Key(key(tcell.KeyUp))
	if d.ring.FocusIdx != 2 || d.ring.BtnFocus != 1 {
		t.Errorf("Up off the row: focus %d btn %d, want 2 and the default", d.ring.FocusIdx, d.ring.BtnFocus)
	}
	d.ring.Key(key(tcell.KeyEnter))
	if !slices.Equal(d.pressed, []int{1}) {
		t.Errorf("Enter in a text field pressed %v, want the default [1]", d.pressed)
	}
}

func TestFieldRingButtonRowSkipsDisabledButtons(t *testing.T) {
	d := newFieldRingRig(t)
	d.disabled = []bool{false, false, true}
	d.ring.SetFocus(3)
	d.ring.Key(key(tcell.KeyRight))
	if d.ring.BtnFocus != 1 {
		t.Errorf("Right toward a disabled last button: btn %d, want 1 unchanged", d.ring.BtnFocus)
	}
}

func TestFieldRingEnterGoesToTheWidgetFirst(t *testing.T) {
	d := newFieldRingRig(t)
	d.ring.SetFocus(1)
	d.ring.Key(key(tcell.KeyEnter))
	if !d.cb.Checked() || len(d.pressed) != 0 {
		t.Errorf("Enter on the checkbox: checked %v, pressed %v; want toggled, nothing pressed", d.cb.Checked(), d.pressed)
	}
}

func TestFieldRingEscapeDismissesFromAFieldAndTheRow(t *testing.T) {
	d := newFieldRingRig(t)
	d.ring.Key(key(tcell.KeyEscape))
	d.ring.SetFocus(3)
	d.ring.Key(key(tcell.KeyEscape))
	if d.dismissed != 2 {
		t.Errorf("dismissed %d times, want 2", d.dismissed)
	}
}

func TestFieldRingStepSkipsADisabledField(t *testing.T) {
	d := newFieldRingRig(t)
	d.cb.SetEnabled(false)
	d.ring.Key(key(tcell.KeyTab))
	if d.ring.FocusIdx != 2 {
		t.Errorf("Tab past a disabled checkbox: focus %d, want 2", d.ring.FocusIdx)
	}
}

func TestFieldRingMouseButtonTakesFocusBeforePress(t *testing.T) {
	d := newFieldRingRig(t)
	var btnAtPress int
	d.ring.Press = func(btn int) { btnAtPress = d.ring.BtnFocus; d.pressed = append(d.pressed, btn) }
	x := d.ButtonRowStartX(fieldRingButtons) + 2
	d.ring.Mouse(tcell.NewEventMouse(x, d.ButtonRowY(), tcell.Button1, tcell.ModNone), &d.ModalDialog)
	if !slices.Equal(d.pressed, []int{0}) || btnAtPress != 0 || !d.ring.OnButtons() {
		t.Errorf("click on Clear: pressed %v, BtnFocus at press %d, focus %d; want [0], 0, the row",
			d.pressed, btnAtPress, d.ring.FocusIdx)
	}
}

func TestFieldRingClickInAFieldClaimsTheGestureAndResetClearsIt(t *testing.T) {
	d := newFieldRingRig(t)
	b := d.note.Bounds()
	d.ring.Mouse(tcell.NewEventMouse(b.Right()-2, b.Y, tcell.Button1, tcell.ModNone), &d.ModalDialog)
	if d.ring.FocusIdx != 2 || d.ring.Drag.Field() == nil {
		t.Fatalf("press in Note: focus %d, drag %v; want 2 and a claimed gesture", d.ring.FocusIdx, d.ring.Drag.Field())
	}
	d.ring.Reset()
	if d.ring.Drag.Field() != nil || d.ring.FocusIdx != 0 || !d.name.Focused() || d.note.Focused() {
		t.Errorf("Reset: drag %v focus %d; want cleared and the first field alone focused", d.ring.Drag.Field(), d.ring.FocusIdx)
	}
}

func TestFieldRingClipboard(t *testing.T) {
	d := newFieldRingRig(t)
	cb := d.cb.Bounds()
	if d.ring.FocusClipboardTargetAt(cb.X, cb.Y) || d.ring.FocusIdx != 0 {
		t.Errorf("right-click on the checkbox took focus: %d", d.ring.FocusIdx)
	}
	nb := d.note.Bounds()
	if !d.ring.FocusClipboardTargetAt(nb.Right()-2, nb.Y) || d.ring.FocusIdx != 2 {
		t.Fatalf("right-click on Note: focus %d, want 2", d.ring.FocusIdx)
	}
	if d.ring.FocusedClipboardTarget() != core.ClipboardTarget(d.note) {
		t.Errorf("FocusedClipboardTarget = %v, want Note", d.ring.FocusedClipboardTarget())
	}
	d.ring.SetFocus(1)
	if tgt := d.ring.FocusedClipboardTarget(); tgt != nil {
		t.Errorf("FocusedClipboardTarget on the checkbox = %v, want nil", tgt)
	}
	d.note.SetEnabled(false)
	if d.ring.FocusClipboardTargetAt(nb.Right()-2, nb.Y) {
		t.Error("right-click on a disabled field took focus")
	}
}

func TestFieldRingRefusesDisabledButton(t *testing.T) {
	d := newFieldRingRig(t)
	d.disabled = []bool{false, true, false} // the default (OK) is greyed

	d.ring.Key(key(tcell.KeyEnter)) // Enter in a field
	if len(d.pressed) != 0 {
		t.Errorf("Enter in a field pressed %v, want nothing (default is disabled)", d.pressed)
	}

	d.ring.SetFocus(2)
	d.ring.Key(key(tcell.KeyTab)) // onto the row
	if !d.ring.OnButtons() || d.ring.BtnFocus != 2 {
		t.Errorf("entering the row: btn %d, want 2 (nearest enabled after the default)", d.ring.BtnFocus)
	}
	d.ring.BtnFocus = 1
	d.ring.Key(key(tcell.KeyEnter))
	if len(d.pressed) != 0 {
		t.Errorf("Enter on a disabled button pressed %v, want nothing", d.pressed)
	}

	x := d.ButtonRowStartX(fieldRingButtons) + len(fieldRingButtons[0]) + 4 + 2
	d.ring.Mouse(tcell.NewEventMouse(x, d.ButtonRowY(), tcell.Button1, tcell.ModNone), &d.ModalDialog)
	if len(d.pressed) != 0 {
		t.Errorf("click on a disabled button pressed %v, want nothing", d.pressed)
	}
}

func TestButtonRowKeysRefuseDisabledEnter(t *testing.T) {
	disabled := []bool{false, true}
	for name, run := range map[string]func(fire func()){
		"ButtonRowKey": func(fire func()) {
			f := 1
			ButtonRowKey(key(tcell.KeyEnter), &f, 2, disabled, fire, func() {}, func(int) {})
		},
		"ButtonsKey": func(fire func()) {
			f := 1
			ButtonsKey(key(tcell.KeyEnter), &f, 2, disabled, fire, func() {})
		},
	} {
		fired := false
		run(func() { fired = true })
		if fired {
			t.Errorf("%s fired a disabled focused button", name)
		}
	}
}
