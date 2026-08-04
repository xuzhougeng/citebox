if (typeof window.t !== 'function') window.t = function(k,f){return f||k};
const DashboardPage = {
    state: {
        recentPapers: [],
        stats: {
            totalPapers: 0,
            completedPapers: 0,
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
        await this.loadData();
    },

    cacheElements() {
        this.summaryStrip = document.getElementById('dashboardSummaryStrip');
        this.recentPaperList = document.getElementById('dashboardRecentPaperList');
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
                completedPayload,
                queuedPayload,
                runningPayload,
                failedPayload,
                cancelledPayload,
                figuresPayload,
                notesPayload
            ] = await Promise.all([
                API.listPapers({ page: 1, page_size: 3 }),
                API.listPapers({ page: 1, page_size: 1 }),
                API.listPapers({ page: 1, page_size: 1, status: 'completed' }),
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
                completedPapers: completedPayload.total || 0,
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

    renderSummary() {
        if (!this.summaryStrip) return;

        const stats = this.state.stats;
        const icons = DashboardPage.statIcons;
        const cards = [
            { label: t('index.stat_total_papers', '文献总数'), value: stats.totalPapers, tone: 'tone-accent', icon: icons.paper },
            { label: t('index.stat_completed', '已完成'), value: stats.completedPapers, tone: 'tone-success', icon: icons.check },
            { label: t('index.stat_processing', '处理中'), value: stats.processingPapers, tone: stats.processingPapers > 0 ? 'tone-info' : '', icon: icons.clock },
            { label: t('index.stat_failed', '解析异常'), value: stats.failedPapers, tone: stats.failedPapers > 0 ? 'tone-error' : '', icon: icons.alert },
            { label: t('index.stat_total_figures', '图片总数'), value: stats.totalFigures, tone: 'tone-accent', icon: icons.image },
            { label: t('index.stat_noted_figures', '已写笔记图片'), value: stats.notedFigures, tone: 'tone-success', icon: icons.note }
        ];

        this.summaryStrip.innerHTML = cards.map((card) => `
            <div class="stat-card with-icon ${card.tone}">
                <span class="stat-card-icon" aria-hidden="true">${card.icon}</span>
                <div class="stat-card-body">
                    <span class="stat-card-label">${card.label}</span>
                    <strong>${card.value}</strong>
                </div>
            </div>
        `).join('');
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

        this.recentPaperList.innerHTML = this.state.recentPapers.map((paper) => {
            const statusClass = Utils.statusTone(paper.extraction_status);

            return `
                <article class="recent-paper-row" data-paper-id="${paper.id}">
                    <div class="recent-paper-main">
                        <div class="recent-paper-head" data-action="open" role="button" title="${t('index.click_to_view', '点击查看详情')}">
                            <span class="status-pill ${statusClass}">${Utils.escapeHTML(Utils.statusLabel(paper.extraction_status))}</span>
                            <h3>${Utils.escapeHTML(paper.title)}</h3>
                        </div>
                        <div class="recent-paper-meta">
                            <span>${Utils.escapeHTML(paper.group_name || t('index.ungrouped', '未分组'))}</span>
                            <span>${paper.figure_count || 0} ${t('index.figures_unit', '张图片')}</span>
                            <span>${Utils.formatDate(paper.updated_at || paper.created_at)}</span>
                        </div>
                        ${paper.extractor_message ? `<p class="notice ${statusClass} recent-paper-notice">${Utils.escapeHTML(paper.extractor_message)}</p>` : ''}
                    </div>
                    <div class="card-actions recent-paper-actions">
                        <button class="btn btn-primary" type="button" data-action="open">${t('index.view_detail', '查看详情')}</button>
                        <a class="btn btn-outline" href="/manual?paper_id=${paper.id}">${t('index.manual_annotate', '手动标注')}</a>
                        ${(paper.extraction_status === 'failed' || paper.extraction_status === 'cancelled') ? `<button class="btn btn-outline" type="button" data-action="reextract">${t('index.reextract', '重新解析')}</button>` : ''}
                    </div>
                </article>
            `;
        }).join('');
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

DashboardPage.statIcons = {
    paper: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M7 3h7l5 5v13H7z"/><path d="M14 3v5h5"/><path d="M10 13h5M10 17h5"/></svg>',
    check: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="8.5"/><path d="M8.5 12.2l2.4 2.4 4.6-4.8"/></svg>',
    clock: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="8.5"/><path d="M12 7.5V12l3 2"/></svg>',
    alert: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 4l9 16H3z"/><path d="M12 10v4.5"/><path d="M12 17.6v.1"/></svg>',
    image: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="5" width="16" height="14" rx="2"/><circle cx="9" cy="10" r="1.6"/><path d="M5.5 17.5l4-4 3 3 3.5-3.5 2.5 2.5"/></svg>',
    note: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M5 19l1-4L16.5 4.5a2.1 2.1 0 013 3L9 18z"/><path d="M14.5 6.5l3 3"/></svg>'
};
