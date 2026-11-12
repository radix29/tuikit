package propsheet

import (
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/controls"
	"github.com/radix29/tuikit/tuikittest"
)

// newGridForm builds a form taller than its rect with a focused GridRow, so
// a release can land on the grid rather than on the form's own bands.
func newGridForm() *Form {
	g := controls.NewDataGrid()
	g.SetData([]string{"A"}, [][]string{{"1"}, {"2"}, {"3"}, {"4"}, {"5"}, {"6"}})
	f := NewForm(Section("S1"), NewGridRow(g, 10), Section("S2"), Note("x"), Section("S3"), Note("y"))
	f.SetBounds(0, 0, 60, 12)
	f.Focus(true)
	f.setFocus(1)
	f.Draw(tuikittest.NewScreen(100, 40))
	return f
}

// A scrollbar drag released over the focused GridRow must still drop the
// gesture. The focused GridRow claims the release that ends its own drag, so a
// reset below the focused row's dispatch would never run.
func TestFormScrollbarLatchClearsOnReleaseOverGrid(t *testing.T) {
	f := newGridForm()

	f.HandleMouse(tcell.NewEventMouse(f.rect.Right()-1, 3, tcell.Button1, tcell.ModNone))
	if z, held := f.gesture.Owner(); !held || z != formBar {
		t.Fatal("press on the scrollbar column did not start a thumb drag")
	}
	f.HandleMouse(tcell.NewEventMouse(10, 4, tcell.ButtonNone, tcell.ModNone))
	if _, held := f.gesture.Owner(); held {
		t.Error("thumb drag still held after the release landed on the focused GridRow")
	}
}

// What the stuck latch actually cost: every later click in the form was
// taken as a scrollbar drag and jumped the scroll to the click's own row.
func TestFormClickAfterScrollbarDragDoesNotScroll(t *testing.T) {
	f := newGridForm()

	f.HandleMouse(tcell.NewEventMouse(f.rect.Right()-1, 1, tcell.Button1, tcell.ModNone))
	f.HandleMouse(tcell.NewEventMouse(10, 4, tcell.ButtonNone, tcell.ModNone))
	f.Draw(tuikittest.NewScreen(100, 40))
	before := f.scroll

	// A fresh press well down the form, nowhere near the scrollbar column.
	f.HandleMouse(tcell.NewEventMouse(10, 11, tcell.Button1, tcell.ModNone))
	if f.scroll != before {
		t.Errorf("click at x=10 changed scroll %d -> %d, want it unchanged", before, f.scroll)
	}
}

// Dragging the thumb to the track's last cell must reach maxScroll, as the
// wheel does. With row starts that don't land exactly on total-H (a two-line
// Section ahead of one-line rows), rounding the drag's line down to its row
// parked the form one row short of the bottom.
func TestFormScrollbarDragReachesBottom(t *testing.T) {
	var rows []Row
	for range 8 {
		rows = append(rows, Section("S"), Static("A", "1"))
	}
	f := NewForm(rows...)
	f.SetBounds(0, 0, 60, 8) // 24 lines, maxScroll's row starts at 17, not 16
	f.Draw(tuikittest.NewScreen(100, 40))
	w := f.contentWidth()
	want := f.maxScroll(w)

	f.HandleMouse(tcell.NewEventMouse(f.rect.Right()-1, 0, tcell.Button1, tcell.ModNone))
	f.HandleMouse(tcell.NewEventMouse(f.rect.Right()-1, f.rect.H-1, tcell.Button1, tcell.ModNone))
	if f.scroll != want {
		t.Errorf("drag to the track's end left scroll at %d, want maxScroll %d", f.scroll, want)
	}
	f.HandleMouse(tcell.NewEventMouse(f.rect.Right()-1, 0, tcell.Button1, tcell.ModNone))
	if f.scroll != 0 {
		t.Errorf("drag back to the track's top left scroll at %d, want 0", f.scroll)
	}
}

// A press claims the gesture for its row: a selection drag begun in one
// GridRow's grid that drifts onto the next GridRow neither moves that grid's
// selection nor moves focus to it, and a thumb drag dragged below the form
// still reaches the end.
func TestFormDragStaysWithTheRowItBeganIn(t *testing.T) {
	grid := func() *controls.DataGrid {
		g := controls.NewDataGrid()
		g.SetData([]string{"A"}, [][]string{{"1"}, {"2"}, {"3"}, {"4"}})
		g.SetCellCursor(true)
		return g
	}
	g1, g2 := grid(), grid()
	f := NewForm(NewGridRow(g1, 7), NewGridRow(g2, 7), Note("x"), Note("y"), Note("z"))
	f.SetBounds(0, 0, 40, 12)
	f.Focus(true)
	f.setFocus(0)
	f.Draw(tuikittest.NewScreen(100, 40))
	var y2 int
	for _, b := range f.bands {
		if b.row == 1 {
			y2 = b.y
		}
	}
	if y2 == 0 {
		t.Fatal("second GridRow not drawn")
	}
	f.HandleMouse(tcell.NewEventMouse(3, 2, tcell.Button1, tcell.ModNone)) // g1's first data row
	f.HandleMouse(tcell.NewEventMouse(3, y2+3, tcell.Button1, tcell.ModNone))
	if r := g2.SelectedRow(); r != 0 {
		t.Errorf("g2 selected row = %d after a drag from g1 crossed it, want 0", r)
	}
	if f.focus != 0 {
		t.Errorf("focus = %d after a drag from row 0 crossed row 1, want 0", f.focus)
	}
	f.HandleMouse(tcell.NewEventMouse(3, 30, tcell.ButtonNone, tcell.ModNone))

	f.HandleMouse(tcell.NewEventMouse(f.rect.Right()-1, 0, tcell.Button1, tcell.ModNone))
	f.HandleMouse(tcell.NewEventMouse(5, 40, tcell.Button1, tcell.ModNone))
	if want := f.maxScroll(f.contentWidth()); f.scroll != want {
		t.Errorf("scroll = %d after dragging the thumb below the form, want %d", f.scroll, want)
	}
}
