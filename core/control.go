package core

import "github.com/gdamore/tcell/v3"

// The control contract, as small interfaces a host can type-assert on instead
// of switching over concrete types. Every interactive tuikit control implements
// the ones that fit it; control_test.go in each package pins which. They live
// in core, not in the packages declaring the controls, so a host (a focus ring,
// propsheet.Form, an application dialog) can hold a widgets.InputField and a
// editor.Editor in one slice: identical interfaces declared in two packages
// would not be interchangeable.

// KeyHandler takes a key. HandleKey returns true only for a key the control
// acted on — one that changed what it shows or fired a callback, so an arrow at
// an edge is false — and a host offers the key to its own navigation (Tab,
// arrows past an edge, Escape) on false.
type KeyHandler interface {
	HandleKey(ev *tcell.EventKey) bool
}

// MouseHandler takes a mouse event. HandleMouse returns false for an event
// outside the control's bounds that it did not latch (a drag it started keeps
// control off its edge), so a host can offer the event to whatever is beneath.
// Inside the bounds, one rule for every control:
//
//   - Plain motion (ButtonNone) is never consumed: the control may track hover
//     from it, but returns true only for a release that ended a gesture it held,
//     or while it has an overlay open. A host reads true as "claimed", and a
//     claimed hover moved a property sheet's keyboard focus onto its page list.
//     A host moves focus on a press only.
//   - A wheel tick over the control is consumed even when it cannot scroll
//     further, so it never scrolls whatever lies beneath.
//   - Any press outside an open menu dismisses the menu. A right-click then
//     falls through (false), so the host can open a new menu where it landed;
//     any other press is consumed, and a wheel tick is consumed with the menu
//     left open.
type MouseHandler interface {
	HandleMouse(ev *tcell.EventMouse) bool
}

// One wheel tick's scroll, the same in every control. WheelRows is vertical, in
// whatever a control scrolls by (lines, visual rows, list items, tree nodes,
// form rows); WheelCells is horizontal, in terminal columns, for a control
// that scrolls by the cell (Editor, TreeView). DataGrid scrolls sideways by
// whole data columns, a different unit, and steps one of them.
const (
	WheelRows  = 1
	WheelCells = 4
)

// Focusable is a control that draws and behaves differently with keyboard
// focus. Focus(false) also dismisses anything the control opened that would
// otherwise linger while keys go elsewhere (a drop-down list, a completion
// popup). Focus is the host's statement, not the control's: no control
// focuses itself on a click, since only the host knows its focus ring.
type Focusable interface {
	Focus(v bool)
	Focused() bool
}

// DragCanceller is a control holding a mouse-gesture latch a host may need to
// drop without a release: CancelMouseDrag forgets the held press (and any thumb
// drag) without moving the cursor, the selection or the scroll. For a host
// taking the control out of a gesture rather than ending one, such as a dialog
// reshown after being dismissed mid-drag (dialogs.FieldGesture.Clear), whose
// release will never arrive. A synthetic ButtonNone would misreport the
// pointer position.
type DragCanceller interface {
	CancelMouseDrag()
}

// Enabler is a control that can be switched off. A disabled control draws
// greyed, refuses keys and clicks, keeps its place in a focus ring (Tab order
// does not shift when a page switches a control off) and its own setters still
// work, so a host can pin a value the server requires and then disable it.
type Enabler interface {
	SetEnabled(v bool)
	Enabled() bool
}

// Bounded is a control placed on the screen. Bounds returns the rect it
// occupies, for a host hit-testing or positioning related UI (an autocomplete
// list under a field, a drop target). A one-row widget's rect covers its label
// and its box; its W is the drawn width, not the value area's (see each
// widget's Width).
type Bounded interface {
	Bounds() Rect
}

// Overlay is a control that can show something floating outside its own rect:
// a drop-down's list, a menu, a completion popup, a cell viewer.
//
// DrawOverlay draws it, and the host calls it after everything that shares the
// space has drawn, or what draws later paints over it. It draws nothing while
// no overlay is shown.
//
// OverlayActive reports whether such an overlay is open. While it is, the host
// gives the control first refusal of every key and mouse event: the overlay is
// positioned independently of the control's rect, so routing by position or
// focus would hand its input to whatever sits under it.
type Overlay interface {
	DrawOverlay(s tcell.Screen)
	OverlayActive() bool
}
