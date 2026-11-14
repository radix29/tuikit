package controls

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/tuikittest"
)

// unlaidGrid is a small grid that has never been given bounds — the state a
// page's grid is in while it is still being built. rect.H is 0, so every
// ensureVisible call gets a negative data height.
func unlaidGrid() *DataGrid {
	g := NewDataGrid()
	g.SetData([]string{"Permission", "State"}, [][]string{
		{"Alter", "[ ]"},
		{"Connect", "[x]"},
		{"Control", "[ ]"},
	})
	g.SetCellCursor(true)
	return g
}

// Every selection setter runs ensureVisible, and on a grid with no viewport
// yet the scroll must not move: scrollRow = selRow - dataH + 1 with a negative
// dataH lands past the last row, and the next Draw paints the header and the
// "N rows" status over blank lines.
func TestSelectionOnAnUnlaidOutGridDoesNotScrollPastTheRows(t *testing.T) {
	cases := map[string]func(*DataGrid){
		"SetSelectedRow":  func(g *DataGrid) { g.SetSelectedRow(2) },
		"SetSelectedCell": func(g *DataGrid) { g.SetSelectedCell(2, 1) },
		"SetDataPreservingView": func(g *DataGrid) {
			g.SetSelectedCell(2, 1)
			g.SetDataPreservingView([]string{"Permission", "State"}, [][]string{
				{"Alter", "[ ]"},
				{"Connect", "[x]"},
				{"Control", "[x]"},
			})
		},
	}
	for name, act := range cases {
		t.Run(name, func(t *testing.T) {
			g := unlaidGrid()
			act(g)

			if got := g.ScrollRow(); got != 0 {
				t.Errorf("ScrollRow() = %d, want 0 — nothing to scroll before layout", got)
			}
			if got := g.ScrollCol(); got != 0 {
				t.Errorf("ScrollCol() = %d, want 0 — nothing to scroll before layout", got)
			}

			// The bug's visible form: give the grid real bounds, the size the
			// first layout pass would, and draw it.
			g.SetBounds(0, 0, 40, 10)
			s := tuikittest.NewScreen(40, 10)
			g.Draw(s)
			out := s.Snapshot()
			for _, want := range []string{"Alter", "Connect", "Control"} {
				if !strings.Contains(out, want) {
					t.Errorf("Draw painted no %q row; grid rendered blank:\n%s", want, out)
				}
			}
		})
	}
}

// The guard must not cost a laid-out grid its scrolling: the same selection on
// a real viewport still brings the row into view.
func TestSelectionOnALaidOutGridStillScrolls(t *testing.T) {
	g := preserveViewGrid() // 30 rows, 7 visible
	g.SetSelectedCell(20, 1)
	if got := g.ScrollRow(); got != 14 {
		t.Errorf("ScrollRow() = %d, want 14 (row 20 scrolled to the bottom edge)", got)
	}
}

// A grid under three rows high shows no data rows. dataRows was once negative
// there, so PgDn paged backwards (selRow -2 on a one-row grid) and PgUp past the
// last row; growing the grid and pressing any key copied the bad row into
// scrollRow and Draw reached rows.Row(-1). Every whole-list key must leave the
// selection on a real row, and a later Draw at full size must survive.
func TestPagingAShortGridKeepsTheSelectionInRange(t *testing.T) {
	keys := map[string]tcell.Key{
		"PgDn": tcell.KeyPgDn, "PgUp": tcell.KeyPgUp,
		"End": tcell.KeyEnd, "Home": tcell.KeyHome,
	}
	for h := range 4 {
		for _, seq := range [][]string{
			{"PgDn"}, {"PgDn", "PgDn", "PgUp"}, {"End", "PgUp"}, {"End", "PgDn"},
			{"Home", "PgUp"}, {"PgDn", "End", "Home", "PgDn"},
		} {
			t.Run(fmt.Sprintf("h%d/%s", h, strings.Join(seq, "+")), func(t *testing.T) {
				g := NewDataGrid()
				g.SetData([]string{"c"}, [][]string{{"1"}, {"2"}, {"3"}, {"4"}})
				g.SetBounds(0, 0, 25, h)
				g.Focus(true)
				for _, k := range seq {
					g.HandleKey(tuikittest.Key(keys[k]))
					if r := g.SelectedRow(); r < 0 || r >= 4 {
						t.Fatalf("after %s: SelectedRow() = %d, want 0..3", k, r)
					}
				}
				g.SetBounds(0, 0, 25, 10)
				g.HandleKey(tuikittest.Key(tcell.KeyDown))
				g.Draw(tuikittest.NewScreen(25, 10))
				if r := g.SelectedRow(); r < 0 || r >= 4 {
					t.Errorf("after grow+Down: SelectedRow() = %d, want 0..3", r)
				}
			})
		}
	}
}

// PgDn and PgUp still move on a grid too short to show a data row: a page is
// at least one row, not zero.
func TestPagingAGridWithNoVisibleRowsStillMoves(t *testing.T) {
	g := NewDataGrid()
	g.SetData([]string{"c"}, [][]string{{"1"}, {"2"}, {"3"}, {"4"}})
	g.SetBounds(0, 0, 25, 2)
	g.Focus(true)
	g.HandleKey(tuikittest.Key(tcell.KeyPgDn))
	if r := g.SelectedRow(); r != 1 {
		t.Errorf("PgDn: SelectedRow() = %d, want 1", r)
	}
	g.HandleKey(tuikittest.Key(tcell.KeyPgUp))
	if r := g.SelectedRow(); r != 0 {
		t.Errorf("PgUp: SelectedRow() = %d, want 0", r)
	}
}

// On a grid too short to show a data row, neither SetScroll nor the wheel may
// scroll past the last row: both bounded by rows.Len()-dataRows(), which is
// rows.Len() there, and the grid drew blank after it grew until a key moved
// the selection. SetDataPreservingView reaches SetScroll with the old offset.
func TestScrollOnAGridWithNoVisibleRowsStaysOnARow(t *testing.T) {
	rows := [][]string{{"1"}, {"2"}, {"3"}, {"4"}}
	for h := range 3 {
		t.Run(fmt.Sprintf("h%d/SetScroll", h), func(t *testing.T) {
			g := NewDataGrid()
			g.SetData([]string{"c"}, rows)
			g.SetBounds(0, 0, 25, h)
			g.SetScroll(10, 0)
			if s := g.ScrollRow(); s != 3 {
				t.Errorf("ScrollRow() = %d, want 3", s)
			}
		})
		t.Run(fmt.Sprintf("h%d/WheelDown", h), func(t *testing.T) {
			if h == 0 {
				t.Skip("a zero-height rect contains no point to wheel over")
			}
			g := NewDataGrid()
			g.SetData([]string{"c"}, rows)
			g.SetBounds(0, 0, 25, h)
			for range 3 {
				g.HandleMouse(tuikittest.Mouse(1, 0, tcell.WheelDown))
			}
			if s := g.ScrollRow(); s != 3 {
				t.Errorf("ScrollRow() = %d, want 3", s)
			}
		})
	}
}
