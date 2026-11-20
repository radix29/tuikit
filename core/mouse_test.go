package core

import (
	"testing"

	"github.com/gdamore/tcell/v3"
)

func TestMouseEventName(t *testing.T) {
	for _, tc := range []struct {
		b    tcell.ButtonMask
		m    tcell.ModMask
		want string
	}{
		{tcell.ButtonNone, tcell.ModNone, "Release"},
		{tcell.Button1, tcell.ModNone, "Button1"},
		{tcell.Button1, tcell.ModShift, "Shift+Button1"},
		{tcell.Button1, tcell.ModCtrl | tcell.ModShift, "Ctrl+Shift+Button1"},
		{tcell.Button2, tcell.ModAlt, "Alt+Button2"},
		{tcell.Button3, tcell.ModMeta, "Meta+Button3"},
		{tcell.Button4, tcell.ModNone, "Button"},
		{tcell.WheelUp, tcell.ModNone, "WheelUp"},
		{tcell.WheelDown, tcell.ModCtrl, "Ctrl+WheelDown"},
		{tcell.WheelLeft, tcell.ModNone, "WheelLeft"},
		{tcell.WheelRight, tcell.ModNone, "WheelRight"},
		{tcell.Button1 | tcell.WheelUp, tcell.ModNone, "WheelUp"},
		{tcell.Button1 | tcell.Button3, tcell.ModNone, "Button1"},
	} {
		if got := MouseEventName(tcell.NewEventMouse(0, 0, tc.b, tc.m)); got != tc.want {
			t.Errorf("MouseEventName(%v, %v) = %q, want %q", tc.b, tc.m, got, tc.want)
		}
	}
}

func TestReleaseNowhere(t *testing.T) {
	ev := tcell.NewEventMouse(7, 4, tcell.Button1, tcell.ModShift|tcell.ModCtrl)
	r := ReleaseNowhere(ev)
	if x, y := r.Position(); x != -1 || y != -1 {
		t.Errorf("position = (%d, %d), want (-1, -1)", x, y)
	}
	if r.Buttons() != tcell.ButtonNone {
		t.Errorf("buttons = %v, want ButtonNone", r.Buttons())
	}
	if r.Modifiers() != ev.Modifiers() {
		t.Errorf("modifiers = %v, want %v", r.Modifiers(), ev.Modifiers())
	}
}
