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
        savingNoteTurnMode: '',
        pendingTurn: null,
        sessions: {}
    },

    init() {
        if (this.initialized) return;
        this.initialized = true;

        this.configSummary = document.getElementById('aiConfigSummary');
        this.currentPaperBar = document.getElementById('aiCurrentPaperBar');
        this.mentionPopover = document.getElementById('aiMentionPopover');
        this.questionMirror = document.getElementById('aiQuestionMirror');
        this.paperMentionLabels = new Set();
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
        if (this.currentPaperBar) {
            this.currentPaperBar.addEventListener('click', async (event) => {
                const action = event.target.closest('[data-ai-paper-action]');
                if (!action) return;
                event.preventDefault();
                if (action.dataset.aiPaperAction === 'open-paper') {
                    await this.openPaperDetail();
                } else if (action.dataset.aiPaperAction === 'switch-paper') {
                    this.triggerMentionFromButton();
                }
            });
        }

        if (this.questionInput) {
            this.questionInput.addEventListener('input', () => {
                this.handleQuestionInput();
                this.renderQuestionMirror();
            });
            this.questionInput.addEventListener('scroll', () => this.syncQuestionMirrorScroll());
            this.questionInput.addEventListener('keydown', (event) => this.handleMentionKeydown(event));
            this.questionInput.addEventListener('blur', () => {
                this._mentionBlurTimer = window.setTimeout(() => this.closeMention(), 150);
            });
            this.questionInput.addEventListener('focus', () => {
                if (this._mentionBlurTimer) {
                    window.clearTimeout(this._mentionBlurTimer);
                    this._mentionBlurTimer = 0;
                }
            });
            this.renderQuestionMirror();
        }

        if (this.mentionPopover) {
            this.mentionPopover.addEventListener('mousedown', (event) => {
                event.preventDefault();
            });
            this.mentionPopover.addEventListener('click', (event) => {
                const item = event.target.closest('.ai-mention-item');
                if (!item) return;
                this.applyMentionItem({
                    type: item.dataset.mentionType,
                    id: Number(item.dataset.paperId || 0),
                    name: item.dataset.roleName || '',
                });
            });
        }

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
            await this.saveTurnToPaperNotes(Number(button.dataset.saveTurnNoteIndex), button.dataset.saveTurnNoteMode);
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
                <span>${t('ai.config_qa_model', '问答模型')}</span>
                <strong>${Utils.escapeHTML(qaModel.provider || 'openai')}</strong>
                <p>${Utils.escapeHTML(qaModel.model || this.defaultModel(qaModel.provider || 'openai'))}</p>
            </article>
            <article class="settings-summary-card">
                <span>${t('ai.config_ai_status', 'AI 状态')}</span>
                <strong>${aiReady ? t('ai.config_configured', '已配置') : t('ai.config_not_configured', '未配置')}</strong>
                <p>${aiReady ? t('ai.config_ai_ready', 'AI 问答已就绪，可以开始提问。') : t('ai.config_ai_not_ready', '请先到配置页设置问答模型和 API Key。')}</p>
            </article>
            <article class="settings-summary-card">
                <span>${t('ai.config_pdf_extractor', 'PDF 提取器')}</span>
                <strong>${extractorReady ? t('ai.config_configured', '已配置') : t('ai.config_not_configured', '未配置')}</strong>
                <p>${Utils.escapeHTML(extractorSettings.effective_extractor_url || t('ai.config_pdf_not_ready', '未配置 PDF 解析服务，上传后需手工标注。'))}</p>
            </article>
            <article class="settings-summary-card action">
                <span>${t('ai.config_entry', '配置入口')}</span>
                <strong><a href="/settings">${t('ai.config_go_settings', '前往配置页')}</a></strong>
                <p>${t('ai.config_entry_desc', '在配置页统一设置 AI 模型和 PDF 解析服务。')}</p>
            </article>
        `;
    },

    renderRolePromptHints() {
        const rolePrompts = this.availableRolePrompts();
        if (!this.rolePromptHint || !this.rolePromptQuickList) {
            return;
        }

        if (!rolePrompts.length) {
            this.rolePromptHint.textContent = t('ai.role_prompt_hint_empty', '还没有配置角色 Prompt。到配置页新增后，就可以在这里用 @角色名 调用。');
            this.rolePromptQuickList.innerHTML = '';
            return;
        }

        this.rolePromptHint.textContent = t('ai.role_prompt_hint', '输入 @角色名 直接调用角色 Prompt，也可以点击下面的快捷项插入。');
        this.rolePromptQuickList.innerHTML = rolePrompts.map((item) => `
            <button class="ai-role-chip" type="button" data-ai-role-name="${Utils.escapeHTML(item.name)}">
                ${Utils.escapeHTML(`@${item.name}`)}
            </button>
        `).join('');
    },

    async loadPapers(keyword = '') {
        const payload = await API.listPapers({
            status: 'completed',
            page: 1,
            page_size: 200,
            keyword: String(keyword || '').trim()
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

        this.renderCurrentPaperBar();
        this.renderConversation();
        this.syncURL();
        if (this.state.selectedPaperID) {
            this.ensurePaperDetail(this.state.selectedPaperID).catch((error) => {
                Utils.showToast(error.message, 'error');
            });
        }
    },

    renderCurrentPaperBar() {
        if (!this.currentPaperBar) return;
        const paper = this.currentPaper();

        if (!paper) {
            this.currentPaperBar.classList.add('empty');
            this.currentPaperBar.innerHTML = `
                <span class="ai-current-paper-icon" aria-hidden="true">@</span>
                <div class="ai-current-paper-body">
                    <span class="ai-current-paper-meta">${Utils.escapeHTML(t('ai.current_paper_empty', '尚未选中文献。在下方输入 @ 即可搜索切换。'))}</span>
                </div>
                <div class="ai-current-paper-actions">
                    <button class="btn btn-outline" type="button" data-ai-paper-action="switch-paper">${Utils.escapeHTML(t('ai.btn_pick_paper', '选择文献'))}</button>
                </div>
            `;
            return;
        }

        this.currentPaperBar.classList.remove('empty');
        const meta = [
            paper.original_filename,
            paper.group_name ? t('ai.paper_group', '分组：${name}').replace('${name}', paper.group_name) : t('ai.paper_no_group', '未分组'),
            t('ai.paper_figures', '图片 ${count}').replace('${count}', paper.figure_count || 0),
        ].filter(Boolean).join(' · ');

        this.currentPaperBar.innerHTML = `
            <span class="ai-current-paper-icon" aria-hidden="true">📄</span>
            <div class="ai-current-paper-body">
                <span class="ai-current-paper-title">${Utils.escapeHTML(paper.title)}</span>
                <span class="ai-current-paper-meta">${Utils.escapeHTML(meta)}</span>
            </div>
            <div class="ai-current-paper-actions">
                <button class="btn btn-outline" type="button" data-ai-paper-action="switch-paper">${Utils.escapeHTML(t('ai.btn_switch_paper', '切换文献'))}</button>
                <button class="btn btn-outline" type="button" data-ai-paper-action="open-paper">${Utils.escapeHTML(t('ai.btn_paper_detail', '文献详情'))}</button>
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
            Utils.showToast(t('ai.msg_select_paper', '请先选择一篇文献'), 'error');
            return;
        }
        if (!this.resolveModelForAction('paper_qa').api_key) {
            Utils.showToast(t('ai.msg_configure_model', '请先到配置页设置问答模型'), 'error');
            return;
        }
        if (this.currentConversation().length >= 5) {
            Utils.showToast(t('ai.msg_limit_reached', '当前文献已达到 5 轮对话上限，请先清空对话'), 'error');
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
        this.runButton.textContent = t('ai.btn_sending', '发送中...');
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
                        throw new Error(event.error || t('ai.msg_stream_error', '流式回答失败'));
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
                        this.renderQuestionMirror();
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
            this.runButton.textContent = t('ai.btn_send', '发送问题');
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

        this.roundBadge.textContent = t('ai.round_badge', '${current} / 5 轮').replace('${current}', Math.min(displayRoundCount, 5));
        this.clearConversationButton.disabled = this.state.loading || (!turns.length && !pending);
        this.exportConversationButton.disabled = !paper || !turns.length || this.state.exportingConversation;
        this.exportConversationButton.textContent = this.state.exportingConversation ? t('ai.btn_exporting_conversation', '导出中...') : t('ai.btn_export_conversation', '对话导出');
        this.questionInput.disabled = !paper || hasLimitReached || this.state.loading;
        this.runButton.disabled = !paper || hasLimitReached || this.state.loading;
        this.stopButton.hidden = !isGenerating;
        this.stopButton.disabled = !isGenerating;

        if (!paper) {
            this.sessionHint.textContent = t('ai.session_hint_no_paper', '先选择一篇可用于 AI伴读的文献，再开始连续提问。');
            this.conversation.innerHTML = `
                <div class="empty-state">
                    <p>${t('ai.empty_no_conversation', '当前还没有选中文献。')}</p>
                </div>
            `;
            return;
        }

        if (pending?.loading) {
            this.sessionHint.textContent = t('ai.session_hint_generating', '正在生成第 ${round} 轮回答，输出会实时显示。').replace('${round}', Math.min(displayRoundCount, 5));
        } else if (pending?.stopped) {
            this.sessionHint.textContent = t('ai.session_hint_stopped', '本次生成已停止；当前片段未计入对话历史。');
        } else if (hasLimitReached) {
            this.sessionHint.textContent = t('ai.session_hint_limit', '当前文献已经达到 5 轮上限；如需继续，请先清空对话。');
        } else if (turns.length > 0) {
            this.sessionHint.textContent = t('ai.session_hint_ongoing', '当前文献已累计 ${count} 轮对话，下一次提问会自动带上前文上下文。').replace('${count}', roundCount);
        } else {
            this.sessionHint.textContent = t('ai.session_hint_ready', '当前还没有对话记录，发送第一个问题后会自动保留上下文。');
        }

        const blocks = [];
        if (!turns.length && !pending) {
            blocks.push(`
                <div class="empty-state">
                    <p>${t('ai.empty_no_questions', '还没有提问记录。你可以先问结论、方法、局限，或者直接追问某段原文。')}</p>
                </div>
            `);
        }

        turns.forEach((turn, index) => {
            const turnKey = this.turnExportKey(index);
            const exporting = this.state.exportingTurnKey === turnKey;
            const noteKey = this.turnNoteKey(index);
            const savingNote = this.state.savingNoteTurnKey === noteKey;
            const savingNoteMode = savingNote ? this.state.savingNoteTurnMode : '';
            const assistantBody = turn.answer
                ? Utils.renderMarkdown(turn.answer, {
                    resolveFigureSrc: (figureID) => this.resolveFigureImageURL(figureID, paper)
                })
                : `<div class="markdown-empty">${t('ai.turn_no_text', '模型没有返回文本结果。')}</div>`;
            blocks.push(`
                <article class="ai-turn ai-turn-user">
                    <div class="ai-turn-meta">
                        <span>${t('ai.turn_label', '第 ${n} 轮').replace('${n}', index + 1)}</span>
                        <strong>${t('ai.turn_user', '你')}</strong>
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
                                    data-save-turn-note-mode="overwrite"
                                    ${savingNote ? 'disabled' : ''}
                                >
                                    ${savingNoteMode === 'overwrite' ? t('ai.btn_overwriting_notes', '覆盖中...') : t('ai.btn_overwrite_notes', '覆盖到文献笔记')}
                                </button>
                                <button
                                    class="btn btn-outline btn-small"
                                    type="button"
                                    data-save-turn-note-index="${index}"
                                    data-save-turn-note-mode="append"
                                    ${savingNote ? 'disabled' : ''}
                                >
                                    ${savingNoteMode === 'append' ? t('ai.btn_appending_notes', '追加中...') : t('ai.btn_append_notes', '追加到文献笔记')}
                                </button>
                                <button
                                    class="btn btn-outline btn-small"
                                    type="button"
                                    data-download-turn-index="${index}"
                                    ${exporting ? 'disabled' : ''}
                                >
                                    ${exporting ? t('ai.btn_exporting_md', '导出中...') : t('ai.btn_download_md', '下载 Markdown')}
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
                : (pending.stopped ? t('ai.turn_stopped_text', '这次生成已被手动停止。') : t('ai.turn_sending_context', '正在把全文、图片和上下文一起发送给模型。'));
            const assistantBodyClass = pending.answer ? 'ai-turn-body markdown-body' : 'ai-turn-body';
            blocks.push(`
                <article class="ai-turn ai-turn-user pending">
                    <div class="ai-turn-meta">
                        <span>${pending.loading ? t('ai.turn_sending', '发送中') : t('ai.turn_stopped', '已停止')}</span>
                        <strong>${t('ai.turn_user', '你')}</strong>
                    </div>
                    <div class="ai-turn-body">${Utils.escapeHTML(pending.question || t('ai.turn_sending_question', '正在发送问题...'))}</div>
                </article>
                <article class="ai-turn ai-turn-assistant pending">
                    <div class="ai-turn-meta">
                        <span>AI</span>
                        <strong>${Utils.escapeHTML(pending.provider || (pending.stopped ? t('ai.turn_stopped', '已停止') : t('ai.turn_processing', '处理中')))}</strong>
                    </div>
                    <div class="${assistantBodyClass}">${assistantBody}</div>
                    <div class="ai-turn-foot">
                        <span>${Utils.escapeHTML(this.turnMeta(pending))}${pending.stopped ? ` · ${t('ai.turn_not_in_history', '未计入历史')}` : ''}</span>
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
                    this.renderCurrentPaperBar();
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
            Utils.showToast(t('ai.msg_no_exportable_md', '当前回答没有可导出的 Markdown 内容'), 'error');
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
                Utils.showToast(t('ai.msg_md_exported', 'Markdown 导出完成'));
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
            Utils.showToast(t('ai.msg_no_exportable_conversation', '当前还没有可导出的对话内容'), 'error');
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
                Utils.showToast(t('ai.msg_conversation_exported', '对话导出完成'));
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        } finally {
            this.state.exportingConversation = false;
            this.renderConversation();
        }
    },

    async saveTurnToPaperNotes(turnIndex, mode = 'append') {
        const paper = this.currentPaper();
        const turns = this.currentConversation();
        const turn = turns[turnIndex];
        if (!paper || !turn || !String(turn.answer || '').trim()) {
            Utils.showToast(t('ai.msg_no_saveable_content', '当前回答没有可保存的内容'), 'error');
            return;
        }

        const saveMode = mode === 'overwrite' ? 'overwrite' : 'append';
        const noteKey = this.turnNoteKey(turnIndex);
        if (this.state.savingNoteTurnKey === noteKey) {
            return;
        }

        this.state.savingNoteTurnKey = noteKey;
        this.state.savingNoteTurnMode = saveMode;
        this.renderConversation();

        try {
            const latestPaper = await API.getPaper(paper.id);
            const noteBlock = this.buildTurnNoteBlock(turn, turnIndex);
            const currentNotes = String(latestPaper.paper_notes_text || '').trim();
            if (saveMode === 'append' && currentNotes.includes(noteBlock.trim())) {
                Utils.showToast(t('ai.msg_note_already_saved', '这轮 AI 内容已经写入文献笔记'));
                return;
            }

            const nextNotes = saveMode === 'overwrite'
                ? noteBlock
                : (currentNotes ? `${currentNotes}\n\n---\n\n${noteBlock}` : noteBlock);
            const payload = await API.updatePaper(
                paper.id,
                PaperViewer.buildUpdatePayload(latestPaper, {
                    paper_notes_text: nextNotes
                })
            );
            this.syncUpdatedPaper(payload.paper);
            Utils.showToast(saveMode === 'overwrite'
                ? t('ai.msg_note_overwritten', 'AI 内容已覆盖文献笔记')
                : t('ai.msg_note_appended', 'AI 内容已追加到文献笔记'));
        } catch (error) {
            Utils.showToast(error.message, 'error');
        } finally {
            this.state.savingNoteTurnKey = '';
            this.state.savingNoteTurnMode = '';
            this.renderConversation();
        }
    },

    turnMeta(turn) {
        const items = [];
        if (turn.provider) items.push(turn.provider);
        if (turn.model) items.push(turn.model);
        if (turn.mode) items.push(turn.mode);
        items.push(t('ai.paper_figures', '图片 ${count}').replace('${count}', Number(turn.includedFigures) || 0));
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
        this.renderQuestionMirror();
    },

    shortPaperMention(title) {
        const text = String(title || '').trim();
        if (!text) return 'paper';
        const head = Array.from(text).slice(0, 5).join('');
        return text.length > head.length ? `${head}…` : head;
    },

    classifyMention(name) {
        const trimmed = String(name || '').trim();
        if (!trimmed) return 'paper';
        const roleSet = new Set(this.availableRolePrompts().map((r) => String(r.name || '').trim()).filter(Boolean));
        if (roleSet.has(trimmed)) return 'role';
        if (this.paperMentionLabels && this.paperMentionLabels.has(trimmed)) return 'paper';
        return 'paper';
    },

    parseQuestionTokens(text) {
        const value = String(text || '');
        const tokens = [];
        const roleNames = this.availableRolePrompts()
            .map((r) => String(r.name || '').trim())
            .filter(Boolean)
            .sort((a, b) => b.length - a.length);

        let i = 0;
        let lastIndex = 0;
        while (i < value.length) {
            const ch = value[i];
            if (ch === '@' && (i === 0 || /\s/.test(value[i - 1]))) {
                const after = value.slice(i + 1);
                let matched = null;

                for (const name of roleNames) {
                    if (after.startsWith(name)) {
                        const nextChar = after[name.length];
                        if (!nextChar || /[\s@\n]/.test(nextChar)) {
                            matched = { name, type: 'role' };
                            break;
                        }
                    }
                }

                if (!matched) {
                    const single = after.match(/^([^\s@\n]+)/);
                    if (single) {
                        matched = { name: single[1], type: 'paper' };
                    }
                }

                if (matched) {
                    if (i > lastIndex) {
                        tokens.push({ type: 'text', text: value.slice(lastIndex, i) });
                    }
                    tokens.push({
                        type: 'mention',
                        text: `@${matched.name}`,
                        mentionType: matched.type,
                    });
                    i += 1 + matched.name.length;
                    lastIndex = i;
                    continue;
                }
            }
            i++;
        }
        if (lastIndex < value.length) {
            tokens.push({ type: 'text', text: value.slice(lastIndex) });
        }
        return tokens;
    },

    renderQuestionMirror() {
        if (!this.questionMirror || !this.questionInput) return;
        const value = this.questionInput.value;
        const tokens = this.parseQuestionTokens(value);
        const html = tokens.map((token) => {
            const safe = Utils.escapeHTML(token.text);
            if (token.type === 'mention') {
                const cls = token.mentionType === 'role' ? 'ai-token-role' : 'ai-token-paper';
                return `<span class="ai-token-mention ${cls}">${safe}</span>`;
            }
            return safe;
        }).join('');
        this.questionMirror.innerHTML = html || '&nbsp;';
        this.syncQuestionMirrorScroll();
    },

    syncQuestionMirrorScroll() {
        if (!this.questionMirror || !this.questionInput) return;
        this.questionMirror.scrollTop = this.questionInput.scrollTop;
        this.questionMirror.scrollLeft = this.questionInput.scrollLeft;
    },

    // ===== Mention popover (@ palette) =====

    handleQuestionInput() {
        if (!this.questionInput || !this.mentionPopover) return;
        const value = this.questionInput.value;
        const caret = this.questionInput.selectionStart;
        const before = value.slice(0, caret);
        const match = before.match(/(^|[\s\n])@([^\s@\n]*)$/);
        if (!match) {
            this.closeMention();
            return;
        }
        const queryLen = match[2].length;
        const atIndex = before.length - queryLen - 1;
        this.openMention(atIndex, match[2]);
    },

    handleMentionKeydown(event) {
        if (!this._mentionState || !this._mentionState.open) {
            return;
        }
        const items = this._mentionState.items;
        if (!items.length && event.key !== 'Escape') {
            return;
        }
        if (event.key === 'ArrowDown') {
            event.preventDefault();
            this._mentionState.activeIndex = (this._mentionState.activeIndex + 1) % Math.max(items.length, 1);
            this.refreshMentionActive();
            return;
        }
        if (event.key === 'ArrowUp') {
            event.preventDefault();
            this._mentionState.activeIndex = (this._mentionState.activeIndex - 1 + items.length) % Math.max(items.length, 1);
            this.refreshMentionActive();
            return;
        }
        if (event.key === 'Enter' || event.key === 'Tab') {
            const item = items[this._mentionState.activeIndex];
            if (item) {
                event.preventDefault();
                this.applyMentionItem(item);
            }
            return;
        }
        if (event.key === 'Escape') {
            event.preventDefault();
            this.closeMention();
        }
    },

    triggerMentionFromButton() {
        if (!this.questionInput) return;
        const input = this.questionInput;
        input.focus();
        const caret = Number.isFinite(input.selectionStart) ? input.selectionStart : input.value.length;
        const before = input.value.slice(0, caret);
        const after = input.value.slice(caret);
        const needsLeadingSpace = before && !/\s$/.test(before);
        const insertion = `${needsLeadingSpace ? ' ' : ''}@`;
        input.value = `${before}${insertion}${after}`;
        const newCaret = before.length + insertion.length;
        input.setSelectionRange(newCaret, newCaret);
        this.handleQuestionInput();
        this.renderQuestionMirror();
    },

    openMention(atIndex, query) {
        const items = this.buildMentionItems(query);
        this._mentionState = {
            open: true,
            atIndex,
            query,
            items,
            activeIndex: 0,
        };
        this.renderMentionPopover();
    },

    closeMention() {
        if (!this._mentionState || !this._mentionState.open) {
            this._mentionState = { open: false };
            if (this.mentionPopover) this.mentionPopover.hidden = true;
            return;
        }
        this._mentionState = { open: false };
        if (this.mentionPopover) {
            this.mentionPopover.hidden = true;
            this.mentionPopover.innerHTML = '';
        }
    },

    buildMentionItems(query) {
        const q = String(query || '').trim().toLowerCase();
        const items = [];

        const roles = this.availableRolePrompts();
        const matchedRoles = roles.filter((role) => {
            if (!q) return true;
            return String(role.name || '').toLowerCase().includes(q);
        }).slice(0, 6);
        matchedRoles.forEach((role) => {
            items.push({
                type: 'role',
                name: role.name,
                title: `@${role.name}`,
                meta: String(role.description || role.prompt || '').slice(0, 80),
            });
        });

        const papers = this.state.papers || [];
        const matchedPapers = papers.filter((paper) => {
            if (!q) return true;
            const haystack = [
                paper.title,
                paper.original_filename,
                paper.group_name,
                paper.doi,
            ].filter(Boolean).join(' ').toLowerCase();
            return haystack.includes(q);
        }).slice(0, 8);
        matchedPapers.forEach((paper) => {
            const metaParts = [
                paper.original_filename,
                paper.group_name || t('ai.paper_no_group', '未分组'),
                t('ai.paper_figures', '图片 ${count}').replace('${count}', paper.figure_count || 0),
            ].filter(Boolean);
            items.push({
                type: 'paper',
                id: paper.id,
                title: paper.title,
                meta: metaParts.join(' · '),
                isCurrent: paper.id === this.state.selectedPaperID,
            });
        });

        return items;
    },

    renderMentionPopover() {
        if (!this.mentionPopover || !this._mentionState) return;
        const { items, activeIndex } = this._mentionState;

        if (!items.length) {
            this.mentionPopover.hidden = false;
            this.mentionPopover.innerHTML = `
                <div class="ai-mention-empty">${Utils.escapeHTML(t('ai.mention_empty', '没有匹配的文献或角色。'))}</div>
            `;
            return;
        }

        const sections = [];
        const roleItems = items.filter((it) => it.type === 'role');
        const paperItems = items.filter((it) => it.type === 'paper');

        const renderItem = (item, globalIndex) => {
            const active = globalIndex === activeIndex ? ' active' : '';
            const current = item.isCurrent ? ' is-current' : '';
            const badge = item.isCurrent
                ? `<span class="ai-mention-item-badge">${Utils.escapeHTML(t('ai.paper_active_badge', '当前文献'))}</span>`
                : '';
            const icon = item.type === 'role' ? '@' : '📄';
            const dataset = item.type === 'role'
                ? `data-mention-type="role" data-role-name="${Utils.escapeHTML(item.name)}"`
                : `data-mention-type="paper" data-paper-id="${item.id}"`;
            return `
                <li class="ai-mention-item${active}${current}" role="option" data-mention-index="${globalIndex}" ${dataset}>
                    <span class="ai-mention-item-icon" aria-hidden="true">${icon}</span>
                    <div class="ai-mention-item-body">
                        <span class="ai-mention-item-title">${Utils.escapeHTML(item.title)}</span>
                        ${item.meta ? `<span class="ai-mention-item-meta">${Utils.escapeHTML(item.meta)}</span>` : ''}
                    </div>
                    ${badge}
                </li>
            `;
        };

        let cursor = 0;
        if (roleItems.length) {
            const roleHTML = roleItems.map((item) => renderItem(item, cursor++)).join('');
            sections.push(`
                <div class="ai-mention-section" data-section="roles">
                    <div class="ai-mention-section-title">${Utils.escapeHTML(t('ai.mention_section_roles', '角色 Prompt'))}</div>
                    <ul class="ai-mention-list" role="presentation">${roleHTML}</ul>
                </div>
            `);
        }
        if (paperItems.length) {
            const paperHTML = paperItems.map((item) => renderItem(item, cursor++)).join('');
            sections.push(`
                <div class="ai-mention-section" data-section="papers">
                    <div class="ai-mention-section-title">${Utils.escapeHTML(t('ai.mention_section_papers', '文献'))}</div>
                    <ul class="ai-mention-list" role="presentation">${paperHTML}</ul>
                </div>
            `);
        }

        this.mentionPopover.hidden = false;
        this.mentionPopover.innerHTML = sections.join('');
        this.scrollMentionActiveIntoView();
    },

    refreshMentionActive() {
        if (!this.mentionPopover || !this._mentionState) return;
        const items = this.mentionPopover.querySelectorAll('.ai-mention-item');
        items.forEach((el) => el.classList.remove('active'));
        const target = this.mentionPopover.querySelector(`[data-mention-index="${this._mentionState.activeIndex}"]`);
        if (target) target.classList.add('active');
        this.scrollMentionActiveIntoView();
    },

    scrollMentionActiveIntoView() {
        if (!this.mentionPopover) return;
        const target = this.mentionPopover.querySelector('.ai-mention-item.active');
        if (target) target.scrollIntoView({ block: 'nearest' });
    },

    applyMentionItem(item) {
        if (!item || !this.questionInput) return;

        const state = this._mentionState;
        const input = this.questionInput;
        const caret = input.selectionStart;
        const beforeAt = (state && Number.isFinite(state.atIndex)) ? input.value.slice(0, state.atIndex) : input.value.slice(0, caret);
        const after = input.value.slice(caret);

        if (item.type === 'role') {
            const mention = `@${item.name} `;
            input.value = `${beforeAt}${mention}${after}`;
            const newCaret = beforeAt.length + mention.length;
            input.setSelectionRange(newCaret, newCaret);
        } else if (item.type === 'paper') {
            const paper = this.state.papers.find((p) => p.id === item.id) || this.state.paperDetails[item.id];
            const title = item.title || paper?.title || '';
            const shortLabel = this.shortPaperMention(title);
            this.paperMentionLabels.add(shortLabel);
            const mention = `@${shortLabel} `;
            input.value = `${beforeAt}${mention}${after}`;
            const newCaret = beforeAt.length + mention.length;
            input.setSelectionRange(newCaret, newCaret);
            if (item.id && item.id !== this.state.selectedPaperID && !this.state.loading) {
                this.state.selectedPaperID = item.id;
                this.renderCurrentPaperBar();
                this.renderConversation();
                this.syncURL();
                this.ensurePaperDetail(item.id).catch((error) => {
                    Utils.showToast(error.message, 'error');
                });
            }
        }

        this.closeMention();
        this.renderQuestionMirror();
        input.focus();
    },

    questionPlaceholder() {
        return t('ai.question_placeholder', '例如：@严格证据模式 请解释这篇文章的核心结论，以及哪些图片最关键。');
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
            `## ${t('ai.note_heading', 'AI伴读 · 第 ${n} 轮').replace('${n}', turnIndex + 1)}`,
            t('ai.note_question', '问题：${text}').replace('${text}', String(turn.question || this.questionPlaceholder()).trim()),
            t('ai.note_time', '记录时间：${time}').replace('${time}', this.formatNoteTimestamp()),
            t('ai.note_model', '模型：${meta}').replace('${meta}', this.turnMeta(turn)),
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
            `# ${t('ai.export_title', 'AI伴读对话导出')}`,
            '',
            `- ${t('ai.export_paper', '文献：${title}').replace('${title}', paper?.title || '')}`,
            `- ${t('ai.export_file', '原始文件：${filename}').replace('${filename}', paper?.original_filename || '')}`,
            `- ${t('ai.export_time', '导出时间：${time}').replace('${time}', this.formatNoteTimestamp())}`,
            `- ${t('ai.export_rounds', '对话轮数：${count}').replace('${count}', turns.length)}`,
            ''
        ];

        const rounds = turns.map((turn, index) => [
            `# ${t('ai.export_round_heading', '第 ${n} 轮').replace('${n}', index + 1)}`,
            '',
            `## ${t('ai.export_user_question', '用户提问')}`,
            String(turn.question || this.questionPlaceholder()).trim(),
            '',
            `## ${t('ai.export_ai_answer', 'AI 回答')}`,
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
        this.renderCurrentPaperBar();
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
