package controls

import (
	"slices"
	"strconv"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/tuikittest"
)

// liveRows is a RowSource whose rows change under the grid, as a live event
// view's do when its store evicts the oldest: the grid holds the pointer, so a
// host's trim is visible to it before DropLeadingRows is called.
type liveRows struct{ rows *[][]string }

func (s liveRows) Len() int           { return len(*s.rows) }
func (s liveRows) Row(i int) []string { return (*s.rows)[i] }

// idRows is n rows whose single cell is the row's original index, so a test
// can tell which rows a selection covers after some were dropped.
func idRows(n int) [][]string {
	rows := make([][]string, n)
	for i := range rows {
		rows[i] = []string{strconv.Itoa(i)}
	}
	return rows
}

// newLiveBlockGrid is a 20-row read-only cell-cursor grid over a live
// source, with rows 10..13 Shift-selected (cursor on 13, anchor on 10).
func newLiveBlockGrid() (*DataGrid, *[][]string) {
	rows := idRows(20)
	g := newTestDataGrid()
	g.SetSource([]string{"ID"}, liveRows{&rows})
	g.SetCellCursor(true)
	g.SetSelectedCell(10, 0)
	for range 3 {
		g.HandleKey(tuikittest.Key(tcell.KeyDown, tcell.ModShift))
	}
	return g, &rows
}

// selectedIDs is the cell text of the rows the selection covers.
func selectedIDs(g *DataGrid) []string {
	var ids []string
	for _, r := range g.SelectedRows() {
		ids = append(ids, g.Row(r)[0])
	}
	return ids
}

func wantRowBounds(t *testing.T, g *DataGrid, r0, r1 int, what string) {
	t.Helper()
	if g0, _, g1, _ := g.SelectionBounds(); g0 != r0 || g1 != r1 {
		t.Fatalf("%s: SelectionBounds rows %d..%d, want %d..%d", what, g0, g1, r0, r1)
	}
}

// A programmatic cursor move collapses a Shift-extended block onto the new
// cursor. It used to move only the cursor end, so a live view's "keep the
// cursor on the same event" after a trim re-shaped the block onto other rows
// (10..13 became 9..10), and following the tail stretched it to the end.
func TestSetSelectedCellCollapsesBlockSelection(t *testing.T) {
	g, _ := newLiveBlockGrid()
	wantRowBounds(t, g, 10, 13, "fixture")

	g.SetSelectedCell(9, 0)
	wantRowBounds(t, g, 9, 9, "SetSelectedCell")
	wantRows(t, g, []int{9}, "SetSelectedCell")

	g, _ = newLiveBlockGrid()
	g.SetSelectedRow(19)
	wantRowBounds(t, g, 19, 19, "SetSelectedRow")

	// Ctrl+click marks go too.
	g, _ = newLiveBlockGrid()
	g.markRow(2)
	g.SetSelectedCell(5, 0)
	wantRows(t, g, []int{5}, "SetSelectedCell after marks")
}

// DropLeadingRows keeps the selection on the same rows when the host removes
// rows from the front: block, marks and scroll all shift with them.
func TestDropLeadingRowsKeepsTheSameRowsSelected(t *testing.T) {
	g, rows := newLiveBlockGrid()
	scroll := g.ScrollRow()
	want := selectedIDs(g)

	*rows = (*rows)[4:]
	g.DropLeadingRows(4)
	wantRowBounds(t, g, 6, 9, "block")
	if got := selectedIDs(g); !slices.Equal(got, want) {
		t.Errorf("block covers %v, want %v", got, want)
	}
	if got := g.ScrollRow(); got != scroll-4 {
		t.Errorf("ScrollRow() = %d, want %d", got, scroll-4)
	}

	// Ctrl+click marks: a run 10..13 with 12 unpicked and 2 picked. Dropping
	// 4 keeps 10, 11, 13 (now 6, 7, 9) and drops 2.
	g, rows = newLiveBlockGrid()
	g.markRow(12)
	g.markRow(2)
	*rows = (*rows)[4:]
	g.DropLeadingRows(4)
	if got, want := selectedIDs(g), []string{"10", "11", "13"}; !slices.Equal(got, want) {
		t.Errorf("marks cover %v, want %v", got, want)
	}

	// A block partly dropped keeps the rows that remain.
	g, rows = newLiveBlockGrid()
	*rows = (*rows)[12:]
	g.DropLeadingRows(12)
	if got, want := selectedIDs(g), []string{"12", "13"}; !slices.Equal(got, want) {
		t.Errorf("partly dropped block covers %v, want %v", got, want)
	}

	// A block wholly dropped collapses on row 0.
	g, rows = newLiveBlockGrid()
	*rows = (*rows)[15:]
	g.DropLeadingRows(15)
	wantRowBounds(t, g, 0, 0, "wholly dropped")
	if g.blockSelecting {
		t.Error("blockSelecting still set with both corners on row 0")
	}
}

// SetSourcePreservingView is for the same rows, so a multi-row selection
// survives it. A live view rebuilt for a new column wiped the selection the
// user was about to copy.
func TestSetSourcePreservingViewKeepsBlockSelection(t *testing.T) {
	g, rows := newLiveBlockGrid()
	g.SetSourcePreservingView([]string{"ID", "New"}, liveRows{rows})
	wantRowBounds(t, g, 10, 13, "same rows")

	// Clamped into a shorter set.
	short := idRows(12)
	g.SetSourcePreservingView([]string{"ID"}, liveRows{&short})
	wantRowBounds(t, g, 10, 11, "shorter set")

	// Marks are kept while every marked row still exists...
	g, rows = newLiveBlockGrid()
	g.markRow(15)
	g.SetSourcePreservingView([]string{"ID"}, liveRows{rows})
	wantRows(t, g, []int{10, 11, 12, 13, 15}, "marks, same rows")

	// ...and dropped once one does not.
	short = idRows(14)
	g.SetSourcePreservingView([]string{"ID"}, liveRows{&short})
	if g.marking {
		t.Errorf("marks kept past the end: %v", g.SelectedRows())
	}
}
