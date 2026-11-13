package controls

import (
	"slices"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/theme"
)

// ---------------------------------------------------------------------------
// menuBox / menuCascade — one open menu and its submenu chain
// ---------------------------------------------------------------------------

// menuBox is one bordered box of an open menu: a MenuBar's dropdown, a
// ContextMenu, or any submenu level under either. Every level draws, scrolls,
// hit-tests and takes keys through the same code, so a context menu or a
// submenu taller than the terminal scrolls exactly as a dropdown does instead
// of losing its rows past the bottom edge.
type menuBox struct {
	items []MenuItem
	hover int // highlighted item index, -1 for none

	// top is the index of the first item drawn, for a box taller than the rows
	// the screen leaves it. Draw keeps hover inside the window.
	top int

	// rect is where the box was last drawn, after every screen clamp. Only Draw
	// sees a tcell.Screen, so HandleMouse hit-tests this cache rather than
	// recomputing — otherwise a box shifted off an edge draws in one place and
	// takes clicks in another. Hosts seed it with the unclamped box when the menu
	// opens, so a menu driven without a Draw still hit-tests where it would paint.
	rect core.Rect

	// rows holds each item's Enabled and suffix as measure last evaluated them:
	// once per item per draw, since gossms's Enabled predicates run queries of
	// their own state and the width, the styles and the suffix all need them.
	rows []menuRowState
}

// menuRowState is one item's per-draw evaluation (menuBox.rows).
type menuRowState struct {
	enabled bool
	suffix  string
}

// measure evaluates every item's state for this draw and returns the box
// width that fits each label and suffix, floored at minW.
func (b *menuBox) measure(minW int) int {
	b.rows = slices.Grow(b.rows[:0], len(b.items))
	w := minW
	for _, it := range b.items {
		st := menuRowState{enabled: it.enabled()}
		st.suffix = rowSuffix(it, st.enabled)
		b.rows = append(b.rows, st)
		w = max(w, menuRowWidth(it.Label, st.suffix))
	}
	return w
}

// liveWidth is measure without the cache, for a rect seeded before any draw.
func (b *menuBox) liveWidth(minW int) int {
	w := minW
	for _, it := range b.items {
		w = max(w, menuRowWidth(it.Label, menuRowSuffix(it)))
	}
	return w
}

// menuRowWidth is the box width one row needs. +6: 2 columns of border/inset
// padding plus a guaranteed 2-column gap between label and suffix for
// whichever item ends up defining the width; without the margin that widest
// item's label and shortcut land flush against each other.
func menuRowWidth(label, suffix string) int {
	return core.DisplayWidth(label) + core.DisplayWidth(suffix) + 6
}

// place sets rect from where the host wants the box, clamped to the screen on
// all four sides. A box that would run off the bottom is shifted up, unless
// pinned (a dropdown hangs off its header); either way a box still taller than
// the screen is cut to it and scrolls. The lower clamps matter as much as the
// upper: a box wider or taller than the screen gives a negative column or
// height, and a negative height turns the box inside out.
func (b *menuBox) place(s tcell.Screen, x, y, w int, pinned bool) {
	sw, sh := s.Size()
	x = core.Clamp(x, 0, max(sw-1, 0))
	w = core.Clamp(w, 1, max(sw-x, 1))
	h := len(b.items) + 2
	if !pinned && y+h > sh {
		y = sh - h
	}
	y = max(y, 0)
	h = core.Clamp(h, 3, max(sh-y, 3))
	b.rect = core.Rect{X: x, Y: y, W: w, H: h}
}

// visibleRows is how many item rows the drawn box has.
func (b *menuBox) visibleRows() int { return b.rect.H - 2 }

// paint draws the box at rect, scrolled so hover is in view. measure must
// have run first in the same draw.
func (b *menuBox) paint(s tcell.Screen) {
	rows := b.visibleRows()
	b.top = clampMenuScroll(b.top, b.hover, len(b.items), rows)
	borderStyle := theme.StyleMenuLine()
	core.FillRect(s, b.rect, ' ', theme.StyleMenuBar())
	core.DrawBox(s, b.rect, borderStyle)
	for i := range rows {
		item := b.top + i
		if item >= len(b.items) {
			break
		}
		drawMenuRow(s, b.rect.X, b.rect.Y+1+i, b.rect.W, b.items[item], b.rows[item], item == b.hover, borderStyle)
	}
	drawMenuScrollMarks(s, b.rect, b.top, len(b.items), rows, borderStyle)
}

