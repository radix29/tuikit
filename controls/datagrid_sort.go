package controls

import "slices"

// datagrid_sort.go is the mechanics of a host's column sort, shared by every
// grid that sorts on a header click: a source shown through a permutation, the
// swap that keeps the cursor on its row, and the cell menu's keyboard route.
// What orders the rows (the comparison, sorting off the UI goroutine, which
// order each view remembers) stays the host's; the grid only draws the arrow
// (SetSortIndicator) and reports the click (OnHeaderClick).

// PermutedSource shows Src in Perm's order: Perm[i] is Src's index of the i-th
// row shown. The rows are not copied, so a host writing cells of Src in place
// reaches the permuted view too.
//
// It passes NullSource and RowKindSource through when Src implements them, so
// reordered NULLs still draw dimmed and marked rows stay marked.
type PermutedSource struct {
	Src  RowSource
	Perm []int
}

func (s PermutedSource) Len() int           { return len(s.Perm) }
func (s PermutedSource) Row(i int) []string { return s.Src.Row(s.Perm[i]) }

// IsNull is Src's IsNull for the row shown at row, false when Src is not a
// NullSource.
func (s PermutedSource) IsNull(row, col int) bool {
	if n, ok := s.Src.(NullSource); ok {
		return n.IsNull(s.Perm[row], col)
	}
	return false
}

// RowKind is Src's RowKind for the row shown at i, RowNormal when Src is not a
// RowKindSource.
func (s PermutedSource) RowKind(i int) RowKind {
	if k, ok := s.Src.(RowKindSource); ok {
		return k.RowKind(s.Perm[i])
	}
	return RowNormal
}

// SetSourceReordered installs rows, the same rows as the grid's in another
// order, keeping the cursor on the row it was on rather than on its old index
// (which now holds another row), plus the scroll, the dragged column widths and
// the sort indicator, as SetSourcePreservingView does. oldPerm and newPerm map
// each shown index to the row's index in the host's underlying order — the
// order before and after; a nil oldPerm is the identity over the rows shown
// now (nothing was sorted). When the cursor's row is not in newPerm the cursor
// stays on its index.
//
// Ctrl+click marks follow their rows too, so a user who marks rows and sorts
// to check them still copies those rows; a marked row missing from newPerm is
// dropped. A Shift-extended block is collapsed: after a re-sort its rows are no
// longer contiguous, and its column range is something marks cannot express.
//
// The host sets the indicator for the new order (SetSortIndicator) before or
// after; this keeps the old one.
func (g *DataGrid) SetSourceReordered(columns []string, rows RowSource, oldPerm, newPerm []int) {
	row, col := g.SelectedCell()
	orig := -1
	switch {
	case oldPerm == nil:
		if row >= 0 && row < g.rows.Len() {
			orig = row
		}
	case row >= 0 && row < len(oldPerm):
		orig = oldPerm[row]
	}
	var marked []int // the marked rows, as indexes in the underlying order
	if g.marking {
		shown := g.rows.Len()
		if oldPerm != nil {
			shown = len(oldPerm)
		}
		for _, r := range g.SelectedRows() {
			switch {
			case r < 0 || r >= shown:
			case oldPerm == nil:
				marked = append(marked, r)
			default:
				marked = append(marked, oldPerm[r])
			}
		}
	}
	g.SetSourcePreservingView(columns, rows)
	if i := slices.Index(newPerm, orig); i >= 0 {
		g.SetSelectedCell(i, col)
	}
	if len(marked) == 0 {
		return
	}
	at := make(map[int]int, len(newPerm)) // underlying row -> shown index
	for i, r := range newPerm {
		at[r] = i
	}
	rowsNow := make([]int, 0, len(marked))
	for _, r := range marked {
		if i, ok := at[r]; ok {
			rowsNow = append(rowsNow, i)
		}
	}
	g.setMarkedRows(rowsNow)
}

// SortMenuItems are a grid cell menu's keyboard route to a header click (an
// OnMenuItems host's menu opens on the cursor's cell): Sort Ascending and Sort
// Descending of the column the host captured in the closures. isCurrent reports
// whether that column is already sorted in that direction, which greys the
// entry; sort applies it. Enabled is evaluated when the menu draws.
func SortMenuItems(isCurrent func(desc bool) bool, sort func(desc bool)) []MenuItem {
	item := func(label string, desc bool) MenuItem {
		return MenuItem{
			Label:   label,
			Enabled: func() bool { return !isCurrent(desc) },
			Action:  func() { sort(desc) },
		}
	}
	return []MenuItem{item(Strings.SortAscending, false), item(Strings.SortDescending, true)}
}
