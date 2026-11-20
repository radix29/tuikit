package dialogs

import (
	"testing"

	"github.com/radix29/tuikit/tuikittest"
)

// otherDialog is a second Dialog type, so slots can be told apart.
type otherDialog struct{ *stackDialog }

func TestLazyBuildsOnceRegistersAndHandsTheScreen(t *testing.T) {
	scr := tuikittest.NewScreen(80, 24)
	var st Stack
	st.SetScreen(scr)

	if _, ok := Built[*stackDialog](&st); ok {
		t.Fatal("Built reported a dialog before Lazy built one")
	}
	builds := 0
	mk := func() *stackDialog {
		builds++
		d := &stackDialog{}
		d.InitModal("t", 20, 6) // no screen: Lazy's Register hands it one
		return d
	}
	a := Lazy(&st, mk)
	if b := Lazy(&st, mk); b != a || builds != 1 {
		t.Fatalf("second Lazy: same=%v builds=%d, want the first dialog, built once", b == a, builds)
	}
	if got, ok := Built[*stackDialog](&st); !ok || got != a {
		t.Fatal("Built did not return the dialog Lazy built")
	}
	if n := len(st.Registered()); n != 1 {
		t.Fatalf("Registered() has %d dialogs, want 1", n)
	}

	// The registered dialog draws and takes input: the rule Lazy exists for.
	a.Show()
	st.Sync()
	if st.Top() != Dialog(a) {
		t.Fatal("a shown Lazy dialog is not on the stack")
	}

	Lazy(&st, func() otherDialog { return otherDialog{newStackDialog(scr, 20, 6)} })
	if _, ok := Built[otherDialog](&st); !ok || len(st.Registered()) != 2 {
		t.Fatalf("a second type did not get its own slot (registered %d)", len(st.Registered()))
	}
}

func TestReplaceSwapsTheSlotWithoutRegistering(t *testing.T) {
	scr := tuikittest.NewScreen(80, 24)
	var st Stack
	first := Lazy(&st, func() *stackDialog { d := &stackDialog{}; d.InitModal("t", 20, 6); return d })

	d := &stackDialog{}
	d.InitModal("t", 20, 6)
	if Replace(&st, d) != d {
		t.Fatal("Replace did not return its argument")
	}
	if got := Lazy(&st, func() *stackDialog { t.Fatal("Lazy rebuilt a replaced slot"); return nil }); got != d {
		t.Fatal("Lazy did not return the replacement")
	}
	if r := st.Registered(); len(r) != 1 || r[0] != Dialog(first) {
		t.Fatal("Replace changed the registered dialogs")
	}

	// SetScreen reaches the unregistered replacement: Show centres against it.
	st.SetScreen(scr)
	d.Show()
	if r := d.Rect(); r.W != 20 || r.X != (80-20)/2 {
		t.Fatalf("replacement laid out at %+v, want centred on the 80-column screen", r)
	}
}
