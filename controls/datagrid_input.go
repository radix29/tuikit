package controls

import (
	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/core"
)

// selectionScreenPos returns the screen coordinates of the selected cell, to
// position the context menu Ctrl+Space opens as a right-click would. "Show
// Value" needs none (screen-centred). Mirrors drawRow's width walk.
func (g *DataGrid) selectionScreenPos() (x, y int) {
	x = g.rect.X + g.gutterWidth()
	for i := g.scrollCol; i < g.selCol && i < len(g.colWidths); i++ {
		x += g.colWidths[i]
	}
	y = g.rect.Y + 2 + (g.selRow - g.scrollRow)
	return x, y
}

// selectionContains reports whether (row, col) is within the current selection,
// so right-click preserves an existing block selection instead of collapsing it.
func (g *DataGrid) selectionContains(row, col int) bool {
	r0, c0, r1, c1 := g.SelectionBounds()
	return row >= r0 && row <= r1 && col >= c0 && col <= c1
}

// extendSelectionMods are the modifiers that make a click extend the selection
// from the anchor rather than start a new one. Alt as well as Shift: a VTE
// terminal (xfce4-terminal, GNOME Terminal) holds Shift back for its own text
// selection whenever the app has mouse reporting on, so the app never sees the
// click; Alt+click is delivered.
const extendSelectionMods = tcell.ModShift | tcell.ModAlt

// HandleKey handles keyboard navigation. It reports true only when the key
// changed something (selection, scroll, marked rows), fired a callback or opened
// a popup: an arrow at an edge, PgDn on an empty grid or Enter on a read-only
// one answer false, so a host (propsheet.Form, a panel's focus cycling) can use
// the key itself. An open context menu or "Show Value" popup takes every key.
func (g *DataGrid) HandleKey(ev *tcell.EventKey) bool {
	if g.ctxMenu.OverlayActive() {
		g.ctxMenu.HandleKey(ev)
		return true
	}
	if g.viewer.HandleKey(ev) {
		return true
	}
	// Ctrl+Space is the keyboard equivalent of right-clicking the selected cell. An
	// editable grid (see editable) has no context menu there, so it falls through to
	// the default case.
	if ev.Modifiers()&tcell.ModCtrl != 0 && core.EvRune(ev) == ' ' &&
		g.cellCursor && g.rows.Len() > 0 && !g.editable() {
		x, y := g.selectionScreenPos()
		g.ctxMenu.Show(x, y, g.cellContextMenuItems())
		return true
	}
	// Detected, never predicted: comparing what the key left behind stays right as
	// the key handling below changes.
	before := g.keyState()
	bound, fired := g.applyKey(ev)
	return bound && (fired || g.keyState() != before)
}

// gridKeyState is what HandleKey compares around a key to tell whether it was
// acted on: everything a navigation key can change that the user can see.
type gridKeyState struct {
	selRow, selCol, scrollRow, scrollCol int
	r0, c0, r1, c1                       int
	marking                              bool
}

func (g *DataGrid) keyState() gridKeyState {
	r0, c0, r1, c1 := g.SelectionBounds()
	return gridKeyState{g.selRow, g.selCol, g.scrollRow, g.scrollCol, r0, c0, r1, c1, g.marking}
}

