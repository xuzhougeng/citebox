const AIReaderPage = {
    state: {
        aiSettings: null,
        extractorSettings: null,
        papers: [],
        paperDetails: {},
        selectedPaperID: null,
        loading: false,
        exportingTurnKey: '',
        exportingConversation: false,
        savingNoteTurnKey: '',
        pendingTurn: null,
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
        this.rolePromptHint = document.getElementById('aiRolePromptHint');
        this.rolePromptQuickList = document.getElementById('aiRolePromptQuickList');
        this.runButton = document.getElementById('runAIReaderButton');
        this.stopButton = document.getElementById('stopAIReaderButton');
        this.exportConversationButton = document.getElementById('exportAIConversationButton');
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
            this.ensurePaperDetail(this.state.selectedPaperID).catch((error) => {
                Utils.showToast(error.message, 'error');
            });
        });

        this.paperSummary.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-ai-paper-action="open-paper"]');
            if (!button) return;
            await this.openPaperDetail();
        });

        this.runButton.addEventListener('click', async () => {
            await this.run();
        });

        this.stopButton.addEventListener('click', () => {
            this.stopRun();
        });

        this.clearConversationButton.addEventListener('click', () => {
            this.clearConversation();
        });

        this.exportConversationButton.addEventListener('click', async () => {
            await this.downloadConversationMarkdown();
        });

        this.rolePromptQuickList.addEventListener('click', (event) => {
            const button = event.target.closest('[data-ai-role-name]');
            if (!button) return;
            this.insertRolePromptMention(button.dataset.aiRoleName || '');
        });

        this.conversation.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-download-turn-index]');
            if (!button) return;
            event.preventDefault();
            await this.downloadTurnMarkdown(Number(button.dataset.downloadTurnIndex));
        });

        this.conversation.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-save-turn-note-index]');
            if (!button) return;
            event.preventDefault();
            await this.saveTurnToPaperNotes(Number(button.dataset.saveTurnNoteIndex));
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
        this.renderRolePromptHints();
    },

    renderConfigSummary() {
        const aiSettings = this.state.aiSettings || {};
        const extractorSettings = this.state.extractorSettings || {};
        const qaModel = this.resolveModelForAction('paper_qa');
        const aiReady = Boolean(qaModel.api_key);
        const extractorReady = Boolean(extractorSettings.effective_extractor_url);

        this.configSummary.innerHTML = `
            <article class="settings-summary-card">
                <span>问答模型</span>
                <strong>${Utils.escapeHTML(qaModel.provider || 'openai')}</strong>
                <p>${Utils.escapeHTML(qaModel.model || this.defaultModel(qaModel.provider || 'openai'))}</p>
            </article>
            <article class="settings-summary-card">
                <span>AI 状态</span>
                <strong>${aiReady ? '已配置' : '未配置'}</strong>
                <p>${aiReady ? '当前问答模型已提供 API Key，可直接发起阅读请求。' : '请先到配置页为问答场景配置可用模型和 API Key。'}</p>
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

    renderRolePromptHints() {
        const rolePrompts = this.availableRolePrompts();
        if (!this.rolePromptHint || !this.rolePromptQuickList) {
            return;
        }

        if (!rolePrompts.length) {
            this.rolePromptHint.textContent = '还没有配置角色 Prompt。到配置页新增后，就可以在这里用 @角色名 调用。';
            this.rolePromptQuickList.innerHTML = '';
            return;
        }

        this.rolePromptHint.textContent = '输入 @角色名 直接调用角色 Prompt，也可以点击下面的快捷项插入。';
        this.rolePromptQuickList.innerHTML = rolePrompts.map((item) => `
            <button class="ai-role-chip" type="button" data-ai-role-name="${Utils.escapeHTML(item.name)}">
                ${Utils.escapeHTML(`@${item.name}`)}
            </button>
        `).join('');
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
        if (this.state.selectedPaperID) {
            this.ensurePaperDetail(this.state.selectedPaperID).catch((error) => {
                Utils.showToast(error.message, 'error');
            });
        }
    },

    renderPaperList() {
        if (this.state.papers.length === 0) {
            this.paperList.innerHTML = `
                <div class="empty-state">
                    <p>没有找到可用于 AI伴读的文献。</p>
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
                <button class="ai-paper-item ${active ? 'active' : ''}" type="button" data-paper-id="${paper.id}" aria-pressed="${active ? 'true' : 'false'}">
                    <div class="ai-paper-item-head">
                        <strong>${Utils.escapeHTML(paper.title)}</strong>
                        <div class="ai-paper-item-state">
                            ${active ? '<span class="ai-paper-active-badge">当前文献</span>' : ''}
                            <span class="status-badge tone-${Utils.statusTone(paper.extraction_status)}">${Utils.escapeHTML(Utils.statusLabel(paper.extraction_status))}</span>
                        </div>
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

        const summaryText = paper.abstract_text || paper.paper_notes_text || paper.notes_text || '当前文献还没有摘要或笔记。';
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
                <div class="card-actions ai-paper-summary-actions">
                    <button class="btn btn-outline" type="button" data-ai-paper-action="open-paper">文献详情</button>
                    <a class="btn btn-outline" href="${Utils.resourceViewerURL('pdf', paper.pdf_url)}">打开 PDF</a>
                </div>
            </div>
        `;
    },

    async openPaperDetail() {
        const paper = this.currentPaper();
        if (!paper?.id) return;

        if (typeof PaperViewer === 'undefined') {
            window.location.href = `/library?paper_id=${paper.id}`;
            return;
        }

        await PaperViewer.open(paper.id, async () => {
            delete this.state.paperDetails[paper.id];
            await this.loadPapers();
            if (this.state.selectedPaperID === paper.id) {
                await this.ensurePaperDetail(paper.id);
            }
        });
    },

    async run() {
        if (this.state.loading) return;
        if (!this.state.selectedPaperID) {
            Utils.showToast('请先选择一篇文献', 'error');
            return;
        }
        if (!this.resolveModelForAction('paper_qa').api_key) {
            Utils.showToast('请先到配置页为问答场景配置可用模型', 'error');
            return;
        }
        if (this.currentConversation().length >= 5) {
            Utils.showToast('当前文献已达到 5 轮对话上限，请先清空对话', 'error');
            return;
        }

        const paperID = this.state.selectedPaperID;
        const pendingQuestion = this.questionInput.value.trim();
        try {
            await this.ensurePaperDetail(paperID);
        } catch (error) {
            Utils.showToast(error.message, 'error');
            return;
        }
        this.state.loading = true;
        this.runButton.disabled = true;
        this.runButton.textContent = '发送中...';
        const requestState = {
            paperID,
            question: pendingQuestion || this.questionPlaceholder(),
            answer: '',
            provider: '',
            model: '',
            mode: '',
            includedFigures: 0,
            abortController: new AbortController(),
            stopped: false,
            loading: true
        };
        this.state.pendingTurn = requestState;
        this.renderConversation();

        try {
            await API.readPaperWithAIStream({
                paper_id: paperID,
                action: 'paper_qa',
                question: pendingQuestion,
                history: this.currentConversation(paperID).map((turn) => ({
                    question: turn.question,
                    answer: turn.answer
                }))
            }, {
                signal: requestState.abortController.signal,
                onEvent: (event) => {
                    if (this.state.pendingTurn !== requestState) return;

                    if (event.type === 'error') {
                        throw new Error(event.error || '流式回答失败');
                    }
                    if (event.type === 'meta' && event.result) {
                        requestState.question = event.result.question || requestState.question;
                        requestState.provider = event.result.provider || '';
                        requestState.model = event.result.model || '';
                        requestState.mode = event.result.mode || '';
                        requestState.includedFigures = event.result.included_figures || 0;
                        this.scheduleConversationRender();
                        return;
                    }
                    if (event.type === 'delta') {
                        requestState.answer += event.delta || '';
                        this.scheduleConversationRender();
                        return;
                    }
                    if (event.type === 'final' && event.result) {
                        this.pushConversationTurn(event.result, paperID);
                        this.state.pendingTurn = null;
                        this.questionInput.value = '';
                        this.renderConversation();
                    }
                }
            });
        } catch (error) {
            if (error.name === 'AbortError') {
                if (this.state.pendingTurn === requestState) {
                    requestState.loading = false;
                    requestState.stopped = true;
                    delete requestState.abortController;
                    this.renderConversation();
                }
                return;
            }
            if (this.state.pendingTurn === requestState) {
                this.state.pendingTurn = null;
            }
            this.renderConversation();
            Utils.showToast(error.message, 'error');
        } finally {
            this.state.loading = false;
            this.runButton.disabled = false;
            this.runButton.textContent = '发送问题';
            this.renderConversation();
        }
    },

    renderConversation() {
        const paper = this.currentPaper();
        const turns = this.currentConversation();
        const pending = this.currentPendingTurn();
        const roundCount = turns.length;
        const displayRoundCount = roundCount + (pending?.loading ? 1 : 0);
        const hasLimitReached = roundCount >= 5;
        const isGenerating = Boolean(pending?.loading);

        this.roundBadge.textContent = `${Math.min(displayRoundCount, 5)} / 5 轮`;
        this.clearConversationButton.disabled = this.state.loading || (!turns.length && !pending);
        this.exportConversationButton.disabled = !paper || !turns.length || this.state.exportingConversation;
        this.exportConversationButton.textContent = this.state.exportingConversation ? '导出中...' : '对话导出';
        this.questionInput.disabled = !paper || hasLimitReached || this.state.loading;
        this.runButton.disabled = !paper || hasLimitReached || this.state.loading;
        this.stopButton.hidden = !isGenerating;
        this.stopButton.disabled = !isGenerating;

        if (!paper) {
            this.sessionHint.textContent = '先选择一篇可用于 AI伴读的文献，再开始连续提问。';
            this.conversation.innerHTML = `
                <div class="empty-state">
                    <p>当前还没有选中文献。</p>
                </div>
            `;
            return;
        }

        if (pending?.loading) {
            this.sessionHint.textContent = `正在生成第 ${Math.min(displayRoundCount, 5)} 轮回答，输出会实时显示。`;
        } else if (pending?.stopped) {
            this.sessionHint.textContent = '本次生成已停止；当前片段未计入对话历史。';
        } else if (hasLimitReached) {
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
            const turnKey = this.turnExportKey(index);
            const exporting = this.state.exportingTurnKey === turnKey;
            const noteKey = this.turnNoteKey(index);
            const savingNote = this.state.savingNoteTurnKey === noteKey;
            const assistantBody = turn.answer
                ? Utils.renderMarkdown(turn.answer, {
                    resolveFigureSrc: (figureID) => this.resolveFigureImageURL(figureID, paper)
                })
                : '<div class="markdown-empty">模型没有返回文本结果。</div>';
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
                    <div class="ai-turn-body markdown-body">${assistantBody}</div>
                    <div class="ai-turn-foot">
                        <span>${Utils.escapeHTML(this.turnMeta(turn))}</span>
                        ${turn.answer ? `
                            <div class="ai-turn-foot-actions">
                                <button
                                    class="btn btn-outline btn-small"
                                    type="button"
                                    data-save-turn-note-index="${index}"
                                    ${savingNote ? 'disabled' : ''}
                                >
                                    ${savingNote ? '写入中...' : '保存到文献笔记'}
                                </button>
                                <button
                                    class="btn btn-outline btn-small"
                                    type="button"
                                    data-download-turn-index="${index}"
                                    ${exporting ? 'disabled' : ''}
                                >
                                    ${exporting ? '导出中...' : '下载 Markdown'}
                                </button>
                            </div>
                        ` : ''}
                    </div>
                </article>
            `);
        });

        if (pending) {
            const assistantBody = pending.answer
                ? Utils.renderMarkdown(pending.answer, {
                    resolveFigureSrc: (figureID) => this.resolveFigureImageURL(figureID, paper)
                })
                : (pending.stopped ? '这次生成已被手动停止。' : '正在把全文、图片和上下文一起发送给模型。');
            const assistantBodyClass = pending.answer ? 'ai-turn-body markdown-body' : 'ai-turn-body';
            blocks.push(`
                <article class="ai-turn ai-turn-user pending">
                    <div class="ai-turn-meta">
                        <span>${pending.loading ? '发送中' : '已停止'}</span>
                        <strong>你</strong>
                    </div>
                    <div class="ai-turn-body">${Utils.escapeHTML(pending.question || '正在发送问题...')}</div>
                </article>
                <article class="ai-turn ai-turn-assistant pending">
                    <div class="ai-turn-meta">
                        <span>AI</span>
                        <strong>${Utils.escapeHTML(pending.provider || (pending.stopped ? '已停止' : '处理中'))}</strong>
                    </div>
                    <div class="${assistantBodyClass}">${assistantBody}</div>
                    <div class="ai-turn-foot">
                        <span>${Utils.escapeHTML(this.turnMeta(pending))}${pending.stopped ? ' · 未计入历史' : ''}</span>
                    </div>
                </article>
            `);
        }

        this.conversation.innerHTML = blocks.join('');
    },

    renderModelSummary() {
        const selectedModel = this.resolveModelForAction('paper_qa');
        const provider = selectedModel.provider || 'openai';
        const model = selectedModel.model || this.defaultModel(provider);
        const mode = provider === 'openai'
            ? (selectedModel.openai_legacy_mode ? 'Chat Completions' : 'Responses')
            : (provider === 'anthropic' ? 'Messages' : 'generateContent');
        this.modelSummary.textContent = `${provider} / ${model} / ${mode}`;
    },

    resolveModelForAction(action) {
        const settings = this.state.aiSettings || {};
        const models = Array.isArray(settings.models) ? settings.models : [];
        const sceneModels = settings.scene_models || {};
        const fallbackProvider = settings.provider || 'openai';
        const fallbackModel = models[0] || {
            provider: fallbackProvider,
            model: settings.model || this.defaultModel(fallbackProvider),
            api_key: settings.api_key || '',
            openai_legacy_mode: Boolean(settings.openai_legacy_mode)
        };

        let modelID = sceneModels.default_model_id || fallbackModel.id || '';
        if (action === 'paper_qa') {
            modelID = sceneModels.qa_model_id || modelID;
        }

        return models.find((item) => item.id === modelID) || fallbackModel;
    },

    currentPaper() {
        if (!this.state.selectedPaperID) return null;
        return this.state.paperDetails[this.state.selectedPaperID]
            || this.state.papers.find((paper) => paper.id === this.state.selectedPaperID)
            || null;
    },

    async ensurePaperDetail(paperID) {
        if (!paperID) return null;
        if (this.state.paperDetails[paperID]?.id === paperID) {
            return this.state.paperDetails[paperID];
        }

        this.paperDetailPromises = this.paperDetailPromises || {};
        if (this.paperDetailPromises[paperID]) {
            return this.paperDetailPromises[paperID];
        }

        this.paperDetailPromises[paperID] = API.getPaper(paperID)
            .then((paper) => {
                this.state.paperDetails[paperID] = paper;
                if (paperID === this.state.selectedPaperID) {
                    this.renderPaperSummary();
                    this.renderConversation();
                }
                return paper;
            })
            .finally(() => {
                delete this.paperDetailPromises[paperID];
            });

        return this.paperDetailPromises[paperID];
    },

    resolveFigureImageURL(figureID, paper = this.currentPaper()) {
        const normalizedID = Number(figureID);
        if (!Number.isFinite(normalizedID) || normalizedID <= 0 || !paper || !Array.isArray(paper.figures)) {
            return '';
        }
        const figure = paper.figures.find((item) => Number(item.id) === normalizedID);
        return figure?.image_url || '';
    },

    currentPendingTurn(paperID = this.state.selectedPaperID) {
        const pending = this.state.pendingTurn;
        if (!paperID || !pending || pending.paperID !== paperID) {
            return null;
        }
        return pending;
    },

    currentConversation(paperID = this.state.selectedPaperID) {
        if (!paperID) return [];
        return this.state.sessions[paperID] || [];
    },

    scheduleConversationRender() {
        if (this.renderConversationFrame) return;
        this.renderConversationFrame = window.requestAnimationFrame(() => {
            this.renderConversationFrame = null;
            this.renderConversation();
        });
    },

    stopRun() {
        const pending = this.state.pendingTurn;
        if (!pending?.loading || !pending.abortController) return;
        pending.abortController.abort();
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
        if (this.state.pendingTurn?.paperID === this.state.selectedPaperID) {
            this.state.pendingTurn = null;
        }
        this.renderConversation();
    },

    async downloadTurnMarkdown(turnIndex) {
        const paper = this.currentPaper();
        const turns = this.currentConversation();
        const turn = turns[turnIndex];
        if (!paper || !turn || !turn.answer) {
            Utils.showToast('当前回答没有可导出的 Markdown 内容', 'error');
            return;
        }

        const turnKey = this.turnExportKey(turnIndex);
        if (this.state.exportingTurnKey === turnKey) {
            return;
        }

        this.state.exportingTurnKey = turnKey;
        this.renderConversation();

        try {
            const result = await API.exportAIReadMarkdown({
                paper_id: paper.id,
                answer: turn.answer,
                turn_index: turnIndex + 1
            });
            const saved = await Utils.saveBlobDownload(result.blob, result.filename || this.fallbackExportFilename(paper, turnIndex + 1));
            if (saved) {
                Utils.showToast('Markdown 导出完成');
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        } finally {
            this.state.exportingTurnKey = '';
            this.renderConversation();
        }
    },

    async downloadConversationMarkdown() {
        const paper = this.currentPaper();
        const turns = this.currentConversation();
        if (!paper || !turns.length) {
            Utils.showToast('当前还没有可导出的对话内容', 'error');
            return;
        }
        if (this.state.exportingConversation) {
            return;
        }

        this.state.exportingConversation = true;
        this.renderConversation();

        try {
            const markdown = this.buildConversationMarkdown(paper, turns);
            const result = await API.exportAIReadMarkdown({
                paper_id: paper.id,
                scope: 'conversation',
                content: markdown
            });
            const saved = await Utils.saveBlobDownload(result.blob, result.filename || this.fallbackConversationExportFilename(paper));
            if (saved) {
                Utils.showToast('对话导出完成');
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        } finally {
            this.state.exportingConversation = false;
            this.renderConversation();
        }
    },

    async saveTurnToPaperNotes(turnIndex) {
        const paper = this.currentPaper();
        const turns = this.currentConversation();
        const turn = turns[turnIndex];
        if (!paper || !turn || !String(turn.answer || '').trim()) {
            Utils.showToast('当前回答没有可保存的内容', 'error');
            return;
        }

        const noteKey = this.turnNoteKey(turnIndex);
        if (this.state.savingNoteTurnKey === noteKey) {
            return;
        }

        this.state.savingNoteTurnKey = noteKey;
        this.renderConversation();

        try {
            const latestPaper = await API.getPaper(paper.id);
            const noteBlock = this.buildTurnNoteBlock(turn, turnIndex);
            const currentNotes = String(latestPaper.paper_notes_text || '').trim();
            if (currentNotes.includes(noteBlock.trim())) {
                Utils.showToast('这轮 AI 内容已经写入文献笔记');
                return;
            }

            const nextNotes = currentNotes ? `${currentNotes}\n\n---\n\n${noteBlock}` : noteBlock;
            const payload = await API.updatePaper(
                paper.id,
                PaperViewer.buildUpdatePayload(latestPaper, {
                    paper_notes_text: nextNotes
                })
            );
            this.syncUpdatedPaper(payload.paper);
            Utils.showToast('AI 内容已写入文献笔记');
        } catch (error) {
            Utils.showToast(error.message, 'error');
        } finally {
            this.state.savingNoteTurnKey = '';
            this.renderConversation();
        }
    },

    turnMeta(turn) {
        const items = [];
        if (turn.provider) items.push(turn.provider);
        if (turn.model) items.push(turn.model);
        if (turn.mode) items.push(turn.mode);
        items.push(`图片 ${Number(turn.includedFigures) || 0}`);
        return items.join(' · ');
    },

    availableRolePrompts() {
        const rolePrompts = this.state.aiSettings?.role_prompts;
        return Array.isArray(rolePrompts) ? rolePrompts.filter((item) => String(item?.name || '').trim()) : [];
    },

    insertRolePromptMention(name) {
        const normalizedName = String(name || '').trim();
        if (!normalizedName || !this.questionInput) return;

        const mention = `@${normalizedName}`;
        if ((this.questionInput.value || '').includes(mention)) {
            this.questionInput.focus();
            return;
        }

        const input = this.questionInput;
        const start = Number.isFinite(input.selectionStart) ? input.selectionStart : input.value.length;
        const end = Number.isFinite(input.selectionEnd) ? input.selectionEnd : input.value.length;
        const prefix = input.value.slice(0, start);
        const suffix = input.value.slice(end);
        const needsLeadingSpace = prefix && !/\s$/.test(prefix);
        const nextValue = `${prefix}${needsLeadingSpace ? ' ' : ''}${mention} ${suffix}`;
        input.value = nextValue;

        const caret = `${prefix}${needsLeadingSpace ? ' ' : ''}${mention} `.length;
        input.focus();
        input.setSelectionRange(caret, caret);
    },

    questionPlaceholder() {
        return '例如：@严格证据模式 请解释这篇文章的核心结论，以及哪些图片最关键。';
    },

    defaultModel(provider) {
        const models = {
            openai: 'gpt-4.1-mini',
            anthropic: 'claude-3-7-sonnet-latest',
            gemini: 'gemini-2.5-flash'
        };
        return models[provider] || models.openai;
    },

    turnExportKey(turnIndex) {
        return `${this.state.selectedPaperID || 0}:${turnIndex}`;
    },

    turnNoteKey(turnIndex) {
        return `${this.state.selectedPaperID || 0}:${turnIndex}`;
    },

    fallbackExportFilename(paper, turnIndex) {
        return `paper_${paper?.id || 'ai'}_ai_reader_turn_${String(turnIndex).padStart(2, '0')}.zip`;
    },

    fallbackConversationExportFilename(paper) {
        return `paper_${paper?.id || 'ai'}_ai_reader_conversation.zip`;
    },

    buildTurnNoteBlock(turn, turnIndex) {
        const lines = [
            `## AI伴读 · 第 ${turnIndex + 1} 轮`,
            `问题：${String(turn.question || this.questionPlaceholder()).trim()}`,
            `记录时间：${this.formatNoteTimestamp()}`,
            `模型：${this.turnMeta(turn)}`,
            '',
            String(turn.answer || '').trim()
        ];
        return lines.join('\n').trim();
    },

    formatNoteTimestamp() {
        return new Date().toLocaleString('zh-CN', {
            year: 'numeric',
            month: '2-digit',
            day: '2-digit',
            hour: '2-digit',
            minute: '2-digit'
        });
    },

    buildConversationMarkdown(paper, turns = []) {
        const header = [
            '# AI伴读对话导出',
            '',
            `- 文献：${paper?.title || '未命名文献'}`,
            `- 原始文件：${paper?.original_filename || '未知文件'}`,
            `- 导出时间：${this.formatNoteTimestamp()}`,
            `- 对话轮数：${turns.length}`,
            ''
        ];

        const rounds = turns.map((turn, index) => [
            `# 第 ${index + 1} 轮`,
            '',
            '## 用户提问',
            String(turn.question || this.questionPlaceholder()).trim(),
            '',
            '## AI 回答',
            String(turn.answer || '').trim(),
            ''
        ].join('\n').trim());

        return [...header, ...rounds].join('\n').trim();
    },

    syncUpdatedPaper(updatedPaper) {
        if (!updatedPaper || !updatedPaper.id) return;
        this.state.paperDetails[updatedPaper.id] = updatedPaper;
        this.state.papers = (this.state.papers || []).map((paper) => (
            paper.id === updatedPaper.id
                ? {
                    ...paper,
                    ...updatedPaper,
                    tags: updatedPaper.tags || paper.tags || [],
                    figure_count: updatedPaper.figure_count ?? paper.figure_count
                }
                : paper
        ));
        this.renderPaperList();
        this.renderPaperSummary();
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
