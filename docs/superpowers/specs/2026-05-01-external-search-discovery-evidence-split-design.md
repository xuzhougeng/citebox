# External Search Discovery / Evidence Split - Design

- **Date**: 2026-05-01
- **Author**: xuzhougeng + Codex
- **Status**: Draft
- **Related**:
  - `docs/superpowers/specs/2026-05-01-ai-external-search-sources-design.md`
  - `docs/superpowers/specs/2026-05-01-at-mention-search-tools-design.md`

## Background

`@PubMed` and `@SemanticScholar` already route correctly into `external_search`, and PubMed recall is working. The current failure mode is later in the pipeline: the external-paper classifier still uses an "evidence / source-check" standard for every query, so discovery-style requests can recall real papers and still end with `hits = 0`.

Observed example:

- User query: `@PubMed 查找2025年的单细胞植物文章`
- Current planner produced valid PubMed queries.
- PubMed returned raw candidates.
- The Sub-Agent classified all candidates out, so the final answer said "没有找到".

This is the wrong product behavior for discovery-style requests. A user asking for related papers is not asking the system to prove a specific claim.

## Problem

The current external-search flow collapses two different user intents into one binary filter:

1. **Discovery**: find related papers, reviews, or recent work in an area.
2. **Evidence**: check whether a paper supports a specific claim, quotation, or conclusion.

Today, both intents pass through the same downstream classifier contract:

- `relevant = true|false`
- `false` means the paper disappears

That contract is acceptable for evidence-checking, but too strict for paper discovery. It turns "found candidates but not strong enough as evidence" into "found nothing".

## Goals

- Keep a single `external_search` tool and a single `external_search` intent.
- Split execution semantics inside the tool into `discovery` and `evidence`.
- Let the planner decide `search_goal`, with non-linguistic fallback behavior.
- Preserve candidate papers for discovery requests even when they are weak or ambiguous.
- Expose result tiers to users explicitly.
- Distinguish `online year` from `issue year` in both ranking and presentation.
- Avoid language-specific routing rules such as Chinese or English keyword matching.

## Non-Goals

- No new intent or separate `external_discovery_search` tool in this phase.
- No change to `@PubMed` / `@SemanticScholar` routing behavior.
- No vector retrieval or embedding-based relevance ranking.
- No attempt to solve all external-search UX issues in one pass.
- No automatic translation layer outside the existing planner/classifier LLM flow.

## Chosen Approach

Keep the current `external_search` tool, but upgrade the planner and classifier contracts:

1. The planner outputs a **search goal** plus a structured interpretation of the request.
2. Discovery requests return **tiered candidates** instead of binary pass/fail filtering.
3. Evidence requests preserve the existing stricter behavior.
4. Query expansion is used for recall only; it must not silently become a hard relevance filter.

This preserves the current architecture while fixing the main false-negative path.

## User-Validated Semantics

The design below reflects the explicitly agreed product behavior:

- Default external-search behavior should support both `discovery` and `evidence`.
- `discovery` vs `evidence` should be decided by the Master planner, not by language-specific text matching.
- If the planner cannot decide, fallback should use structural signals and otherwise bias toward `discovery`.
- If PubMed or Semantic Scholar recall candidates, the system must show them instead of saying "no results".
- The user should see explicit tiers such as `强相关 / 弱相关 / 待核查`.
- A query such as "find plant single-cell papers" should not be narrowed by incidental expansion terms such as `root`, `epidermis`, or `development`.
- Year handling must distinguish:
  - `2025 online / 2026 issue`
  - `2025 issue`

## High-Level Architecture

### Current shape

`planner -> source queries -> external recall -> classifier -> binary keep/drop -> cards + answer`

### New shape

`planner -> source queries + search_goal + must_match + soft_preferences + year constraint -> external recall -> heuristic pre-rank -> tier classifier -> goal-aware filtering -> cards + answer`

The major change is that the classifier is no longer the single pass/fail gate for discovery requests.

## Planner Contract

### New plan fields

Extend `ExternalSearchPlan` to include:

- `search_goal`: `discovery | evidence`
- `queries_by_source`
- `must_match`: normalized hard constraints that must hold for a strong match
- `soft_preferences`: normalized ranking preferences that improve ordering but do not eliminate a paper
- `target_year`: optional target year or year window
- `rationale`

Example interpretation:

#### Query A

`Find a few plant single-cell papers`

- `search_goal = discovery`
- `must_match = { plant, single-cell transcriptomics }`
- `soft_preferences = {}`

#### Query B

