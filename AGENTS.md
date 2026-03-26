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
- `scripts/macos-desktop-ui-smoke.zsh smoke`: macOS-only desktop UI smoke test for close prompt, "minimize to tray", and Dock reopen flows. Start the desktop app first, and make sure Terminal/`osascript` already has Accessibility permission.

For targeted macOS desktop UI checks, `scripts/macos-desktop-ui-smoke.zsh` also exposes helper commands: `processes`, `windows`, `tree`, `close-prompt`, `to-tray`, `dock-items`, and `dock-reopen`. Override the default process/window/Dock names with `CITEBOX_MACOS_PROCESS_NAME`, `CITEBOX_MACOS_WINDOW_NAME`, and `CITEBOX_MACOS_DOCK_ICON_NAME` when the local app label differs.

When editing frontend code, also syntax-check touched files, for example:

```bash
node --check web/static/js/library.js
```

## Coding Style & Naming Conventions

Use `gofmt` for all Go files; keep package names lowercase and exported identifiers in `PascalCase`. Follow the existing layered design: handlers should stay thin, services own workflow logic, repositories own SQL and migrations. Frontend code uses plain JavaScript objects/modules, 4-space indentation, `camelCase` identifiers, and descriptive `data-*` hooks in HTML. Prefer ASCII unless a file already contains localized copy.

## I18n Guidelines

- Treat i18n as required for all user-visible frontend copy. Do not add or keep new hardcoded UI text in HTML or JavaScript when it can be served through the existing locale system.
- When adding or changing frontend text, update both `web/static/locales/zh-CN/` and `web/static/locales/en/` in the same change, and keep keys aligned across languages.
- Put page-specific strings in the matching page locale file and reserve `shared.json` for copy reused across multiple pages or features.
- When wiring UI text, prefer the existing translation hooks and helpers already used in the repo, such as `data-i18n` attributes and `t(...)`/`CiteBoxI18n` lookups.

## Testing Guidelines

Go tests are colocated as `*_test.go` files, especially under `internal/repository` and `internal/service`. Name tests as `Test<Behavior>`. Add repository tests for schema changes, migrations, constraints, and search behavior. Use `go test ./...` before submitting; for UI-only changes, include at least a JS syntax check and a brief manual verification note.

When changing macOS desktop window-management flows, tray behavior, close-confirmation dialogs, or Dock reopen behavior, run `scripts/macos-desktop-ui-smoke.zsh smoke` as part of manual verification. If the full smoke command fails, use the script's helper subcommands to isolate whether the issue is process discovery, window selection, close prompt rendering, tray minimization, or Dock reopening.

## Commit & Pull Request Guidelines

Recent history favors short imperative subjects, e.g. `Add notes workspace and markdown preview` or scoped fixes like `fix: set http.Server.Addr to respect ServerPort config`. Keep commits focused. PRs should describe user-visible impact, call out schema/config changes, list verification commands, and include screenshots for navigation or layout changes. If you touch database structure, update `docs/database.md` in the same PR. If you change frontend-to-backend API routes, request fields, response shapes, or API usage semantics, update `docs/api.md` in the same PR.

When changing database schema, note models, or note semantics, update the relevant documentation immediately in the same change instead of leaving docs drift for later.

## TODO Workflow

When the user asks to work from `TODO`:

- Treat the repository-root `TODO` file as the current backlog source of truth unless the user says otherwise.
- Read the relevant section first, estimate scope, then implement items in the requested order.
- Update `TODO` in the same change: mark finished items as `[x]`. If the file already has an `已完成` section, move completed entries there instead of leaving duplicate copies in the active section.
- Keep the original task wording unless a clarification is necessary to reflect the final behavior more accurately.

## Release Note Workflow

When the user asks for release notes:

- Only use committed changes inside the requested tag range. Never include uncommitted files.
- Check `git log --oneline <from_tag>..<to_tag>` and `git diff --stat <from_tag>..<to_tag>` first, then read specific files only when needed.
- Group by user-visible themes, not by commit. Skip internal-only refactors unless they affect behavior.
- Mention platform or page scope when relevant, such as `macOS desktop`, `AI 伴读页`, or `viewer-page`.
- Use this format unless the user asks otherwise:

```md
## What's Changed

### 功能标题

- 说明一个用户可见变化
- 说明具体行为变化
- 说明带来的实际收益
- 如有必要，说明影响范围或平台

### 功能标题

- 说明一个用户可见变化
- 说明具体行为变化
- 说明带来的实际收益

**Full Changelog**: https://github.com/xuzhougeng/citebox/compare/<from_tag>...<to_tag>
```