// applyKey acts on a navigation or activation key. bound is false for a key the
// grid has no binding for; fired is true when it reached OnActivateCell.
func (g *DataGrid) applyKey(ev *tcell.EventKey) (bound, fired bool) {
	// Shift+Arrow extends a block selection from the cell the cursor was on, anchor
	// fixed across repeats; a plain arrow collapses to one cell. Read-only
	// cell-cursor grids only; see blockSelecting.
	canBlockSelect := g.cellCursor && !g.editable()
	shiftHeld := ev.Modifiers()&tcell.ModShift != 0
	isArrowKey := false
	switch ev.Key() {
	case tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight:
		isArrowKey = true
	}
	if canBlockSelect {
		if shiftHeld && isArrowKey {
			if !g.blockSelecting {
				g.selAnchorRow, g.selAnchorCol = g.selRow, g.selCol
			}
			g.blockSelecting = true
		} else {
			g.blockSelecting = false
		}
	}
	// Any cursor move drops a Ctrl+click selection, Shift+Arrow included (a file
	// manager's rule); a marked set is extended only by more Ctrl+clicks.
	switch ev.Key() {
	case tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight,
		tcell.KeyPgUp, tcell.KeyPgDn, tcell.KeyHome, tcell.KeyEnd:
		g.ClearMarkedRows()
	}
	dataH := g.dataRows()
	// The four whole-list jumps do nothing on an empty grid (the guard
	// SetSelectedRow/SetSelectedCell carry). PgDn and End derive selRow from
	// rows.Len()-1, -1 with no rows; ensureVisible copies that into scrollRow, and
	// Draw's row loop bounds dataIdx only from above, so it reaches rows.Row(-1) and
	// panics on the UI goroutine, which has no recover. Up/Down use a live index.
	switch ev.Key() {
	case tcell.KeyPgUp, tcell.KeyPgDn, tcell.KeyHome, tcell.KeyEnd:
		if g.rows.Len() == 0 {
			return true, false
		}
	}
	prevRow := g.selRow
	switch ev.Key() {
	case tcell.KeyUp:
		if g.selRow > 0 {
			g.selRow--
			g.ensureVisible(dataH)
		}
	case tcell.KeyDown:
		if g.selRow < g.rows.Len()-1 {
			g.selRow++
			g.ensureVisible(dataH)
		}
	// A page is at least one row, so the keys still move on a grid too short to
	// show any data row.
	case tcell.KeyPgUp:
		g.selRow = max(0, g.selRow-max(1, dataH))
		g.ensureVisible(dataH)
	case tcell.KeyPgDn:
		g.selRow = min(g.rows.Len()-1, g.selRow+max(1, dataH))
		g.ensureVisible(dataH)
	case tcell.KeyHome:
		g.selRow, g.scrollRow = 0, 0
	case tcell.KeyEnd:
		g.selRow = g.rows.Len() - 1
		g.ensureVisible(dataH)
	case tcell.KeyLeft:
		if g.cellCursor {
			if g.selCol > 0 {
				g.selCol--
				g.ensureVisibleCol()
			}
		} else if g.scrollCol > 0 {
			g.scrollCol--
		}
	case tcell.KeyRight:
		if g.cellCursor {
			if g.selCol < len(g.columns)-1 {
				g.selCol++
				g.ensureVisibleCol()
			}
		} else if g.scrollCol < len(g.columns)-1 {
			g.scrollCol++
		}
	case tcell.KeyEnter:
		if g.cellCursor && g.rows.Len() > 0 {
			fired = g.activateCell()
		}
	default:
		if g.cellCursor && g.rows.Len() > 0 && core.EvRune(ev) == ' ' {
			return true, g.activateCell()
		}
		return false, false
	}
	// Only on an actual move, as a click does: PgUp on the first row reports
	// nothing to a page that reloads its detail pane per selection.
	if g.selRow != prevRow && g.OnSelectRow != nil {
		g.OnSelectRow(g.selRow)
	}
	return true, fired
}

// gridZone is the part of the grid a Button1 press began in, which owns every
// event of the gesture until the release (DataGrid.gesture). Zones are
// hit-tested only on the fresh press.
type gridZone int

const (
	// gridInert swallows the rest of a press that started nothing: a header
	// click (OnHeaderClick fires on the press), the status row, blank space.
	gridInert gridZone = iota
	// gridCells is a cell or row selection drag, or a toggle grid painting the
	// cells it crosses.
	gridCells
	// gridRowHeader selects whole rows from a press on the row-number gutter,
	// as SSMS's row header does: a drag onto the cells keeps selecting rows.
	gridRowHeader
	// gridMark is a Ctrl+press, which picks one row out once per press: tcell
	// resends Button1 while held, and a second toggle would undo the first. It
	// never drags: the modifier says "this row", not "this run".
	gridMark
	// gridVBar and gridHBar drag the vertical and horizontal scrollbar thumbs.
	gridVBar
	gridHBar
	// gridResize drags a column separator in the header row.
	gridResize
)

