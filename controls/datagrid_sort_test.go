package controls

import (
	"slices"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/tuikittest"
)

// headerText reads the grid's header row off the screen.
func headerText(s tcell.Screen, w int) string {
	var b strings.Builder
	for x := range w {
		str, _, _ := s.Get(x, 0)
		b.WriteString(str)
	}
	return b.String()
}

// A press on a header cell fires OnHeaderClick once for that column, however
// many Button1 resends tcell sends while it is held, and again only after a
// release.
func TestDataGridHeaderClickFiresOncePerPress(t *testing.T) {
	g := newTestDataGrid()
	g.SetCellCursor(true)
	g.SetData([]string{"Name", "Rows"}, [][]string{{"a", "1"}, {"b", "2"}})
	var clicks []int
	g.OnHeaderClick = func(col int) { clicks = append(clicks, col) }

	x := g.ColumnWidth(0) + 1 // inside column 1, clear of its separator
	for range 3 {
		g.HandleMouse(tcell.NewEventMouse(x, 0, tcell.Button1, tcell.ModNone))
	}
	if len(clicks) != 1 || clicks[0] != 1 {
		t.Fatalf("clicks = %v after one held press, want [1]", clicks)
	}
	// A drag off the header onto the rows stays the header's.
	g.HandleMouse(tcell.NewEventMouse(1, 3, tcell.Button1, tcell.ModNone))
	if r, _ := g.SelectedCell(); r != 0 {
		t.Errorf("a drag from the header moved the cursor to row %d", r)
	}
	g.HandleMouse(tcell.NewEventMouse(1, 3, tcell.ButtonNone, tcell.ModNone))
	g.HandleMouse(tcell.NewEventMouse(1, 0, tcell.Button1, tcell.ModNone))
	if len(clicks) != 2 || clicks[1] != 0 {
		t.Fatalf("clicks = %v after a second press on column 0, want [1 0]", clicks)
	}
}

// The separator resizes, and a drag begun on a data cell is a selection: neither
// is a header click.
func TestDataGridHeaderClickSkipsSeparatorAndCellDrags(t *testing.T) {
	g := newTestDataGrid()
	g.SetCellCursor(true)
	g.SetData([]string{"Name", "Rows"}, [][]string{{"a", "1"}, {"b", "2"}})
	fired := false
	g.OnHeaderClick = func(int) { fired = true }

	sep := g.ColumnWidth(0) - 1
	g.HandleMouse(tcell.NewEventMouse(sep, 0, tcell.Button1, tcell.ModNone))
	g.HandleMouse(tcell.NewEventMouse(sep, 0, tcell.ButtonNone, tcell.ModNone))
	if fired {
		t.Error("a press on the column separator fired OnHeaderClick")
	}
	g.HandleMouse(tcell.NewEventMouse(1, 2, tcell.Button1, tcell.ModNone))
	g.HandleMouse(tcell.NewEventMouse(1, 0, tcell.Button1, tcell.ModNone))
	if fired {
		t.Error("a drag from a data cell onto the header fired OnHeaderClick")
	}
}

// The indicator is drawn after the sorted column's label, widens the column to
// fit, is cleared by SetData (a new column set) and kept by
// SetDataPreservingView.
func TestDataGridSortIndicator(t *testing.T) {
	s := tuikittest.NewMockScreen(t, 40, 10)
	g := newTestDataGrid()
	g.SetData([]string{"Name", "Row Count"}, [][]string{{"a", "1"}})
	before := g.ColumnWidth(1)

	g.SetSortIndicator(1, true)
	g.Draw(s)
	if h := headerText(s, 40); !strings.Contains(h, "Row Count"+sortDescGlyph) {
		t.Errorf("header = %q, want the descending arrow after Row Count", h)
	}
	if got, want := g.ColumnWidth(1), before+core.DisplayWidth(sortDescGlyph); got != want {
		t.Errorf("sorted column width = %d, want %d (label plus arrow)", got, want)
	}

	g.SetDataPreservingView([]string{"Name", "Row Count"}, [][]string{{"b", "2"}})
	if col, desc := g.SortIndicator(); col != 1 || !desc {
		t.Errorf("SetDataPreservingView left indicator (%d, %v), want (1, true)", col, desc)
	}
	g.SetData([]string{"Property", "Value"}, nil)
	if col, _ := g.SortIndicator(); col != -1 {
		t.Errorf("SetData left the indicator on column %d of a new column set", col)
	}
	g.Draw(s)
	if h := headerText(s, 40); strings.ContainsAny(h, "▲▼") {
		t.Errorf("header = %q after SetData, want no arrow", h)
	}
}

