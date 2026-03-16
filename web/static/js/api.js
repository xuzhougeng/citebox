const API_BASE = '/api';

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
    const response = await fetch(path, options);
    const payload = await parseJSONResponse(response);

    if (!response.ok) {
        throw new Error(payload.error || `请求失败 (${response.status})`);
    }

    return payload;
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

    listTags() {
        return requestJSON(`${API_BASE}/tags`);
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

    updateAISettings(data) {
        return requestJSON(`${API_BASE}/ai/settings`, {
            method: 'PUT',
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

    async readPaperWithAIStream(data, options = {}) {
        const response = await fetch(`${API_BASE}/ai/read/stream`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data),
            signal: options.signal
        });

        if (!response.ok) {
            const payload = await parseJSONResponse(response);
            throw new Error(payload.error || `请求失败 (${response.status})`);
        }

        if (!response.body) {
            throw new Error('当前浏览器不支持流式响应');
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
            const { done, value } = await reader.read();
            if (done) break;

            buffer += decoder.decode(value, { stream: true });
            let newlineIndex = buffer.indexOf('\n');
            while (newlineIndex >= 0) {
                const line = buffer.slice(0, newlineIndex).trim();
                buffer = buffer.slice(newlineIndex + 1);
                if (line) {
                    options.onEvent?.(JSON.parse(line));
                }
                newlineIndex = buffer.indexOf('\n');
            }
        }

        const tail = (buffer + decoder.decode()).trim();
        if (tail) {
            options.onEvent?.(JSON.parse(tail));
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

    importDatabase(formData) {
        return requestJSON(`${API_BASE}/database/import`, {
            method: 'POST',
            body: formData
        });
    },

    getAuthSettings() {
        return requestJSON(`${API_BASE}/auth/settings`);
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