// HandleMouse handles mouse events.
func (g *DataGrid) HandleMouse(ev *tcell.EventMouse) bool {
	// A right-click outside the open menu dismisses it and falls through
	// (core.MouseHandler), so it opens a fresh menu on the cell it landed on.
	if g.ctxMenu.OverlayActive() && (g.ctxMenu.HandleMouse(ev) || g.ctxMenu.OverlayActive()) {
		return true
	}
	// The popup, while showing, and the tail of the gesture that closed it.
	if g.viewer.HandleMouse(ev) {
		return true
	}
	// A release ends the gesture wherever it lands. Plain motion is claimed only
	// when it ended one (core.MouseHandler): a hover reported as handled read as a
	// click to hosts that move focus on true.
	if ev.Buttons() == tcell.ButtonNone {
		_, held := g.gesture.Owner()
		g.gesture.Release()
		return held
	}
	// Ahead of the bounds test: a held drag keeps its zone after leaving the grid,
	// so a thumb dragged below it still reaches the end and a selection dragged
	// past an edge keeps scrolling toward it.
	if g.gesture.Route(ev, g.routeDrag) {
		return true
	}
	mx, my := ev.Position()
	if !g.rect.Contains(mx, my) {
		return false
	}
	if ev.Buttons() == tcell.Button1 {
		z, doubleRow := g.press(ev)
		g.gesture.Arm(ev, z)
		// After the press has moved the selection onto the row, and after Arm: a host
		// opening Show Value from here ends the gesture (ShowValue), which an Arm
		// coming later would undo.
		if doubleRow >= 0 {
			g.OnRowDoubleClick(doubleRow)
		}
		return true
	}
	dataH := g.dataRows()
	switch ev.Buttons() {
	case tcell.Button2:
		// Right-click on the row-number gutter's blank header cell offers whole-grid
		// actions instead of a per-cell menu.
		if gw := g.gutterWidth(); gw > 0 && my == g.rect.Y && mx >= g.rect.X && mx < g.rect.X+gw {
			if items := g.gridContextMenuItems(); len(items) > 0 {
				g.ctxMenu.Show(mx, my, items)
			}
			return true
		}
		// Right-click on a data cell: select it and, on a read-only grid, offer
		// "Copy"/"Show Value". A click inside an existing block selection preserves it
		// so "Copy" takes the whole block; otherwise it collapses to the clicked cell, as
		// a spreadsheet does.
		if row := g.rowAtY(my); g.cellCursor && row >= 0 {
			if col, ok := g.colAt(mx); ok {
				if !g.selectionContains(row, col) && !g.rowMarked(row) {
					prevRow := g.selRow
					g.selRow, g.selCol = row, col
					g.blockSelecting = false
					g.ClearMarkedRows()
					// A move like any other: without it a grid-plus-detail page (Replication
					// Monitor) keeps describing the row the highlight just left. Fired before the
					// menu opens, so its actions see the page synced to the clicked row.
					if g.selRow != prevRow && g.OnSelectRow != nil {
						g.OnSelectRow(g.selRow)
					}
				}
				if !g.editable() {
					g.ctxMenu.Show(mx, my, g.cellContextMenuItems())
				}
			}
		}
	case tcell.WheelUp:
		// Shift+wheel is the desktop convention for horizontal scroll, and some
		// terminals report it that way rather than as WheelLeft/WheelRight; honour both.
		if ev.Modifiers()&tcell.ModShift != 0 {
			g.scrollColBy(-horizontalWheelCols)
		} else {
			g.scrollRow = max(0, g.scrollRow-core.WheelRows)
		}
	case tcell.WheelDown:
		if ev.Modifiers()&tcell.ModShift != 0 {
			g.scrollColBy(horizontalWheelCols)
		} else {
			// SetScroll's bound: a viewport of at least one row, or a grid too short
			// for a data row scrolls past its last row.
			g.scrollRow = max(g.scrollRow, min(g.rows.Len()-max(1, dataH), g.scrollRow+core.WheelRows))
		}
	case tcell.WheelLeft:
		g.scrollColBy(-horizontalWheelCols)
	case tcell.WheelRight:
		g.scrollColBy(horizontalWheelCols)
	}
	return true
}

