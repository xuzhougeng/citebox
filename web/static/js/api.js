if (typeof window.t !== 'function') window.t = function(k,f){return f||k};
const API_BASE = '/api';

function clearLegacyAuthState() {
    sessionStorage.removeItem('citebox_auth');
    localStorage.removeItem('citebox_auth');
    localStorage.removeItem('citebox_logged_in');
    localStorage.removeItem('citebox_username');
    localStorage.removeItem('citebox_password');
}

function handleUnauthenticatedResponse() {
    clearLegacyAuthState();
    if (window.location.pathname !== '/login' && window.location.pathname !== '/login.html') {
        window.location.href = '/login';
    }
}

async function parseJSONResponse(response) {
    let payload = {};

    try {
        payload = await response.json();
    } catch (error) {
        payload = {};
    }

    return payload;
}

async function requestJSON(path, options = {}) {
    const response = await fetch(path, {
        credentials: 'same-origin',
        ...options
    });
    const payload = await parseJSONResponse(response);

    if (response.status === 401) {
        handleUnauthenticatedResponse();
    }

    if (!response.ok) {
        const error = new Error(payload.error || `${t('shared.api.request_failed', '请求失败')} (${response.status})`);
        error.code = payload.code || '';
        error.status = response.status;
        error.payload = payload;
        throw error;
    }

    return payload;
}

function parseContentDispositionFilename(headerValue = '') {
    const value = String(headerValue || '');
    if (!value) return '';

    const encodedMatch = value.match(/filename\*\s*=\s*UTF-8''([^;]+)/i);
    if (encodedMatch?.[1]) {
        try {
            return decodeURIComponent(encodedMatch[1].trim());
        } catch (error) {
            return encodedMatch[1].trim();
        }
    }

    const quotedMatch = value.match(/filename\s*=\s*"([^"]+)"/i);
    if (quotedMatch?.[1]) {
        return quotedMatch[1].trim();
    }

    const plainMatch = value.match(/filename\s*=\s*([^;]+)/i);
    return plainMatch?.[1]?.trim() || '';
}

async function requestBlob(path, options = {}) {
    const response = await fetch(path, {
        credentials: 'same-origin',
        ...options
    });

    if (response.status === 401) {
        handleUnauthenticatedResponse();
    }

    if (!response.ok) {
        const payload = await parseJSONResponse(response);
        const error = new Error(payload.error || `${t('shared.api.request_failed', '请求失败')} (${response.status})`);
        error.code = payload.code || '';
        error.status = response.status;
        error.payload = payload;
        throw error;
    }

    return {
        blob: await response.blob(),
        filename: parseContentDispositionFilename(response.headers.get('Content-Disposition'))
    };
}

