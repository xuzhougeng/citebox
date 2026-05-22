if (typeof window.t !== 'function') window.t = function(k,f){return f||k};

const HighlightLibraryPage = {
    nextLoadRequestId: 0,

    state: {
        query: '',
        sort: 'updated_desc',
        page: 1,
        pageSize: 50,
        annotations: [],
        pagination: { page: 1, page_size: 50, total: 0, total_pages: 0 },
        loading: false,
    },

    init() {
        this.queryInput = document.getElementById('highlightQuery');
        this.sortInput = document.getElementById('highlightSort');
        this.resultMeta = document.getElementById('highlightResultMeta');
        this.list = document.getElementById('highlightList');
        this.pagination = document.getElementById('highlightPagination');
        if (!this.list) return;

        this.queryInput?.addEventListener('input', Utils.debounce ? Utils.debounce(() => {
            this.state.query = this.queryInput.value.trim();
            this.state.page = 1;
            this.load();
        }, 250) : () => {
            this.state.query = this.queryInput.value.trim();
            this.state.page = 1;
            this.load();
        });
        this.sortInput?.addEventListener('change', () => {
            this.state.sort = this.sortInput.value || 'updated_desc';
            this.state.page = 1;
            this.load();
        });
        this.list.addEventListener('click', async (event) => {
            const deleteButton = event.target.closest('[data-highlight-action="delete"]');
            if (deleteButton) {
                event.preventDefault();
                await this.deleteHighlight(deleteButton.dataset.paperId, deleteButton.dataset.annotationId);
                return;
            }
            const openButton = event.target.closest('[data-highlight-action="open"]');
            if (openButton) {
                event.preventDefault();
                this.openHighlight(openButton.dataset.annotationId);
                return;
            }
            const card = event.target.closest('[data-highlight-id]');
            if (card) {
                this.openHighlight(card.dataset.highlightId);
            }
        });
        Utils.bindPagination?.(this.pagination, async (page) => {
            this.state.page = page;
            await this.load();
        });
        this.load();
    },

    async load(options = {}) {
        if (typeof API === 'undefined' || typeof API.listPDFAnnotationsGlobal !== 'function') return;
        const requestId = this.nextLoadRequestId + 1;
        this.nextLoadRequestId = requestId;
        const requestedPage = this.state.page;
        this.state.loading = true;
        try {
            const payload = await API.listPDFAnnotationsGlobal({
                query: this.state.query,
                sort: this.state.sort,
                page: requestedPage,
                page_size: this.state.pageSize,
            });

            if (requestId !== this.nextLoadRequestId) return;

            const annotations = Array.isArray(payload.annotations) ? payload.annotations : [];
            const pagination = payload.pagination || { page: 1, page_size: this.state.pageSize, total: 0, total_pages: 0 };
            const totalPages = Number(pagination.total_pages || 0);
            if (!options.clampedReload && annotations.length === 0 && totalPages > 0 && requestedPage > totalPages) {
                this.state.page = totalPages;
                await this.load({ clampedReload: true });
                return;
            }

            this.state.annotations = annotations;
            this.state.pagination = pagination;
            this.state.page = Number(this.state.pagination.page) || this.state.page;
            this.render();
        } catch (error) {
            if (requestId !== this.nextLoadRequestId) return;
            Utils.showToast?.(t('highlights.load_failed', '高亮库加载失败'), 'error');
            this.render();
        } finally {
            if (requestId === this.nextLoadRequestId) {
                this.state.loading = false;
            }
        }
    },

    render() {
        const total = Number(this.state.pagination?.total || 0);
        if (this.resultMeta) {
            this.resultMeta.textContent = t('highlights.result_meta', '共 {count} 条高亮').replace('{count}', total);
        }
        if (!this.list) return;
        if (!this.state.annotations.length) {
            const hasQuery = Boolean(this.state.query);
            this.list.innerHTML = `<div class="empty-state"><h3>${Utils.escapeHTML(t(hasQuery ? 'highlights.no_results_title' : 'highlights.empty_title', hasQuery ? '没有匹配的高亮' : '还没有 PDF 高亮'))}</h3><p>${Utils.escapeHTML(t(hasQuery ? 'highlights.no_results_text' : 'highlights.empty_text', hasQuery ? '换一个关键词，或清空搜索条件后再试。' : '打开文献 PDF，划选文本后点击高亮，这里会集中展示。'))}</p></div>`;
        } else {
            this.list.innerHTML = this.state.annotations.map((item) => this.renderHighlight(item)).join('');
        }
        Utils.renderPagination?.(this.pagination, this.state.page, Number(this.state.pagination?.total_pages || 0));
    },

    renderHighlight(item) {
        const id = String(item.id || '');
        const paperId = String(item.paper_id || '');
        const pageLabel = this.pageLabel(item);
        const dateText = Utils.formatDate(item.updated_at || item.created_at);
        return `
            <article class="highlight-card" data-highlight-id="${Utils.escapeHTML(id)}">
                <div class="highlight-card-main">
                    <p class="highlight-quote">${Utils.escapeHTML(item.quote_text || '')}</p>
                    <div class="highlight-meta">
                        <span class="highlight-paper-title">${Utils.escapeHTML(item.paper_title || item.paper_original_filename || '')}</span>
                        <span>${Utils.escapeHTML(pageLabel)}</span>
                        <span>${Utils.escapeHTML(dateText)}</span>
                    </div>
                </div>
                <div class="highlight-card-actions">
                    <button class="btn btn-outline" type="button" data-highlight-action="open" data-annotation-id="${Utils.escapeHTML(id)}">${Utils.escapeHTML(t('highlights.open_pdf', '打开 PDF'))}</button>
                    <button class="btn btn-outline" type="button" data-highlight-action="delete" data-paper-id="${Utils.escapeHTML(paperId)}" data-annotation-id="${Utils.escapeHTML(id)}">${Utils.escapeHTML(t('highlights.delete', '删除'))}</button>
                </div>
            </article>
        `;
    },

    pageLabel(item) {
        const start = Math.max(1, Math.floor(Number(item.page_start) || 1));
        const end = Math.max(start, Math.floor(Number(item.page_end) || start));
        if (start === end) {
            return t('highlights.page_label', '第 {page} 页').replace('{page}', start);
        }
        return t('highlights.page_range_label', '第 {start}-{end} 页')
            .replace('{start}', start)
            .replace('{end}', end);
    },

    findHighlight(annotationId) {
        const id = String(annotationId || '');
        return this.state.annotations.find((item) => String(item.id || '') === id) || null;
    },

    openHighlight(annotationId) {
        const item = this.findHighlight(annotationId);
        if (!item) return;
        if (!item.paper_pdf_url) {
            Utils.showToast?.(t('shared.paper.no_pdf_url', '当前文献缺少 PDF 文件地址'), 'error');
            return;
        }
        window.location.href = Utils.resourceViewerURL('pdf', item.paper_pdf_url, window.location.href, {
            paperId: item.paper_id,
            annotationId: item.id,
            page: item.page_start,
        });
    },

    async deleteHighlight(paperId, annotationId) {
        if (!paperId || !annotationId || typeof API === 'undefined') return;
        const confirmed = await Utils.confirm(t('highlights.delete_confirm', '删除这条高亮？'));
        if (!confirmed) return;
        try {
            await API.deletePDFAnnotation(paperId, annotationId);
            Utils.showToast?.(t('highlights.deleted', '高亮已删除'));
            await this.load();
        } catch (error) {
            Utils.showToast?.(t('highlights.delete_failed', '高亮删除失败'), 'error');
        }
    },
};

document.addEventListener('DOMContentLoaded', () => {
    if (window.CiteBoxI18n && typeof CiteBoxI18n.init === 'function') {
        CiteBoxI18n.init().then(() => HighlightLibraryPage.init());
    } else {
        HighlightLibraryPage.init();
    }
});
