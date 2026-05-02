(function (root, factory) {
    const api = factory();
    if (typeof module === 'object' && module.exports) {
        module.exports = api;
    }
    root.CiteBoxSettingsAnchor = api;
}(typeof globalThis !== 'undefined' ? globalThis : this, function () {
    'use strict';

    function normalizeHash(hash) {
        return String(hash || '').trim().replace(/^#/, '');
    }

    function resolveSettingsNavigation(hash, options) {
        const raw = normalizeHash(hash);
        const defaultCategoryId = String(options?.defaultCategoryId || '');
        const categoryIds = Array.isArray(options?.categoryIds) ? options.categoryIds : [];
        const sectionCategoryById = options?.sectionCategoryById || {};
        const legacySectionAliasByHash = options?.legacySectionAliasByHash || {};

        if (!raw) {
            return { categoryId: defaultCategoryId, sectionId: '' };
        }

        if (raw.startsWith('category-')) {
            const categoryId = raw.slice('category-'.length);
            return {
                categoryId: categoryIds.includes(categoryId) ? categoryId : defaultCategoryId,
                sectionId: '',
            };
        }

        const sectionId = legacySectionAliasByHash[raw] || raw;
        const categoryId = sectionCategoryById[sectionId];
        if (categoryId) {
            return { categoryId, sectionId };
        }

        if (categoryIds.includes(sectionId)) {
            return { categoryId: sectionId, sectionId: '' };
        }

        return { categoryId: defaultCategoryId, sectionId: '' };
    }

    return {
        resolveSettingsNavigation,
    };
}));
