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
