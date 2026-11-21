package widgets

import (
	"testing"

	"github.com/gdamore/tcell/v3"
)

func TestInputFieldArrowCollapsesSelection(t *testing.T) {
	mk := func() *InputField {
		f := NewInputField("", 20, false)
		f.Focus(true)
		f.SetValue("hello world")
		f.SelectAll()
		return f
	}
	f := mk()
	f.HandleKey(tcell.NewEventKey(tcell.KeyLeft, "", tcell.ModNone))
	if f.cursor != 0 || f.HasSelection() {
		t.Fatalf("Left: cursor %d sel %v", f.cursor, f.HasSelection())
	}
	f = mk()
	f.cursor, f.selAnchor = 3, 8
	f.HandleKey(tcell.NewEventKey(tcell.KeyRight, "", tcell.ModNone))
	if f.cursor != 8 || f.HasSelection() {
		t.Fatalf("Right: cursor %d sel %v", f.cursor, f.HasSelection())
	}
	f = mk()
	f.cursor = 0
	f.selAnchor = 5
	f.HandleKey(tcell.NewEventKey(tcell.KeyLeft, "", tcell.ModShift))
	if !f.HasSelection() {
		t.Fatal("Shift+Left must extend")
	}
}
