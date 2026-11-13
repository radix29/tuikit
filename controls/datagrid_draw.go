package controls

import (
	"strconv"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/theme"
)

// Draw renders the data grid. If the cell-content popup is open, call
// DrawOverlay afterwards, once every other widget in the frame has drawn.
func (g *DataGrid) Draw(s tcell.Screen) {
	// A RefreshColumnWidths since the last frame takes effect now (see there).
	if g.widthsDirty {
		g.computeColWidths()
	}
	core.FillRect(s, g.rect, ' ', theme.StylePanel())
	if g.rect.H < 3 {
		return
	}
	gw := g.gutterWidth()
	if gw > 0 {
		g.drawGutterCell(s, g.rect.Y, "", theme.StyleGridHeader())
	}
	nulls, _ := g.rows.(NullSource)
	g.drawRow(s, g.rect.Y, -1, g.headerCells(), nil, theme.StyleGridHeader(), gw, 1, 0)
	sep := theme.On(theme.Active().GridHeader, theme.Active().GridBorder)
	core.DrawHLine(s, g.rect.X, g.rect.Y+1, g.rect.W, sep)

	r0, c0, r1, c1 := g.SelectionBounds()
	dataH := g.dataRows()
	for row := range dataH {
		dataIdx := g.scrollRow + row
		y := g.rect.Y + 2 + row
		if dataIdx >= g.rows.Len() {
			core.FillRect(s, core.Rect{X: g.rect.X, Y: y, W: g.rect.W, H: 1}, ' ', theme.StylePanel())
			continue
		}
		style := theme.StyleGridRow()
		if dataIdx%2 == 1 {
			style = theme.StyleGridRowAlt()
		}
		kind := g.RowKindAt(dataIdx)
		if kind == RowMarked {
			style = style.Foreground(theme.Active().Warning)
		}
		if dataIdx == g.selRow && !g.cellCursor {
			style = theme.StyleGridSelected()
		}
		if gw > 0 {
			g.drawGutterCell(s, y, strconv.Itoa(dataIdx+1), style)
		}
		cells := g.rows.Row(dataIdx)
		if kind == RowGroup {
			g.drawGroupRow(s, y, cells, gw, dataIdx == g.selRow || g.rowMarked(dataIdx) ||
				(g.cellCursor && dataIdx >= r0 && dataIdx <= r1))
			continue
		}
		// A Ctrl+click-marked row is highlighted whole: the marked set is rows,
		// not cells, so a rectangle's column range says nothing about it.
		selFrom, selTo := 1, 0 // none
		switch {
		case g.rowMarked(dataIdx):
			selFrom, selTo = 0, max(0, len(g.columns)-1)
		case g.cellCursor && dataIdx >= r0 && dataIdx <= r1:
			selFrom, selTo = c0, c1
		}
		g.drawRow(s, y, dataIdx, cells, nulls, style, gw, selFrom, selTo)
	}

	// Status bar
	p := theme.Active()
	statusStyle := theme.On(p.GridHeader, p.TextDim)
	if g.statusStyle != nil {
		statusStyle = g.statusStyle()
	}
	core.FillRect(s, core.Rect{X: g.rect.X, Y: g.rect.Y + g.rect.H - 1, W: g.rect.W, H: 1}, ' ', statusStyle)
	core.DrawTextRight(s, g.rect.X+1, g.rect.Y+g.rect.H-1, g.rect.W-2, statusStyle, g.status)

	// Scrollbars
	sbStyle := theme.On(p.GridHeader, p.Border)
	sbThumb := theme.StyleScrollThumb()
	if g.showsVScrollbar() {
		core.DrawScrollbar(s, g.rect.Right()-1, g.rect.Y+2, dataH,
			g.rows.Len(), dataH, g.scrollRow, sbStyle, sbThumb)
	}
	if x, y, w, total, visible, offset, ok := g.hScrollbar(); ok {
		hStyle := sbStyle
		if g.statusStyle != nil {
			// The status row is the grid's own colour only when not overridden (the
			// query-results grid paints it yellow); keep the track on whatever the row is so
			// the bar doesn't sit in a stripe of its own.
			hStyle = statusStyle.Foreground(p.Border)
		}
		core.DrawScrollbarH(s, x, y, w, total, visible, offset, hStyle, sbThumb)
	}
}

// hScrollbar returns the horizontal scrollbar's screen span and the
// character-space total/visible/offset describing it, or ok false when every
// column fits (or there's no room).
//
// It shares the status row rather than taking a data row: the status text is
// right-aligned, so the row's left is free, and the grid's bottom edge is where a
// horizontal bar belongs. Measurements are in characters, not column counts:
// columns differ in width, so a column-counting thumb would jump past a wide one.
func (g *DataGrid) hScrollbar() (x, y, w, total, visible, offset int, ok bool) {
	if g.rect.H < 3 || len(g.colWidths) == 0 {
		return 0, 0, 0, 0, 0, 0, false
	}
	gutter := g.gutterWidth()
	visible = g.rect.W - gutter
	if g.showsVScrollbar() {
		// The vertical bar overdraws the rightmost column (see Draw), so a scroll
		// that fits the last column into the full width still hid its right edge.
		visible--
	}
	for i, cw := range g.colWidths {
		total += cw
		if i < g.scrollCol {
			offset += cw
		}
	}
	if total <= visible || visible <= 0 {
		return 0, 0, 0, 0, 0, 0, false
	}
	// Stop short of the right-aligned status text, and of the vertical
	// scrollbar's column above it, so the two never collide.
	x, y = g.rect.X, g.rect.Y+g.rect.H-1
	w = g.rect.W - core.DisplayWidth(g.status) - 3
	if w < hScrollbarMinWidth {
		return 0, 0, 0, 0, 0, 0, false
	}
	return x, y, w, total, visible, offset, true
}

