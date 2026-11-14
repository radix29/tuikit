package widgets

import (
	"fmt"
	"math/rand/v2"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/tuikittest"
)

// The InputField stress test drives one field through a seeded random sequence
// of keys, mouse events, resizes, SetValue, Paste, Cut, SelectAll and enable
// toggles, checking after every step that the caret is inside the value, the
// view isn't scrolled past the value's end, and Draw survives. It found Cut
// leaving the scroll past the remaining text (the field drew blank over a value
// that was still there); the per-feature tests each cut from the front.
//
// A failure reports the seed and the last operations; runInputFieldMonkey(t,
// seed) replays that one seed.

const (
	inputMonkeySeeds = 500
	inputMonkeyOps   = 400
)

var inputMonkeyKeys = []tcell.Key{
	tcell.KeyLeft, tcell.KeyRight, tcell.KeyHome, tcell.KeyEnd,
	tcell.KeyBackspace, tcell.KeyDelete, tcell.KeyCtrlA, tcell.KeyEnter,
	tcell.KeyEscape, tcell.KeyTab, tcell.KeyUp, tcell.KeyDown,
}

var inputMonkeyMods = []tcell.ModMask{
	tcell.ModNone, tcell.ModNone, tcell.ModShift, tcell.ModShift,
	tcell.ModCtrl, tcell.ModCtrl | tcell.ModShift, tcell.ModAlt,
}

var inputMonkeyText = []string{"", "a", "abc def", "注释漢字", "👍🏽", "é", "x.y z", strings.Repeat("w", 25), "line1\nline2"}

type inputMonkey struct {
	r   *rand.Rand
	f   *InputField
	s   *tuikittest.Screen
	ops []string
}

func (m *inputMonkey) text() string { return inputMonkeyText[m.r.IntN(len(inputMonkeyText))] }

func (m *inputMonkey) step() string {
	f := m.f
	switch n := m.r.IntN(20); {
	case n < 8:
		k := inputMonkeyKeys[m.r.IntN(len(inputMonkeyKeys))]
		mod := inputMonkeyMods[m.r.IntN(len(inputMonkeyMods))]
		f.HandleKey(tuikittest.Key(k, mod))
		return fmt.Sprintf("Key(%v,%v)", k, mod)
	case n < 11:
		r := []rune("ab漢 .")[m.r.IntN(5)]
		f.HandleKey(tuikittest.Rune(r))
		return fmt.Sprintf("Rune(%q)", r)
	case n < 13:
		x, btn := m.r.IntN(f.Width()+6)-2, []tcell.ButtonMask{tcell.Button1, tcell.ButtonNone}[m.r.IntN(2)]
		f.HandleMouse(tuikittest.Mouse(x, 0, btn))
		return fmt.Sprintf("Mouse(%d,%v)", x, btn)
	case n < 14:
		w := m.r.IntN(15) - 1
		f.SetWidth(w)
		return fmt.Sprintf("SetWidth(%d)", w)
	case n < 15:
		v := m.text()
		f.SetValue(v)
		return fmt.Sprintf("SetValue(%q)", v)
	case n < 16:
		v := m.text()
		f.Paste(v)
		return fmt.Sprintf("Paste(%q)", v)
	case n < 17:
		f.Cut()
		return "Cut"
	case n < 18:
		f.SelectAll()
		return "SelectAll"
	case n < 19:
		f.ShowFromStart()
		return "ShowFromStart"
	default:
		on := m.r.IntN(4) != 0
		f.SetEnabled(on)
		return fmt.Sprintf("SetEnabled(%v)", on)
	}
}

// check is the per-step invariant: the caret is a rune index into the value,
// and the view starts no later than the column that keeps the value's tail
// (plus the caret's cell after it) filling the box — any later and the field
// draws blank over text that is there.
func (m *inputMonkey) check() error {
	f := m.f
	f.Draw(m.s)
	_ = f.SelectedText()
	if f.cursor < 0 || f.cursor > len(f.value) {
		return fmt.Errorf("cursor = %d, want 0..%d", f.cursor, len(f.value))
	}
	width := core.ColumnOfRune(f.value, len(f.value))
	if last := max(0, width-f.Width()+1); f.scroll < 0 || f.scroll > last {
		return fmt.Errorf("scroll = %d over %q (%d columns, box %d), want 0..%d", f.scroll, string(f.value), width, f.Width(), last)
	}
	return nil
}

func TestInputFieldSurvivesRandomOperations(t *testing.T) {
	seeds := uint64(inputMonkeySeeds)
	if testing.Short() {
		seeds /= 10
	}
	for seed := range seeds {
		runInputFieldMonkey(t, seed)
	}
}

func runInputFieldMonkey(t *testing.T, seed uint64) {
	t.Helper()
	m := &inputMonkey{
		r: rand.New(rand.NewPCG(seed, 0x696e707574)),
		f: NewInputField("", 10, false),
		s: tuikittest.NewScreen(30, 2),
	}
	m.f.SetBounds(0, 0)
	m.f.Focus(true)

	var failure any
	func() {
		defer func() {
			if p := recover(); p != nil {
				failure = fmt.Sprintf("panic: %v\n%s", p, debug.Stack())
			}
		}()
		for range inputMonkeyOps {
			m.ops = append(m.ops, m.step())
			if err := m.check(); err != nil {
				failure = err
				return
			}
		}
	}()
	if failure != nil {
		tail := m.ops[max(0, len(m.ops)-25):]
		t.Fatalf("seed %d, op %d: %v\nlast operations:\n  %s", seed, len(m.ops), failure, strings.Join(tail, "\n  "))
	}
}
