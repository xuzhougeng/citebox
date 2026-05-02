# External Search Goal Hint Shortcuts - Design

- **Date**: 2026-05-02
- **Author**: xuzhougeng + Codex
- **Status**: Draft
- **Related**:
  - `docs/superpowers/specs/2026-05-01-external-search-discovery-evidence-split-design.md`
  - `docs/superpowers/specs/2026-05-01-at-mention-search-tools-design.md`

## Background

The external-search pipeline now supports two internal goals:

- `discovery`
- `evidence`

Under normal conditions, the planner decides which goal applies. This works for multilingual natural-language input and keeps the main path flexible.

The remaining weakness is fallback behavior. When the planner is unavailable, malformed, or returns an unusable `search_goal`, the backend still needs to choose between `discovery` and `evidence`. We intentionally do **not** want that decision to depend mainly on language-specific keyword matching.

The cleanest fix is to let the UI send an explicit goal hint whenever the UI itself already knows the user’s intent.

## Problem

Today, the AI composer can explicitly tell the backend:

- which tool family to use via `intent_hint`
- which external source to use via `sources`

But it cannot explicitly say:

- “this external search is a broad paper-finding request”
- “this external search is a source-finding / evidence-check request”

That means even when the user clicks an obvious UI shortcut, the backend may still need to infer the goal indirectly.

## Goals

- Add an explicit request field for external-search goal hints.
- Let known UI shortcuts set that field without relying on language parsing.
- Keep the current planner-driven path for ordinary free-form input.
- Preserve one-turn shortcut behavior rather than introducing a sticky mode.
- Avoid changing the existing `@PubMed` / `@SemanticScholar` routing model in this increment.

## Non-Goals

- No new persistent composer mode.
- No new `@Evidence` mention tag in this increment.
- No attempt to infer `search_goal_hint` from arbitrary user text on the frontend.
- No redesign of the planner or classifier contracts introduced in the prior feature.
- No replacement of planner `search_goal`; this is a high-priority hint, not a new single source of truth for every path.

## User-Validated Decisions

The design below reflects explicit decisions made in discussion:

- `discovery / evidence` should primarily be carried explicitly in the request rather than guessed server-side.
- When the frontend does **not** clearly know the intent, it should **not** invent a guess.
- Explicit external-search shortcuts should be one-turn actions.
- The shortcut labels should be:
  - `查全库`
  - `查外部`
  - `找出处`
- `查外部` should map to external-search discovery.
- `找出处` should map to external-search evidence.

## Chosen Approach

Add a new request field:

```json
"search_goal_hint": "discovery" | "evidence"
```

This field is optional and only meaningful for `intent_hint == "external_search"`.

Composer shortcuts become:

- `查全库`
- `查外部`
- `找出处`

Behavior:

- `查全库`
  - sends `intent_hint = "library_search"`
- `查外部`
  - sends `intent_hint = "external_search"`
  - sends `search_goal_hint = "discovery"`
- `找出处`
  - sends `intent_hint = "external_search"`
  - sends `search_goal_hint = "evidence"`

All three remain one-turn shortcuts: after send, the shortcut resets.

## UI Behavior

### Composer shortcuts

The existing AI composer shortcut strip already supports one-turn `intent_hint` actions. This design extends that strip with one more external-search-specific shortcut.

The resulting visible set is:

- `查全库`
- `查外部`
- `找出处`
- `读文献`
- `看图/图文`

No new toggle, modal, or persistent mode is introduced.

### Why `找出处`

This label is more operationally precise than softer conceptual wording such as “言之有物”.

It clearly signals:

- source finding
- support checking
- evidence-oriented external search

without forcing the backend to infer that intent from natural language.

## Frontend Request Semantics

### Explicit shortcuts

When the user clicks a composer shortcut, the next outgoing request should carry:

| Shortcut | `intent_hint` | `search_goal_hint` |
|---|---|---|
| `查全库` | `library_search` | omitted |
| `查外部` | `external_search` | `discovery` |
| `找出处` | `external_search` | `evidence` |

### Plain input

When the user types free-form text and does **not** use these shortcuts:

- the frontend should not guess `search_goal_hint`
- the field should be omitted
- the planner remains the primary decider

### `@PubMed` / `@SemanticScholar`

This increment does not change the mention-tag parser.

If the user types:

- `@PubMed ...`
- `@SemanticScholar ...`

and does not also use a shortcut, the request still carries:

- `intent_hint = "external_search"`
- `sources = [...]`
- no `search_goal_hint`

That path continues to rely on the planner first, then server fallback.

## Backend Precedence

For `external_search`, the backend should determine the effective goal in this order:

1. `search_goal_hint` from the request
2. planner-produced `search_goal`
3. existing fallback heuristic

This keeps the hint explicit and high-priority without forcing it into unrelated tool families.

### Scope of `search_goal_hint`

`search_goal_hint` should be ignored unless the request is actually executing `external_search`.

That avoids leaking external-search semantics into:

- `library_search`
- `paper_read`
- `figure_lookup`

## Transport Changes

### Frontend

The composer payload needs one new optional field:

```js
payload.search_goal_hint = 'discovery' | 'evidence';
```

### HTTP handler

The conversation message request body accepts:

```json
"search_goal_hint": "discovery"
```

and passes it through to conversation / orchestrator / tool input structs.

### Assistant tool input

The external-search tool input should gain a matching optional field so the backend can apply the precedence rule above.

## Compatibility

- Old clients that do not send `search_goal_hint` continue to work.
- Existing planner behavior remains valid.
- Existing `intent_hint` and `sources` semantics remain unchanged.
- Existing free-form flows remain planner-first.

## Error Handling

- Unknown `search_goal_hint` values should be ignored or normalized away rather than crashing the request.
- If `search_goal_hint` is present with a non-external intent, ignore it.
- If `search_goal_hint` conflicts with a planner result, the explicit hint wins.

## Testing Strategy

### Frontend

- composer shortcut tests or focused JS tests verifying:
  - `查外部` sends `intent_hint=external_search` plus `search_goal_hint=discovery`
  - `找出处` sends `intent_hint=external_search` plus `search_goal_hint=evidence`
  - shortcut selection resets after send
  - plain input without shortcut omits `search_goal_hint`

### Handler / transport

- request body with `search_goal_hint` is decoded and forwarded
- requests without it preserve current behavior

### Orchestrator / tool

- explicit `search_goal_hint` overrides planner result
- omitted hint falls back to planner result
- if both hint and planner are absent/unusable, existing fallback still applies

## Rollout Scope

This is intentionally a small follow-up increment on top of the already-implemented discovery/evidence split:

- add one request field
- add one composer shortcut
- wire explicit precedence through the external-search path

It should not require any data migration or schema change.
