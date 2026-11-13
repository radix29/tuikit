package controls

import (
	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/theme"
)

// ---------------------------------------------------------------------------
// MenuBar
// ---------------------------------------------------------------------------

// MenuBar is the horizontal application menu bar with drop-down menus: a strip
// of headers over a menuCascade, whose root box is the open menu's dropdown.
type MenuBar struct {
	rect      core.Rect
	menus     []Menu
	openMenu  int // -1 = closed
	hoverMenu int

	// press distinguishes a fresh Button1 press, which toggles the header, from a
	// continued hold over it. Without it, all-motion tracking resends Button1 on
	// every motion while the button is down, so a click that twitches re-toggles
	// the header it just opened back closed.
	press core.PressLatch

	// menu is the open dropdown (its root box) and any submenu levels under it,
	// replaced whenever the dropdown opens or switches menus. A dropdown hangs off
	// its own header and can't be shifted up as a ContextMenu can, so one taller
	// than the rows below the bar scrolls (Edit's 21 items need 23 rows; on a
	// 20-row terminal the last three would otherwise paint past the bottom edge,
	// invisible and unreachable). Capping the height alone would hide them behind
	// a tidy border, which is worse.
	menu menuCascade

	// OnBeforeOpen, when set, is called as a closed bar opens a dropdown, by
	// key or click, before the dropdown reads its items — the host's chance to
	// SetMenus afresh when an item's list (not just its enabled state, which
	// Enabled already answers live) depends on state that has moved since.
	OnBeforeOpen func()
}

// dropdownMinW is a dropdown's narrowest box.
const dropdownMinW = 28

// NewMenuBar creates a MenuBar.
func NewMenuBar() *MenuBar {
	return new(MenuBar{openMenu: -1, hoverMenu: -1})
}

// SetBounds positions the menu bar.
func (mb *MenuBar) SetBounds(x, y, w int) {
	mb.rect = core.Rect{X: x, Y: y, W: w, H: 1}
}

// LabelsEnd returns the column just past the last menu label — the left limit
// for anything sharing the bar's row, such as a right-aligned Toolbar.
func (mb *MenuBar) LabelsEnd() int { return mb.menuHeaderOffset(len(mb.menus)) }

// SetMenus replaces all menus. An open dropdown whose menu is gone closes; one
// that remains keeps its highlight when that row still exists and is
// selectable, else moves it to the first selectable row. Without this, fewer
// menus left openMenu past the end and the next key or mouse event indexed
// menus[openMenu].
func (mb *MenuBar) SetMenus(menus []Menu) {
	mb.menus = menus
	if mb.hoverMenu >= len(menus) {
		mb.hoverMenu = -1
	}
	if mb.openMenu < 0 {
		return
	}
	if mb.openMenu >= len(menus) {
		mb.Close()
		return
	}
	items := menus[mb.openMenu].Items
	root := mb.menu.root()
	root.items = items
	if root.hover >= len(items) || (root.hover >= 0 && menuItemSkippable(items[root.hover])) {
		root.hover = firstSelectableItem(items)
		mb.menu.reset()
	}
}

// OverlayActive reports whether a dropdown is open (core.Overlay).
func (mb *MenuBar) OverlayActive() bool { return mb.openMenu >= 0 }

// Bounds returns the bar's row, as SetBounds last placed it. An open dropdown
// is outside it (DrawOverlay).
func (mb *MenuBar) Bounds() core.Rect { return mb.rect }

// Close closes any open dropdown and every submenu under it.
func (mb *MenuBar) Close() {
	mb.openMenu = -1
	mb.menu.boxes = mb.menu.boxes[:0]
}

// openDropdown shows menu i's dropdown, highlighting its first selectable
// item, with the box seeded where it would draw (menuBox.rect).
func (mb *MenuBar) openDropdown(i int) {
	mb.openMenu = i
	mb.hoverMenu = i
	items := mb.menus[i].Items
	mb.menu.setRoot(items, firstSelectableItem(items))
	w := mb.menu.root().liveWidth(dropdownMinW)
	mb.menu.root().rect = core.Rect{X: mb.dropdownColumn(i, w), Y: mb.rect.Y + 1, W: w, H: len(items) + 2}
}

// Open opens the first menu without a mouse click — the F10 convention. Does
// nothing if there are no menus, or one is already open.
func (mb *MenuBar) Open() {
	if mb.openMenu < 0 && len(mb.menus) > 0 {
		if mb.OnBeforeOpen != nil {
			mb.OnBeforeOpen()
			if len(mb.menus) == 0 { // the hook may SetMenus
				return
			}
		}
		mb.openDropdown(0)
	}
}

