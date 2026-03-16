const API_BASE = '/api';

async function requestJSON(path, options = {}) {
    const response = await fetch(path, options);
    let payload = {};

    try {
        payload = await response.json();
    } catch (error) {
        payload = {};
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
    }
};
