const AIReaderPage = {
    state: {
        aiSettings: null,
        extractorSettings: null,
        papers: [],
        selectedPaperID: null,
        loading: false,
        sessions: {}
    },

    init() {
        if (this.initialized) return;
        this.initialized = true;

        this.configSummary = document.getElementById('aiConfigSummary');
        this.paperSearchInput = document.getElementById('aiPaperSearchInput');
        this.paperList = document.getElementById('aiPaperList');
        this.paperSummary = document.getElementById('aiPaperSummary');
        this.conversation = document.getElementById('aiConversation');
        this.roundBadge = document.getElementById('aiRoundBadge');
        this.sessionHint = document.getElementById('aiSessionHint');
        this.questionInput = document.getElementById('aiQuestionInput');
        this.runButton = document.getElementById('runAIReaderButton');
        this.clearConversationButton = document.getElementById('clearAIConversationButton');
        this.modelSummary = document.getElementById('aiModelSummary');

        this.bindEvents();
        this.bootstrap();
    },

    bindEvents() {
        this.paperSearchInput.addEventListener('input', Utils.debounce(() => this.loadPapers(), 250));

        this.paperList.addEventListener('click', (event) => {
            const button = event.target.closest('[data-paper-id]');
            if (!button) return;
            if (this.state.loading) return;
            this.state.selectedPaperID = Number(button.dataset.paperId);
            this.renderPaperList();
            this.renderPaperSummary();
            this.renderConversation();
            this.syncURL();
        });

        this.runButton.addEventListener('click', async () => {
            await this.run();
        });

        this.clearConversationButton.addEventListener('click', () => {
            this.clearConversation();
        });

        this.keydownHandler = async (event) => {
            if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') {
                event.preventDefault();
                await this.run();
                return;
            }
        };

        document.addEventListener('keydown', this.keydownHandler);
    },

    async bootstrap() {
        try {
            await Promise.all([this.loadConfigSummary(), this.loadPapers()]);
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async loadConfigSummary() {
        const [aiSettings, extractorSettings] = await Promise.all([
            API.getAISettings(),
            API.getExtractorSettings()
        ]);
        this.state.aiSettings = aiSettings;
        this.state.extractorSettings = extractorSettings;
        this.renderConfigSummary();
        this.renderModelSummary();
    },

    renderConfigSummary() {
        const aiSettings = this.state.aiSettings || {};
        const extractorSettings = this.state.extractorSettings || {};
        const aiReady = Boolean(aiSettings.api_key);
        const extractorReady = Boolean(extractorSettings.effective_extractor_url);

        this.configSummary.innerHTML = `
            <article class="settings-summary-card">
                <span>AI Provider</span>
                <strong>${Utils.escapeHTML(aiSettings.provider || 'openai')}</strong>
                <p>${Utils.escapeHTML(aiSettings.model || this.defaultModel(aiSettings.provider || 'openai'))}</p>
            </article>
            <article class="settings-summary-card">
                <span>AI 状态</span>
                <strong>${aiReady ? '已配置' : '未配置'}</strong>
                <p>${aiReady ? 'API Key 已提供，可直接发起阅读请求。' : '请先到配置页保存 AI Key 与模型参数。'}</p>
            </article>
            <article class="settings-summary-card">
                <span>PDF 提取器</span>
                <strong>${extractorReady ? '已配置' : '未配置'}</strong>
                <p>${Utils.escapeHTML(extractorSettings.effective_extractor_url || '尚未配置提取接口，上传后无法自动解析 PDF。')}</p>
            </article>
            <article class="settings-summary-card action">
                <span>配置入口</span>
                <strong><a href="/settings">前往配置页</a></strong>
                <p>统一维护 AI 与 PDF 提取服务参数。</p>
            </article>
        `;
    },

    async loadPapers() {
        const payload = await API.listPapers({
            status: 'completed',
            page: 1,
            page_size: 100,
            keyword: this.paperSearchInput.value.trim()
        });

        this.state.papers = payload.papers || [];

        const queryPaperID = this.queryPaperID();
        if (queryPaperID && !this.state.papers.some((paper) => paper.id === queryPaperID)) {
            try {
                const paper = await API.getPaper(queryPaperID);
                if (paper.extraction_status === 'completed') {
                    this.state.papers = [paper, ...this.state.papers];
                }
            } catch (error) {
                // Ignore invalid query paper IDs and fall back to current result set.
            }
        }

        if (queryPaperID && this.state.papers.some((paper) => paper.id === queryPaperID)) {
            this.state.selectedPaperID = queryPaperID;
        } else if (!this.state.selectedPaperID && this.state.papers.length > 0) {
            this.state.selectedPaperID = this.state.papers[0].id;
        } else if (!this.state.papers.some((paper) => paper.id === this.state.selectedPaperID)) {
            this.state.selectedPaperID = this.state.papers[0]?.id || null;
        }

        this.renderPaperList();
        this.renderPaperSummary();
        this.renderConversation();
        this.syncURL();
    },

    renderPaperList() {
        if (this.state.papers.length === 0) {
            this.paperList.innerHTML = `
                <div class="empty-state">
                    <p>没有找到可用于 AI伴读的已解析文献。</p>
                </div>
            `;
            return;
        }

        this.paperList.innerHTML = this.state.papers.map((paper) => {
            const active = paper.id === this.state.selectedPaperID;
            const tags = (paper.tags || []).slice(0, 3).map((tag) => `
                <span class="tag-pill" style="background:${tag.color || '#A45C40'}22;color:${tag.color || '#A45C40'};">
                    ${Utils.escapeHTML(tag.name)}
                </span>
            `).join('');

            return `
                <button class="ai-paper-item ${active ? 'active' : ''}" type="button" data-paper-id="${paper.id}">
                    <div class="ai-paper-item-head">
                        <strong>${Utils.escapeHTML(paper.title)}</strong>
                        <span class="status-badge tone-${Utils.statusTone(paper.extraction_status)}">${Utils.escapeHTML(Utils.statusLabel(paper.extraction_status))}</span>
                    </div>
                    <div class="ai-paper-item-meta">
                        <span>${Utils.escapeHTML(paper.original_filename)}</span>
                        <span>${paper.group_name ? `分组：${Utils.escapeHTML(paper.group_name)}` : '未分组'}</span>
                        <span>图片 ${paper.figure_count || 0}</span>
                    </div>
                    ${tags ? `<div class="ai-paper-item-tags">${tags}</div>` : ''}
                </button>
            `;
        }).join('');
    },

    renderPaperSummary() {
        const paper = this.currentPaper();
        if (!paper) {
            this.paperSummary.innerHTML = `
                <div class="empty-state">
                    <p>请选择一篇文献。</p>
                </div>
            `;
            return;
        }

        const summaryText = paper.abstract_text || paper.notes_text || '当前文献还没有摘要或备注。';
        const tags = (paper.tags || []).map((tag) => `
            <span class="tag-pill" style="background:${tag.color || '#A45C40'}22;color:${tag.color || '#A45C40'};">
                ${Utils.escapeHTML(tag.name)}
            </span>
        `).join('');

        this.paperSummary.innerHTML = `
            <div class="ai-paper-summary-card">
                <div class="manager-head">
                    <h2>${Utils.escapeHTML(paper.title)}</h2>
                    <p>${Utils.escapeHTML(paper.original_filename)}</p>
                </div>
                <div class="paper-list-meta">
                    <span>${paper.group_name ? `分组：${Utils.escapeHTML(paper.group_name)}` : '未分组'}</span>
                    <span>图片 ${paper.figure_count || 0}</span>
                    <span>${Utils.escapeHTML(Utils.formatDate(paper.updated_at))}</span>
                </div>
                <p class="paper-list-summary">${Utils.escapeHTML(summaryText)}</p>
                ${tags ? `<div class="paper-list-tags">${tags}</div>` : ''}
            </div>
        `;
    },

    async run() {
        if (this.state.loading) return;
        if (!this.state.selectedPaperID) {
            Utils.showToast('请先选择一篇文献', 'error');
            return;
        }
        if (!this.state.aiSettings?.api_key) {
            Utils.showToast('请先到配置页保存 AI 配置', 'error');
            return;
        }
        if (this.currentConversation().length >= 5) {
            Utils.showToast('当前文献已达到 5 轮对话上限，请先清空对话', 'error');
            return;
        }

        const paperID = this.state.selectedPaperID;
        const pendingQuestion = this.questionInput.value.trim();
        this.state.loading = true;
        this.runButton.disabled = true;
        this.runButton.textContent = '发送中...';
        this.renderConversation({
            pendingQuestion: pendingQuestion || this.questionPlaceholder(),
            loading: true
        });

        try {
            const result = await API.readPaperWithAI({
                paper_id: paperID,
                action: 'paper_qa',
                question: pendingQuestion,
                history: this.currentConversation(paperID).map((turn) => ({
                    question: turn.question,
                    answer: turn.answer
                }))
            });
            this.pushConversationTurn(result, paperID);
            this.questionInput.value = '';
            this.renderConversation();
        } catch (error) {
            this.renderConversation();
            Utils.showToast(error.message, 'error');
        } finally {
            this.state.loading = false;
            this.runButton.disabled = false;
            this.runButton.textContent = '发送问题';
            this.renderConversation();
        }
    },

    renderConversation(pending = null) {
        const paper = this.currentPaper();
        const turns = this.currentConversation();
        const roundCount = turns.length;
        const hasLimitReached = roundCount >= 5;

        this.roundBadge.textContent = `${roundCount} / 5 轮`;
        this.clearConversationButton.disabled = !turns.length && !pending;
        this.questionInput.disabled = !paper || hasLimitReached || this.state.loading;
        this.runButton.disabled = !paper || hasLimitReached || this.state.loading;

        if (!paper) {
            this.sessionHint.textContent = '先选择一篇已解析文献，再开始连续提问。';
            this.conversation.innerHTML = `
                <div class="empty-state">
                    <p>当前还没有选中文献。</p>
                </div>
            `;
            return;
        }

        if (hasLimitReached) {
            this.sessionHint.textContent = '当前文献已经达到 5 轮上限；如需继续，请先清空对话。';
        } else if (turns.length > 0) {
            this.sessionHint.textContent = `当前文献已累计 ${roundCount} 轮对话，下一次提问会自动带上前文上下文。`;
        } else {
            this.sessionHint.textContent = '当前还没有对话记录，发送第一个问题后会自动保留上下文。';
        }

        const blocks = [];
        if (!turns.length && !pending) {
            blocks.push(`
                <div class="empty-state">
                    <p>还没有提问记录。你可以先问结论、方法、局限，或者直接追问某段原文。</p>
                </div>
            `);
        }

        turns.forEach((turn, index) => {
            blocks.push(`
                <article class="ai-turn ai-turn-user">
                    <div class="ai-turn-meta">
                        <span>第 ${index + 1} 轮</span>
                        <strong>你</strong>
                    </div>
                    <div class="ai-turn-body">${Utils.escapeHTML(turn.question)}</div>
                </article>
                <article class="ai-turn ai-turn-assistant">
                    <div class="ai-turn-meta">
                        <span>AI</span>
                        <strong>${Utils.escapeHTML(turn.provider || 'AI')}</strong>
                    </div>
                    <div class="ai-turn-body">${Utils.escapeHTML(turn.answer || '模型没有返回文本结果。')}</div>
                    <div class="ai-turn-foot">${Utils.escapeHTML(this.turnMeta(turn))}</div>
                </article>
            `);
        });

        if (pending?.loading) {
            blocks.push(`
                <article class="ai-turn ai-turn-user pending">
                    <div class="ai-turn-meta">
                        <span>发送中</span>
                        <strong>你</strong>
                    </div>
                    <div class="ai-turn-body">${Utils.escapeHTML(pending.pendingQuestion || '正在发送问题...')}</div>
                </article>
                <article class="ai-turn ai-turn-assistant pending">
                    <div class="ai-turn-meta">
                        <span>AI</span>
                        <strong>处理中</strong>
                    </div>
                    <div class="ai-turn-body">正在把全文、图片和上下文一起发送给模型。</div>
                </article>
            `);
        }

        this.conversation.innerHTML = blocks.join('');
    },

    renderModelSummary() {
        const settings = this.state.aiSettings || {};
        const provider = settings.provider || 'openai';
        const model = settings.model || this.defaultModel(provider);
        const mode = provider === 'openai'
            ? (settings.openai_legacy_mode ? 'Chat Completions' : 'Responses')
            : (provider === 'anthropic' ? 'Messages' : 'generateContent');
        this.modelSummary.textContent = `${provider} / ${model} / ${mode}`;
    },

    currentPaper() {
        return this.state.papers.find((paper) => paper.id === this.state.selectedPaperID) || null;
    },

    currentConversation(paperID = this.state.selectedPaperID) {
        if (!paperID) return [];
        return this.state.sessions[paperID] || [];
    },

    pushConversationTurn(result, paperID = this.state.selectedPaperID) {
        if (!paperID) return;

        const turns = this.currentConversation(paperID).slice(0, 5);
        turns.push({
            question: result.question || this.questionPlaceholder(),
            answer: result.answer || '',
            provider: result.provider || '',
            model: result.model || '',
            mode: result.mode || '',
            includedFigures: result.included_figures || 0
        });
        this.state.sessions[paperID] = turns;
    },

    clearConversation() {
        if (!this.state.selectedPaperID) return;
        delete this.state.sessions[this.state.selectedPaperID];
        this.renderConversation();
    },

    turnMeta(turn) {
        const items = [];
        if (turn.provider) items.push(turn.provider);
        if (turn.model) items.push(turn.model);
        if (turn.mode) items.push(turn.mode);
        items.push(`图片 ${Number(turn.includedFigures) || 0}`);
        return items.join(' · ');
    },

    questionPlaceholder() {
        return '例如：请解释这篇文章的核心结论，以及哪些图片最关键。';
    },

    defaultModel(provider) {
        const models = {
            openai: 'gpt-4.1-mini',
            anthropic: 'claude-3-7-sonnet-latest',
            gemini: 'gemini-2.5-flash'
        };
        return models[provider] || models.openai;
    },

    queryPaperID() {
        const value = Number(new URLSearchParams(window.location.search).get('paper_id'));
        return Number.isFinite(value) && value > 0 ? value : null;
    },

    syncURL() {
        const params = new URLSearchParams(window.location.search);
        if (this.state.selectedPaperID) {
            params.set('paper_id', this.state.selectedPaperID);
        } else {
            params.delete('paper_id');
        }
        params.delete('action');
        const query = params.toString();
        const nextURL = `${window.location.pathname}${query ? `?${query}` : ''}`;
        window.history.replaceState({}, '', nextURL);
    }
};