// rowY returns the screen row item i is drawn on (it may be scrolled out).
func (b *menuBox) rowY(i int) int { return b.rect.Y + 1 + i - b.top }

// itemAt returns the item index drawn at screen row my, or -1 on a border row.
func (b *menuBox) itemAt(my int) int {
	row := my - b.rect.Y - 1
	if row < 0 || row >= b.visibleRows() || b.top+row >= len(b.items) {
		return -1
	}
	return b.top + row
}

// wheel scrolls a box taller than its window by one wheel tick and reports
// whether the hover moved (it follows the window: paint scrolls back to the
// hover, so leaving it behind would make the next draw undo the scroll).
func (b *menuBox) wheel(ev *tcell.EventMouse) (hoverMoved bool) {
	rows, n := b.visibleRows(), len(b.items)
	if rows <= 0 || n <= rows {
		return false
	}
	switch ev.Buttons() {
	case tcell.WheelUp:
		b.top = core.Clamp(b.top-core.WheelRows, 0, n-rows)
	case tcell.WheelDown:
		b.top = core.Clamp(b.top+core.WheelRows, 0, n-rows)
	default:
		return false
	}
	if b.hover < 0 {
		return false
	}
	h := core.Clamp(b.hover, b.top, b.top+rows-1)
	moved := h != b.hover
	b.hover = h
	return moved
}

// menuCascade is an open menu: boxes[0] is the root (a dropdown or a context
// menu) and boxes[i+1] is the submenu of boxes[i]'s hovered item. Everything a
// host does inside the boxes — keys, hover, clicks, the wheel — goes through
// it; the host owns only what lies outside (the header strip, dismissal).
type menuCascade struct {
	boxes []menuBox
}

// setRoot shows items as the root with nothing open under it.
func (c *menuCascade) setRoot(items []MenuItem, hover int) {
	c.boxes = append(c.boxes[:0], menuBox{items: items, hover: hover})
}

// root returns the root box; setRoot must have run.
func (c *menuCascade) root() *menuBox { return &c.boxes[0] }

// depth returns how many submenu levels are open (0 = none).
func (c *menuCascade) depth() int { return max(len(c.boxes)-1, 0) }

// reset closes every open submenu.
func (c *menuCascade) reset() { c.popTo(0) }

// popTo closes every submenu deeper than level.
func (c *menuCascade) popTo(level int) {
	level = max(level, 0)
	if level+1 < len(c.boxes) {
		c.boxes = c.boxes[:level+1]
	}
}

// openAt opens the submenu of item row at the given level, closing anything
// deeper first, and reports whether that item has one. Opening the submenu
// that is already open at that row is a no-op, so a mouse hover resent on
// every motion event doesn't reset its highlighted row.
func (c *menuCascade) openAt(level, row int) bool {
	if level >= len(c.boxes) {
		return false
	}
	items := c.boxes[level].items
	if row < 0 || row >= len(items) || len(items[row].Sub) == 0 {
		return false
	}
	if len(c.boxes) > level+1 && c.boxes[level].hover == row {
		c.popTo(level + 1)
		return true
	}
	c.popTo(level)
	c.boxes[level].hover = row
	sub := items[row].Sub
	c.boxes = append(c.boxes, menuBox{items: sub, hover: firstSelectableItem(sub)})
	return true
}

// sync re-reads each submenu's items from its parent's hovered item, since a
// host may SetMenus while a submenu is open, and closes any level whose
// parent no longer opens one.
func (c *menuCascade) sync() {
	for i := 1; i < len(c.boxes); i++ {
		p := &c.boxes[i-1]
		if p.hover < 0 || p.hover >= len(p.items) || len(p.items[p.hover].Sub) == 0 {
			c.boxes = c.boxes[:i]
			return
		}
		c.boxes[i].items = p.items[p.hover].Sub
	}
}

// draw paints the root at rect (placed by the host after measure) and every
// open submenu: to the right of its parent, first row aligned with the item
// that opened it, flipped to the parent's left when it wouldn't fit.
func (c *menuCascade) draw(s tcell.Screen) {
	c.sync()
	sw, _ := s.Size()
	c.boxes[0].paint(s)
	for i := 1; i < len(c.boxes); i++ {
		parent, b := &c.boxes[i-1], &c.boxes[i]
		w := b.measure(20)
		x := parent.rect.X + parent.rect.W
		if x+w > sw {
			x = parent.rect.X - w
		}
		b.place(s, x, parent.rowY(parent.hover)-1, w, false)
		b.paint(s)
	}
}