// permTestRows is a NullSource and RowKindSource over rows: null names the cells
// holding NULL, marked the marked rows, by their index in rows.
type permTestRows struct {
	rows   [][]string
	null   map[[2]int]bool
	marked map[int]bool
}

func (s permTestRows) Len() int                 { return len(s.rows) }
func (s permTestRows) Row(i int) []string       { return s.rows[i] }
func (s permTestRows) IsNull(row, col int) bool { return s.null[[2]int{row, col}] }
func (s permTestRows) RowKind(i int) RowKind {
	if s.marked[i] {
		return RowMarked
	}
	return RowNormal
}

// PermutedSource reads each shown row, its NULLs and its kind from the
// underlying row Perm names, not from the shown index.
func TestPermutedSourcePassesCapabilitiesThrough(t *testing.T) {
	src := permTestRows{
		rows:   [][]string{{"a"}, {"NULL"}, {"c"}},
		null:   map[[2]int]bool{{1, 0}: true},
		marked: map[int]bool{2: true},
	}
	p := PermutedSource{Src: src, Perm: []int{2, 1, 0}}
	if p.Len() != 3 || p.Row(0)[0] != "c" || p.Row(2)[0] != "a" {
		t.Fatalf("rows = %v %v %v, want c NULL a", p.Row(0), p.Row(1), p.Row(2))
	}
	if !p.IsNull(1, 0) || p.IsNull(0, 0) {
		t.Errorf("IsNull(1,0)=%v IsNull(0,0)=%v, want true false", p.IsNull(1, 0), p.IsNull(0, 0))
	}
	if p.RowKind(0) != RowMarked || p.RowKind(2) != RowNormal {
		t.Errorf("RowKind(0)=%v RowKind(2)=%v, want marked normal", p.RowKind(0), p.RowKind(2))
	}
	// A source with neither capability has no NULLs and only normal rows.
	plain := PermutedSource{Src: SliceRowSource{{"x"}}, Perm: []int{0}}
	if plain.IsNull(0, 0) || plain.RowKind(0) != RowNormal {
		t.Error("a plain source's permuted view reports a NULL or a marked row")
	}
}

// SetSourceReordered keeps the cursor on the row it was on, through the old
// order and into the new, keeps its column, and keeps the indicator; a row
// missing from the new order leaves the cursor on its index.
func TestSetSourceReorderedFollowsTheRow(t *testing.T) {
	base := SliceRowSource{{"a", "1"}, {"b", "2"}, {"c", "3"}, {"d", "4"}}
	g := newTestDataGrid()
	g.SetCellCursor(true)
	g.SetSource([]string{"N", "V"}, base)
	g.SetSelectedCell(1, 1) // "b", unsorted

	desc := []int{3, 2, 1, 0}
	g.SetSourceReordered([]string{"N", "V"}, PermutedSource{Src: base, Perm: desc}, nil, desc)
	g.SetSortIndicator(0, true)
	if row, col := g.SelectedCell(); row != 2 || col != 1 {
		t.Fatalf("cursor = (%d,%d) after the first sort, want (2,1) on b", row, col)
	}

	asc := []int{0, 1, 2, 3}
	g.SetSourceReordered([]string{"N", "V"}, PermutedSource{Src: base, Perm: asc}, desc, asc)
	if row, _ := g.SelectedCell(); row != 1 {
		t.Fatalf("cursor row = %d after reversing, want 1 on b", row)
	}
	if col, d := g.SortIndicator(); col != 0 || !d {
		t.Errorf("indicator = (%d,%v), want the old one kept until the host sets it", col, d)
	}

	// The cursor's row gone from the new order: the cursor stays on its index.
	g.SetSelectedCell(3, 0)
	short := []int{0, 1, 2}
	g.SetSourceReordered([]string{"N", "V"}, PermutedSource{Src: base, Perm: short}, asc, short)
	if row, _ := g.SelectedCell(); row != 2 {
		t.Errorf("cursor row = %d after its row left, want 2 (index 3 clamped)", row)
	}
}

