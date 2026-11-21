package clipboard

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
)

// fakeTarget records every Paste it receives.
type fakeTarget struct{ pastes []string }

func (f *fakeTarget) Paste(text string) { f.pastes = append(f.pastes, text) }

// osc52Screen is a screen whose OSC 52 calls and DA1/XTVERSION replies the
// test controls.
type osc52Screen struct {
	tcell.Screen
	da52     bool
	termName string
	set      []byte
	gets     int
}

func (s *osc52Screen) SetClipboard(b []byte)      { s.set = b }
func (s *osc52Screen) GetClipboard()              { s.gets++ }
func (s *osc52Screen) HasClipboard() bool         { return s.da52 }
func (s *osc52Screen) Terminal() (string, string) { return s.termName, "" }

// harness is a Dispatcher whose Post queues for the test to drain, standing
// in for a host's event loop.
type harness struct {
	d      *Dispatcher
	screen *osc52Screen
	active Target
	tok    any
	pasted []Target

	mu     sync.Mutex
	posted []func()
}

func newHarness() *harness {
	h := &harness{screen: &osc52Screen{}}
	h.d = &Dispatcher{
		Screen:  func() tcell.Screen { return h.screen },
		Resolve: func() Target { return h.active },
		Token:   func() any { return h.tok },
		Post: func(fn func()) {
			h.mu.Lock()
			h.posted = append(h.posted, fn)
			h.mu.Unlock()
		},
		Pasted: func(t Target) { h.pasted = append(h.pasted, t) },
	}
	return h
}

// drain waits for at least one posted callback and runs everything queued.
func (h *harness) drain(t *testing.T) {
	t.Helper()
	for range 1000 {
		h.mu.Lock()
		fns := h.posted
		h.posted = nil
		h.mu.Unlock()
		if len(fns) > 0 {
			for _, fn := range fns {
				fn()
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("nothing was posted within the timeout")
}

// Quick copies reach the clipboard one at a time, and the last one wins: with
// a goroutine each, a copy whose tool ran slow finished after the next one and
// left the older text on the clipboard. One superseded while it waited for the
// lock is not written at all.
func TestCopyNewestWins(t *testing.T) {
	h := newHarness()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	var writes []string
	h.d.NativeWrite = func(text string) bool {
		if text == "A" {
			close(entered)
			<-release
		}
		mu.Lock()
		writes = append(writes, text)
		mu.Unlock()
		if text == "C" {
			close(done)
		}
		return true
	}

	h.d.Copy("A")
	<-entered
	h.d.Copy("B")
	h.d.Copy("C")
	// Fixed sleeps, not synctest: a goroutine parked on a sync.Mutex is not
	// durably blocked. Too short only weakens the test, never fails it.
	time.Sleep(20 * time.Millisecond)
	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("C never reached the clipboard")
	}
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if want := []string{"A", "C"}; !slices.Equal(writes, want) {
		t.Errorf("writes = %q, want %q: in order, the superseded B skipped", writes, want)
	}
	if n := h.d.Copies(); n != 3 {
		t.Errorf("Copies() = %d, want 3", n)
	}
}

// With no native tool, Copy writes OSC 52 on the UI goroutine and hedges only
// where the terminal neither advertised OSC 52 nor is tmux.
func TestCopyFallsBackToOSC52(t *testing.T) {
	for _, tc := range []struct {
		name      string
		da52      bool
		termName  string
		wantHedge bool
	}{
		{"VTE", false, "", true},
		{"advertises 52", true, "", false},
		{"tmux", false, "tmux", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness()
			h.screen.da52, h.screen.termName = tc.da52, tc.termName
			h.d.NativeWrite = func(string) bool { return false }
			hedged := false
			h.d.Unconfirmed = func() { hedged = true }

			h.d.Copy("SELECT 1")
			h.drain(t)
			if string(h.screen.set) != "SELECT 1" {
				t.Fatalf("OSC 52 write = %q, want the copied text", h.screen.set)
			}
			if hedged != tc.wantHedge {
				t.Errorf("Unconfirmed called = %v, want %v", hedged, tc.wantHedge)
			}
		})
	}
}

// A paste is dropped, not re-aimed, when the widget it was aimed at is no
// longer the target by the time the text arrives (its dialog closed).
func TestPasteDroppedWhenTargetChanged(t *testing.T) {
	h := newHarness()
	field, editor := &fakeTarget{}, &fakeTarget{}
	h.active = field
	release := make(chan struct{})
	h.d.NativeRead = func() (string, bool) { <-release; return "ubusql2", true }

	h.d.Paste()
	h.active = editor // the dialog closed while the read ran
	close(release)
	h.drain(t)

	if len(field.pastes)+len(editor.pastes) != 0 {
		t.Errorf("field got %q, editor got %q; want both untouched", field.pastes, editor.pastes)
	}
}

// The token carries the same check one level deeper: same Target, different
// field.
func TestPasteDroppedWhenTokenChanged(t *testing.T) {
	h := newHarness()
	sheet := &fakeTarget{}
	h.active, h.tok = sheet, 0
	release := make(chan struct{})
	h.d.NativeRead = func() (string, bool) { <-release; return "hunter2", true }

	h.d.Paste()
	h.tok = 1 // the user tabbed to the next row
	close(release)
	h.drain(t)
	if len(sheet.pastes) != 0 {
		t.Errorf("pasted %q after focus moved within the target", sheet.pastes)
	}

	// Unmoved, it goes in, BOMs stripped, and Pasted hears of it.
	h.d.NativeRead = func() (string, bool) { return "\uFEFFhunter2", true }
	h.d.Paste()
	h.drain(t)
	if want := []string{"hunter2"}; !slices.Equal(sheet.pastes, want) {
		t.Errorf("pastes = %q, want %q", sheet.pastes, want)
	}
	if len(h.pasted) != 1 || h.pasted[0] != Target(sheet) {
		t.Errorf("Pasted calls = %v, want one for the sheet", h.pasted)
	}
}

// With no native tool, Paste asks the terminal; its reply goes to what the
// Ctrl+V was aimed at, once. An unsolicited reply pastes nothing anywhere.
func TestPasteOSC52Reply(t *testing.T) {
	h := newHarness()
	field := &fakeTarget{}
	h.active = field
	h.d.NativeRead = func() (string, bool) { return "", false }

	h.d.Paste()
	h.drain(t)
	if h.screen.gets != 1 {
		t.Fatalf("GetClipboard called %d times, want 1", h.screen.gets)
	}
	if consumed, skip := h.d.HandleEvent(tcell.NewEventClipboard([]byte("x"))); !consumed || skip {
		t.Errorf("EventClipboard: consumed=%v skipFrame=%v, want true, false", consumed, skip)
	}
	h.d.HandleEvent(tcell.NewEventClipboard([]byte("unsolicited")))
	if want := []string{"x"}; !slices.Equal(field.pastes, want) {
		t.Errorf("pastes = %q, want %q", field.pastes, want)
	}
}

// A bracketed paste reaches the target as one Paste, its keys consumed and
// drawing no frame, and keys outside one are left to the host.
func TestBracketedPasteIsOneEdit(t *testing.T) {
	h := newHarness()
	field := &fakeTarget{}
	h.active = field

	key := tcell.NewEventKey(tcell.KeyRune, "a", tcell.ModNone)
	if consumed, _ := h.d.HandleEvent(key); consumed {
		t.Fatal("a key outside a paste was consumed")
	}
	h.d.HandleEvent(tcell.NewEventPaste(true))
	for _, ev := range []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyRune, "\uFEFFx", tcell.ModNone),
		tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, "y", tcell.ModNone),
	} {
		if consumed, skip := h.d.HandleEvent(ev); !consumed || !skip {
			t.Errorf("pasted key: consumed=%v skipFrame=%v, want true, true", consumed, skip)
		}
	}
	if len(field.pastes) != 0 {
		t.Fatal("the paste was applied before its end marker")
	}
	if consumed, skip := h.d.HandleEvent(tcell.NewEventPaste(false)); !consumed || skip {
		t.Errorf("end marker: consumed=%v skipFrame=%v, want true, false", consumed, skip)
	}
	if want := []string{"x\ny"}; !slices.Equal(field.pastes, want) {
		t.Errorf("pastes = %q, want %q", field.pastes, want)
	}
	if len(h.pasted) != 1 {
		t.Errorf("Pasted called %d times, want 1", len(h.pasted))
	}
	if consumed, _ := h.d.HandleEvent(key); consumed {
		t.Error("a key after the paste ended was consumed")
	}
}

