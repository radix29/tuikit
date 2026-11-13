package dialogs

import (
	"testing"

	"github.com/gdamore/tcell/v3"
)

// latchedControl stands in for any control a dialog hands FieldGesture (IN7:
// it held only an InputField, so a ListBox or Editor in a dialog had no
// protection from a drag wandering onto another control).
type latchedControl struct {
	events    []tcell.ButtonMask
	cancelled bool
}

func (c *latchedControl) HandleMouse(ev *tcell.EventMouse) bool {
	c.events = append(c.events, ev.Buttons())
	return true
}

func (c *latchedControl) CancelMouseDrag() { c.cancelled = true }

func TestFieldGestureHoldsAnyMouseHandler(t *testing.T) {
	var g FieldGesture
	c := &latchedControl{}
	g.Claim(c, tcell.NewEventMouse(1, 1, tcell.Button1, tcell.ModNone))
	if !g.Replay(tcell.NewEventMouse(30, 9, tcell.Button1, tcell.ModNone)) {
		t.Fatal("a held drag's resend was not replayed to the control that claimed it")
	}
	g.Release(tcell.NewEventMouse(30, 9, tcell.ButtonNone, tcell.ModNone))
	if want := []tcell.ButtonMask{tcell.Button1, tcell.Button1, tcell.ButtonNone}; len(c.events) != 3 ||
		c.events[0] != want[0] || c.events[1] != want[1] || c.events[2] != want[2] {
		t.Fatalf("control saw %v, want %v", c.events, want)
	}
	if g.Field() != nil {
		t.Fatal("the gesture is still held after the release")
	}

	g.Claim(c, tcell.NewEventMouse(1, 1, tcell.Button1, tcell.ModNone))
	g.Clear()
	if !c.cancelled || g.Field() != nil {
		t.Fatalf("Clear: cancelled = %v, held = %v; want the control's latch dropped and nothing held", c.cancelled, g.Field() != nil)
	}
}
