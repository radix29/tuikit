package controls

import (
	"strconv"
	"testing"

	"github.com/gdamore/tcell/v3"
)

// gestureZone returns the zone owning the held press, nil when none is held.
func (g *DataGrid) gestureZone() *gridZone {
	if z, ok := g.gesture.Owner(); ok {
		return &z
	}
	return nil
}

// gestureIs reports whether a press is held and z owns it.
func (g *DataGrid) gestureIs(z gridZone) bool {
	got := g.gestureZone()
	return got != nil && *got == z
}

// newTallWideGrid is a read-only cell-cursor grid, 40x10 (7 data rows), with
// 50 rows and 8 columns of width 8, so both scrollbars show.
func newTallWideGrid() *DataGrid {
	g := newTestDataGrid()
	cols := make([]string, 8)
	for i := range cols {
		cols[i] = "col" + strconv.Itoa(i) + "xxx"
	}
	rows := make([][]string, 50)
	for r := range rows {
		rows[r] = make([]string, len(cols))
		for c := range cols {
			rows[r][c] = strconv.Itoa(r)
		}
	}
	g.SetData(cols, rows)
	g.SetCellCursor(true)
	g.SetStatus("")
	return g
}

func mouse(g *DataGrid, x, y int, b tcell.ButtonMask) bool {
	return g.HandleMouse(tcell.NewEventMouse(x, y, b, tcell.ModNone))
}

// BG2: a cell-selection drag owns its gesture. Drifting onto a header
// separator, the vertical scrollbar column, the status row's horizontal bar or
// a sortable header must not start what a press there would.
func TestDataGridSelectionDragDoesNotGrabOtherZones(t *testing.T) {
	t.Run("separator", func(t *testing.T) {
		g := newTallWideGrid()
		sep := g.colWidths[0]
		mouse(g, 2, 3, tcell.Button1)
		mouse(g, sep-1, 0, tcell.Button1) // column 0's separator on the header row
		mouse(g, sep+4, 0, tcell.Button1)
		if g.colWidths[0] != sep {
			t.Errorf("colWidths[0] = %d after a selection drag crossed its separator, want %d", g.colWidths[0], sep)
		}
	})
	t.Run("vertical bar", func(t *testing.T) {
		g := newTallWideGrid()
		mouse(g, 2, 3, tcell.Button1)
		mouse(g, g.rect.Right()-1, 8, tcell.Button1) // the bar's last track cell
		if g.scrollRow > 1 {
			t.Errorf("scrollRow = %d after a selection drag reached the bar, want the selection to own the drag", g.scrollRow)
		}
		if !g.gestureIs(gridCells) {
			t.Errorf("gesture zone = %v, want gridCells", g.gestureZone())
		}
	})
	t.Run("horizontal bar", func(t *testing.T) {
		g := newTallWideGrid()
		_, y, _, _, _, _, ok := g.hScrollbar()
		if !ok {
			t.Fatal("fixture has no horizontal bar")
		}
		mouse(g, 2, 3, tcell.Button1)
		mouse(g, 30, y, tcell.Button1) // far along the track
		if g.scrollCol != 0 {
			t.Errorf("scrollCol = %d after a selection drag reached the status row, want 0", g.scrollCol)
		}
	})
	t.Run("sortable header", func(t *testing.T) {
		g := newTallWideGrid()
		g.SetCellCursor(false)
		clicks := 0
		g.OnHeaderClick = func(int) { clicks++ }
		mouse(g, 2, 4, tcell.Button1)
		mouse(g, 2, 0, tcell.Button1)
		mouse(g, 3, 0, tcell.Button1)
		if clicks != 0 {
			t.Errorf("OnHeaderClick fired %d times during a row drag, want 0", clicks)
		}
		mouse(g, 3, 0, tcell.ButtonNone)
		mouse(g, 3, 0, tcell.Button1)
		if clicks != 1 {
			t.Errorf("OnHeaderClick fired %d times for a fresh press on the header, want 1", clicks)
		}
	})
}

// BG2: the zones arm only on a fresh press; a drag begun on the header or the
// status row does not turn into a selection on reaching the cells.
func TestDataGridPressOnInertZoneSwallowsTheDrag(t *testing.T) {
	g := newTallWideGrid()
	g.SetSelectedCell(0, 0)
	mouse(g, 2, 1, tcell.Button1) // the separator line under the header
	mouse(g, 20, 5, tcell.Button1)
	if r, c := g.SelectedCell(); r != 0 || c != 0 || g.blockSelecting {
		t.Errorf("selection = (%d,%d) block=%v after a drag from the separator line, want (0,0) unchanged", r, c, g.blockSelecting)
	}
}

