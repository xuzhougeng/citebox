# AI External Search Sources Design

**Date:** 2026-05-01
**Status:** Draft approved in brainstorming
**Scope:** AI assistant external evidence search only

## Background

CiteBox already has a Semantic Scholar powered `/research` page and an AI assistant external-search tool that uses the same Semantic Scholar service. This works, but the AI assistant becomes fragile when Semantic Scholar is used without an API key because anonymous access is rate limited. PubMed is a better default external evidence source for many biomedical citation and evidence queries, and it supports anonymous NCBI E-utilities access with optional API-key based higher limits.

This design adds configurable, multi-source external academic search for the AI assistant. It covers every AI path that performs external evidence lookup, including the assistant's `external_search` tool and the older external-evidence injection path. It does not expand the `/research` page, the research basket, or library import workflow.

## Goals

- Let users explicitly configure which external search sources the AI assistant uses.
- Default AI external search to PubMed.
- Allow one or more enabled sources at the same time.
- Generate source-specific search queries for enabled sources.
- Continue with successful sources when another enabled source fails.
- Deduplicate overlapping papers across sources by DOI, PMID, and normalized title.
- Keep `/research` as the existing Semantic Scholar research panel.
- Keep PubMed as an AI evidence source only in the first version.

## Non-Goals

- No PubMed UI in `/research`.
- No PubMed research basket support.
- No PubMed import-to-library workflow.
- No PubMed references, citations, or recommendation tabs.
- No automatic source guessing outside the enabled-source list.
- No automatic fallback to a disabled source.
- No full-text PubMed Central ingestion.

## Architecture

Add an AI-only external search layer:

```text
internal/service/pubmed/
  client.go
  types.go
  client_test.go

internal/service/ai_external/
  types.go
  service.go
  s2_adapter.go
  pubmed_adapter.go
  merge.go
```

The AI assistant keeps the existing external-search entry points, but external evidence lookup depends on `ai_external.Service` instead of directly depending on `research.Service`.

```text
AI assistant external_search_tool
  -> ai_external.Service
      -> enabled source settings
      -> source-specific planner
      -> Semantic Scholar adapter / PubMed adapter
      -> partial-failure handling
      -> cross-source merge
  -> existing classifier
  -> cards, citations, answer context
```

The existing `internal/service/research` package remains the Semantic Scholar Graph API service for `/research` and for the Semantic Scholar adapter. PubMed is not added to that package.

All AI code that currently says "external Semantic Scholar" for evidence lookup should become source-aware. This includes `ai_assistant.ExternalSearchTool` and `ai_conversation` evidence injection.

## Configuration

### AI Search Sources

Add an app setting:

```text
ai_external_search_sources = ["pubmed"]
```

Allowed source IDs:

- `pubmed`
- `semantic_scholar`

Semantics:

- Default value is `["pubmed"]`.
- Users may enable one or more sources.
- Source order is significant. Ranking preserves this order before per-source rank.
- An empty list means no external AI search source is enabled.
- This setting only controls source selection. It does not replace the existing AI intent and per-request external-search behavior.

The settings page shows this in the AI configuration area as checkboxes:

```text
AI 外部搜索源
[x] PubMed
[ ] Semantic Scholar
```

### PubMed / NCBI Settings

Add app settings:

```text
pubmed_api_key
pubmed_email
pubmed_tool
```

Semantics:

- All fields are optional.
- Anonymous PubMed access is allowed.
- When values exist, the PubMed client adds them to NCBI E-utilities requests.
- `pubmed_tool` defaults to `citebox` when omitted.
- Saving settings hot-updates the live PubMed client, similar to the existing Semantic Scholar API key handling.

## Planner

Extend the existing external search planner from a single list of queries to source-specific queries. The LLM-backed planner may remain in the AI assistant package, but its output is passed to `ai_external.Service` as a source-keyed plan.

```json
{
  "queries_by_source": {
    "pubmed": [
      "forward genetic screening cell fate gene redundancy",
      "\"cell fate\" AND \"forward genetic screen\""
    ],
    "semantic_scholar": [
      "forward genetic screening cell fate gene discovery redundancy",
      "model organism genetic screen saturation gene family redundancy"
    ]
  },
  "rationale": "PubMed uses biomedical Boolean-style terms; Semantic Scholar uses broader recall queries."
}
```

Rules:

- The planner only returns queries for enabled sources.
- Each source receives at most four queries.
- If the planner fails, every enabled source uses the existing local fallback query generation.
- If the planner returns no queries for one enabled source, only that source uses fallback queries.
- Planning process output reports per-source query counts, for example `PubMed 2 条，Semantic Scholar 2 条`.

## PubMed Behavior

The first PubMed implementation supports only AI evidence search.

### Search

```text
ESearch db=pubmed term=query retmax=limit
  -> EFetch or ESummary metadata hydration
  -> ai_external.Paper
```

The adapter returns:

- `source = pubmed`
- `source_paper_id = PMID`
- `pmid`
- `pmcid`
- `doi`
- `title`
- `abstract`
- `venue` / journal
- `year`
- `authors`
- `url = https://pubmed.ncbi.nlm.nih.gov/{pmid}/`

### Snippets

PubMed does not provide a Semantic Scholar style snippet search. `SnippetSearch` reuses PubMed search results and builds evidence snippets from title and abstract text.

### Rate Limiting

PubMed uses a separate rate limiter:

- Anonymous: approximately 3 requests per second.
- API key configured: approximately 10 requests per second.
- On rate limit or transient upstream failure, retry once with short backoff.
- If the retry fails, report PubMed as a failed source and let other enabled sources continue.

## Search Execution

For one AI external-search call:

1. Read enabled sources.
2. Generate source-specific queries.
3. Execute enabled sources in parallel.
4. Enforce each source's own rate limit.
5. Merge successful results.
6. Deduplicate by DOI, PMID, then normalized title.
7. Return merged candidates to the existing classifier.
8. Render cards, citations, and answer context with source metadata.

Failure semantics:

- If one source fails and another succeeds, the AI answers using successful evidence.
- Failed sources appear in process stages and process notes.
- If all enabled sources fail, the external-search tool reports failure.
- If no source is enabled, the tool returns a clear "no external search source enabled" result without making network requests.
- Disabled sources are never used as automatic fallbacks.

## Merge And Dedupe

Deduplication priority:

1. Normalized DOI equality.
2. PMID equality.
3. Normalized title equality.

Merged paper behavior:

- `Sources` accumulates all contributing source IDs.
- `SourcePaperIDs` maps each source to the source-local ID.
- DOI, PMID, PMCID, arXiv IDs are preserved when available.
- Abstract uses the longer non-empty abstract.
- Semantic Scholar citation counts remain source-specific metadata and are not fabricated for PubMed.
- First-version ranking is stable: preserve configured source order and per-source rank, then truncate to limit after merge.

## AI Cards And Citations

Extend the `external_paper` card payload with source metadata:

```json
{
  "sources": ["PubMed", "Semantic Scholar"],
  "source_ids": {
    "pubmed": "12345678",
    "semantic_scholar": "abc..."
  },
  "pmid": "12345678",
  "pmcid": "PMC...",
  "doi": "10....",
  "url": "https://pubmed.ncbi.nlm.nih.gov/12345678/"
}
```

Display behavior:

- Single source: `来源: PubMed`.
- Multiple sources: `来源: PubMed + Semantic Scholar`.
- Evidence prompts no longer say "外部 Semantic Scholar" globally. They say "外部学术搜索" and label each evidence item with its actual source.
- Process stages report per-source query count, returned count, merged count, and failures.

## Compatibility

- `/research` remains Semantic Scholar only.
- Existing Semantic Scholar settings continue to work.
- Existing AI external-search user flow remains the same except the default source changes to PubMed.
- Existing external-evidence injection in AI conversations becomes source-aware and uses the same enabled-source setting.
- Existing tests that assert hardcoded "Semantic Scholar" strings in AI external search must be updated to source-aware expectations.
- Existing `research.Paper` remains unchanged for the `/research` surface; AI-only types live under `ai_external`.

## Testing

### PubMed Client Tests

Use `httptest.Server` to cover:

- ESearch success.
- Metadata hydration success.
- Empty search results.
- 429 / rate-limit retry.
- Missing XML fields.
- DOI, PMID, PMCID, title, abstract, journal, year, and authors parsing.

### AI External Service Tests

Cover:

- Default enabled source is PubMed.
- Reading multiple enabled sources.
- Source-specific query planning.
- Planner total failure with fallback queries.
- One source missing queries with per-source fallback.
- Partial source failure continues with successful sources.
- All sources failing returns an error.
- Empty source list makes no upstream requests.

### Merge Tests

Cover:

- DOI dedupe.
- PMID dedupe.
- Normalized title dedupe.
- Multiple source IDs preserved.
- Longer abstract selection.
- Stable configured-source ordering.

### AI Assistant Tests

Cover:

- Process notes list enabled sources.
- Cards include `sources` and source IDs.
- Citations label PubMed and Semantic Scholar correctly.
- Classifier receives merged candidates.
- Existing Semantic Scholar-only behavior works when only that source is enabled.
- AI conversation external-evidence injection labels evidence with the actual configured source.

### Frontend Verification

- `node --check web/static/js/settings.js`
- AI settings page saves source checkboxes and PubMed settings.
- A "查外部" query shows PubMed process stages by default.
- With PubMed and Semantic Scholar both enabled, a fake failure in one source still returns results from the other.

## Rollout

1. Add PubMed settings and source checkbox UI.
2. Add PubMed client with tests.
3. Add `ai_external` interfaces, adapters, merge logic, and tests.
4. Update external-search planner prompt and parser.
5. Wire AI external-search tool to `ai_external.Service`.
6. Update AI result cards, process text, and evidence source labels.
7. Update docs for AI external search configuration.

The rollout keeps `/research` untouched and lets implementation ship behind the new AI source settings.