// hScrollbarMinWidth is the narrowest track worth drawing: below it the thumb
// says nothing useful and the status text is the better use of the row.
const hScrollbarMinWidth = 8

// drawGutterCell renders one row-number column cell (or the blank header cell)
// at y: right-aligned and dim, since unlike data columns it is never selectable.
func (g *DataGrid) drawGutterCell(s tcell.Screen, y int, text string, style tcell.Style) {
	w := g.gutterWidth()
	p := theme.Active()
	gstyle := style.Foreground(p.TextDim)
	core.FillRect(s, core.Rect{X: g.rect.X, Y: y, W: w, H: 1}, ' ', gstyle)
	core.DrawTextRight(s, g.rect.X, y, w-1, gstyle, text)
	core.PutRune(s, g.rect.X+w-1, y, '|', style.Foreground(p.GridBorder))
}

// drawRow renders cells starting at the scrollCol-th column, at screen x
// xOffset+g.rect.X (xOffset reserves the row-number gutter, 0 when off).
// scrollCol, like scrollRow, is a data index (leading columns hidden), not a
// pixel offset, so a scrolled grid's columns start flush left and boundaries
// never split mid-cell. row is the data row, for nulls (nil for the header row);
// a NULL cell is dimmed.
//
// Columns selFrom..selTo (none when selFrom > selTo) draw in the selection
// style, dimmed while the grid is inactive — a marked row, or the row's slice of
// the cell-cursor block. A selected column past the row's last cell still
// draws, blank. One pass: painting the row and then its selection over it drew
// every selected cell twice.
func (g *DataGrid) drawRow(s tcell.Screen, y, row int, cells []string, nulls NullSource, style tcell.Style, xOffset, selFrom, selTo int) {
	p := theme.Active()
	selSt := theme.StyleGridSelected()
	if !g.active {
		selSt = theme.On(p.GridRowAlt, p.TextHighlight)
	}
	col := g.rect.X + xOffset
	for i := g.scrollCol; i < len(g.colWidths); i++ {
		cw := g.colWidths[i]
		if col >= g.rect.Right() {
			break
		}
		selected := i >= selFrom && i <= selTo
		if i >= len(cells) && !selected {
			col += cw
			continue
		}
		var cell string
		if i < len(cells) {
			cell = cells[i]
		}
		cellStyle, sepStyle := style, style
		if selected {
			cellStyle = selSt
		}
		if nulls != nil && nulls.IsNull(row, i) {
			cellStyle = cellStyle.Foreground(p.TextDim)
		}
		if selected {
			sepStyle = cellStyle
		}
		avail := min(cw, g.rect.Right()-col)
		core.FillRect(s, core.Rect{X: col, Y: y, W: avail, H: 1}, ' ', cellStyle)
		core.DrawTextLine(s, col+1, y, avail-2, cellStyle, cell)
		if col+cw-1 < g.rect.Right() {
			core.PutRune(s, col+cw-1, y, '|', sepStyle.Foreground(p.GridBorder))
		}
		col += cw
	}
}

// drawGroupRow renders a RowGroup row at y: its first cell a label, not scrolled
// with the columns, spilling across the empty cells to its right as a
// spreadsheet's text does. It stops at the first non-empty cell on screen,
// clipped with "…"; from there cells draw as an ordinary row's, scrolled with
// their columns. selected highlights the row whole, dimmed while unfocused.
func (g *DataGrid) drawGroupRow(s tcell.Screen, y int, cells []string, xOffset int, selected bool) {
	st := theme.StyleGridHeader()
	if selected {
		st = theme.StyleGridSelected()
		if !g.active {
			p := theme.Active()
			st = theme.On(p.GridRowAlt, p.TextHighlight)
		}
	}
	r := core.Rect{X: g.rect.X + xOffset, Y: y, W: g.rect.W - xOffset, H: 1}
	core.FillRect(s, r, ' ', st)
	// The first non-empty cell on screen, and its x. The label is cell 0,
	// never a cell here: scrolled or not, it is drawn from the left edge.
	stop, stopX := -1, r.Right()
	col := r.X
	for i := g.scrollCol; i < len(g.colWidths) && col < r.Right(); i++ {
		if i > 0 && i < len(cells) && cells[i] != "" {
			stop, stopX = i, col
			break
		}
		col += g.colWidths[i]
	}
	if len(cells) > 0 {
		// A cell's separator sits in the column before it: the label ends one
		// short of that, as a cell's text ends one short of its own.
		w := stopX - r.X - 2
		if stop < 0 {
			w = r.W - 2
		}
		core.DrawTextLine(s, r.X+1, y, w, st, cells[0])
	}
	if stop < 0 {
		return
	}
	sep := st.Foreground(theme.Active().GridBorder)
	if stopX > r.X {
		core.PutRune(s, stopX-1, y, '|', sep)
	}
	col = stopX
	for i := stop; i < len(g.colWidths) && col < r.Right(); i++ {
		cw := g.colWidths[i]
		if i < len(cells) {
			avail := min(cw, r.Right()-col)
			core.DrawTextLine(s, col+1, y, avail-2, st, cells[i])
		}
		if col+cw-1 < r.Right() {
			core.PutRune(s, col+cw-1, y, '|', sep)
		}
		col += cw
	}
}
