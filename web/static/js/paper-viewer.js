const PaperViewer = {
    init() {
        this.modal = document.getElementById('paperModal');
        this.body = document.getElementById('paperModalBody');
        this.closeButton = document.getElementById('closePaperModal');
        if (!this.modal || this.initialized) return;
        this.initialized = true;

        this.handleKeydown = (event) => {
            if (!this.modal || this.modal.classList.contains('hidden')) return;
            if (event.key === 'Escape') {
                this.close();
            }
        };

        this.closeButton.addEventListener('click', () => this.close());
        this.modal.addEventListener('click', (event) => {
            if (event.target === this.modal) {
                this.close();
            }
        });
        document.addEventListener('keydown', this.handleKeydown);

        this.body.addEventListener('submit', async (event) => {
            const form = event.target.closest('#paperViewerForm');
            if (!form) return;
            event.preventDefault();
            await this.save();
        });

        this.body.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-modal-action]');
            if (!button) return;
            if (button.dataset.modalAction === 'reextract-paper') {
                await this.reextract();
            }
            if (button.dataset.modalAction === 'delete-paper') {
                await this.remove();
            }
            if (button.dataset.modalAction === 'view-pdf-text') {
                this.openPdfTextViewer();
            }
            if (button.dataset.modalAction === 'preview-figure') {
                await this.openFigurePreview(Number(button.dataset.figureIndex));
            }
            if (button.dataset.modalAction === 'delete-figure') {
                await this.deleteFigure(Number(button.dataset.figureId));
            }
        });
    },

    renderTagChips(tags = []) {
        if (!tags.length) {
            return '<span class="muted">无标签</span>';
        }
        return tags.map((tag) => `<span class="chip" style="--chip-color:${tag.color}">${Utils.escapeHTML(tag.name)}</span>`).join('');
    },

    async open(id, onChanged) {
        this.init();
        this.onChanged = onChanged;
        try {
            const [paper, groupsPayload] = await Promise.all([API.getPaper(id), API.listGroups()]);
            this.paper = paper;
            this.groups = groupsPayload.groups || [];
            this.render();
            this.modal.classList.remove('hidden');
            document.body.classList.add('modal-open');
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    close() {
        if (!this.modal) return;
        this.modal.classList.add('hidden');
        document.body.classList.remove('modal-open');
    },

    render() {
        const paper = this.paper;
        const groupOptions = ['<option value="">未分组</option>']
            .concat(this.groups.map((group) => `
                <option value="${group.id}" ${String(group.id) === String(paper.group_id || '') ? 'selected' : ''}>
                    ${Utils.escapeHTML(group.name)}
                </option>
            `))
            .join('');
        const figures = paper.figures || [];
        const statusClass = Utils.statusTone(paper.extraction_status);
        
        // 解析框选结果为人类可读格式
        const boxesHtml = this.renderBoxes(paper.boxes);
        const figureSection = figures.length ? figures.map((figure, index) => `
            <article class="figure-preview-card figure-detail-card">
                <div class="figure-preview-stage">
                    <button class="figure-preview-media" type="button" data-modal-action="preview-figure" data-figure-index="${index}" aria-label="查看大图">
                        <img src="${figure.image_url}" alt="${Utils.escapeHTML(figure.original_name || paper.title)}">
                    </button>
                    <div class="figure-preview-badges">
                        <span class="figure-badge figure-badge-strong">第 ${figure.page_number || '-'} 页</span>
                        <span class="figure-badge">#${figure.figure_index || '-'}</span>
                        ${figure.source === 'manual' ? '<span class="figure-badge">人工提取</span>' : ''}
                    </div>
                </div>
                <div class="figure-preview-body">
                    <div class="figure-preview-head">
                        <span class="figure-preview-label">来源文献</span>
                        <strong class="figure-preview-title">${Utils.escapeHTML(paper.title)}</strong>
                    </div>
                    <div class="figure-preview-tags ${paper.tags?.length ? '' : 'is-empty'}">
                        ${paper.tags?.length ? this.renderTagChips(paper.tags || []) : '<span class="figure-preview-empty">无标签</span>'}
                    </div>
                    <div class="card-actions">
                        <button class="btn btn-primary" type="button" data-modal-action="preview-figure" data-figure-index="${index}">查看大图</button>
                        <button class="btn btn-outline danger" type="button" data-modal-action="delete-figure" data-figure-id="${figure.id}">删除图片</button>
                        <a class="btn btn-outline" href="${figure.image_url}" target="_blank" rel="noreferrer">原图</a>
                    </div>
                </div>
            </article>
        `).join('') : `<p class="muted">${Utils.isProcessingStatus(paper.extraction_status) ? '后台解析完成后会在这里显示提取图片。' : '没有可展示的提取图片。'}</p>`;

        this.body.innerHTML = `
            <div class="detail-head">
                <div>
                    <p class="eyebrow">文献详情</p>
                    <h2>${Utils.escapeHTML(paper.title)}</h2>
                </div>
                <span class="status-pill ${statusClass}">
                    ${Utils.escapeHTML(Utils.statusLabel(paper.extraction_status))}
                </span>
            </div>

            <form id="paperViewerForm" class="detail-form">
                <div class="form-grid detail-form-grid">
                    <label class="field field-span-2">
                        <span>标题</span>
                        <input id="paperViewerTitle" class="form-input" type="text" value="${Utils.escapeHTML(paper.title)}">
                    </label>
                    <label class="field">
                        <span>分组</span>
                        <select id="paperViewerGroup" class="form-input">${groupOptions}</select>
                    </label>
                    <label class="field">
                        <span>标签</span>
                        <input id="paperViewerTags" class="form-input" type="text" value="${Utils.escapeHTML(Utils.joinTags(paper.tags || []))}" placeholder="逗号分隔">
                    </label>
                    <label class="field field-span-2">
                        <span>摘要</span>
                        <textarea id="paperViewerAbstract" class="form-textarea" rows="4" placeholder="为这篇文献补充摘要或核心结论">${Utils.escapeHTML(paper.abstract_text || '')}</textarea>
                    </label>
                    <label class="field field-span-2">
                        <span>备注</span>
                        <textarea id="paperViewerNotes" class="form-textarea" rows="5" placeholder="记录你的整理备注、阅读结论或迁移补充信息">${Utils.escapeHTML(paper.notes_text || '')}</textarea>
                    </label>
                </div>
                <div class="detail-actions">
                    <button class="btn btn-primary" type="submit">保存</button>
                    <a class="btn btn-outline" href="/manual?paper_id=${paper.id}" target="_blank" rel="noreferrer">人工处理</a>
                    ${(paper.extraction_status === 'failed' || paper.extraction_status === 'cancelled') ? '<button class="btn btn-outline" type="button" data-modal-action="reextract-paper">重新解析</button>' : ''}
                    <button class="btn btn-outline danger" type="button" data-modal-action="delete-paper">删除文献</button>
                    <a class="btn btn-outline" href="/ai?paper_id=${paper.id}" target="_blank" rel="noreferrer">AI 阅读</a>
                    <a class="btn btn-outline" href="${paper.pdf_url}" target="_blank" rel="noreferrer">打开 PDF</a>
                </div>
            </form>

            <div class="detail-meta-panel">
                <div><span>原始文件</span><strong>${Utils.escapeHTML(paper.original_filename)}</strong></div>
                <div><span>PDF 大小</span><strong>${Utils.formatFileSize(paper.file_size || 0)}</strong></div>
                <div><span>提取图片</span><strong>${figures.length}</strong></div>
                <div><span>当前标签</span><strong>${this.renderTagChips(paper.tags || [])}</strong></div>
                <div><span>最近更新</span><strong>${Utils.formatDate(paper.updated_at || paper.created_at)}</strong></div>
            </div>

            ${paper.extractor_message ? `<p class="notice ${statusClass}">${Utils.escapeHTML(paper.extractor_message)}</p>` : ''}

            <section class="detail-section">
                <div class="section-head">
                    <h3>提取图片</h3>
                    <span>${figures.length} 张</span>
                </div>
                <div class="figure-preview-grid detail-figure-grid">
                    ${figureSection}
                </div>
            </section>

            <section class="detail-section">
                <div class="section-head section-head-boxes">
                    <h3>框选结果</h3>
                    <span class="boxes-count">${boxesHtml.count} 个框选区域</span>
                </div>
                <div class="boxes-content">
                    ${boxesHtml.html}
                </div>
            </section>

            <section class="detail-section">
                <div class="section-head section-head-pdf-text">
                    <h3>PDF 原文</h3>
                    <button type="button" class="btn btn-small btn-outline" data-modal-action="view-pdf-text">查看原文</button>
                </div>
                <div class="pdf-text-preview">
                    ${paper.pdf_text ? `
                        <p class="pdf-text-snippet">${Utils.escapeHTML(paper.pdf_text.substring(0, 200))}${paper.pdf_text.length > 200 ? '...' : ''}</p>
                        <p class="pdf-text-meta">共 ${paper.pdf_text.length.toLocaleString()} 字符</p>
                    ` : '<p class="muted">暂无 PDF 原文</p>'}
                </div>
            </section>
        `;
    },

    paperFiguresForViewer() {
        const paper = this.paper;
        return (paper.figures || []).map((figure) => ({
            ...figure,
            paper_id: paper.id,
            paper_title: paper.title,
            group_id: paper.group_id,
            group_name: paper.group_name || '',
            tags: paper.tags || []
        }));
    },

    async openFigurePreview(index) {
        if (typeof FigureViewer === 'undefined') {
            window.open(this.paper?.figures?.[index]?.image_url, '_blank', 'noreferrer');
            return;
        }

        await FigureViewer.open({
            figures: this.paperFiguresForViewer(),
            index,
            page: 1,
            totalPages: 1,
            onOpenPaper: async () => {
                FigureViewer.close();
            },
            onMetaChanged: async (paper) => {
                if (!paper) return;
                this.paper = paper;
                this.render();
                if (typeof this.onChanged === 'function') {
                    await this.onChanged();
                }
            }
        });
    },

    async save() {
        try {
            const payload = await API.updatePaper(this.paper.id, {
                title: document.getElementById('paperViewerTitle').value.trim(),
                abstract_text: document.getElementById('paperViewerAbstract').value.trim(),
                notes_text: document.getElementById('paperViewerNotes').value.trim(),
                group_id: document.getElementById('paperViewerGroup').value ? Number(document.getElementById('paperViewerGroup').value) : null,
                tags: Utils.splitTags(document.getElementById('paperViewerTags').value)
            });
            this.paper = payload.paper;
            Utils.showToast('文献信息已更新');
            this.render();
            if (typeof this.onChanged === 'function') {
                await this.onChanged();
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async remove() {
        const confirmed = await Utils.confirm('删除后会移除 PDF、提取图片以及相关关联。');
        if (!confirmed) return;

        try {
            await API.deletePaper(this.paper.id);
            Utils.showToast('文献已删除');
            this.close();
            if (typeof this.onChanged === 'function') {
                await this.onChanged();
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async reextract() {
        try {
            const payload = await API.reextractPaper(this.paper.id);
            this.paper = payload.paper;
            Utils.showToast('文献已重新提交解析', 'info');
            this.render();
            if (typeof this.onChanged === 'function') {
                await this.onChanged();
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async deleteFigure(figureID) {
        const confirmed = await Utils.confirm('删除后会移除这张图片文件，但不会删除整篇文献。');
        if (!confirmed) return;

        try {
            const payload = await API.deleteFigure(figureID);
            this.paper = payload.paper;
            Utils.showToast('图片已删除');
            this.render();
            if (typeof this.onChanged === 'function') {
                await this.onChanged();
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    // 解析并渲染框选结果为人类可读格式
    renderBoxes(boxesData) {
        if (!boxesData) {
            return { html: '<p class="muted">暂无框选结果</p>', count: 0 };
        }
        
        let boxes = [];
        try {
            // boxesData 可能是字符串或已解析的对象
            boxes = typeof boxesData === 'string' ? JSON.parse(boxesData) : boxesData;
            // 处理不同格式：可能是数组直接是 boxes，或 {boxes: [...]}
            if (boxes && typeof boxes === 'object' && !Array.isArray(boxes)) {
                boxes = boxes.boxes || boxes.regions || boxes.regions || [];
            }
        } catch (e) {
            return { html: `<pre class="code-block">${Utils.escapeHTML(String(boxesData))}</pre>`, count: 0 };
        }
        
        if (!Array.isArray(boxes) || boxes.length === 0) {
            return { html: '<p class="muted">暂无框选结果</p>', count: 0 };
        }
        
        // 按页码分组统计
        const pageGroups = {};
        boxes.forEach((box, index) => {
            const page = box.page || box.page_number || box.pageNumber || '-';
            if (!pageGroups[page]) pageGroups[page] = [];
            pageGroups[page].push({ ...box, index: index + 1 });
        });
        
        const pages = Object.keys(pageGroups).sort((a, b) => {
            if (a === '-') return 1;
            if (b === '-') return -1;
            return parseInt(a) - parseInt(b);
        });
        
        let html = '<div class="boxes-timeline">';
        pages.forEach(page => {
            const pageBoxes = pageGroups[page];
            html += `
                <div class="boxes-page-group">
                    <div class="boxes-page-header">
                        <span class="boxes-page-number">第 ${page} 页</span>
                        <span class="boxes-page-count">${pageBoxes.length} 个区域</span>
                    </div>
                    <div class="boxes-list">
                        ${pageBoxes.map(box => this.renderBoxItem(box)).join('')}
                    </div>
                </div>
            `;
        });
        html += '</div>';
        
        return { html, count: boxes.length };
    },
    
    // 渲染单个框选项
    renderBoxItem(box) {
        const bbox = box.bbox || box.box || box.region || [];
        const [x1, y1, x2, y2] = Array.isArray(bbox) ? bbox : [bbox.x1, bbox.y1, bbox.x2, bbox.y2];
        const width = x1 !== undefined && x2 !== undefined ? Math.round(x2 - x1) : '-';
        const height = y1 !== undefined && y2 !== undefined ? Math.round(y2 - y1) : '-';
        
        // 提取类型/标签信息
        const type = box.type || box.label || box.category || '框选区域';
        const confidence = box.confidence ? `${(box.confidence * 100).toFixed(1)}%` : null;
        
        return `
            <div class="box-item">
                <div class="box-item-header">
                    <span class="box-item-index">#${box.index}</span>
                    <span class="box-item-type">${Utils.escapeHTML(type)}</span>
                    ${confidence ? `<span class="box-item-confidence">${confidence}</span>` : ''}
                </div>
                <div class="box-item-coords">
                    <span class="coord-item" title="左上角坐标">(${x1 !== undefined ? Math.round(x1) : '-'}, ${y1 !== undefined ? Math.round(y1) : '-'})</span>
                    <span class="coord-arrow">→</span>
                    <span class="coord-item" title="右下角坐标">(${x2 !== undefined ? Math.round(x2) : '-'}, ${y2 !== undefined ? Math.round(y2) : '-'})</span>
                    <span class="coord-size">${width}×${height}</span>
                </div>
                ${box.text ? `<div class="box-item-text">${Utils.escapeHTML(box.text.substring(0, 100))}${box.text.length > 100 ? '...' : ''}</div>` : ''}
            </div>
        `;
    },
    
    // 打开 PDF 原文查看器
    openPdfTextViewer() {
        const paper = this.paper;
        if (!paper.pdf_text) {
            Utils.showToast('暂无 PDF 原文', 'info');
            return;
        }
        
        // 创建新窗口/标签页展示原文
        const win = window.open('', '_blank');
        if (!win) {
            Utils.showToast('请允许弹窗以查看原文', 'error');
            return;
        }
        
        win.document.write(`
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>${Utils.escapeHTML(paper.title)} - PDF 原文</title>
    <style>
        :root {
            --bg: #efe6d7;
            --bg-soft: #f7f2e8;
            --ink: #241b16;
            --muted: #6e5b4e;
            --accent: #a45c40;
            --accent-deep: #6f3622;
        }
        * { box-sizing: border-box; }
        body {
            margin: 0;
            font-family: "Avenir Next", "PingFang SC", "Microsoft YaHei", sans-serif;
            color: var(--ink);
            background: var(--bg);
            line-height: 1.8;
        }
        .header {
            position: sticky;
            top: 0;
            z-index: 10;
            padding: 1rem 2rem;
            background: rgba(247, 242, 232, 0.95);
            backdrop-filter: blur(10px);
            border-bottom: 1px solid rgba(36, 27, 22, 0.08);
            display: flex;
            justify-content: space-between;
            align-items: center;
            gap: 1rem;
        }
        .header h1 {
            margin: 0;
            font-size: 1.2rem;
            color: var(--accent-deep);
        }
        .header-meta {
            color: var(--muted);
            font-size: 0.9rem;
        }
        .toolbar {
            display: flex;
            gap: 0.5rem;
        }
        .btn {
            padding: 0.5rem 1rem;
            border: 1px solid rgba(36, 27, 22, 0.14);
            border-radius: 999px;
            background: rgba(255, 251, 245, 0.92);
            color: var(--ink);
            font-size: 0.85rem;
            cursor: pointer;
            transition: all 0.2s;
        }
        .btn:hover {
            background: var(--accent);
            color: white;
            border-color: var(--accent);
        }
        .content {
            max-width: 900px;
            margin: 2rem auto;
            padding: 2.5rem;
            background: #fffaf4;
            border-radius: 20px;
            box-shadow: 0 20px 60px rgba(78, 51, 33, 0.12);
            white-space: pre-wrap;
            word-wrap: break-word;
            font-size: 1rem;
            line-height: 2;
        }
        .empty {
            text-align: center;
            color: var(--muted);
            padding: 4rem;
        }
        @media (max-width: 768px) {
            .header { padding: 1rem; flex-wrap: wrap; }
            .header h1 { font-size: 1rem; }
            .content { margin: 1rem; padding: 1.5rem; }
        }
    </style>
</head>
<body>
    <div class="header">
        <div>
            <h1>${Utils.escapeHTML(paper.title)}</h1>
            <span class="header-meta">${paper.pdf_text.length.toLocaleString()} 字符</span>
        </div>
        <div class="toolbar">
            <button class="btn" onclick="copyText()">复制全文</button>
            <button class="btn" onclick="window.close()">关闭</button>
        </div>
    </div>
    <div class="content" id="content">${Utils.escapeHTML(paper.pdf_text)}</div>
    <script>
        function copyText() {
            const text = document.getElementById('content').innerText;
            navigator.clipboard.writeText(text).then(() => {
                alert('已复制到剪贴板');
            }).catch(err => {
                console.error('复制失败:', err);
            });
        }
    <\/script>
</body>
</html>
        `);
        win.document.close();
    }
};
