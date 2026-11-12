package dialogs

import (
	"slices"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/core"
)

// ---------------------------------------------------------------------------
// Stack: the z-ordered set of open dialogs
// ---------------------------------------------------------------------------

// Dialog is the shape every modal dialog has by embedding ModalDialog (directly
// or via PropertiesDialog) and adding Draw/HandleKey/HandleMouse.
type Dialog interface {
	Visible() bool
	Draw(s tcell.Screen)
	HandleKey(ev *tcell.EventKey) bool
	HandleMouse(ev *tcell.EventMouse) bool
	Relayout()
}

// Stack tracks which registered dialogs are open, bottom to top, and owns the
// parts of hosting them that a host would otherwise get subtly wrong: drawing
// each through a fresh clip (see Draw), sending a mouse release to every open
// dialog rather than only the top one (see HandleMouse), and giving the
// frontmost dialog the clipboard outright (see ClipboardTarget). The zero
// value is ready to use.
//
// Dialogs open and close themselves by flipping Visible (Show, Escape, a Close
// button), with no way to tell the stack, so the host calls Sync at the points
// where the answer matters: before routing an event (a callback it ran may have
// opened one) and before drawing (the event may have opened or closed one).
type Stack struct {
	all  []Dialog // every registered dialog, in registration order
	open []Dialog // the open ones, bottom to top
}

// Register adds d to the dialogs Sync considers. A dialog that is shown but
// never registered is never drawn and never gets input. Registration order is
// only a tie-break for dialogs that became visible since the same Sync.
func (st *Stack) Register(d Dialog) { st.all = append(st.all, d) }

// Registered returns every registered dialog, open or not, in registration
// order — for settings a host pushes to all of them. The slice is the Stack's
// own; do not modify it.
func (st *Stack) Registered() []Dialog { return st.all }

// Sync reconciles the stack with each dialog's Visible: an entry no longer
// visible is dropped, and a registered dialog that became visible is pushed as
// the new top. Re-showing an open dialog leaves its place unchanged.
func (st *Stack) Sync() {
	st.open = slices.DeleteFunc(st.open, func(d Dialog) bool { return !d.Visible() })
	for _, d := range st.all {
		if d.Visible() && !slices.Contains(st.open, d) {
			st.open = append(st.open, d)
		}
	}
}

// Top returns the frontmost open dialog as of the last Sync, or nil. It alone
// gets keyboard and mouse presses: a dialog opened from another leaves its
// parent inert underneath.
func (st *Stack) Top() Dialog {
	if len(st.open) == 0 {
		return nil
	}
	return st.open[len(st.open)-1]
}

// Open returns the open dialogs bottom to top, as of the last Sync. The slice
// is the Stack's own; do not modify it.
func (st *Stack) Open() []Dialog { return st.open }

// Relayout re-fits every open dialog to the current screen size. Dialogs centre
// themselves rather than taking bounds from a layout, so a host calls this on
// every terminal resize (see ModalDialog.Relayout).
func (st *Stack) Relayout() {
	for _, d := range st.open {
		d.Relayout()
	}
}

// Draw renders every open dialog bottom to top, so a nested dialog paints over
// its parent. Each draws on one core.ClipScreen over s, its clip reset to the
// whole screen first: ModalDialog.DrawBase narrows it to the box when the
// terminal is too small for the requested size, which is what keeps content
// laid out for that size from spilling across the screen, and a nested dialog
// must not inherit its parent's clip.
func (st *Stack) Draw(s tcell.Screen) {
	if len(st.open) == 0 {
		return
	}
	cs := core.NewClipScreen(s)
	for _, d := range st.open {
		cs.ResetClip()
		d.Draw(cs)
	}
}

// HandleKey gives ev to the top dialog and reports whether a dialog was open.
// An open dialog consumes every key, whatever its own HandleKey returns: the
// layer beneath must not act on it.
func (st *Stack) HandleKey(ev *tcell.EventKey) bool {
	top := st.Top()
	if top == nil {
		return false
	}
	top.HandleKey(ev)
	return true
}

// HandleMouse routes ev and reports whether a dialog was open (and so consumed
// it). A press or drag goes to the top dialog only. A release (ButtonNone, which
// is also plain pointer motion) goes to every open dialog, bottom to top: a
// dialog clears its press latch on the release its own HandleMouse sees, and one
// that opened a nested dialog from a button press never sees it while the child
// is up, so its next press would be refused as a continuation of the click that
// opened the child. Acting needs Button1, so the dialogs underneath only reset.
func (st *Stack) HandleMouse(ev *tcell.EventMouse) bool {
	if len(st.open) == 0 {
		return false
	}
	if ev.Buttons() == tcell.ButtonNone {
		for _, d := range st.open {
			d.HandleMouse(ev)
		}
		return true
	}
	st.Top().HandleMouse(ev)
	return true
}

// ClipboardTarget resolves Copy/Cut/Paste while a dialog is open. owned is true
// whenever one is: the frontmost dialog then owns the clipboard outright, and
// the host must use target even when it is nil ("nothing here takes this key")
// rather than fall through to a widget underneath. target is the top dialog's
// focused field if it is a core.ClipboardHost, else nil.
func (st *Stack) ClipboardTarget() (target core.ClipboardTarget, owned bool) {
	top := st.Top()
	if top == nil {
		return nil, false
	}
	if h, ok := top.(core.ClipboardHost); ok {
		return h.FocusedClipboardTarget(), true
	}
	return nil, true
}

// ClipboardEdited tells the top dialog that a Cut or Paste changed target, if it
// is a core.ClipboardEditHandler.
func (st *Stack) ClipboardEdited(target core.ClipboardTarget) {
	if h, ok := st.Top().(core.ClipboardEditHandler); ok {
		h.ClipboardEdited(target)
	}
}
