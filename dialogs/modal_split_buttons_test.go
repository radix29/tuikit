package dialogs

import (
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/tuikittest"
)

var (
	splitLeft  = []string{"Delete", "Profile..."}
	splitRight = []string{"Reset", "Connect", "Cancel"}
)

// splitDialog is a Connect-shaped dialog (78 wide) on a w-column terminal,
// drawn with its split button row.
func splitDialog(t *testing.T, w int) (*ModalDialog, *tuikittest.Screen) {
	t.Helper()
	rec := tuikittest.NewScreen(w, 30)
	d := &ModalDialog{}
	d.InitModal(rec, "Connect", 78, 20)
	d.Show()
	cs := core.NewClipScreen(rec)
	d.DrawBase(cs)
	d.DrawSplitButtons(cs, splitLeft, splitRight, -1, nil)
	return d, rec
}

// At its requested width both groups share the button row, the left one at
// the left margin and the right one where DrawButtons would put it.
func TestSplitButtonsShareTheRowWhenTheyFit(t *testing.T) {
	d, rec := splitDialog(t, 100)
	y := d.ButtonRowY()
	if x, row, ok := rec.Find("[ Delete ]"); !ok || row != y || x != d.InnerRect().X+1 {
		t.Errorf("Delete at (%d,%d) ok=%v, want (%d,%d)", x, row, ok, d.InnerRect().X+1, y)
	}
	if x, row, ok := rec.Find("[ Reset ]"); !ok || row != y || x != d.ButtonRowStartX(splitRight) {
		t.Errorf("Reset at (%d,%d) ok=%v, want (%d,%d)", x, row, ok, d.ButtonRowStartX(splitRight), y)
	}
}

// Clamped onto 60 columns the groups used to overlap ("[ Profile... ]set ]");
// the left group now drops to the row below, and both stay whole and
// clickable where they are drawn.
func TestSplitButtonsNeverOverlapOnANarrowTerminal(t *testing.T) {
	for _, w := range []int{60, 55, 50, 46} {
		d, rec := splitDialog(t, w)
		y := d.ButtonRowY()
		for i, label := range append(append([]string{}, splitLeft...), splitRight...) {
			text := "[ " + label + " ]"
			x, row, ok := rec.Find(text)
			if !ok {
				t.Errorf("w=%d: %q not drawn whole:\n%s", w, text, rec.Snapshot())
				continue
			}
			if i < len(splitLeft) && row != y+1 || i >= len(splitLeft) && row != y {
				t.Errorf("w=%d: %q on row %d (button row %d)", w, text, row, y)
			}
			ev := tcell.NewEventMouse(x+1, row, tcell.Button1, tcell.ModNone)
			if got := d.SplitButtonClicked(ev, splitLeft, splitRight); got != i {
				t.Errorf("w=%d: click on %q = %d, want %d", w, text, got, i)
			}
			d.TrackPress(tcell.NewEventMouse(x+1, row, tcell.ButtonNone, tcell.ModNone))
		}
	}
}

// The gap is where a dialog draws status between the groups; it must end
// before the right group and start after the left one when they share a row.
func TestSplitButtonLayoutGap(t *testing.T) {
	for _, w := range []int{100, 60} {
		d, _ := splitDialog(t, w)
		l := d.SplitButtonLayout(splitLeft, splitRight)
		if l.GapX+l.GapW > l.RightX-2 {
			t.Errorf("w=%d: gap [%d,%d) runs into the right group at %d", w, l.GapX, l.GapX+l.GapW, l.RightX)
		}
		if l.LeftY == l.RightY && l.GapX < l.LeftX+d.ButtonRowWidth(splitLeft)+2 {
			t.Errorf("w=%d: gap starts at %d, inside the left group", w, l.GapX)
		}
	}
}

// A press that clicked one group's button is spent: it does not click the
// other group too, and its held resends click nothing.
func TestSplitButtonClickedConsumesThePress(t *testing.T) {
	d, rec := splitDialog(t, 100)
	x, y, _ := rec.Find("[ Delete ]")
	ev := tcell.NewEventMouse(x+1, y, tcell.Button1, tcell.ModNone)
	if got := d.SplitButtonClicked(ev, splitLeft, splitRight); got != 0 {
		t.Fatalf("click = %d, want 0", got)
	}
	rx, _, _ := rec.Find("[ Cancel ]")
	if got := d.SplitButtonClicked(tcell.NewEventMouse(rx+1, y, tcell.Button1, tcell.ModNone), splitLeft, splitRight); got != -1 {
		t.Errorf("held press dragged onto Cancel = %d, want -1", got)
	}
}
