# CLAUDE.md

Guidance for working in this repository. This file holds **only** high-level
coding and operational principles. It must never describe product functionality
or per-feature behaviour. Keep all feature description in the README.md.

## Coding principles

General and meant to be reused verbatim across projects.

- **Complete names.** Use descriptive, whole-word names for non-trivial
  variables (`tuiWidth`, not `boxW`). Short names are acceptable only for
  receivers, loop indices, `err`, `ok`, and a framework's own idiomatic short
  names — `cmd` for a `tea.Cmd`, `msg` for a `tea.Msg`. Keep those reserved for
  that exact type: a command that is not a `tea.Cmd` is `command`, not `cmd`.
- **Breathing room.** Separate a statement that produces a value from the
  statement that consumes it with a blank line — e.g. an assignment, a blank
  line, then the `if err != nil` check. Group code into readable paragraphs.
- **Guard clauses.** Handle edge cases and errors first and return early, so the
  happy path stays unindented and reads straight down the function.
- **No comments.** Code must explain itself through naming and structure.
  (Struct tags are not comments.) The rare exception is a constraint the code
  cannot express on its own — an external or internal protocol, not a
  restatement of what the code does — such as noting that a flag exists only
  because another process invokes it.
- **Procedural code.** Always inline simple logic so readers don't have to jump
  around to see what small functions do. 1-3 line functions shouldn't exist
  unless there's a very good reason — e.g. they wrap something that could change,
  such as a hardcoded filepath. Never create a function that is only used once.
  Long procedural functions are fine; reading one top to bottom should describe
  its entire behaviour with minimal jumping around.
- **Functional code.** Prefer functional, stateless code. Some libraries demand
  statefulness and it's fine to follow their style, but everywhere else avoid
  mutable state.
- **Switch over ladders.** Prefer a `switch` (including a type switch) to a long
  `if` / `else if` chain.
- **Co-location.** A self-contained unit lives entirely in its own file — its
  config, state, behaviour, and rendering together. Its only references from
  elsewhere are where it is wired in at the composition root. Removing it means
  deleting its file and that one line of wiring — nothing scattered across the
  project.
- **Table-driven tests.** Express tests as a table of input → expected cases
  iterated in a loop, not as repeated near-identical assertions.

## Architecture principles

- **The Elm Architecture (Bubble Tea).** State lives in models, transitions
  happen in `Update`, side effects are expressed as `Cmd`s. Never block and
  never spawn goroutines directly — express asynchronous work as a `Cmd`.
- **Widgets are independent.** They coordinate only through messages, never by
  calling one another.
- **No import cycles.** Widgets never import the root `tui` package; they
  satisfy its interfaces structurally.
- **Capability interfaces, not fat ones.** A unit implements a small core
  interface and opts into extra behaviour only by satisfying additional, single-
  purpose interfaces that the composition root detects with a type assertion.
  New capabilities never widen the core interface.
- **Declarative, decentralized config.** Each unit owns its config struct,
  defaults, and section name; one generic, unit-agnostic loader overlays the
  on-disk file. Adding or changing config never touches the loader.

## Operational principles

- Build and run with cgo disabled for a static, dependency-free binary:
  `CGO_ENABLED=0 go build`.

## Maintenance

After adding or changing a major feature, re-read this file and update it so the
principles stay accurate.
