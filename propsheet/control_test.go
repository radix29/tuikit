package propsheet

import (
	"fmt"
	"slices"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/controls"
	"github.com/radix29/tuikit/controls/editor"
	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/tuikittest"
	"github.com/radix29/tuikit/widgets"
)

// Which of core's control interfaces Form and the rows with a popup implement;
// see the widgets package's control_test.go for why these are pinned. Form
// finds a row's overlay only through core.Overlay, so a row losing either
// method would stop drawing its popup last, or stop getting first refusal of
// input while it is open.
var (
	_ interface {
		core.Focusable
		core.Bounded
		core.KeyHandler
		core.MouseHandler
		core.Overlay
	} = (*Form)(nil)
	_ core.Overlay = (*SelectRow)(nil)
	_ core.Overlay = (*EditorRow)(nil)
	_ core.Overlay = (*GridRow)(nil)
)

// A SelectRow hears a key or click through its DropDown's OnChange, given the
// item's text; SetSelected stays silent.
func TestSelectRowOnChangeFiresOnUserPicks(t *testing.T) {
	r := Select("Recovery", []string{"FULL", "BULK_LOGGED", "SIMPLE"}, 0)
	var got []string
	r.SetOnChange(func(v string) { got = append(got, v) })
	scr := tuikittest.NewScreen(80, 10)
	r.Layout(0, 0, 80)
	r.Draw(scr, true) // focuses the dropdown

	r.SetSelected(1) // setter: silent
	r.HandleKey(tuikittest.Key(tcell.KeyEnter))
	r.HandleKey(tuikittest.Key(tcell.KeyDown))
	r.HandleKey(tuikittest.Key(tcell.KeyEnter))

	if want := []string{"SIMPLE"}; !slices.Equal(got, want) {
		t.Errorf("OnChange calls = %q, want %q", got, want)
	}
}

// The contract tests below (AR2) run Form, with one row of every kind,
// through the rules of core.MouseHandler and core.KeyHandler and tuikit's
// ARCHITECTURE.md § Mouse gestures; controls/control_test.go has the same
// tables for the controls. The sheet's own page list is
// TestSheetFocusZoneMovesOnlyOnAPress's. A new row kind joins contractForm.

// contractForm is a focused form, taller than its 60x20 rect, holding every
// row kind, drawn once so its bands are known.
func contractForm() *Form {
	grid := controls.NewDataGrid()
	var rows [][]string
	for i := range 20 {
		rows = append(rows, []string{fmt.Sprint(i), "value"})
	}
	grid.SetData([]string{"N", "Value"}, rows)
	ed := editor.NewEditor(nil)
	ed.SetText("SELECT 1\nFROM t\nWHERE x = 1\n")
	toggles := NewToggleGrid([]string{"CPU", "Affinity"}, []int{1}, 4)
	toggles.SetRows([][]string{{"Processor 0"}, {"Processor 1"}}, [][]bool{{false}, {true}})
	f := NewForm(
		Section("General"),
		Text("Name", "some text to select", 30),
		Password("Password", 20),
		Int("Size", 8, 1, 100, "MB"),
		Check("Read-only", false),
		Radio("Mode", []string{"Fast", "Thorough"}, 0),
		Select("Recovery", []string{"FULL", "SIMPLE"}, 0),
		Static("Owner", "sa"),
		Note("A note long enough to say something about the rows above."),
		Hint(),
		Section("Grids"),
		NewGridRow(grid, 6),
		toggles,
		NewEditorRow("Command", ed, 5),
		Buttons(widgets.NewButton("Add", nil), widgets.NewButton("Remove", nil)),
	)
	f.SetBounds(0, 0, 60, 20)
	f.Focus(true)
	f.Draw(tuikittest.NewScreen(80, 30))
	return f
}

// Plain motion is never claimed anywhere over a form, whichever row is
// focused: a host reads a claimed hover as a press and moves focus.
func TestContractHoverIsNotClaimed(t *testing.T) {
	f := contractForm()
	for i := range f.rows {
		if !f.focusableAt(i) {
			continue
		}
		f.setFocus(i)
		f.Draw(tuikittest.NewScreen(80, 30))
		for y := range 20 {
			for x := range 60 {
				if f.HandleMouse(tuikittest.Mouse(x, y, tcell.ButtonNone)) {
					t.Errorf("row %d (%T) focused: hover at (%d,%d) claimed", i, f.rows[i], x, y)
				}
			}
		}
	}
}

