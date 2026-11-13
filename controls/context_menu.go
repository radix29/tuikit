package controls

import (
	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/core"
)

// ---------------------------------------------------------------------------
// ContextMenu
// ---------------------------------------------------------------------------

// ContextMenu is a floating popup menu shown on right-click.
type ContextMenu struct {
	x, y    int // requested position, from Show() — unclamped
	visible bool

	// menu is the root box and any open submenu levels (menuCascade); its rects
	// are where DrawOverlay last placed each box, which HandleMouse hit-tests.
	// Show resets it, so a cascade never survives into the menu's next showing —
	// the same rule ModalDialog.Show follows for its drag latch.
	menu menuCascade

	// press distinguishes a fresh Button1 press from a continued hold,
	// needed once a click can leave the menu open: clicking a cascade item keeps the
	// popup up, and tcell's all-motion tracking resends Button1 on every motion
	// while held, so without the latch a press that twitches over a leaf item would
	// fire its Action repeatedly.
	press core.PressLatch
}

// contextMenuMinW is a context menu's (and every submenu's) narrowest box.
const contextMenuMinW = 20

// Show displays the menu at (x,y) with the given items.
func (cm *ContextMenu) Show(x, y int, items []MenuItem) {
	cm.x, cm.y = x, y
	cm.menu.setRoot(items, -1)
	root := cm.menu.root()
	root.rect = core.Rect{X: x, Y: y, W: root.liveWidth(contextMenuMinW), H: len(items) + 2}
	cm.visible = true
	cm.press.Clear()
}

// SetHover puts the keyboard cursor on item i, for a caller that rebuilds and
// re-shows its own menu in place — a checklist whose rows carry a ☑/☐ the
// toggle has to redraw. Show resets the hover to nothing, so without this every
// tick would send the cursor back above the first row and the next Down key
// would start the list again. Out-of-range and unselectable indexes are
// ignored, leaving the hover where Show left it.
func (cm *ContextMenu) SetHover(i int) {
	items := cm.Items()
	if i >= 0 && i < len(items) && !menuItemSkippable(items[i]) {
		cm.menu.root().hover = i
	}
}

// Hide dismisses the menu and every submenu open under it.
func (cm *ContextMenu) Hide() {
	cm.visible = false
	if len(cm.menu.boxes) > 0 {
		cm.menu.reset()
	}
}

// OverlayActive reports whether the menu is shown (core.Overlay).
func (cm *ContextMenu) OverlayActive() bool { return cm.visible }

// Items returns the entries last shown, for a caller that has to act on the
// menu without a screen to click on — the entry in force is bulleted by
// whoever built the list, so match on a substring rather than the whole label.
func (cm *ContextMenu) Items() []MenuItem {
	if len(cm.menu.boxes) == 0 {
		return nil
	}
	return cm.menu.root().items
}

// DrawOverlay renders the menu, if shown. The whole menu is an overlay, so the
// host calls it after everything it may cover has drawn. A menu that would run
// off the right or bottom edge shifts left/up from where it was asked for; one
// taller than the screen scrolls.
func (cm *ContextMenu) DrawOverlay(s tcell.Screen) {
	if !cm.visible {
		return
	}
	sw, _ := s.Size()
	root := cm.menu.root()
	w := root.measure(contextMenuMinW)
	x := cm.x
	if x+w > sw {
		x = sw - w
	}
	root.place(s, x, cm.y, w, false)
	cm.menu.draw(s)
}

// drawnRect returns the root box as DrawOverlay last placed it.
func (cm *ContextMenu) drawnRect() core.Rect { return cm.menu.root().rect }

// HandleKey processes keyboard events. Every key is consumed while the menu
// is shown.
func (cm *ContextMenu) HandleKey(ev *tcell.EventKey) bool {
	if !cm.visible {
		return false
	}
	switch res, run := cm.menu.handleKey(ev); res {
	case menuKeyDismiss:
		cm.Hide()
	case menuKeyRun:
		cm.Hide()
		if run != nil {
			run()
		}
	}
	return true
}

// HandleMouse processes mouse events.
func (cm *ContextMenu) HandleMouse(ev *tcell.EventMouse) bool {
	if !cm.visible {
		return false
	}
	mx, my := ev.Position()
	cm.press.Release(ev)

	level, item, inside := cm.menu.hit(mx, my)
	if !inside {
		// A click outside dismisses the menu and stops there: it must not also reach
		// whatever it landed on (see ModalDialog.ConsumeOutsideClick, which likewise
		// consumes). A right-click dismisses and falls through, so the host can open
		// the next menu where it landed; a wheel tick is swallowed rather than
		// scrolling the pane the menu is anchored to. Motion with no button held
		// still falls through, so hover tracking elsewhere keeps working while the
		// menu is open.
		dismiss, consumed := outsideMenu(ev)
		if dismiss {
			cm.Hide()
		}
		return consumed
	}
	if dismiss, run := cm.menu.handleMouse(ev, level, item, cm.press.Fresh(ev)); dismiss {
		cm.Hide()
		if run != nil {
			run()
		}
	}
	return true
}
