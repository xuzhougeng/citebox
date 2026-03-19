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
        this.summaryStrip.innerHTML = `
            <div class="stat-card">
                <span>文献总数</span>
                <strong>${stats.totalPapers}</strong>
            </div>
            <div class="stat-card">
                <span>已完成</span>
                <strong>${stats.completedPapers}</strong>
            </div>
            <div class="stat-card">
                <span>处理中</span>
                <strong>${stats.processingPapers}</strong>
            </div>
            <div class="stat-card">
                <span>解析异常</span>
                <strong>${stats.failedPapers}</strong>
            </div>
            <div class="stat-card">
                <span>图片总数</span>
                <strong>${stats.totalFigures}</strong>
            </div>
            <div class="stat-card">
                <span>已写笔记图片</span>
                <strong>${stats.notedFigures}</strong>
            </div>
        `;
    },

    renderRecentPapers() {
        if (!this.recentPaperList) return;

        if (!this.state.recentPapers.length) {
            this.recentPaperList.innerHTML = `
                <div class="empty-state">
                    <h3>还没有文献</h3>
                    <p>先上传 PDF，系统会在这里展示最近更新的文献。</p>
                    <a class="btn btn-primary" href="/upload">上传文献</a>
                </div>
            `;
            return;
        }

        this.recentPaperList.innerHTML = this.state.recentPapers.map((paper) => {
            const statusClass = Utils.statusTone(paper.extraction_status);

            return `
                <article class="recent-paper-row" data-paper-id="${paper.id}">
                    <div class="recent-paper-main">
                        <div class="recent-paper-head" data-action="open" role="button" title="点击查看详情">
                            <span class="status-pill ${statusClass}">${Utils.escapeHTML(Utils.statusLabel(paper.extraction_status))}</span>
                            <h3>${Utils.escapeHTML(paper.title)}</h3>
                        </div>
                        <div class="recent-paper-meta">
                            <span>${Utils.escapeHTML(paper.group_name || '未分组')}</span>
                            <span>${paper.figure_count || 0} 张图片</span>
                            <span>${Utils.formatDate(paper.updated_at || paper.created_at)}</span>
                        </div>
                        ${paper.extractor_message ? `<p class="notice ${statusClass} recent-paper-notice">${Utils.escapeHTML(paper.extractor_message)}</p>` : ''}
                    </div>
                    <div class="card-actions recent-paper-actions">
                        <button class="btn btn-primary" type="button" data-action="open">查看详情</button>
                        <a class="btn btn-outline" href="/manual?paper_id=${paper.id}">手动标注</a>
                        ${(paper.extraction_status === 'failed' || paper.extraction_status === 'cancelled') ? '<button class="btn btn-outline" type="button" data-action="reextract">重新解析</button>' : ''}
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
            Utils.showToast('文献已重新提交解析', 'info');
            await this.loadData();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    }
};
