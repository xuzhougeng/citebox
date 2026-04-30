# AI Evidence Search Plan

## Goal

Build the AI reader's strict evidence mode into a local-library evidence miner:

- Users ask a question or paste a claim.
- CiteBox searches the currently pinned papers first.
- The answer must cite retrieved evidence snippets.
- If evidence is missing, the model must say evidence is insufficient.

This feature is designed for "言之有理": making statements traceable to the user's existing paper library.

## Non-goals

- No embeddings.
- No vector database.
- No semantic-vector recall.
- No database schema change for the first implementation.

## Evidence Sources

Strict evidence mode has two evidence sources:

1. Local evidence, enabled by default.
   - Searches title, abstract, notes, and full `pdf_text` from currently pinned papers.
   - Uses rule-based query expansion and literal text matching.
   - Works without DOI, Semantic Scholar IDs, or network access.

2. External evidence, optional.
   - Uses Semantic Scholar snippet search for pinned papers that have usable external IDs.
   - Supplements local evidence but does not replace it.
   - Failure or rate limiting does not disable local evidence.

## Retrieval Flow

```text
User question / claim
  -> Extract and expand search terms
  -> Search pinned local paper text
  -> Optional: search Semantic Scholar snippets
  -> Build numbered evidence block
  -> Final answer model responds only from those snippets
```

## First Implementation

- Replaces the strict evidence injection layer with local-first evidence retrieval.
- Keeps existing citation footnotes by storing local snippets in `citations_json`.
- Adds an "External Evidence" checkbox on the AI page.
- Stores the external-evidence preference in browser local storage.
- Sends the external-evidence flag per message; strict evidence itself remains conversation-level.

## Next Steps

- Add a cheap-model evidence judge after local recall to classify snippets as `supports`, `partially_supports`, `related_only`, `contradicts`, or `irrelevant`.
- Cache evidence judgments by paper ID, normalized claim, snippet offset, and prompt version.
- Add a dedicated "查文献 / 言之有理" entry point that uses the same local-first evidence pipeline.
