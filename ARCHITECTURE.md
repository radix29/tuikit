# Architecture

`README.md` maps the packages, their dependency direction and the design
principles. This file holds the long-form *why* behind the idioms a host must
follow, and the rules that the code doesn't make obvious. Each rule is a bug
that shipped in an application built on tuikit (goSSMS); the test named with
it is the one that fails when the rule is broken.

## Threading

tuikit does no locking. Every widget, and every package-level setting
(README § Design principles, "Package-level settings"), belongs to the host's one UI goroutine.
A background goroutine never touches a widget: it posts its result to the UI
goroutine, which applies it. A host on tcell v3 wakes its loop through the
screen's `EventQ()` channel — there is no `PostEvent`/`PollEvent` — and must
send that wakeup from the background goroutine itself, not from inside the
callback it posted: a wakeup queued inside a callback that runs only after a
wakeup never fires, and the result sits until an unrelated keypress.

**Async state is data, not goroutines.** No tuikit control spawns a goroutine
or owns a timer:

- `widgets.Spinner` is a function of elapsed time (`Frame(elapsed)`,
  `FrameSince(start)`), redrawn from the host's clock.
- `propsheet.PropertySheet` asks for a page with `OnLoadPage(page, seq)` and
  takes the answer through `SetPageForm`/`SetPageError`, both on the UI
  goroutine; a stale `seq` is ignored. While the host applies, the sheet
  locks its form (`formLocked`), so nothing writes a widget a pipeline
  goroutine is reading.
- `dialogs.FileDialog` is the one place that paints outside the draw cycle:
  `FileSystem` is synchronous, and for a `BlockingFileSystem` (a remote one)
  `showBusy` draws a "Listing ..." line and calls `Screen.Show()` before the
  call, because there is no frame between the keypress and the blocked call.
  An async `FileSystem` was deliberately not built: it turns Tab completion
  and the overwrite check into callback chains for a wait the indicator
  explains.

## Mouse gestures

tcell's all-motion tracking resends the held button on every motion event.
Everything below follows from that.

### Press latches

Any widget acting on `Button1` (toolbar button, menu label, tree toggle)
holds a `core.PressLatch`: set on the press, cleared on the matching
`ButtonNone`, or the action refires on every motion while the button is held.
It guards only a resend over the widget that armed it. `InputField` holds a
hand-rolled `mouseDragging` bool for the same purpose; the widgets with
several drag zones hold one `core.Gesture` instead (§ One owner per press
inside a widget).

### The dialog button row

`ModalDialog`'s button row sees the whole gesture: every event passes
`TrackPress` (via `ConsumeOutsideClick` and `ButtonClicked`), which calls
`Fresh` on every `Button1`, so only a gesture's first press can click a button
— a drag from the message, a list, a scrollbar, a checkbox or from outside the
box does not. A dialog that offers events to an open drop-down list ahead of
`ConsumeOutsideClick` calls `TrackPress` first, or the list's pick goes unseen
and its resends look fresh.

A dialog button closes the dialog on the *press*, so the release never reaches
its latch reset (the dialog is already hidden). The next showing then refused
its first click and looked frozen; `ModalDialog.Show()` clears both latches.

A row with a left group as well (a destructive button kept away from the
default, like goSSMS's Connect dialog's Delete) is one row, not two calls:
`DrawSplitButtons` and `SplitButtonClicked` share `SplitButtonLayout`, which
drops the left group to the row below when a clamped dialog cannot fit both
side by side, so the groups never overlap. Draw status between them in its
`GapX`/`GapW`, not at an offset of your own.

### Routers: a press claims the gesture

A positional router (the host's top-level mouse handler, a panel holding
several children) sees a drag that began elsewhere drift onto a latch-owning
widget as a fresh-looking `Button1`. So **a press claims the gesture**: the
router records the claiming region and replays every event to it until
release. That record is `core.Gesture[Z]`, one per router (`propsheet`'s sheet
holds one):

- `Release` on `ButtonNone`, beside forwarding the release to latch-bearing
  children;
- `Route(ev, routeDrag)` ahead of any bounds check;
- `Arm` at every claiming branch.

