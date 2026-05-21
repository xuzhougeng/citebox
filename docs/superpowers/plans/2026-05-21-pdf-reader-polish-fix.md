# PDF Reader Polish Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the PDF reader look and behave like a real reader on desktop and mobile, and fix broken text-selection highlighting.

**Architecture:** Keep the existing `/viewer?kind=pdf` route and pdf.js rendering branch. Replace the ad hoc PDF toolbar styling with responsive reader chrome, and replace the incomplete text-layer CSS with pdf.js-compatible text layer rules. Keep selection translation behavior intact.

**Tech Stack:** Plain HTML/CSS/JavaScript, pdf.js TextLayer, Playwright browser smoke checks, Node syntax checks.

---

## File Map

- `web/viewer.html`: reader toolbar layout, responsive CSS, pdf.js text layer CSS.
- `web/static/js/viewer.js`: PDF render scale defaults, text layer sizing cleanup, selection menu positioning cleanup.
- `docs/superpowers/plans/2026-05-21-pdf-reader-polish-fix.md`: implementation checklist.

## Task 1: Fix Text Layer Rendering and Selection

- [x] **Step 1: Capture the broken behavior**

Use the existing PDF route in a browser and verify that selecting PDF text creates fragmented blue highlight blocks across the page.

- [x] **Step 2: Replace incomplete text layer CSS**

In `web/viewer.html`, update `.viewer-pdf-text-layer` and child rules to match pdf.js expectations: transparent text, `forced-color-adjust: none`, `transform-origin: 0 0`, text spans positioned absolutely, and selection highlighting constrained to actual text spans.

- [x] **Step 3: Remove stale text layer state before re-render**

In `web/static/js/viewer.js`, clear text layer state before each page render and keep width/height/font scaling synchronized with the viewport.

- [x] **Step 4: Verify selection**

Open a PDF in the browser, drag-select one sentence, and confirm the menu appears near the selection and only the selected text highlights.

## Task 2: Redesign Desktop and Mobile Reader Chrome

- [x] **Step 1: Replace oversized toolbar layout**

In `web/viewer.html`, update the PDF toolbar to wrap predictably: compact page controls, zoom controls, and actions. Keep labels available through i18n and aria labels, but make controls visually smaller and stable.

- [x] **Step 2: Improve PDF viewport**

Adjust `.viewer-stage.pdf-mode`, `.viewer-pdf-scroll`, and `.viewer-pdf-page` so the document is centered, the background is quiet, and the page has stable dimensions without excessive shadow or padding.

- [x] **Step 3: Add mobile-specific rules**

At mobile width, stack the header cleanly: back button, title/file name, then compact PDF controls. Ensure no button overlaps or dominates the first screen.

- [x] **Step 4: Verify desktop and mobile screenshots**

Use Playwright at desktop and mobile viewport sizes. Confirm the first page is readable, controls fit, and image viewer layout still works.

## Task 3: Verification and Release Follow-up

- [x] **Step 1: Run syntax checks**

Run:

```bash
node --check web/static/js/viewer.js
git diff --check
```

Expected: both exit 0.

- [x] **Step 2: Run focused browser smoke**

Verify PDF loading, page navigation, zoom/fit width, selection copy/translate menu, mobile layout, and image viewer regression.

- [x] **Step 3: Commit**

Commit with:

```bash
git add web/viewer.html web/static/js/viewer.js docs/superpowers/plans/2026-05-21-pdf-reader-polish-fix.md
git commit -m "Polish PDF reader layout and selection"
```

## Self-Review

- Spec coverage: fixes the two reported issues: bad selection highlight and poor mobile/desktop reading layout.
- Placeholder scan: no deferred implementation placeholders.
- Type consistency: only existing `ResourceViewerPage` and PDF state fields are used.