`Find 2025 plant epidermis development single-cell papers`

- `search_goal = discovery`
- `must_match = { plant, single-cell transcriptomics, 2025 }`
- `soft_preferences = { epidermis, development }`

#### Query C

`Does this paper support the claim that plant epidermal development trajectories can be reconstructed from single-cell RNA-seq?`

- `search_goal = evidence`
- `must_match = { claim support, plant epidermal development trajectories, single-cell RNA-seq }`
- `soft_preferences = {}`

### Key rule: recall terms are not hard filters

Planner-generated query expansion is allowed to broaden recall, but those extra terms must not be treated as required conditions unless the planner explicitly places them into `must_match`.

This prevents the bug where a broad request becomes over-narrow during post-filtering.

## Search Goal Detection

### Primary path

The Master planner decides `search_goal`. This supports multilingual requests because the LLM can interpret intent across languages.

### Fallback path

Fallback must not depend on matching Chinese or English trigger phrases. Instead:

- Use structural context where available:
  - explicit paper context or pinned-paper verification
  - selected citation or quoted claim context
  - compare/check-support workflows
- If there is no reliable structural evidence, default to `discovery`

The bias toward `discovery` is intentional because it is safer than producing a false "no results" for broad search requests.

## Candidate Ranking and Tiering

### Two-stage post-recall handling

After PubMed/S2 recall:

1. **Heuristic pre-rank**
   - push obviously irrelevant candidates down
   - improve classifier input quality
   - do not make final keep/drop decisions for borderline papers
2. **LLM tier classification**
   - classify each candidate into an explicit tier
   - produce reasons tied to `must_match` and `soft_preferences`

### Tier definitions

Use a single internal tier set for both goals:

- `strong_match`
- `weak_match`
- `needs_review`
- `drop`

Interpretation:

#### `strong_match`

- satisfies all core `must_match` constraints
- title/abstract/TLDR clearly support the paper being in scope

#### `weak_match`

- matches the core topic, but has one mild mismatch or incompleteness
- examples:
  - year ambiguity
  - broader organism/object scope than requested
  - review article when the user probably wants primary studies

#### `needs_review`

- title or abstract suggests relevance, but available metadata is insufficient for stable judgment
- classifier uncertainty should land here, not in `drop`

#### `drop`

- clearly fails the core `must_match` constraints
- examples:
  - not plant-related
  - not single-cell / single-nucleus transcriptomics
  - only generic omics language with no real single-cell connection

## Goal-Aware Result Policy

### Discovery mode

Keep all non-dropped candidates:

- `strong_match`
- `weak_match`
- `needs_review`

Only `drop` is removed.

This guarantees that a discovery query with recalled candidates cannot end as "0 hits" unless every candidate is clearly out of scope.

### Evidence mode

Evidence mode remains stricter:

- `strong_match` is usable as supporting evidence
- `weak_match` and `needs_review` may still be surfaced as non-supporting or manual-review candidates, but must not be treated as supporting citations
- `drop` is removed

For the first iteration, final answer synthesis and citation generation should only treat `strong_match` as support in evidence mode.

## Date Semantics

### Why this matters

PubMed frequently contains papers with:

- `epubdate` in one year
- `issue` / journal publication year in the next year

For example:

- `2025 online / 2026 issue`

Treating only one year field as authoritative causes recall and ranking confusion.

### Required behavior

For external results, store and expose:

- `online_year`
- `issue_year`
- display label such as:
  - `2025 online / 2026 issue`
  - `2025 issue`
  - `2025 online`

### Matching policy

If the user asks for `2025`, a candidate is year-matching if either:

- `online_year == 2025`, or
- `issue_year == 2025`

Candidates that only partially match the year signal can still be `weak_match`, but they should not be silently discarded in discovery mode.

## Data Model Changes

### Planner

Extend `ExternalSearchPlan` with:

- `SearchGoal string`
- `MustMatch []string`
- `SoftPreferences []string`
- `TargetYear string`

### External classification

Replace the current binary `ExternalPaperClassificationResult` shape with a tiered shape, for example:

```json
{
  "tier": "strong_match",
  "reason": "Matches plant + single-cell constraints and online year 2025.",
  "matched_constraints": ["plant", "single-cell transcriptomics", "2025-online"],
  "matched_preferences": ["epidermis"],
  "article_role": "research",
  "annotations": []
}
```

Suggested fields:

- `Tier`
- `Reason`
- `MatchedConstraints`
- `MatchedPreferences`
- `ArticleRole`
- `Annotations`

