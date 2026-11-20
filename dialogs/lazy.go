package dialogs

import "reflect"

// ---------------------------------------------------------------------------
// Lazy dialog slots: one dialog per type, built and registered on first use
// ---------------------------------------------------------------------------

// Lazy returns st's one dialog of type T, building it with mk and registering
// it on first use. A host that reaches each dialog through an accessor over
// Lazy never builds a dialog a session does not open, and cannot build one
// without registering it: a shown but unregistered dialog is never drawn and
// never gets input (Register). Registering on first use is safe for z-order
// because Sync stacks dialogs that open together by when they were shown, not
// by registration order.
//
// One slot per type: two accessors may not return the same T, or they would
// share one dialog. A host needing two dialogs of one type gives each its own
// named type, or builds and registers the second itself.
//
// UI goroutine only, like every other Stack method.
func Lazy[T Dialog](st *Stack, mk func() T) T {
	key := reflect.TypeFor[T]()
	if d, ok := st.slots[key]; ok {
		return d.(T)
	}
	d := mk()
	st.setSlot(key, d)
	st.Register(d)
	return d
}

// Built returns st's dialog of type T if Lazy (or Replace) has put one in its
// slot, for code that only asks about an open dialog (is it recording?) and
// must not build one to get the answer.
func Built[T Dialog](st *Stack) (T, bool) {
	d, ok := st.slots[reflect.TypeFor[T]()].(T)
	return d, ok
}

// Replace puts d in T's slot, replacing whatever Lazy built, and gives it the
// Stack's screen if SetScreen has set one (and every later SetScreen). It does
// not register d — the dialog it replaces stays registered — so a test that
// substitutes a dialog and routes input through the Stack registers d itself.
// For tests; a host builds its dialogs through Lazy.
func Replace[T Dialog](st *Stack, d T) T {
	st.setSlot(reflect.TypeFor[T](), d)
	if u, ok := Dialog(d).(screenUser); ok && st.screen != nil {
		u.SetScreen(st.screen)
	}
	return d
}

func (st *Stack) setSlot(key reflect.Type, d Dialog) {
	if st.slots == nil {
		st.slots = make(map[reflect.Type]Dialog)
	}
	st.slots[key] = d
}