// A Paste issued after a Copy reads only once that write has landed: with a
// slow native tool the read used to overtake it and paste the old contents.
func TestPasteWaitsForEarlierCopy(t *testing.T) {
	h := newHarness()
	h.active = &fakeTarget{}
	tgt := h.active.(*fakeTarget)
	entered, release := make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	clip := "old"
	h.d.NativeWrite = func(text string) bool {
		close(entered)
		<-release
		mu.Lock()
		clip = text
		mu.Unlock()
		return true
	}
	h.d.NativeRead = func() (string, bool) {
		mu.Lock()
		defer mu.Unlock()
		return clip, true
	}

	h.d.Copy("new")
	<-entered
	h.d.Paste()
	time.Sleep(20 * time.Millisecond)
	h.mu.Lock()
	n := len(h.posted)
	h.mu.Unlock()
	if n != 0 {
		t.Fatal("Paste read the clipboard before the Copy finished writing")
	}
	close(release)
	h.drain(t)
	if !slices.Equal(tgt.pastes, []string{"new"}) {
		t.Errorf("pastes = %q, want [new]", tgt.pastes)
	}
}

// Two quick Copies then a Paste: the superseded one still counts as done, and
// the paste gets the second.
func TestPasteAfterSupersededCopy(t *testing.T) {
	h := newHarness()
	tgt := &fakeTarget{}
	h.active = tgt
	entered, release := make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	clip := "old"
	h.d.NativeWrite = func(text string) bool {
		if text == "A" {
			close(entered)
			<-release
		}
		mu.Lock()
		clip = text
		mu.Unlock()
		return true
	}
	h.d.NativeRead = func() (string, bool) {
		mu.Lock()
		defer mu.Unlock()
		return clip, true
	}
	h.d.Copy("A")
	<-entered
	h.d.Copy("B")
	h.d.Paste()
	close(release)
	h.drain(t)
	if !slices.Equal(tgt.pastes, []string{"B"}) {
		t.Errorf("pastes = %q, want [B]", tgt.pastes)
	}
}
