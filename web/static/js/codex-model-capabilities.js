(function (root, factory) {
    const api = factory();
    if (typeof module === 'object' && module.exports) {
        module.exports = api;
    } else {
        root.CiteBoxCodexModels = api;
    }
}(typeof globalThis !== 'undefined' ? globalThis : this, function () {
    const reasoningEfforts = Object.freeze(['minimal', 'low', 'medium', 'high', 'xhigh', 'max', 'ultra']);

    function supportedReasoningEfforts(model) {
        const advertised = Array.isArray(model?.supported_reasoning_efforts)
            ? model.supported_reasoning_efforts
            : [];
        return [...new Set(advertised
            .map((value) => String(value || '').trim().toLowerCase())
            .filter((value) => reasoningEfforts.includes(value)))];
    }

    function resolveCapabilities(models, modelID) {
        const normalizedID = String(modelID || '').trim();
        const model = (Array.isArray(models) ? models : []).find((item) => String(item?.id || '').trim() === normalizedID);
        if (!model) return null;

        const efforts = supportedReasoningEfforts(model);
        const advertisedDefault = String(model.default_reasoning_effort || '').trim().toLowerCase();
        const defaultReasoningEffort = efforts.includes(advertisedDefault)
            ? advertisedDefault
            : (efforts[0] || '');
        const modalities = Array.isArray(model.input_modalities)
            ? model.input_modalities.map((value) => String(value || '').trim().toLowerCase())
            : [];

        return {
            supportsImages: modalities.length === 0 || modalities.includes('image'),
            supportedReasoningEfforts: efforts,
            defaultReasoningEffort
        };
    }

    return {
        reasoningEfforts,
        resolveCapabilities
    };
}));