// markForReorder Ctrl+clicks rows of g the way a mouse does: the mark, then
// the cursor on the clicked row.
func markForReorder(g *DataGrid, rows ...int) {
	for _, r := range rows {
		g.markRow(r)
		g.selRow, g.selAnchorRow = r, r
	}
}

// SetSourceReordered carries Ctrl+click marks to their rows' new indexes,
// through an unsorted (nil) and a sorted old order, and drops a marked row the
// new order no longer holds. A re-sort used to clear them, so Copy took only
// the cursor row.
func TestSetSourceReorderedKeepsMarks(t *testing.T) {
	base := SliceRowSource{{"c"}, {"a"}, {"d"}, {"b"}}
	g := newTestDataGrid()
	g.SetSource([]string{"N"}, base)
	g.SetSelectedCell(0, 0) // c
	markForReorder(g, 1, 3) // a, b
	if got := g.SelectedRows(); !slices.Equal(got, []int{0, 1, 3}) {
		t.Fatalf("marked before = %v, want [0 1 3]", got)
	}

	asc := []int{1, 3, 0, 2} // a b c d
	g.SetSourceReordered([]string{"N"}, PermutedSource{Src: base, Perm: asc}, nil, asc)
	if got := g.SelectedRows(); !slices.Equal(got, []int{0, 1, 2}) {
		t.Fatalf("marked after the first sort = %v, want [0 1 2] (a b c)", got)
	}

	desc := []int{2, 0, 3, 1} // d c b a
	g.SetSourceReordered([]string{"N"}, PermutedSource{Src: base, Perm: desc}, asc, desc)
	if got := g.SelectedRows(); !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("marked after re-sorting = %v, want [1 2 3] (c b a)", got)
	}

	// b (underlying 3) leaves the new order: c and a stay marked.
	short := []int{2, 0, 1} // d c a
	g.SetSourceReordered([]string{"N"}, PermutedSource{Src: base, Perm: short}, desc, short)
	if got := g.SelectedRows(); !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("marked after b left = %v, want [1 2] (c a)", got)
	}

	// Every marked row gone: no marks, the cursor row is the selection again.
	g.SetSelectedCell(0, 0)
	markForReorder(g, 0) // unmark the cursor row d ...
	markForReorder(g, 2) // ... and mark a
	only := []int{2}     // d
	g.SetSourceReordered([]string{"N"}, PermutedSource{Src: base, Perm: only}, short, only)
	if g.marking || g.SelectedRowCount() != 1 {
		t.Errorf("marking=%v count=%d with every marked row gone, want no marks and the cursor row", g.marking, g.SelectedRowCount())
	}
}

// SortMenuItems offers both directions, greys the one in force, and applies
// the one picked.
func TestSortMenuItems(t *testing.T) {
	curDesc := false
	var got []bool
	items := SortMenuItems(func(desc bool) bool { return desc == curDesc }, func(desc bool) { got = append(got, desc) })
	if len(items) != 2 || items[0].Label != Strings.SortAscending || items[1].Label != Strings.SortDescending {
		t.Fatalf("items = %+v, want Sort Ascending, Sort Descending", items)
	}
	if items[0].Enabled() || !items[1].Enabled() {
		t.Errorf("enabled = %v %v while ascending, want false true", items[0].Enabled(), items[1].Enabled())
	}
	items[1].Action()
	if len(got) != 1 || !got[0] {
		t.Errorf("sort calls = %v, want [true]", got)
	}
}
