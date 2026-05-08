# GitHub Copilot Instructions for KeyMapr

## Project Reference
Refer to `project.md` in the repository root as the **primary source of truth** for this project.
It is a living document and is kept up to date with all architecture decisions, design choices,
sprint progress, and implementation notes. Always read it before suggesting changes.

---

## Working Agreement
- Before running any command, explain what it does and why, and wait for approval.
- All the research outputs and input are in the research folder and should be updated as the project progresses.
- This is a **learning project** in Go — prefer clear, idiomatic code over clever optimisations.
  Take the time to explain what and why, not just how.
- Keep changes surgical — do not refactor unrelated code in the same change.
- If a decision is made that isn't already in `project.md`, add it there.

---

## Coding Conventions

### Go Style
- Follow standard Go idioms and [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments).
- Use `fmt.Errorf("...: %w", err)` for error wrapping; use `errors.Is` / `errors.As` for unwrapping.
- Prefer table-driven tests in `*_test.go` files.
- Use `//go:build darwin` build tags on all CGo/macOS-specific files.
- Package names are lowercase, single words matching their directory name.

### CGo
- All CGo code lives in its own `internal/<package>/` directory.
- Free C memory with `C.free(unsafe.Pointer(...))` immediately after use.
- Never store `C.*` types in Go structs; convert to native Go types at the CGo boundary.

### Comments
- Only comment non-obvious logic — especially CGo memory management and macOS API quirks.
- Architecture decisions belong in `project.md`, not inline comments.