The router keeps only its zone enum and `routeDrag`'s `switch`. `Route`
swallows a mid-gesture wheel tick; a router whose grids take the wheel
mid-gesture reads `Owner()` instead. Regions that already acted swallow
repeats. `layout.Splitter` starts a resize only from a press on its bar, so a
selection drag crossing it doesn't grab it.

Two consequences:

- A host with an early `return` in `HandleMouse` forwards `ButtonNone` to its
  latch-bearing children first, or a drag ending outside leaves a child
  latched and swallowing its next press.
- A host that opens a dialog from a press (a context-menu item) drops the
  held `Button1` when its modal layer changes mid-gesture; otherwise
  twitching before release fires whatever dialog button is under the pointer.

### One owner per press inside a widget

A widget with several drag zones is a router too. `DataGrid` (cells, row
header, Ctrl+mark, both scrollbars, column separator, inert header/status),
`Editor` (text, both scrollbars, completion item and scrollbar), `TreeView`
(an acted-on row or title button, both scrollbars) and `propsheet.Form` (a
row's index, its scrollbar) each hold one `core.Gesture` over their own zone
type. Zones are hit-tested only on the fresh press; every later `Button1`
goes to the owner, routed ahead of the bounds test, and a release anywhere
ends it. One latch per zone, each branch testing only some of the others,
let a cell-selection drag resize a column, grab a scrollbar or re-sort on a
header. A press that acted or did nothing still arms a zone whose route does
nothing, so a drag from it never reaches a cell as a fresh press. Scrollbars
use `core.OnScrollbar`/`OnScrollbarH` for the press and
`ScrollOffsetForDrag` for every event; `HandleScrollbarDrag` stays for
widgets with a single bool latch.

A selection drag (DataGrid cells and row header, Editor text, both wrap
modes) that leaves the content clamps the pointer to the row or column just
beyond the view and scrolls one toward it per motion event: tuikit has no
timer, so a pointer resting outside does not keep scrolling. A toggle grid's
drag paints only cells under the pointer and never scrolls.

### dialogs.Stack: releases go to every open dialog

`Stack.HandleMouse` hands a release to every open dialog, not just the front
one. A dialog that opened a child from a button press never sees that release,
so its next press was refused as a continuation. Dialogs underneath only reset
latches; acting needs `Button1`.

### dialogs.FieldGesture

A click in a `widgets.InputField` starts a selection drag (as one in an
`Editor` does, or an item or thumb drag in a `ListBox`), and the dialog must
end it on release *wherever* the pointer lands, replay motion to the control
without hit-testing, and drop the latch on `Show`. `FieldGesture` holds any
`core.MouseHandler`; one that is also a `core.DragCanceller` (InputField,
ListBox, Editor) has its own latch dropped by `Clear`. Each call's placement
is non-local:

- `Release` at the very top of `HandleMouse`, **before**
  `ConsumeOutsideClick` and any mode early-return — both return without
  looking at the latch, and a release outside the dialog or after it switched
  to a progress view is exactly what strands it.
- `Replay` after `ConsumeOutsideClick`, before any hit-testing — hit-testing
  motion ends the selection when the pointer leaves the field, and lets a drag
  over the button row fire a button.
- `Claim` on the press in a field, so the press is routed through the gesture
  rather than the widget's own bounds check (which a stale latch skips).
- `Clear` in `Show`, which also drops the control's own latch
  (`CancelMouseDrag`): a dialog that reuses its fields and was
  dismissed mid-drag handed back a latched field whose next press armed no
  anchor.

Seven dialogs hand-rolled this; `FieldGesture` holds it once. Hand-rolled
copies dragged to the button row and pressed it — accepting a rename or a
typed confirmation. `TestFileDialogDragOutOfPathFieldKeepsExtending` fails on
a mutation of any of the three main calls.

### Not every mouse event draws

A host may skip the frame after motion-only events while more are queued (a
grid drag fell seconds behind otherwise). State a motion handler depends on is
set in the handler, never in `Draw`.

## Overlays

A widget whose open state floats outside its rect — `DropDown`'s list,
`DataGrid`'s cell menu and "Show Value" popup, `Editor`'s completion popup,
`MenuBar`'s dropdown, a `ContextMenu` — implements `core.Overlay`. The host:

- calls `DrawOverlay(s)` after everything sharing that space. A host drawing a
  `DataGrid` calls `grid.DrawOverlay(s)` after every grid; without it the menu
  opens invisibly and swallows keys until Escape. A host that cannot draw the
  overlay from the method that draws the grid exposes its own `DrawOverlay`
  for *its* host to call last (`propsheet.GridRow` does).
- offers every event to an `OverlayActive()` widget first in its own
  `HandleKey`/`HandleMouse` — the wheel included — before positional routing.
  A `DropDown`'s open list is clamped to the *screen*, not the dialog, so a
  dialog routing one by hand offers it every event before
  `ConsumeOutsideClick`.
- skips a timer-driven `SetData` while a grid's overlay is open: `SetData`
  closes the popup a user opened to read.

A hover hint (`Toolbar`, `TreeView`'s title button) is an `Overlay` whose
`OverlayActive` is always false: it draws last but takes no input.

## Dialogs

- **Clip contract.** A dialog clamped to a small terminal confines its content
  only when drawn on a `core.ClipScreen`; `Stack.Draw` supplies one, a host
  drawing dialogs itself must too. `DrawBase` resets the clip before narrowing
  it, so dialogs sharing one never inherit each other's
  (`TestADialogDoesNotInheritAnotherDialogsClip`).
- **A dialog-level scrollbar uses `DrawContentScrollbar`**, not
  `core.DrawScrollbar` at `Rect().Right()-1`: the border column is outside the
  clip. A child widget's own scrollbar needs nothing.
- **Content rows stop above `ButtonRowY()`** — derive the count from it, not
  from `InnerRect().H`. Rows drawn from `inner.Y+1` fit
  `ButtonRowY() - inner.Y - 1` (one fewer again above a `DrawSeparator`
  line); `inner.H - 2` ran one row onto the buttons and hid the last entry.
- **The button row is reachable with Tab**, never only through a function key
  or the mouse.
- **A dialog with a text field is a `core.ClipboardHost`** returning an
  *explicit* nil on a miss (a typed nil is not a nil interface); the host
  routes Copy/Cut/Paste to `Stack.ClipboardTarget`, never past the frontmost
  dialog.
- **A dialog sized from the terminal re-decides its layout in `Show` and
  `Relayout`**, not only its constructor: `ModalDialog.Relayout` recentres at
  the last requested size, so override it to recompute and `SetSize`.
- **A tabbed dialog rebuilds its focus ring per tab** rather than skipping
  hidden controls in draw — hidden controls left in the ring take invisible
  keystrokes.

## Widgets and grids

- **Read colours at draw time**, from `theme.Active()` or a `theme.Style*()`
  call — never once at construction: `theme.SetPalette` switches live, and a
  captured style keeps the old theme. Build a style with a `theme.Style*()`
  helper, or `theme.On(bg, fg)` for a pair no helper names; a new `Style*`
  function goes in `styleFuncs` (`theme/contrast_test.go`), so
  `TestEveryStyleMeetsTheContrastFloor` checks it in every palette.
- **Pick the foreground that belongs to the ground** — on `DialogBg` the
  `Dialog*` foregrounds, on `MenuBar` the `Menu*` ones; text on an accent
  ground is `SelectedText`, and a ground with its own text role uses it. Dark
  and Light set each pair equal, so a mismatch only shows in the mc palettes.
- **Fill a `core.Buffer` with a styled ground before rendering a chart onto
  it** — `NewBuffer` starts every cell colourless and `Blit` copies cells as
  they are, so an unpainted gutter shows the screen's default ground as a
  strip, and `core.DimArea` skips it.
- **Draw a cell with `core.PutRune` or `Screen.Put`, never `SetContent`**
  (two allocations per cell per frame); fill a rect with `core.FillRect`
  (`Screen.FillArea`: one lock, ~7x faster than a `Put` per cell). A test
  draws into a `tuikittest.Screen`, not a hand-written fake —
  `tuikittest.NewMockScreen` when it must see the cells as tcell does.
- **`HandleKey`/`HandleMouse` return `true` only for events acted on** —
  never "focused, so consumed". `propsheet.Form` falls back to its own
  Tab/Up/Down only on `false`, so an always-`true` widget is a keyboard trap.
  A key is acted on when it changed something the user can see (selection,
  cursor, scroll, text, an expanded node, a popup) or fired a callback: Up on
  the first row, PgDn on an empty grid and Enter on a read-only one are
  `false`. `DataGrid`, `Editor`, `TreeView` and `ListBox` compare their
  visible state around the key — detect, never predict — so `propsheet`'s
  `GridRow` and `EditorRow` forward the answer as it is. A control does not
  test its own focus: the host routes keys to the one it focused.
  (`widgets.InputField` does not follow this yet: it tests its focus and
  claims every key it binds.)
- **Mouse return values follow one rule** (`core.MouseHandler`'s doc):
  plain motion is never claimed (hover may be tracked, but `true` only for a
  release ending the control's own gesture, or with an overlay open); a wheel
  tick over a control is claimed even at its limit; any press outside an open
  menu dismisses it, a right-click then falling through so the host can open
  the next menu there. A host moves keyboard focus on a *press* only, never
  on a claimed hover or wheel: `PropertySheet` once moved focus to its page
  list whenever the pointer crossed it. One tick scrolls `core.WheelRows`
  vertically and `core.WheelCells` sideways in every control (`DataGrid`
  sideways: one data column).
- **`SetBounds` does nothing when the rect hasn't changed** — hosts lay out in
  `Draw`, every frame; a `ListBox` re-running `ensureVisible` there snapped
  the wheel back to the selection. A changed rect pulls the scroll back inside
  the content and never moves the selection or cursor (`ListBox` and
  `TreeView` also scroll the selection into view); a wrapped `Editor` keeps
  its top line, as its visual-row count changes with the width.
  `TestContractSetBounds*` holds every control to it. A `DataGrid` host that changes cells in
  place calls `RefreshColumnWidths`; a setter that changes widths defers its
  recompute to `Draw`.
- **Never `DataGrid.SetData` on a grid being navigated** — it drops cursor,
  scroll and dragged widths; inside the grid's own `OnSelectRow` it undoes the
  move. Use `SetDataPreservingView`, which collapses a multi-row selection
  onto the cursor; `SetSourceKeepingSelection` keeps it, for a rebuild that
  leaves every row where it was.
- **A host acting on whole rows reads `DataGrid.SelectedRows()`**, never
  `SelectionBounds()`: bounds are a rectangle, so Ctrl+click rows 1 and 3
  covers row 2 too.
- **Setting a value programmatically drops the selection** — an anchor past a
  shorter new value paints blanks as selected. A new `SetXxx` clears the
  selection and re-anchors on the caret, as `SetValue` does.
- **Pre-filled fields read from their start**: `SetValue` leaves the caret at
  the end (right for a typed path); call `InputField.ShowFromStart()` after
  setting a picked value.
- **A disabled control keeps its place in the focus ring** — drawn greyed,
  refusing input, never removed (removal renumbers focus indexes).
  `propsheet.Form` is the exception: it skips a disabled row when moving
  focus, as it finds the next focusable row by walking its rows each time and
  holds no index a skip could break.
- **Don't rely on a mouse modifier, Shift especially**: VTE terminals take
  Shift+mouse for their own selection and forward nothing. A Shift+mouse
  gesture also accepts another modifier (`DataGrid` takes Shift or Alt) and a keyboard
  route.

## Editor

- **Call `SetIndentWidth` at construction, before any `SetText`** — `SetText`
  expands tabs at the current width, so a later change reads as an edit to a
  host comparing text. A runtime change doesn't re-expand existing text.
- **Auto-indent lives in the `KeyEnter` branch of `editor_input.go`, never in
  `insertNewline`** — `Paste` calls `insertNewline` per line, so indenting
  there staircases a paste. The branch reads `blockEditing()` *before*
  `deleteSelection`, and the spaces go under the Enter's own undo step. The
  host's `SetIndentRule` decides the extra level from the text left of the
  cursor.
- **`SetText` normalizes** (tabs, CRLF, invalid UTF-8 → U+FFFD): a host
  tracking "unsaved changes" seeds its baseline from `Text()` after
  `SetText`, never from the string it passed in.
