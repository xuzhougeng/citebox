// ai-mention-tags.js — pure logic for the @ tool/source palette.
//
// Exposes parseToolTags(text) and commitToolTag(state, name) so they can be
// unit-tested under Node and reused inside the browser-only ai-mention.js.
//
// UMD-ish: under Node it sets module.exports; in the browser it attaches to
// window.AIReader.toolTags.

(function (root, factory) {
    if (typeof module !== 'undefined' && module.exports) {
        module.exports = factory();
    } else {
        root.AIReader = root.AIReader || {};
        root.AIReader.toolTags = factory();
    }
}(typeof window !== 'undefined' ? window : globalThis, function () {
    'use strict';

    // Canonical tag list. Order matters — the popover renders tools in this
    // order so users see the same layout regardless of session.
    const KNOWN_TOOL_TAGS = [
        { name: 'PubMed',          family: 'external', source: 'pubmed' },
        { name: 'SemanticScholar', family: 'external', source: 'semantic_scholar' },
        { name: 'Library',         family: 'library',  source: null },
        { name: 'Figure',          family: 'figure',   source: null },
    ];

    const FAMILY_INTENT = {
        external: 'external_search',
        library:  'library_search',
        figure:   'figure_lookup',
    };

    const NAME_LOOKUP = {};
    KNOWN_TOOL_TAGS.forEach((t) => { NAME_LOOKUP[t.name.toLowerCase()] = t; });

    function familyOf(name) {
        const t = NAME_LOOKUP[String(name || '').toLowerCase()];
        return t ? t.family : null;
    }

    // \b breaks at the boundary between word/non-word chars; we require either
    // start-of-string or whitespace before @ so emails ("foo@bar") never match.
    const TAG_RE = /(^|\s)@(PubMed|SemanticScholar|Library|Figure)\b/gi;

    function scanTags(text) {
        // Returns array of { name, family, source, start, end } in order of
        // appearance. Names are normalized to canonical case.
        const out = [];
        const value = String(text || '');
        TAG_RE.lastIndex = 0;
        let m;
        while ((m = TAG_RE.exec(value)) !== null) {
            const lead = m[1] || '';
            const raw = m[2];
            const t = NAME_LOOKUP[raw.toLowerCase()];
            if (!t) continue;
            const start = m.index + lead.length;            // position of '@'
            const end = start + 1 + raw.length;             // after the name
            out.push({ name: t.name, family: t.family, source: t.source, start, end });
        }
        return out;
    }

    function parseToolTags(text) {
        const tags = scanTags(text);
        if (tags.length === 0) {
            return { intentHint: '', sources: [], conflict: null };
        }

        // Cross-family last-wins. Walk in order; whenever a tag's family
        // differs from the running family, drop everything and reset.
        let keptFamily = tags[0].family;
        let kept = [tags[0]];
        let conflict = null;
        for (let i = 1; i < tags.length; i++) {
            const tag = tags[i];
            if (tag.family !== keptFamily) {
                // Last-wins: record only the most recent family transition.
                conflict = { dropped: keptFamily, kept: tag.family };
                keptFamily = tag.family;
                kept = [tag];
            } else {
                kept.push(tag);
            }
        }

        const sources = [];
        const seen = {};
        kept.forEach((tag) => {
            if (!tag.source) return;
            if (seen[tag.source]) return;
            seen[tag.source] = true;
            sources.push(tag.source);
        });

        return {
            intentHint: FAMILY_INTENT[keptFamily] || '',
            sources: sources,
            conflict: conflict,
        };
    }

    // commitToolTag is invoked when the popover commits a tool tag. It returns
    // { value, caret } and is pure — the caller writes back into the textarea.
    //
    // state shape: { value, selectionStart, atIndex, query }
    //   - atIndex: byte index of the '@' that triggered the popover
    //   - query:   the chars typed after '@' so far
    function commitToolTag(state, newTagName) {
        const value = String(state && state.value || '');
        const atIndex = Number.isFinite(state && state.atIndex) ? state.atIndex : value.length;
        const query = String(state && state.query || '');

        // Splice the typed-so-far "@<query>" out: caret currently sits right
        // after that. After splice we reinsert "@NewTag " at the same spot.
        const beforeAt = value.slice(0, atIndex);
        const afterTriggerEnd = atIndex + 1 + query.length;
        const tail = value.slice(afterTriggerEnd);

        const newTag = canonicalNameOf(newTagName);
        const newFamily = familyOf(newTag);

        // Strip *other-family* tool tags from beforeAt + tail. We must rescan
        // them on the joined string and remove only those that don't match the
        // new family. Same-family tags are preserved.
        const composed = beforeAt + tail;
        const composedTags = scanTags(composed);
        const removals = composedTags.filter((t) => t.family !== newFamily);

        let cleaned = composed;
        // Walk removals back-to-front so earlier indices stay valid.
        for (let i = removals.length - 1; i >= 0; i--) {
            const r = removals[i];
            // Trim a trailing space if the tag was followed by " " — keeps the
            // text tidy when we rip it out of the middle of the string.
            let endCut = r.end;
            if (cleaned[endCut] === ' ') endCut++;
            cleaned = cleaned.slice(0, r.start) + cleaned.slice(endCut);
        }

        // Recompute the splice point after removals: every removed tag that
        // sat *before* atIndex shifted atIndex left by its byte length.
        let adjustedAt = atIndex;
        removals.forEach((r) => {
            if (r.start < atIndex) {
                let cutLen = r.end - r.start;
                if (composed[r.end] === ' ') cutLen++;
                adjustedAt -= cutLen;
            }
        });
        if (adjustedAt < 0) adjustedAt = 0;
        if (adjustedAt > cleaned.length) adjustedAt = cleaned.length;

        const before = cleaned.slice(0, adjustedAt);
        const rest = cleaned.slice(adjustedAt);
        const insertion = '@' + newTag + ' ';
        // If the residual text already starts with a space, our insertion's trailing
        // space would create a double space. Drop one.
        const trimmedRest = rest.startsWith(' ') ? rest.slice(1) : rest;
        const finalValue = before + insertion + trimmedRest;
        return { value: finalValue, caret: before.length + insertion.length };
    }

    function canonicalNameOf(raw) {
        const t = NAME_LOOKUP[String(raw || '').toLowerCase()];
        return t ? t.name : String(raw || '');
    }

    return {
        KNOWN_TOOL_TAGS: KNOWN_TOOL_TAGS,
        familyOf: familyOf,
        parseToolTags: parseToolTags,
        commitToolTag: commitToolTag,
    };
}));
