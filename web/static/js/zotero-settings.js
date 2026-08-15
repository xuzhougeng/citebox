const ZoteroSettings = {
    pollTimer: null,
    collections: [],
    selectedKeys: new Set(),
    currentRun: null,

    init() {
        if (this.initialized) return;
        this.initialized = true;
        this.form = document.getElementById('zoteroSettingsForm');
        if (!this.form) return;
        this.baseURLInput = document.getElementById('zoteroBaseURLInput');
        this.includeChildrenInput = document.getElementById('zoteroIncludeChildrenInput');
        this.connectionSummary = document.getElementById('zoteroConnectionSummary');
        this.connectionStatus = document.getElementById('zoteroConnectionStatus');
        this.testButton = document.getElementById('zoteroTestButton');
        this.collectionTree = document.getElementById('zoteroCollectionTree');
        this.importStatus = document.getElementById('zoteroImportStatus');
        this.loadCollectionsButton = document.getElementById('zoteroLoadCollectionsButton');
        this.previewButton = document.getElementById('zoteroPreviewButton');
        this.importButton = document.getElementById('zoteroImportButton');
        this.importSummary = document.getElementById('zoteroImportSummary');
        this.missingList = document.getElementById('zoteroMissingList');

        this.form.addEventListener('submit', async (event) => {
            event.preventDefault();
            await this.saveSettings();
        });
        this.testButton?.addEventListener('click', () => this.testConnection());
        this.loadCollectionsButton?.addEventListener('click', () => this.loadCollections());
        this.previewButton?.addEventListener('click', () => this.preview());
        this.importButton?.addEventListener('click', () => this.startImport());
        this.bootstrap();
    },

    async bootstrap() {
        try {
            const settings = await API.getZoteroSettings();
            this.applySettings(settings || {});
            if (settings?.last_run_id) {
                await this.loadRun(settings.last_run_id);
            }
        } catch (error) {
            this.setStatus(this.connectionStatus, error.message, true);
        }
    },

    applySettings(settings) {
        if (this.baseURLInput) {
            this.baseURLInput.value = settings.base_url || 'http://127.0.0.1:23119/api';
        }
        if (this.includeChildrenInput) {
            this.includeChildrenInput.checked = settings.include_children !== false;
        }
        this.selectedKeys = new Set(settings.last_collection_keys || []);
    },

    settingsPayload() {
        return {
            base_url: this.baseURLInput?.value || '',
            include_children: Boolean(this.includeChildrenInput?.checked),
            last_collection_keys: Array.from(this.selectedKeys)
        };
    },

    async saveSettings() {
        try {
            const settings = await API.updateZoteroSettings(this.settingsPayload());
            this.applySettings(settings || {});
            this.setStatus(this.connectionStatus, t('settings.zotero.saved', 'Zotero 设置已保存'));
        } catch (error) {
            this.setStatus(this.connectionStatus, error.message, true);
        }
    },

    async testConnection() {
        try {
            await this.saveSettings();
            const status = await API.getZoteroStatus();
            this.renderConnection(status);
            if (status?.reachable) {
                await this.loadCollections();
            }
        } catch (error) {
            this.setStatus(this.connectionStatus, error.message, true);
        }
    },

    renderConnection(status) {
        if (this.connectionSummary) {
            this.connectionSummary.innerHTML = `
                <div><span>${t('settings.zotero.reachable', '连接状态')}</span><strong>${status?.reachable ? t('settings.zotero.reachable_yes', '已连接') : t('settings.zotero.reachable_no', '未连接')}</strong></div>
                <div><span>${t('settings.zotero.library_prefix', '文库')}</span><strong>${status?.library_prefix || '-'}</strong></div>
                <div><span>${t('settings.zotero.collection_count', 'Collection 数')}</span><strong>${status?.collection_count ?? 0}</strong></div>
            `;
        }
        this.setStatus(this.connectionStatus, status?.message || '');
    },

    async loadCollections() {
        try {
            const payload = await API.listZoteroCollections();
            this.collections = payload.collections || [];
            this.renderCollectionTree();
            this.setStatus(this.importStatus, t('settings.zotero.collections_loaded', '已加载 Zotero collections'));
        } catch (error) {
            this.setStatus(this.importStatus, error.message, true);
        }
    },

    renderCollectionTree() {
        if (!this.collectionTree) return;
        if (!this.collections.length) {
            this.collectionTree.innerHTML = `<p class="muted">${t('settings.zotero.no_collections', '还没有 collection。请先测试连接。')}</p>`;
            return;
        }
        this.collectionTree.innerHTML = this.collections.map((node) => this.renderCollectionNode(node, 0)).join('');
        this.collectionTree.querySelectorAll('input[data-zotero-collection]').forEach((input) => {
            input.addEventListener('change', () => {
                if (input.checked) this.selectedKeys.add(input.value);
                else this.selectedKeys.delete(input.value);
            });
        });
    },

    renderCollectionNode(node, depth) {
        const checked = this.selectedKeys.has(node.key) ? 'checked' : '';
        const children = (node.children || []).map((child) => this.renderCollectionNode(child, depth + 1)).join('');
        return `
            <label class="zotero-collection-item" style="padding-left:${12 + depth * 16}px">
                <input type="checkbox" data-zotero-collection value="${this.escapeAttr(node.key)}" ${checked}>
                <span>${this.escapeHTML(node.path || node.name)}</span>
            </label>
            ${children}
        `;
    },

    selectionPayload() {
        return {
            collection_keys: Array.from(this.selectedKeys),
            include_children: Boolean(this.includeChildrenInput?.checked)
        };
    },

    async preview() {
        try {
            const run = await API.previewZoteroImport(this.selectionPayload());
            this.renderRun(run, { preview: true });
            this.setStatus(this.importStatus, t('settings.zotero.preview_done', '预览完成'));
        } catch (error) {
            this.setStatus(this.importStatus, error.message, true);
        }
    },

    async startImport() {
        try {
            const run = await API.startZoteroImport(this.selectionPayload());
            this.renderRun(run);
            this.setStatus(this.importStatus, t('settings.zotero.import_started', '已开始导入'));
            this.pollRun(run.id);
        } catch (error) {
            this.setStatus(this.importStatus, error.message, true);
        }
    },

    async loadRun(id) {
        const run = await API.getZoteroImportRun(id);
        this.renderRun(run);
        if (run.status === 'queued' || run.status === 'running') {
            this.pollRun(run.id);
        }
    },

    pollRun(id) {
        if (this.pollTimer) clearInterval(this.pollTimer);
        this.pollTimer = setInterval(async () => {
            try {
                const run = await API.getZoteroImportRun(id);
                this.renderRun(run);
                if (run.status === 'completed' || run.status === 'failed') {
                    clearInterval(this.pollTimer);
                    this.pollTimer = null;
                    this.setStatus(this.importStatus, t('settings.zotero.import_done', '导入已结束'));
                }
            } catch (error) {
                clearInterval(this.pollTimer);
                this.pollTimer = null;
                this.setStatus(this.importStatus, error.message, true);
            }
        }, 1000);
    },

    renderRun(run, options = {}) {
        this.currentRun = run;
        if (!this.importSummary) return;
        const summary = run?.summary || {};
        this.importSummary.innerHTML = `
            <div><span>${t('settings.zotero.summary_status', '任务状态')}</span><strong>${this.escapeHTML(run.status || '-')}</strong></div>
            <div><span>${t('settings.zotero.summary_total', '条目')}</span><strong>${summary.total || 0}</strong></div>
            <div><span>${t('settings.zotero.summary_imported', '已入库')}</span><strong>${summary.imported || 0}</strong></div>
            <div><span>${t('settings.zotero.summary_skipped', '已存在')}</span><strong>${summary.skipped_existing || 0}</strong></div>
            <div><span>${t('settings.zotero.summary_missing', '待补 PDF')}</span><strong>${summary.missing_pdf || 0}</strong></div>
            <div><span>${t('settings.zotero.summary_error', '失败')}</span><strong>${summary.error || 0}</strong></div>
        `;
        this.renderMissing(run, options.preview);
    },

    renderMissing(run, preview) {
        if (!this.missingList) return;
        const items = (run?.items || []).filter((item) => !item.has_local_pdf || item.status === 'missing_pdf');
        if (!items.length) {
            this.missingList.innerHTML = `<p class="muted">${t('settings.zotero.no_missing', '没有待补 PDF 的条目。')}</p>`;
            return;
        }
        this.missingList.innerHTML = items.map((item) => `
            <div class="zotero-missing-item" data-item-key="${this.escapeAttr(item.item_key)}">
                <div>
                    <strong>${this.escapeHTML(item.title || item.item_key)}</strong>
                    <p class="muted">${this.escapeHTML(item.doi || t('settings.zotero.no_doi', '无 DOI'))} · ${this.escapeHTML(item.collection_path || '')}</p>
                    <p class="muted">${this.escapeHTML(item.reason || '')}</p>
                </div>
                <div class="zotero-missing-actions">
                    <label class="btn btn-outline">
                        ${t('settings.zotero.attach_pdf', '选择本地 PDF')}
                        <input type="file" accept="application/pdf,.pdf" data-zotero-attach="${this.escapeAttr(item.item_key)}" ${preview || !run.id ? 'disabled' : ''}>
                    </label>
                    <button class="btn btn-outline" type="button" data-zotero-doi="${this.escapeAttr(item.item_key)}" ${preview || !run.id || !item.doi ? 'disabled' : ''}>
                        ${t('settings.zotero.try_doi', '尝试开放获取')}
                    </button>
                </div>
            </div>
        `).join('');
        this.missingList.querySelectorAll('input[data-zotero-attach]').forEach((input) => {
            input.addEventListener('change', async () => {
                const file = input.files && input.files[0];
                if (!file || !this.currentRun?.id) return;
                try {
                    const run = await API.attachZoteroImportPDF(this.currentRun.id, input.dataset.zoteroAttach, file);
                    this.renderRun(run);
                    Utils.showToast(t('settings.zotero.attach_done', '已按标准链路补传 PDF'), 'success');
                } catch (error) {
                    Utils.showToast(error.message, 'error');
                } finally {
                    input.value = '';
                }
            });
        });
        this.missingList.querySelectorAll('button[data-zotero-doi]').forEach((button) => {
            button.addEventListener('click', async () => {
                if (!this.currentRun?.id) return;
                button.disabled = true;
                try {
                    const run = await API.importZoteroItemByDOI(this.currentRun.id, button.dataset.zoteroDoi);
                    this.renderRun(run);
                    Utils.showToast(t('settings.zotero.doi_done', '已尝试按 DOI 入库'), 'success');
                } catch (error) {
                    Utils.showToast(error.message, 'error');
                } finally {
                    button.disabled = false;
                }
            });
        });
    },

    setStatus(el, message, isError) {
        if (!el) return;
        el.textContent = message || '';
        el.classList.toggle('error', Boolean(isError));
    },

    escapeHTML(value) {
        return String(value ?? '').replace(/[&<>"']/g, (ch) => ({
            '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
        }[ch]));
    },

    escapeAttr(value) {
        return this.escapeHTML(value);
    }
};

document.addEventListener('DOMContentLoaded', () => {
    if (document.getElementById('zoteroSettingsForm')) {
        ZoteroSettings.init();
    }
});