### External paper metadata

Extend external paper structures so the tool can preserve year details and richer article metadata where available:

- `OnlineYear`
- `IssueYear`
- `YearLabel`
- optional article-type hints if available from upstream metadata

This requires extending PubMed normalization beyond the current single `Year` field.

## Tool Execution Changes

### `ExternalSearchTool`

The main tool changes are:

1. planner produces structured search semantics
2. classifier returns tiers instead of `relevant=true|false`
3. filtering depends on `search_goal`
4. process summary reports tier counts instead of only `hits`

### Process summary

Replace the misleading single `hits` summary with richer counts:

- `returned`
- `strong_match`
- `weak_match`
- `needs_review`
- `dropped`

Example summary for discovery:

- `PubMed returned 8 candidates; 3 strong, 2 weak, 3 need review`

If `strong_match == 0` but other tiers exist, the answer must say that explicitly instead of claiming no results.

## Card and Answer Output

### Card payload

Extend `external_paper` cards with:

- `search_goal`
- `tier`
- `match_reasons`
- `soft_hits`
- `year_labels`
- `article_role`

### Display behavior

Sort and group cards by tier:

1. `强相关`
2. `弱相关`
3. `待核查`

Within each group, sort by:

1. year match quality
2. primary research over review when otherwise tied
3. coverage of `must_match`

### Answer wording

Discovery responses should use wording like:

- `找到 8 篇候选，其中 3 篇强相关、2 篇弱相关、3 篇待核查。`

If there are no strong matches:

- `没有强相关命中，但找到了 5 篇候选，已按弱相关和待核查列出。`

The answer must not say "没有找到" if non-dropped candidates exist.

## Failure Handling

### Planner failure

If planner fails:

- fallback to `search_goal = discovery`
- derive minimal `must_match` conservatively from source/context structure and baseline recall terms
- never introduce language-specific keyword routing as the fallback mechanism

### Classifier failure

Classifier failure should degrade differently by goal:

- `discovery`:
  - fallback to heuristic tiering where possible
  - otherwise preserve ambiguous candidates as `needs_review`
- `evidence`:
  - fallback to heuristics if a strong heuristic applies
  - otherwise do not mark unsupported papers as supporting evidence
  - ambiguous candidates may still be surfaced as manual-review items, but not as supporting citations

### Upstream source failure

Source failure behavior remains unchanged:

- one source failing must not block the other
- disabled-source handling remains as currently designed

## Testing Strategy

Add focused tests around the new semantics.

### Planner tests

- multilingual discovery query -> `search_goal = discovery`
- evidence-style claim-check query -> `search_goal = evidence`
- broad discovery query keeps only core requirements in `must_match`
- expansion terms land in `soft_preferences` or recall queries, not in hard filters

### Tool tests

- discovery query with recalled candidates returns non-zero tiered cards even when no candidate would satisfy the old strict evidence filter
- evidence query only uses `strong_match` papers as supporting citations
- discovery query with zero strong matches but non-zero weak/review matches does not produce "no results"

### Year tests

- `2025 online / 2026 issue` matches a `2025` query
- `2026 online / 2025 issue` also matches
- year labels are preserved in card payload

### Classifier tests

- generic query such as "find plant single-cell papers" must not require incidental terms such as `root`, `epidermis`, or `development`
- clear out-of-domain candidates become `drop`
- uncertain candidates become `needs_review`, not `drop`

### Regression tests

Add a regression around the validated example:

- query equivalent to `@PubMed 查找2025年的单细胞植物文章`
- raw candidates exist
- final discovery result contains tiered cards rather than zero hits

## Scope and Rollout

This is intentionally scoped as a behavior fix inside the existing external-search pipeline. It does not require:

- a new intent
- a new tool family
- a database migration

Recommended implementation order:

1. extend planner/result contracts
2. add year metadata plumbing
3. convert classifier to tier output
4. update `ExternalSearchTool` filtering and summaries
5. expose card fields to the frontend
6. add regression tests

## Open Questions Resolved By This Design

- **Should discovery and evidence become separate tools?**
  - No. Keep one tool for now; split semantics internally.
- **Should mode detection rely on language-specific text matching?**
  - No. Use planner intent plus structural fallback.
- **Should query expansion terms become hard requirements?**
  - No. Only planner-declared `must_match` constraints are hard filters.
- **Should discovery hide weak or ambiguous candidates?**
  - No. Keep them and label them explicitly.
- **How should 2025-style date queries behave?**
  - Match either online year or issue year, and display both clearly.