// A release anywhere ends every gesture the form or any row held: afterwards
// neither plain motion nor a fresh press outside is claimed. Presses land on
// each band's label and value, and on the form's scrollbar.
func TestContractReleaseAnywhereClearsEveryLatch(t *testing.T) {
	var presses [][2]int
	for _, b := range contractForm().bands {
		if b.y >= 0 && b.y < 20 {
			presses = append(presses, [2]int{5, b.y}, [2]int{LabelWidth + 3, b.y})
		}
	}
	presses = append(presses, [2]int{59, 3})
	for _, outside := range []bool{false, true} {
		for _, p := range presses {
			f := contractForm()
			rel := p
			if outside {
				rel = [2]int{70, 30}
			}
			f.HandleMouse(tuikittest.Press(p[0], p[1]))
			f.HandleMouse(tuikittest.Mouse(p[0]+2, p[1]+1, tcell.Button1))
			f.HandleMouse(tuikittest.Release(rel[0], rel[1]))
			if f.OverlayActive() {
				continue // the press opened a popup: the overlay rules apply
			}
			if f.HandleMouse(tuikittest.Mouse(p[0]+1, p[1], tcell.ButtonNone)) {
				t.Errorf("press at %v, release at %v: next motion claimed", p, rel)
			}
			if f.HandleMouse(tuikittest.Press(70, 30)) {
				t.Errorf("press at %v, release at %v: a later press outside claimed", p, rel)
			}
		}
	}
}

// Navigation keys on a form with no rows, or with none focusable, neither
// panic nor break the next Draw, sized or not.
func TestContractNavKeysOnEmptyContentKeepDrawWorking(t *testing.T) {
	keys := []tcell.Key{
		tcell.KeyHome, tcell.KeyEnd, tcell.KeyPgUp, tcell.KeyPgDn,
		tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight,
		tcell.KeyEnter, tcell.KeyTab, tcell.KeyBacktab,
	}
	forms := map[string]func() *Form{
		"no rows":      func() *Form { return NewForm() },
		"no focusable": func() *Form { return NewForm(Section("S"), Note("n"), Static("a", "b")) },
	}
	for name, mk := range forms {
		for _, sized := range []bool{true, false} {
			for _, mod := range []tcell.ModMask{tcell.ModNone, tcell.ModShift, tcell.ModCtrl} {
				func() {
					defer func() {
						if r := recover(); r != nil {
							t.Errorf("%s (sized %v, mod %v): panic: %v", name, sized, mod, r)
						}
					}()
					f := mk()
					if sized {
						f.SetBounds(0, 0, 60, 20)
					}
					f.Focus(true)
					scr := tuikittest.NewScreen(80, 30)
					for _, k := range keys {
						f.HandleKey(tuikittest.Key(k, mod))
						f.Draw(scr)
						f.DrawOverlay(scr)
					}
				}()
			}
		}
	}
}

// SetBounds with the same rect changes nothing; a taller one pulls a form
// scrolled to its end back inside the content without moving focus (IN3).
func TestContractFormSetBoundsKeepsFocusAndScrollInContent(t *testing.T) {
	mk := func() *Form {
		var rows []Row
		for i := range 30 {
			rows = append(rows, Text(fmt.Sprintf("Row %d", i), "v", 10))
		}
		f := NewForm(rows...)
		f.SetBounds(0, 0, 60, 10)
		f.Focus(true)
		for range 29 {
			f.HandleKey(tuikittest.Key(tcell.KeyDown, tcell.ModNone))
		}
		f.Draw(tuikittest.NewScreen(60, 10))
		return f
	}
	f := mk()
	focus, scroll := f.FocusedRow(), f.scroll
	if scroll == 0 {
		t.Fatal("setup: the form did not scroll")
	}
	f.SetBounds(0, 0, 60, 10)
	if f.FocusedRow() != focus || f.scroll != scroll {
		t.Fatalf("same rect: focus %d scroll %d, want %d %d", f.FocusedRow(), f.scroll, focus, scroll)
	}
	f.SetBounds(0, 0, 60, 25)
	if f.FocusedRow() != focus {
		t.Errorf("taller rect moved focus %d -> %d", focus, f.FocusedRow())
	}
	if want := f.maxScroll(f.contentWidth()); f.scroll > want {
		t.Errorf("taller rect left scroll %d past the content (max %d)", f.scroll, want)
	}
}