// press acts on a fresh Button1 press inside the grid and returns the zone that
// owns the rest of the gesture, and the row of a double-click it completed (-1
// for none) for the caller to report once the gesture is armed. The order is the hit-test's precedence: the
// separators sit on the header row, and the vertical bar is drawn over the
// rightmost column, where it would read as a click on that column's cell.
func (g *DataGrid) press(ev *tcell.EventMouse) (z gridZone, doubleRow int) {
	doubleRow = -1
	mx, my := ev.Position()
	if col, ok := g.sepColAt(mx, my); ok {
		if g.sepClicks.Press(col, ev.When()) {
			g.SetColumnWidth(col, 0)
		}
		g.resizeCol, g.resizeStartX, g.resizeStartW = col, mx, g.colWidths[col]
		return gridResize, doubleRow
	}
	if x, y, w, _, _, _, ok := g.hScrollbar(); ok && my == y && mx >= x && mx < x+w {
		g.dragHScrollbar(mx)
		return gridHBar, doubleRow
	}
	if dataH := g.dataRows(); core.OnScrollbar(mx, my, g.rect.Right()-1, g.rect.Y+2, dataH, g.rows.Len()) {
		g.dragVScrollbar(my)
		return gridVBar, doubleRow
	}
	if my == g.rect.Y {
		if col, ok := g.colAt(mx); ok && g.OnHeaderClick != nil && !g.errorMode && col < len(g.columns) {
			g.OnHeaderClick(col)
		}
		return gridInert, doubleRow
	}
	// rowAtY is -1 outside the data rows. rect covers the header, separator and
	// status bar too; without the bound a click on the status bar resolves to
	// scrollRow+dataH, the first row below the view, moving the selection out of
	// sight.
	row := g.rowAtY(my)
	if row < 0 {
		return gridInert, doubleRow
	}
	canBlockSelect := g.cellCursor && !g.editable()
	// Checked ahead of the double-click: a double-click on a row number selects the
	// row, it doesn't open the cell under the cursor in Show Value.
	if canBlockSelect && g.inGutter(mx) {
		return g.rowHeaderPress(ev, row), doubleRow
	}
	if g.rowDoubleClicked(ev, row) {
		doubleRow = row
	}
	prevRow := g.selRow
	if canBlockSelect {
		col, ok := g.colAt(mx)
		if !ok {
			return gridInert, doubleRow
		}
		if ev.Modifiers()&tcell.ModCtrl != 0 {
			g.markRow(row)
			g.selRow, g.selCol = row, col
			g.selAnchorRow, g.selAnchorCol = row, col
			g.blockSelecting = false
			if g.OnSelectRow != nil {
				g.OnSelectRow(g.selRow)
			}
			return gridMark, doubleRow
		}
		// A press without Ctrl starts a new selection, dropping what Ctrl+click marked,
		// including under Shift, which extends from the anchor rather than adding to the
		// marked set.
		g.ClearMarkedRows()
		if ev.Modifiers()&extendSelectionMods != 0 {
			if !g.blockSelecting {
				g.selAnchorRow, g.selAnchorCol = g.selRow, g.selCol
			}
		} else {
			g.selAnchorRow, g.selAnchorCol = row, col
		}
		g.selRow, g.selCol = row, col
		g.blockSelecting = g.selRow != g.selAnchorRow || g.selCol != g.selAnchorCol
		if g.selRow != prevRow && g.OnSelectRow != nil {
			g.OnSelectRow(g.selRow)
		}
		return gridCells, doubleRow
	}
	g.selRow = row
	if g.cellCursor {
		if col, ok := g.colAt(mx); ok {
			g.toggleRow, g.toggleCol = row, col
			g.selCol = col
			// Select, then activate, as the keyboard does: without the select, a
			// click on a cell-cursor grid moves the highlight without telling the
			// page, so a detail panel wired to OnSelectRow keeps describing the row
			// the keyboard left it on. Gated on an actual move like the keyboard
			// path: a page that redraws from inside OnActivateCell must not have its
			// selection callback re-entered on every toggle of the row it is on.
			if row != prevRow && g.OnSelectRow != nil {
				g.OnSelectRow(row)
			}
			g.activateCell()
			return gridCells, doubleRow
		}
		// A press on the gutter of an editable grid selects the row and paints
		// nothing; the drag still paints the cells it reaches.
		g.toggleRow, g.toggleCol = -1, -1
	}
	if row != prevRow && g.OnSelectRow != nil {
		g.OnSelectRow(g.selRow)
	}
	return gridCells, doubleRow
}

