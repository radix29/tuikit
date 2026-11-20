package core

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"
)

func paneRows(b *Buffer) string {
	rows := b.Rows()
	for i, r := range rows {
		rows[i] = strings.TrimRight(r, " ")
	}
	return strings.Join(rows, "\n")
}

func TestLinePaneScrollClamps(t *testing.T) {
	var p LinePane
	if p.ScrollBy(-1, 10, 4) || p.Scroll != 0 {
		t.Fatalf("ScrollBy(-1) at the top moved to %d", p.Scroll)
	}
	if !p.ScrollBy(100, 10, 4) || p.Scroll != 6 {
		t.Fatalf("ScrollBy(100) -> %d, want 6 (10 lines in 4 rows)", p.Scroll)
	}
	if p.ScrollBy(1, 10, 4) {
		t.Error("ScrollBy past the end reported a move")
	}
	if !p.CanUp() || p.CanDown(10, 4) {
		t.Errorf("at the end: CanUp=%v CanDown=%v, want true,false", p.CanUp(), p.CanDown(10, 4))
	}
	// Lines that fit leave nothing to scroll.
	if !p.ScrollBy(0, 3, 4) || p.Scroll != 0 || p.MaxScroll(3, 4) != 0 {
		t.Errorf("3 lines in 4 rows: Scroll=%d Max=%d, want 0,0", p.Scroll, p.MaxScroll(3, 4))
	}
}

func TestLineWheel(t *testing.T) {
	r := Rect{X: 2, Y: 3, W: 5, H: 4}
	cases := []struct {
		name   string
		x, y   int
		btn    tcell.ButtonMask
		mod    tcell.ModMask
		want   int
		wantOK bool
	}{
		{"up inside", 2, 3, tcell.WheelUp, 0, -1, true},
		{"down inside", 6, 6, tcell.WheelDown, 0, 1, true},
		// Shift stays vertical: a LinePane has no horizontal axis.
		{"shift up inside", 4, 4, tcell.WheelUp, tcell.ModShift, -1, true},
		{"shift down inside", 4, 4, tcell.WheelDown, tcell.ModShift, 1, true},
		{"down right of r", 7, 4, tcell.WheelDown, 0, 0, false},
		{"up below r", 4, 7, tcell.WheelUp, 0, 0, false},
		{"wheel left", 4, 4, tcell.WheelLeft, 0, 0, false},
		{"press", 4, 4, tcell.Button1, 0, 0, false},
		{"release", 4, 4, tcell.ButtonNone, 0, 0, false},
	}
	for _, c := range cases {
		d, ok := LineWheel(tcell.NewEventMouse(c.x, c.y, c.btn, c.mod), r)
		if d != c.want || ok != c.wantOK {
			t.Errorf("%s: LineWheel = %d,%v, want %d,%v", c.name, d, ok, c.want, c.wantOK)
		}
	}
}

func TestLinePaneDrawInsetHintAndPlaceholder(t *testing.T) {
	b := NewBuffer(14, 3)
	p := LinePane{More: "▾ more"}
	lines := []string{"one", "two", "three", "four"}
	p.Draw(b, b.Rect(), tcell.StyleDefault, lines, nil)
	want := " one\n two\n three ▾ more"
	if got := paneRows(b); got != want {
		t.Errorf("top:\n%s\nwant:\n%s", got, want)
	}
	// At the bottom the hint goes.
	p.ScrollBy(1, len(lines), 3)
	p.Draw(b, b.Rect(), tcell.StyleDefault, lines, nil)
	if got, want := paneRows(b), " two\n three\n four"; got != want {
		t.Errorf("bottom:\n%s\nwant:\n%s", got, want)
	}
	p.DrawEmpty(b, b.Rect(), tcell.StyleDefault, "None")
	if got, want := paneRows(b), " None\n\n"; got != want {
		t.Errorf("empty:\n%q\nwant:\n%q", got, want)
	}
}

// A pane grown taller (or text rewrapped shorter) under an older Scroll shows
// its last line at the bottom, not blank rows below it.
func TestLinePaneDrawClampsStaleScroll(t *testing.T) {
	b := NewBuffer(8, 3)
	p := LinePane{Scroll: 5}
	p.Draw(b, b.Rect(), tcell.StyleDefault, []string{"a", "b", "c", "d"}, nil)
	if p.Scroll != 1 {
		t.Fatalf("Scroll = %d, want 1", p.Scroll)
	}
	if got, want := paneRows(b), " b\n c\n d"; got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestLinePaneDrawPerLineStyle(t *testing.T) {
	b := NewBuffer(8, 2)
	red := tcell.StyleDefault.Foreground(color.Red)
	var p LinePane
	p.Draw(b, b.Rect(), tcell.StyleDefault, []string{"a", "b"}, func(i int) tcell.Style {
		if i == 1 {
			return red
		}
		return tcell.StyleDefault
	})
	if b.StyleAt(1, 1) != red || b.StyleAt(1, 0) == red {
		t.Error("style(i) not applied to line i only")
	}
}
