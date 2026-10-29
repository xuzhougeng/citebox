if (typeof window.t !== 'function') window.t = function(k,f){return f||k};
const DashboardPage = {
    RECENT_LIMIT: 6,
    MAX_LISTED_AUTHORS: 3,
    state: {
        recentPapers: [],
        stats: {
            totalPapers: 0,
            processingPapers: 0,
            failedPapers: 0,
            totalFigures: 0,
            notedFigures: 0
        }
    },

    async init() {
        this.autoRefreshTimer = null;
        this.cacheElements();
        this.bindEvents();
        this.loadExtractionHint();
        await this.loadData();
    },

    cacheElements() {
        this.summaryStrip = document.getElementById('dashboardSummaryStrip');
        this.recentPaperList = document.getElementById('dashboardRecentPaperList');
        this.extractionHint = document.getElementById('dashboardExtractionHint');
    },

    bindEvents() {
        if (!this.recentPaperList) return;

        this.recentPaperList.addEventListener('click', async (event) => {
            const action = event.target.closest('[data-action]');
            if (!action) return;

            const row = event.target.closest('[data-paper-id]');
            if (!row) return;

            const paperId = Number(row.dataset.paperId);
            if (action.dataset.action === 'open') {
                await PaperViewer.open(paperId, async () => {
                    await this.loadData();
                });
            }
            if (action.dataset.action === 'reextract') {
                await this.reextractPaper(paperId);
            }
        });

        window.addEventListener('beforeunload', () => this.stopAutoRefresh());
    },

    async loadData(options = {}) {
        try {
            const [
                recentPayload,
                allPayload,
                queuedPayload,
                runningPayload,
                failedPayload,
                cancelledPayload,
                figuresPayload,
                notesPayload
            ] = await Promise.all([
                API.listPapers({ page: 1, page_size: this.RECENT_LIMIT, sort_by: 'updated_at' }),
                API.listPapers({ page: 1, page_size: 1 }),
                API.listPapers({ page: 1, page_size: 1, status: 'queued' }),
                API.listPapers({ page: 1, page_size: 1, status: 'running' }),
                API.listPapers({ page: 1, page_size: 1, status: 'failed' }),
                API.listPapers({ page: 1, page_size: 1, status: 'cancelled' }),
                API.listFigures({ page: 1, page_size: 1 }),
                API.listFigures({ page: 1, page_size: 1, has_notes: true })
            ]);

            this.state.recentPapers = recentPayload.papers || [];
            this.state.stats = {
                totalPapers: allPayload.total || 0,
                processingPapers: (queuedPayload.total || 0) + (runningPayload.total || 0),
                failedPapers: (failedPayload.total || 0) + (cancelledPayload.total || 0),
                totalFigures: figuresPayload.total || 0,
                notedFigures: notesPayload.total || 0
            };

            this.renderSummary();
            this.renderRecentPapers();
            this.syncAutoRefresh();
        } catch (error) {
            if (!options.silent) {
                Utils.showToast(error.message, 'error');
            }
        }
    },

    // Shown once for the page instead of repeating the per-paper notice.
    async loadExtractionHint() {
        if (!this.extractionHint || typeof API.getExtractorSettings !== 'function') return;
        try {
            const settings = await API.getExtractorSettings();
            const profile = String(settings?.extractor_profile || '').trim();
            const missing = profile !== 'manual' && profile !== 'open_source_vision' && !settings?.effective_extractor_url;
            this.extractionHint.hidden = !missing;
            this.extractionHint.innerHTML = missing ? `
                <span>${t('index.extraction_hint', '未配置自动解析服务：新文献会直接入库，图片可通过手动标注补录。')}</span>
                <a href="/settings#settings-section-12">${t('index.extraction_hint_link', '前往配置')}</a>
            ` : '';
        } catch (error) {
            this.extractionHint.hidden = true;
        }
    },

    renderSummary() {
        if (!this.summaryStrip) return;

        const stats = this.state.stats;
        const items = [
            { label: t('index.stat_total_papers', '文献总数'), value: stats.totalPapers, href: '/library' },
            stats.processingPapers > 0 ? { label: t('index.stat_processing', '处理中'), value: stats.processingPapers, tone: 'info' } : null,
            stats.failedPapers > 0 ? { label: t('index.stat_failed', '解析异常'), value: stats.failedPapers, tone: 'error' } : null,
            { label: t('index.stat_total_figures', '图片总数'), value: stats.totalFigures, href: '/figures' },
            { label: t('index.stat_noted_figures', '已写笔记图片'), value: stats.notedFigures, href: '/notes' }
        ].filter(Boolean);

        this.summaryStrip.innerHTML = items.map((item) => {
            const content = `<span class="dashboard-stat-label">${item.label}</span><strong>${item.value}</strong>`;
            const tone = item.tone ? ` tone-${item.tone}` : '';
            return item.href
                ? `<a class="dashboard-stat${tone}" href="${item.href}">${content}</a>`
                : `<span class="dashboard-stat${tone}">${content}</span>`;
        }).join('');
    },

    formatAuthors(value) {
        const raw = String(value || '').trim();
        if (!raw) return '';
        const semicolons = /[;；]/.test(raw);
        const separator = semicolons ? '; ' : ', ';
        const names = (semicolons ? raw.split(/[;；]/) : raw.split(/[,，]/))
            .map((name) => name.trim())
            .filter(Boolean);
        if (names.length <= this.MAX_LISTED_AUTHORS) return names.join(separator);
        return t('index.authors_et_al', '{names} 等').replace('{names}', names.slice(0, this.MAX_LISTED_AUTHORS).join(separator));
    },

    publishedYear(value) {
        const raw = String(value || '').trim();
        const match = raw.match(/\b(\d{4})\b/);
        return match ? match[1] : raw;
    },

    renderRecentPapers() {
        if (!this.recentPaperList) return;

        if (!this.state.recentPapers.length) {
            this.recentPaperList.innerHTML = `
                <div class="empty-state">
                    <h3>${t('index.empty_title', '还没有文献')}</h3>
                    <p>${t('index.empty_desc', '先上传 PDF，系统会在这里展示最近更新的文献。')}</p>
                    <a class="btn btn-primary" href="/upload">${t('index.upload_paper', '上传文献')}</a>
                </div>
            `;
            return;
        }

        const items = this.state.recentPapers.map((paper) => {
            const status = paper.extraction_status;
            const showStatus = status && status !== 'completed';
            const failed = status === 'failed' || status === 'cancelled';
            const meta = [this.formatAuthors(paper.authors_text), this.publishedYear(paper.published_at)]
                .filter(Boolean)
                .map((part) => Utils.escapeHTML(part))
                .join('<span aria-hidden="true"> · </span>');

            return `
                <li class="dashboard-recent-item" data-paper-id="${paper.id}">
                    <div class="dashboard-recent-main">
                        <button type="button" class="paper-title-link" data-action="open" title="${t('index.click_to_view', '点击查看详情')}">${Utils.escapeHTML(paper.title)}</button>
                        ${meta ? `<span class="dashboard-recent-meta">${meta}</span>` : ''}
                        ${failed && paper.extractor_message ? `<span class="dashboard-recent-message">${Utils.escapeHTML(paper.extractor_message)}</span>` : ''}
                    </div>
                    <div class="dashboard-recent-side">
                        ${showStatus ? `<span class="status-pill ${Utils.statusTone(status)}">${Utils.escapeHTML(Utils.statusLabel(status))}</span>` : ''}
                        ${failed ? `<button class="btn btn-outline btn-small" type="button" data-action="reextract">${t('index.reextract', '重新解析')}</button>` : ''}
                    </div>
                </li>
            `;
        }).join('');
        this.recentPaperList.innerHTML = `<ul class="dashboard-recent-items">${items}</ul>`;
    },

    syncAutoRefresh() {
        if (this.state.stats.processingPapers > 0 && !this.autoRefreshTimer) {
            this.autoRefreshTimer = window.setInterval(() => {
                this.loadData({ silent: true });
            }, 5000);
            return;
        }

        if (this.state.stats.processingPapers === 0) {
            this.stopAutoRefresh();
        }
    },

    stopAutoRefresh() {
        if (this.autoRefreshTimer) {
            window.clearInterval(this.autoRefreshTimer);
            this.autoRefreshTimer = null;
        }
    },

    async reextractPaper(id) {
        try {
            await API.reextractPaper(id);
            Utils.showToast(t('index.reextract_submitted', '文献已重新提交解析'), 'info');
            await this.loadData();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    }
};
