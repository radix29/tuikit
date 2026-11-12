# CLAUDE.md

Context for Claude Code sessions on **tuikit**.

## What this is

tuikit is a widget/dialog/layout/chart library for terminal applications on
tcell v3. Module `github.com/radix29/tuikit`, Go 1.27, MIT license, no CGO.
It was extracted from goSSMS (sibling checkout `~/go/gossms`, which builds
against this tree through `replace github.com/radix29/tuikit => ../tuikit`).

- `README.md` — package map, dependency direction, design principles, adding
  a control or a dialog. Read § Design principles before changing any control.
- `ARCHITECTURE.md` — threading, the mouse-gesture protocol, overlays, dialog
  and widget rules. Read § Mouse gestures before touching any `HandleMouse`.

## Rules that shape the code

- **Dependencies: the standard library, tcell v3, displaywidth — nothing
  else**, and nothing of any application.
  `TestTuikitImportsOnlyItsDeclaredDependencies` and `TestTuikitLayering`
  (`core/imports_test.go`) enforce it and the one-way graph between packages.
- **Verify against real source, don't guess.** Check `go doc` or the module
  cache before writing a tcell call. tcell v3 has no `PollEvent`/`PostEvent`
  (`EventQ()` channel); the modifier accessor is `Modifiers()`.
- **Display width, never `len(s)`**, for column math: `core.DisplayWidth`,
  and `core/runecol.go` for rune-indexed text.
- One file per type or tight group per package, plus `common.go`/`doc.go`.
- The long "why" comments name shipped bugs a plausible simplification would
  bring back — not a cleanup target.
- tuikit is a library with users outside goSSMS: don't remove or narrow
  exported surface because goSSMS doesn't call it.

## Build & verify

```sh
go build ./... && go vet ./... && gofmt -l .
go test ./...
staticcheck ./...          # must stay clean; deadcode lists exported API, not a cleanup list
go test -race -count=1 ./<pkg>/...   # a change crossing goroutines
```

Green unit tests are not verification: build goSSMS against this tree and
drive the changed widget in its binary (`~/go/gossms/docs/testing.md`).

## Repo hygiene

- Don't commit, amend, push or tag. The user does that.
- Check a path exists before writing to it.