// routeDrag delivers a held Button1 to the zone its press began in.
func (g *DataGrid) routeDrag(ev *tcell.EventMouse, z gridZone) {
	mx, my := ev.Position()
	switch z {
	case gridResize:
		g.SetColumnWidth(g.resizeCol, max(minResizeWidth, g.resizeStartW+mx-g.resizeStartX))
	case gridHBar:
		g.dragHScrollbar(mx)
	case gridVBar:
		g.dragVScrollbar(my)
	case gridCells:
		g.dragCells(mx, my)
	case gridRowHeader:
		g.dragRowHeader(my)
	}
}

// dragVScrollbar points the view at the track position of screen row my.
func (g *DataGrid) dragVScrollbar(my int) {
	if dataH := g.dataRows(); dataH > 0 && g.rows.Len() > dataH {
		g.scrollRow = core.ScrollOffsetForDrag(my-(g.rect.Y+2), dataH, g.rows.Len(), dataH)
	}
}

// dragHScrollbar points the view at the track position of screen column mx (see
// DataGrid.hScrollbar). core.ScrollOffsetForDrag gives the character offset the
// track position asks for, clamped so the last screenful can't be scrolled past;
// colAtOffset rounds it up to a column boundary since Draw never splits a cell.
// core.HandleScrollbarDragH's math can't serve: it treats the track width as
// the visible count, while this track is characters wide and scrolls a column
// index.
func (g *DataGrid) dragHScrollbar(mx int) {
	if x, _, w, total, visible, _, ok := g.hScrollbar(); ok {
		g.scrollCol = g.colAtOffset(core.ScrollOffsetForDrag(mx-x, w, total, visible))
	}
}

// dragCells continues a gridCells gesture at (mx, my). A selection follows the
// pointer past an edge: the pointer is clamped to the row and column just
// beyond the view, and the view scrolls one of each per motion event (tuikit
// has no timer to scroll while the pointer rests). A toggle grid paints only
// cells actually under the pointer, never scrolling onto ones the user cannot
// see.
func (g *DataGrid) dragCells(mx, my int) {
	if g.rows.Len() == 0 {
		return
	}
	prevRow := g.selRow
	if g.cellCursor && g.editable() {
		row := g.rowAtY(my)
		if row < 0 || !g.rect.Contains(mx, my) {
			return
		}
		g.selRow = row
		// Same cell as the last press or drag-move: don't re-toggle on resends from one
		// stationary click.
		if col, ok := g.colAt(mx); ok && (row != g.toggleRow || col != g.toggleCol) {
			g.toggleRow, g.toggleCol = row, col
			g.selCol = col
			if row != prevRow && g.OnSelectRow != nil {
				g.OnSelectRow(row)
			}
			g.activateCell()
			return
		}
	} else {
		g.selRow = g.dragRow(my)
		if g.cellCursor {
			g.selCol = g.dragCol(mx)
			g.blockSelecting = g.selRow != g.selAnchorRow || g.selCol != g.selAnchorCol
			g.ensureVisibleCol()
		}
		g.ensureVisible(g.dataRows())
	}
	// Only on a move, as the keyboard fires: tcell resends Button1 on every motion
	// while held, and a host reloading on selection (Replication Monitor's agent
	// history) re-read on each one.
	if g.selRow != prevRow && g.OnSelectRow != nil {
		g.OnSelectRow(g.selRow)
	}
}

// dragRow is the data row a selection drag at screen row my reaches: the row
// under it, the last row for the blank space below it, and the row just outside
// the view when my is above the data area (the header included) or below it, so
// ensureVisible scrolls one row toward the pointer.
func (g *DataGrid) dragRow(my int) int {
	top, dataH := g.rect.Y+2, g.dataRows()
	line := core.Clamp(my-top, -1, max(0, dataH))
	return core.Clamp(g.scrollRow+line, 0, g.rows.Len()-1)
}