// hit reports which open level the point falls in, and the item drawn there
// (-1 on a border row). Deepest level first, so a submenu overlapping its
// parent isn't robbed of the click by the box underneath.
func (c *menuCascade) hit(mx, my int) (level, item int, ok bool) {
	for i, b := range slices.Backward(c.boxes) {
		if b.rect.Contains(mx, my) {
			return i, b.itemAt(my), true
		}
	}
	return 0, -1, false
}

// menuKey is what menuCascade.handleKey left for its host to do.
type menuKey int

const (
	menuKeyDone      menuKey = iota // consumed; nothing more to do
	menuKeyUnhandled                // the host's: Left/Right on the root, F10, anything unbound
	menuKeyDismiss                  // Escape on the root: close the menu
	menuKeyRun                      // close the menu, then call run if non-nil
)

// handleKey processes a key for the deepest open level. On menuKeyRun the
// host closes the whole menu before calling run, so an action never fires with
// a stale menu still on screen.
func (c *menuCascade) handleKey(ev *tcell.EventKey) (res menuKey, run func()) {
	c.sync()
	d := len(c.boxes) - 1
	b := &c.boxes[d]
	switch ev.Key() {
	case tcell.KeyEscape:
		if d == 0 {
			return menuKeyDismiss, nil
		}
		c.popTo(d - 1)
	case tcell.KeyLeft:
		if d == 0 {
			return menuKeyUnhandled, nil
		}
		c.popTo(d - 1)
	case tcell.KeyRight:
		if !c.openAt(d, b.hover) && d == 0 {
			return menuKeyUnhandled, nil
		}
	case tcell.KeyUp:
		b.hover = stepSelectableItem(b.items, b.hover, -1)
	case tcell.KeyDown:
		b.hover = stepSelectableItem(b.items, b.hover, 1)
	case tcell.KeyEnter:
		if b.hover < 0 || b.hover >= len(b.items) || c.openAt(d, b.hover) {
			return menuKeyDone, nil
		}
		item := b.items[b.hover]
		if !item.Divider && item.enabled() {
			return menuKeyRun, item.Action
		}
		if d == 0 {
			return menuKeyRun, nil // a root leaf that can't act still closes the menu
		}
	default:
		return menuKeyUnhandled, nil
	}
	return menuKeyDone, nil
}

// handleMouse processes an event that hit reported inside the menu, at
// (level, item). fresh is the host's PressLatch verdict on it. It reports
// whether the host must close the whole menu, and the action (possibly nil)
// to call after.
//
// A click anywhere inside the menu that can't act — a divider, a disabled
// row, a border — still dismisses it, or a disabled item leaves the popup
// stuck open. Hovering a row opens its submenu and closes any sibling's.
func (c *menuCascade) handleMouse(ev *tcell.EventMouse, level, item int, fresh bool) (dismiss bool, run func()) {
	b := &c.boxes[level]
	if ev.Buttons()&(tcell.WheelUp|tcell.WheelDown) != 0 {
		if b.wheel(ev) {
			c.popTo(level) // the hovered row moved, so its submenu is no longer its own
		}
		return false, nil
	}
	if item < 0 {
		return fresh, nil
	}
	it := b.items[item]
	if it.Divider || !it.enabled() {
		return fresh, nil
	}
	if !c.openAt(level, item) {
		b.hover = item
		c.popTo(level)
	}
	if fresh && len(it.Sub) == 0 {
		return true, it.Action
	}
	return false, nil
}

// clampMenuScroll returns the first-item index keeping sel visible in a window
// of rows items out of n, scrolling as little as possible. rows <= 0 yields 0
// rather than a negative index.
func clampMenuScroll(top, sel, n, rows int) int {
	if rows <= 0 || n <= rows {
		return 0
	}
	top = core.Clamp(top, 0, n-rows)
	if sel < 0 {
		return top
	}
	if sel < top {
		return sel
	}
	if sel >= top+rows {
		return sel - rows + 1
	}
	return top
}

// drawMenuScrollMarks paints the ▲/▼ saying a scrolled menu box has items past
// its top or bottom edge. Without them a partial menu is indistinguishable from
// a complete one.
func drawMenuScrollMarks(s tcell.Screen, r core.Rect, top, n, rows int, style tcell.Style) {
	if rows <= 0 || n <= rows {
		return
	}
	if top > 0 {
		core.DrawText(s, r.X+r.W-2, r.Y, style, "▲")
	}
	if top+rows < n {
		core.DrawText(s, r.X+r.W-2, r.Y+r.H-1, style, "▼")
	}
}
