// Package core provides the foundational types and drawing primitives used by
// every other tuikit sub-package. Its only tuikit dependency is theme, which
// Init reads the default style from.
//
//   - geometry.go    — Rect
//   - screen.go      — Init (tcell.Screen setup)
//   - clip_screen.go — ClipScreen, a Screen that drops writes outside a rect
//   - buffer.go      — Buffer, an off-screen Screen (chart canvases, test screens)
//   - drawing.go     — DrawText/DrawBox/FillRect/DrawScrollbar/... primitives
//   - strutil.go     — DisplayWidth/Truncate/WrapText/PadRight/EvRune
//   - viewport.go    — Viewport, the offset/clamp/wheel/scrollbars of a view
//     onto a canvas larger than it in both axes
//   - linepane.go    — LinePane, the scroll offset/clamp/draw of a read-only
//     pane of text lines (details panes), and LineWheel, its wheel tick
//   - wrap_hanging.go — WrapHanging, a wrap that keeps spacing and hangs a
//     continuation under a line's second column (key references)
//   - runecol.go     — rune-index <-> terminal-column conversion
//   - wordutil.go    — word-boundary helpers for Ctrl+arrow navigation
//   - clipboard.go   — the ClipboardTarget/ClipboardHost interfaces and their
//     optional ClipboardFocuser/ClipboardTargetTokener/ClipboardEditHandler
//     companions
//   - control.go     — the control interfaces: Focusable, Enabler, Bounded,
//     KeyHandler, MouseHandler, Overlay
//   - gesture.go     — Gesture, a mouse router's claimed-zone record
//   - drag.go        — Drag, a drag-and-drop gesture carrying a payload, with
//     a start threshold and a ghost drawn beside the pointer
//   - tabstrip.go    — TabStripSegments/TabLabelWidth, the column layout every
//     tab bar draws and hit-tests with, so the two can't drift apart
//   - mathutil.go    — Clamp
//   - keys.go        — NormalizeKey, the Ctrl+letter fold every key handler
//     and host applies before matching KeyCtrlA..KeyCtrlZ
//   - mouse.go       — MouseEventName, EventKey.Name()'s missing counterpart
//     for a mouse event
package core