// dragCol is dragRow's horizontal counterpart: the column under mx, the last
// column for blank space right of it, and the column just outside the view when
// mx is left of the cells (the gutter included) or right of the grid.
func (g *DataGrid) dragCol(mx int) int {
	last := max(0, len(g.colWidths)-1)
	switch {
	case mx < g.rect.X+g.gutterWidth():
		return max(0, g.scrollCol-1)
	case mx >= g.rect.Right():
		if col, ok := g.colAt(g.rect.Right() - 1); ok {
			return min(last, col+1)
		}
		return last
	}
	if col, ok := g.colAt(mx); ok {
		return col
	}
	return last
}

// inGutter reports whether screen x falls on the row-number gutter (never, with
// SetRowNumbers off).
func (g *DataGrid) inGutter(x int) bool {
	gw := g.gutterWidth()
	return gw > 0 && x >= g.rect.X && x < g.rect.X+gw
}

// rowHeaderPress handles a fresh Button1 press on the row-number gutter, over
// data row row, and returns the zone owning the rest of the gesture: it selects
// the whole row, as SSMS's row header does. Shift (or Alt) extends from the
// current anchor, and Ctrl toggles the row in the discontiguous selection, once
// per press as on a cell. The selection is a block spanning all columns, so Copy
// and a Shift+Arrow afterwards act on whole rows.
func (g *DataGrid) rowHeaderPress(ev *tcell.EventMouse, row int) gridZone {
	if ev.Modifiers()&tcell.ModCtrl != 0 {
		g.markRow(row)
		g.selRow, g.selCol = row, 0
		g.selAnchorRow, g.selAnchorCol = row, 0
		g.blockSelecting = false
		if g.OnSelectRow != nil {
			g.OnSelectRow(g.selRow)
		}
		return gridMark
	}
	g.ClearMarkedRows()
	if ev.Modifiers()&extendSelectionMods != 0 {
		if !g.blockSelecting {
			g.selAnchorRow = g.selRow
		}
	} else {
		g.selAnchorRow = row
	}
	g.selectRowsTo(row)
	return gridRowHeader
}

// dragRowHeader continues a gridRowHeader gesture: every row the drag crosses
// joins the selection, scrolling past an edge as a cell drag does.
func (g *DataGrid) dragRowHeader(my int) {
	if g.rows.Len() == 0 {
		return
	}
	g.selectRowsTo(g.dragRow(my))
	g.ensureVisible(g.dataRows())
}

// selectRowsTo moves the moving corner of a whole-row selection to row.
func (g *DataGrid) selectRowsTo(row int) {
	prevRow := g.selRow
	g.selAnchorCol = 0
	g.selRow, g.selCol = row, max(0, len(g.columns)-1)
	g.blockSelecting = g.selRow != g.selAnchorRow || g.selCol != g.selAnchorCol
	if g.selRow != prevRow && g.OnSelectRow != nil {
		g.OnSelectRow(g.selRow)
	}
}

// horizontalWheelCols is how many data columns one horizontal wheel tick
// scrolls. Not core.WheelCells: the grid scrolls sideways by whole columns,
// which are many cells wide.
const horizontalWheelCols = 1

// scrollColBy shifts scrollCol by delta (negative scrolls left), clamped
// to the valid column range.
func (g *DataGrid) scrollColBy(delta int) {
	g.scrollCol = core.Clamp(g.scrollCol+delta, 0, max(0, len(g.columns)-1))
}

// sepColAt returns the column whose right-hand separator is drawn at (x, y), the
// column a drag there resizes. Only the header row grabs: the glyph runs down
// every data row and claiming it there would steal clicks from cell selection.
func (g *DataGrid) sepColAt(x, y int) (col int, ok bool) {
	if y != g.rect.Y || g.rect.H < 3 {
		return 0, false
	}
	cx := g.rect.X + g.gutterWidth()
	for i := g.scrollCol; i < len(g.colWidths); i++ {
		cx += g.colWidths[i]
		// drawRow omits the separator once it would fall outside the rect.
		if cx-1 >= g.rect.Right() {
			break
		}
		if x == cx-1 {
			return i, true
		}
	}
	return 0, false
}

