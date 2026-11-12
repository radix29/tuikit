package propsheet

import (
	"testing"

	"github.com/gdamore/tcell/v3"

	"github.com/radix29/tuikit/tuikittest"
)

// Moving the pointer across the page list, or across the form's grid, must
// not move keyboard focus: the list reported plain motion as handled and the
// sheet read that as a click, so keys typed into a field went to the list.
// Only a press changes zone; a wheel tick scrolls what it is over and leaves
// focus alone too.
func TestSheetFocusZoneMovesOnlyOnAPress(t *testing.T) {
	p := newScrollableSheet(t)
	p.Draw(tuikittest.NewScreen(100, 40))
	list := p.pageList.Bounds()
	lx, ly := list.X+1, list.Y

	p.setZone(zoneForm)
	for _, b := range []tcell.ButtonMask{tcell.ButtonNone, tcell.WheelDown, tcell.WheelUp} {
		p.HandleMouse(tuikittest.Mouse(lx, ly, b))
		if p.zone != zoneForm {
			t.Fatalf("zone = %v after %v over the page list, want zoneForm", p.zone, b)
		}
	}

	p.setZone(zonePages)
	fr := p.PageForm(0).Bounds()
	gx, gy := fr.X+3, fr.Y+2
	p.HandleMouse(tuikittest.Mouse(gx, gy, tcell.ButtonNone))
	if p.zone != zonePages {
		t.Fatalf("zone = %v after hovering the form's grid, want zonePages", p.zone)
	}

	p.setZone(zoneForm)
	p.HandleMouse(tuikittest.Press(lx, ly))
	p.HandleMouse(tuikittest.Release(lx, ly))
	if p.zone != zonePages {
		t.Fatalf("zone = %v after a click on the page list, want zonePages", p.zone)
	}
}