// BG3: a thumb dragged below the grid keeps control and reaches the end.
func TestDataGridThumbDragPastTheEdge(t *testing.T) {
	g := newTallWideGrid()
	mouse(g, g.rect.Right()-1, 2, tcell.Button1)
	if !mouse(g, g.rect.Right()-1, g.rect.Bottom()+5, tcell.Button1) {
		t.Fatal("a held thumb drag below the grid was not consumed")
	}
	if want := g.rows.Len() - g.dataRows(); g.scrollRow != want {
		t.Errorf("scrollRow = %d, want %d (the end)", g.scrollRow, want)
	}
	if !mouse(g, 0, 50, tcell.ButtonNone) || g.gestureZone() != nil {
		t.Error("a release far outside did not end the thumb drag")
	}
}

// BG3: a selection dragged past an edge scrolls one row or column per event
// toward the pointer and extends the selection onto what scrolls in.
func TestDataGridSelectionDragPastTheEdgeScrolls(t *testing.T) {
	g := newTallWideGrid()
	mouse(g, 2, 2, tcell.Button1) // row 0, col 0
	below := g.rect.Bottom() + 3
	for range 5 {
		mouse(g, 2, below, tcell.Button1)
	}
	dataH := g.dataRows()
	if g.scrollRow != 5 || g.selRow != dataH-1+5 {
		t.Errorf("after 5 events below: scrollRow=%d selRow=%d, want 5 and %d", g.scrollRow, g.selRow, dataH-1+5)
	}
	r0, _, r1, _ := g.SelectionBounds()
	if r0 != 0 || r1 != g.selRow {
		t.Errorf("selection rows %d..%d, want 0..%d", r0, r1, g.selRow)
	}
	// Back up through the header: one row per event toward the top.
	for range 3 {
		mouse(g, 2, 0, tcell.Button1)
	}
	if g.scrollRow != 2 || g.selRow != 2 {
		t.Errorf("after 3 events above: scrollRow=%d selRow=%d, want 2 and 2", g.scrollRow, g.selRow)
	}
	// Right, past the grid: one column per event.
	for range 2 {
		mouse(g, g.rect.Right()+4, 4, tcell.Button1)
	}
	if g.scrollCol == 0 || g.selCol <= 4 {
		t.Errorf("after 2 events to the right: scrollCol=%d selCol=%d, want scrolled and past column 4", g.scrollCol, g.selCol)
	}
	// Left, onto the gutter-less left edge and beyond.
	for range 10 {
		mouse(g, -3, 4, tcell.Button1)
	}
	if g.scrollCol != 0 || g.selCol != 0 {
		t.Errorf("after dragging left: scrollCol=%d selCol=%d, want 0 and 0", g.scrollCol, g.selCol)
	}
	mouse(g, -3, 4, tcell.ButtonNone)
	if g.gestureZone() != nil {
		t.Error("release outside did not end the selection drag")
	}
}

// A row-header drag past the bottom keeps selecting whole rows as it scrolls.
func TestDataGridRowHeaderDragPastTheEdge(t *testing.T) {
	g := newTallWideGrid()
	g.SetRowNumbers(true)
	mouse(g, 0, 2, tcell.Button1)
	for range 3 {
		mouse(g, 0, g.rect.Bottom()+1, tcell.Button1)
	}
	r0, c0, r1, c1 := g.SelectionBounds()
	if r0 != 0 || c0 != 0 || r1 != g.dataRows()-1+3 || c1 != len(g.columns)-1 {
		t.Errorf("selection = (%d,%d)-(%d,%d), want whole rows 0..%d", r0, c0, r1, c1, g.dataRows()-1+3)
	}
	if g.scrollCol != 0 {
		t.Errorf("scrollCol = %d, a row-header drag must not scroll sideways", g.scrollCol)
	}
}

// A toggle grid's drag paints only cells under the pointer: dragging below the
// grid neither scrolls nor toggles a cell the user cannot see.
func TestDataGridToggleDragDoesNotPaintPastTheEdge(t *testing.T) {
	g := newTallWideGrid()
	var got [][2]int
	g.OnActivateCell = func(r, c int) { got = append(got, [2]int{r, c}) }
	mouse(g, 2, 2, tcell.Button1)
	mouse(g, 2, 3, tcell.Button1)
	mouse(g, 2, g.rect.Bottom()+2, tcell.Button1)
	if len(got) != 2 || g.scrollRow != 0 {
		t.Errorf("activations %v scrollRow %d, want two cells and no scroll", got, g.scrollRow)
	}
}

// Every reset path that leaves the old rows behind ends a held gesture.
func TestDataGridResetsEndTheGesture(t *testing.T) {
	for name, reset := range map[string]func(*DataGrid){
		"SetData":       func(g *DataGrid) { g.SetData([]string{"A"}, [][]string{{"1"}}) },
		"SetError":      func(g *DataGrid) { g.SetError(errTest{"x"}) },
		"SetBrowseOnly": func(g *DataGrid) { g.SetBrowseOnly(true) },
		"ShowValue":     func(g *DataGrid) { g.ShowValue() },
	} {
		g := newTallWideGrid()
		mouse(g, 2, 3, tcell.Button1)
		reset(g)
		if g.gestureZone() != nil {
			t.Errorf("%s left the gesture held", name)
		}
	}
}