// rowAtY returns the data row drawn at screen row y, or -1 for the header,
// separator, status bar and blank filler below the last row.
func (g *DataGrid) rowAtY(y int) int {
	dataH := g.dataRows()
	line := y - g.rect.Y - 2
	if line < 0 || line >= dataH {
		return -1
	}
	row := g.scrollRow + line
	if row >= g.rows.Len() {
		return -1
	}
	return row
}

// colAtOffset returns the first column starting at or after character offset
// off: the inverse of the running sum hScrollbar reports. It rounds up, not
// down: the track's end asks for offset total-visible, and the column starting
// before it left the last columns cut off, reachable only with the arrow keys.
// Rounding up there leaves at most visible characters to the right, so the
// last column ends on screen.
func (g *DataGrid) colAtOffset(off int) int {
	acc := 0
	for i, cw := range g.colWidths {
		if acc >= off {
			return i
		}
		acc += cw
	}
	return max(0, len(g.colWidths)-1)
}

// colAt returns the column whose cell contains screen x in cell-cursor mode,
// honouring horizontal scroll and the gutter. ok is false if x falls outside
// every column, the gutter included.
func (g *DataGrid) colAt(x int) (col int, ok bool) {
	cx := g.rect.X + g.gutterWidth()
	for i := g.scrollCol; i < len(g.colWidths); i++ {
		w := g.colWidths[i]
		if x >= cx && x < cx+w {
			return i, true
		}
		cx += w
	}
	return 0, false
}

// ensureVisible scrolls vertically so selRow is on screen, given the data area's
// height (rect.H less the header and status rows).
//
// A grid not yet laid out has rect.H == 0, so dataH is 0 and the second
// test below is true for every row: the scroll would jump past the whole list and
// the next Draw paints the header and "N rows" over blank lines. Callers reach a
// grid before its first SetBounds (SetSelectedCell via SetDataPreservingView), so
// the scroll is left alone until there is a viewport.
func (g *DataGrid) ensureVisible(dataH int) {
	if dataH <= 0 {
		return
	}
	if g.selRow < g.scrollRow {
		g.scrollRow = g.selRow
	}
	if g.selRow >= g.scrollRow+dataH {
		g.scrollRow = g.selRow - dataH + 1
	}
}

// ensureVisibleCol scrolls horizontally so selCol is on screen. Columns vary in
// width, so it walks scrollCol rightward until selCol fits.
func (g *DataGrid) ensureVisibleCol() {
	if g.selCol < g.scrollCol {
		g.scrollCol = g.selCol
		return
	}
	// Before the first SetBounds there is no width to fit into, and a negative avail
	// walks scrollCol all the way to selCol, scrolling the first column off a grid
	// nobody has drawn. See ensureVisible.
	avail := g.rect.W - g.gutterWidth()
	if avail <= 0 {
		return
	}
	for g.scrollCol < g.selCol {
		w := 0
		for i := g.scrollCol; i <= g.selCol && i < len(g.colWidths); i++ {
			w += g.colWidths[i]
		}
		if w <= avail {
			break
		}
		g.scrollCol++
	}
}

// rowDoubleClicked reports whether a fresh Button1 press on data row row is the
// second of a double-click there, and records it as the press the next one pairs
// with. Called only on the press, never on the resends while it is held (the
// gesture routes those). A press with a modifier is a selection gesture, never
// half of a double-click.
func (g *DataGrid) rowDoubleClicked(ev *tcell.EventMouse, row int) bool {
	if g.OnRowDoubleClick == nil || g.editable() || g.errorMode ||
		ev.Modifiers()&(tcell.ModCtrl|extendSelectionMods) != 0 {
		g.rowClicks.Reset()
		return false
	}
	return g.rowClicks.Press(row, ev.When())
}

// activateCell fires OnActivateCell for grids with editable cells (toggle grids,
// permission-state cycling). A grid leaving it nil does nothing here
// (right-click's "Show Value" opens the viewer), nor does a browse-only grid; see
// SetBrowseOnly. It reports whether the callback ran.
func (g *DataGrid) activateCell() bool {
	if !g.editable() {
		return false
	}
	g.OnActivateCell(g.selRow, g.selCol)
	return true
}
