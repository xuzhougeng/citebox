[English](./README.en.md) | [简体中文](./README.md)

# CiteBox

> Collect. Cite. Create.

CiteBox is a local-first workspace for paper organization and figure-centric reading. It is built with Go, SQLite, and a native HTML/CSS/JavaScript frontend, and it supports both a web server mode and an embedded desktop app.

The project is intentionally optimized around a narrow workflow instead of trying to be a general-purpose reference manager:

1. Import a paper from a local PDF or DOI-backed Open Access source.
2. Extract full text and figures automatically or manually.
3. Organize papers, figures, groups, tags, notes, and palettes in one place.
4. Continue the workflow in the AI Reader for Q&A, translation, summarization, and note-taking.

## Current Capabilities

- Paper import from local PDFs and DOI-based Open Access sources.
- Three extraction paths: external automatic extraction, built-in multimodal coordinate detection, or manual figure extraction.
- Built-in workspaces for library, figures, groups, tags, notes, palettes, and manual backfill.
- AI Reader with paper Q&A, figure interpretation, tag suggestions, streaming output, and export.
- Integrations for Wolai note export, WeChat IM bridge, TTS settings, and release checks.
- Web and desktop runtime modes backed by the same SQLite and local-file data model.

## Stack

- Go 1.21+
- SQLite
- Native HTML / CSS / JavaScript
- `webview_go` desktop shell

## Quick Start

### Web Mode

```bash
make run
```

Default address:

- `http://localhost:8080`
- Default username: `citebox`
- Default password: `citebox123`

For local development with authentication disabled:

```bash
make dev
```

### Desktop Mode

```bash
make run-desktop
```

Desktop mode starts an embedded local server on a random localhost port and opens the app in a native window. By default, data is stored in the OS user config directory, for example:

- Linux: `~/.config/CiteBox/`
- macOS: `~/Library/Application Support/CiteBox/`
- Windows: `%AppData%/CiteBox/`

### Common Commands

```bash
make build
make build-desktop
make test
make prepare-web-assets
```

## Configuration

Most runtime settings can be managed from the in-app Settings page. If you want to bootstrap with environment variables, these are the most common ones:

| Variable | Default | Description |
| --- | --- | --- |
| `SERVER_PORT` | `8080` | Web server port |
| `DATABASE_PATH` | `./data/library.db` | SQLite database path |
| `STORAGE_DIR` | `./data/library` | Storage root for PDFs and extracted figures |
| `ADMIN_USERNAME` | `citebox` | Login username |
| `ADMIN_PASSWORD` | `citebox123` | Initial login password |
| `DISABLE_AUTH` | `false` | Disable auth for local development |
| `OA_CONTACT_EMAIL` | empty | Improves DOI-based Open Access lookup coverage |
| `PDF_EXTRACTOR_PROFILE` | automatic extraction | Extraction mode; switch between automatic extraction, built-in multimodal detection, or manual mode in Settings |
| `PDF_EXTRACTOR_URL` | empty | External extraction service URL |
| `PDF_EXTRACTOR_TOKEN` | empty | Bearer token for the external extraction service |
| `WEIXIN_BRIDGE_ENABLED` | `false` | Default WeChat IM bridge flag before settings are saved to the database |

### Extraction Profiles

- External automatic extraction: uses an external service for standard automatic figure extraction.
- Built-in multimodal detection: uses a configured multimodal model to detect figure regions, while full text is primarily extracted via `pdf.js`.
- Manual mode: skips auto figure extraction, but still allows full-text persistence and later manual figure backfill.

## Docs

- [Frontend / Backend API Notes](./docs/api.md)
- [Database Notes](./docs/database.md)
- [macOS Development Notes](./docs/macos-development.md)

The README intentionally stays high-level. For endpoint details, schema notes, and migration behavior, rely on the files under `docs/` and the current code.

## Project Layout

```text
.
├── cmd/
│   ├── server/
│   └── desktop/
├── internal/
│   ├── app/
│   ├── config/
│   ├── handler/
│   ├── repository/
│   ├── service/
│   └── ...
├── web/
│   ├── *.html
│   └── static/
├── docs/
├── scripts/
└── Makefile
```