// Draw renders just the menu bar row. Call DrawOverlay afterwards, once all
// other content has been drawn, to render any open dropdown on top: it extends
// below the bar into rows other panels draw into.
func (mb *MenuBar) Draw(s tcell.Screen) {
	barStyle := theme.StyleMenuBar()
	core.FillRect(s, mb.rect, ' ', barStyle)

	col := mb.rect.X + 1
	for i, m := range mb.menus {
		label := " " + m.Label + " "
		st := barStyle
		if i == mb.openMenu || i == mb.hoverMenu {
			st = theme.StyleMenuSelected()
		}
		core.DrawText(s, col, mb.rect.Y, st, label)
		col += core.DisplayWidth(label)
	}
}

// DrawOverlay renders the open dropdown, if any. Must be called after every
// other panel has drawn, so the dropdown isn't painted over.
func (mb *MenuBar) DrawOverlay(s tcell.Screen) {
	if mb.openMenu < 0 || mb.openMenu >= len(mb.menus) {
		return
	}
	root := mb.menu.root()
	w := root.measure(dropdownMinW)
	root.place(s, mb.dropdownColumn(mb.openMenu, w), mb.rect.Y+1, w, true)
	mb.menu.draw(s)
}

// menuHeaderOffset returns the column where the dropdown for menu index idx
// should begin, measured by display width of the preceding menu headers.
func (mb *MenuBar) menuHeaderOffset(idx int) int {
	col := mb.rect.X + 1
	for i := range idx {
		col += core.DisplayWidth(" " + mb.menus[i].Label + " ")
	}
	return col
}

// dropdownRect returns the open dropdown's box as DrawOverlay last placed it.
func (mb *MenuBar) dropdownRect() core.Rect {
	if mb.openMenu < 0 {
		return core.Rect{}
	}
	return mb.menu.root().rect
}

// HandleKey processes keyboard when a dropdown is open.
func (mb *MenuBar) HandleKey(ev *tcell.EventKey) bool {
	if mb.openMenu < 0 {
		return false
	}
	switch res, run := mb.menu.handleKey(ev); res {
	case menuKeyDismiss:
		mb.Close()
	case menuKeyRun:
		mb.Close()
		if run != nil {
			run()
		}
	case menuKeyUnhandled:
		// Left and Right on the dropdown itself (Right only when the item has no
		// submenu to open) move between headers.
		switch ev.Key() {
		case tcell.KeyF10:
			mb.Close()
		case tcell.KeyLeft:
			mb.openDropdown((mb.openMenu - 1 + len(mb.menus)) % len(mb.menus))
		case tcell.KeyRight:
			mb.openDropdown((mb.openMenu + 1) % len(mb.menus))
		}
	}
	return true
}

// HandleMouse processes mouse events for the bar and any open dropdown. While a
// dropdown is open every event is swallowed, so nothing underneath can react or
// take focus; a hover outside never closes it, only a click does.
func (mb *MenuBar) HandleMouse(ev *tcell.EventMouse) bool {
	mx, my := ev.Position()
	wasOpen := mb.openMenu >= 0

	held := mb.press.Held()
	released := mb.press.Release(ev)

	// Cleared off the bar's row as well, or a label stays lit after the pointer
	// leaves it downwards.
	mb.hoverMenu = -1
	if my == mb.rect.Y {
		col := mb.rect.X + 1
		for i, m := range mb.menus {
			label := " " + m.Label + " "
			labelW := core.DisplayWidth(label)
			if mx >= col && mx < col+labelW {
				mb.hoverMenu = i
				if mb.press.Fresh(ev) {
					if mb.openMenu == i {
						mb.Close()
					} else {
						mb.Close()
						if mb.OnBeforeOpen != nil {
							mb.OnBeforeOpen()
							if i >= len(mb.menus) { // the hook may SetMenus
								return true
							}
						}
						mb.openDropdown(i)
					}
				}
				// Hover lights the label but is not claimed while every menu is
				// closed (core.MouseHandler).
				return !released || held || wasOpen
			}
			col += labelW
		}
		// On the bar row but off every label, over the toolbar say: a press
		// still dismisses an open menu. Only a right-click falls through
		// (outsideMenu).
		if !wasOpen {
			return false
		}
		dismiss, consumed := outsideMenu(ev)
		if dismiss {
			mb.Close()
		}
		return consumed || ev.Buttons() == tcell.ButtonNone
	}

	if wasOpen {
		level, item, inside := mb.menu.hit(mx, my)
		if !inside {
			// Motion outside is swallowed while the dropdown is open: the bar
			// owns the pointer until it closes.
			dismiss, consumed := outsideMenu(ev)
			if dismiss {
				mb.Close()
			}
			return consumed || ev.Buttons() == tcell.ButtonNone
		}
		if dismiss, run := mb.menu.handleMouse(ev, level, item, mb.press.Fresh(ev)); dismiss {
			mb.Close()
			if run != nil {
				run()
			}
		}
		return true
	}
	return false
}

// dropdownColumn returns where the dropdown for menu idx starts given its
// width w, shifted left to stay inside the bar; menuBox.place then clamps it
// to the screen.
func (mb *MenuBar) dropdownColumn(idx, w int) int {
	return min(mb.menuHeaderOffset(idx), mb.rect.X+mb.rect.W-w)
}