const API = {
    listPapers(params = {}) {
        const query = new URLSearchParams();
        Object.entries(params).forEach(([key, value]) => {
            if (value !== undefined && value !== null && value !== '') {
                query.set(key, value);
            }
        });
        const suffix = query.toString() ? `?${query.toString()}` : '';
        return requestJSON(`${API_BASE}/papers${suffix}`);
    },

    getPaper(id) {
        return requestJSON(`${API_BASE}/papers/${id}`);
    },

    uploadPaper(formData) {
        return requestJSON(`${API_BASE}/papers`, {
            method: 'POST',
            body: formData
        });
    },

    updatePaper(id, data) {
        return requestJSON(`${API_BASE}/papers/${id}`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    updatePaperPDFText(id, data) {
        return requestJSON(`${API_BASE}/papers/${id}/pdf-text`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    deletePaper(id) {
        return requestJSON(`${API_BASE}/papers/${id}`, {
            method: 'DELETE'
        });
    },

    reextractPaper(id) {
        return requestJSON(`${API_BASE}/papers/${id}/reextract`, {
            method: 'POST'
        });
    },

    getManualExtractionWorkspace(id) {
        return requestJSON(`${API_BASE}/papers/${id}/manual-extraction`);
    },

    manualPreviewURL(id, page) {
        return `${API_BASE}/papers/${id}/manual-preview?page=${encodeURIComponent(page)}`;
    },

    manualExtractFigures(id, data) {
        return requestJSON(`${API_BASE}/papers/${id}/manual-extraction`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    purgeLibrary() {
        return requestJSON(`${API_BASE}/papers/purge`, {
            method: 'POST'
        });
    },

    listFigures(params = {}) {
        const query = new URLSearchParams();
        Object.entries(params).forEach(([key, value]) => {
            if (value !== undefined && value !== null && value !== '') {
                query.set(key, value);
            }
        });
        const suffix = query.toString() ? `?${query.toString()}` : '';
        return requestJSON(`${API_BASE}/figures${suffix}`);
    },

    deleteFigure(id) {
        return requestJSON(`${API_BASE}/figures/${id}`, {
            method: 'DELETE'
        });
    },

    updateFigure(id, data) {
        return requestJSON(`${API_BASE}/figures/${id}`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    createSubfigures(id, data) {
        return requestJSON(`${API_BASE}/figures/${id}/subfigures`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    createFigurePalette(id, data) {
        return requestJSON(`${API_BASE}/figures/${id}/palette`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    listPalettes(params = {}) {
        const query = new URLSearchParams();
        Object.entries(params).forEach(([key, value]) => {
            if (value !== undefined && value !== null && value !== '') {
                query.set(key, value);
            }
        });
        const suffix = query.toString() ? `?${query.toString()}` : '';
        return requestJSON(`${API_BASE}/palettes${suffix}`);
    },

    deletePalette(id) {
        return requestJSON(`${API_BASE}/palettes/${id}`, {
            method: 'DELETE'
        });
    },

    listGroups() {
        return requestJSON(`${API_BASE}/groups`);
    },

    createGroup(data) {
        return requestJSON(`${API_BASE}/groups`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    updateGroup(id, data) {
        return requestJSON(`${API_BASE}/groups/${id}`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    deleteGroup(id) {
        return requestJSON(`${API_BASE}/groups/${id}`, {
            method: 'DELETE'
        });
    },

    listTags(params = {}) {
        const query = new URLSearchParams();
        Object.entries(params).forEach(([key, value]) => {
            if (value !== undefined && value !== null && value !== '') {
                query.set(key, value);
            }
        });
        const suffix = query.toString() ? `?${query.toString()}` : '';
        return requestJSON(`${API_BASE}/tags${suffix}`);
    },

    createTag(data) {
        return requestJSON(`${API_BASE}/tags`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    updateTag(id, data) {
        return requestJSON(`${API_BASE}/tags/${id}`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    deleteTag(id) {
        return requestJSON(`${API_BASE}/tags/${id}`, {
            method: 'DELETE'
        });
    },

    getAISettings() {
        return requestJSON(`${API_BASE}/ai/settings`);
    },

    getDefaultAISettings() {
        return requestJSON(`${API_BASE}/ai/settings/defaults`);
    },

    updateAISettings(data) {
        return requestJSON(`${API_BASE}/ai/settings`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    updateAIModelSettings(data) {
        return requestJSON(`${API_BASE}/ai/settings/models`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    updateAIPromptSettings(data) {
        return requestJSON(`${API_BASE}/ai/settings/prompts`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    getAIRolePrompts() {
        return requestJSON(`${API_BASE}/ai/role-prompts`);
    },

    updateAIRolePrompts(data) {
        return requestJSON(`${API_BASE}/ai/role-prompts`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    checkAIModel(data) {
        return requestJSON(`${API_BASE}/ai/settings/check-model`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    readPaperWithAI(data) {
        return requestJSON(`${API_BASE}/ai/read`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    translateWithAI(data) {
        return requestJSON(`${API_BASE}/ai/translate`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    detectAIFigureRegions(data) {
        return requestJSON(`${API_BASE}/ai/detect-figure-regions`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    exportAIReadMarkdown(data) {
        return requestBlob(`${API_BASE}/ai/read/export`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    async readPaperWithAIStream(data, options = {}) {
        const response = await fetch(`${API_BASE}/ai/read/stream`, {
            method: 'POST',
            credentials: 'same-origin',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data),
            signal: options.signal
        });

        if (!response.ok) {
            const payload = await parseJSONResponse(response);
            if (response.status === 401) {
                handleUnauthenticatedResponse();
            }
            throw new Error(payload.error || `${t('shared.api.request_failed', '请求失败')} (${response.status})`);
        }

        if (!response.body) {
            throw new Error(t('shared.api.stream_unsupported', '当前浏览器不支持流式响应'));
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';
        const emitEvent = (raw, sourceLabel) => {
            let parsed;
            try {
                parsed = JSON.parse(raw);
            } catch (e) {
                console.warn(`SSE JSON parse error (${sourceLabel}):`, e, 'payload:', raw);
                return;
            }
            options.onEvent?.(parsed);
        };

        while (true) {
            const { done, value } = await reader.read();
            if (done) break;

            buffer += decoder.decode(value, { stream: true });
            let newlineIndex = buffer.indexOf('\n');
            while (newlineIndex >= 0) {
                const line = buffer.slice(0, newlineIndex).trim();
                buffer = buffer.slice(newlineIndex + 1);
                if (line) {
                    emitEvent(line, 'line');
                }
                newlineIndex = buffer.indexOf('\n');
            }
        }

        const tail = (buffer + decoder.decode()).trim();
        if (tail) {
            emitEvent(tail, 'tail');
        }
    },

    getExtractorSettings() {
        return requestJSON(`${API_BASE}/settings/extractor`);
    },

    updateExtractorSettings(data) {
        return requestJSON(`${API_BASE}/settings/extractor`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    getWolaiSettings() {
        return requestJSON(`${API_BASE}/settings/wolai`);
    },

    updateWolaiSettings(data) {
        return requestJSON(`${API_BASE}/settings/wolai`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    testWolaiSettings(data) {
        return requestJSON(`${API_BASE}/settings/wolai/test`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    insertWolaiTestPage(data) {
        return requestJSON(`${API_BASE}/settings/wolai/test-page`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    getWeixinBridgeSettings() {
        return requestJSON(`${API_BASE}/settings/weixin-bridge`);
    },

    updateWeixinBridgeSettings(data) {
        return requestJSON(`${API_BASE}/settings/weixin-bridge`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    getVersionStatus(forceRefresh = false) {
        const suffix = forceRefresh ? '?refresh=1' : '';
        return requestJSON(`${API_BASE}/settings/version${suffix}`);
    },

    importDatabase(formData) {
        return requestJSON(`${API_BASE}/database/import`, {
            method: 'POST',
            body: formData
        });
    },

    getAuthSettings() {
        return requestJSON(`${API_BASE}/auth/settings`);
    },

    startWeixinBinding() {
        return requestJSON(`${API_BASE}/auth/weixin/bind`, {
            method: 'POST'
        });
    },

    unbindWeixin() {
        return requestJSON(`${API_BASE}/auth/weixin/bind`, {
            method: 'DELETE'
        });
    },

    getWeixinBindingStatus(qrcode) {
        const query = new URLSearchParams({ qrcode: String(qrcode || '') });
        return requestJSON(`${API_BASE}/auth/weixin/bind/status?${query.toString()}`);
    },

    savePaperNoteToWolai(id, data) {
        return requestJSON(`${API_BASE}/wolai/papers/${id}/notes`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    saveFigureNoteToWolai(id, data) {
        return requestJSON(`${API_BASE}/wolai/figures/${id}/notes`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    changePassword(data) {
        return requestJSON(`${API_BASE}/auth/change-password`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    },

    logout() {
        return requestJSON(`${API_BASE}/auth/logout`, {
            method: 'POST'
        });
    }
};
