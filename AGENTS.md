# Repository Guidelines

## Project Structure & Module Organization

CiteBox is a Go + SQLite application with a native HTML/CSS/JavaScript frontend. Main entrypoints live in `cmd/server` and `cmd/desktop`. Core backend layers are under `internal/`: `handler` (HTTP), `service` (business logic), `repository` (SQLite), `model`, `config`, and shared app wiring in `internal/app`. Frontend pages are in `web/`, with shared assets in `web/static/`. Packaging scripts live in `scripts/`, build targets in `Makefile`, and longer-form docs in `docs/`.

## Build, Test, and Development Commands

- `make run`: start the web server at `http://localhost:8080`.
- `make run-desktop`: launch the desktop client with embedded local server.
- `make build` / `make build-desktop`: compile server or desktop binaries into `bin/`.
- `make test`: run the full Go test suite (`go test ./...`).
- `make prepare-web-assets`: fetch `pdf.js` runtime assets required by some web flows.
- `make package-desktop-linux|darwin|windows`: create desktop distribution archives in `dist/`.

When editing frontend code, also syntax-check touched files, for example:

```bash
node --check web/static/js/library.js
```

## Coding Style & Naming Conventions

Use `gofmt` for all Go files; keep package names lowercase and exported identifiers in `PascalCase`. Follow the existing layered design: handlers should stay thin, services own workflow logic, repositories own SQL and migrations. Frontend code uses plain JavaScript objects/modules, 4-space indentation, `camelCase` identifiers, and descriptive `data-*` hooks in HTML. Prefer ASCII unless a file already contains localized copy.

## Testing Guidelines

Go tests are colocated as `*_test.go` files, especially under `internal/repository` and `internal/service`. Name tests as `Test<Behavior>`. Add repository tests for schema changes, migrations, constraints, and search behavior. Use `go test ./...` before submitting; for UI-only changes, include at least a JS syntax check and a brief manual verification note.

## Commit & Pull Request Guidelines

Recent history favors short imperative subjects, e.g. `Add notes workspace and markdown preview` or scoped fixes like `fix: set http.Server.Addr to respect ServerPort config`. Keep commits focused. PRs should describe user-visible impact, call out schema/config changes, list verification commands, and include screenshots for navigation or layout changes. If you touch database structure, update `docs/database.md` in the same PR.

When changing database schema, note models, or note semantics, update the relevant documentation immediately in the same change instead of leaving docs drift for later.
