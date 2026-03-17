const SettingsPage = {
    init() {
        if (this.initialized) return;
        this.initialized = true;

        this.aiSettingsForm = document.getElementById('aiSettingsForm');
        this.aiModelList = document.getElementById('aiModelList');
        this.addAIModelButton = document.getElementById('addAIModelButton');
        this.defaultModelSelect = document.getElementById('aiDefaultModelSelect');
        this.qaModelSelect = document.getElementById('aiQAModelSelect');
        this.figureModelSelect = document.getElementById('aiFigureModelSelect');
        this.tagModelSelect = document.getElementById('aiTagModelSelect');
        this.groupModelSelect = document.getElementById('aiGroupModelSelect');
        this.temperatureInput = document.getElementById('aiTemperatureInput');
        this.maxFiguresInput = document.getElementById('aiMaxFiguresInput');
        this.systemPromptInput = document.getElementById('aiSystemPromptInput');
        this.qaPromptInput = document.getElementById('aiQAPromptInput');
        this.figurePromptInput = document.getElementById('aiFigurePromptInput');
        this.tagPromptInput = document.getElementById('aiTagPromptInput');
        this.groupPromptInput = document.getElementById('aiGroupPromptInput');
        this.restoreAIPromptsButton = document.getElementById('restoreAIPromptsButton');
        this.aiModelModal = document.getElementById('aiModelModal');
        this.closeAIModelModalButton = document.getElementById('closeAIModelModal');
        this.aiModelModalTitle = document.getElementById('aiModelModalTitle');
        this.aiModelEditorForm = document.getElementById('aiModelEditorForm');
        this.aiModelNameInput = document.getElementById('aiModelNameInput');
        this.aiModelProviderInput = document.getElementById('aiModelProviderInput');
        this.aiModelIdentifierInput = document.getElementById('aiModelIdentifierInput');
        this.aiModelMaxTokensInput = document.getElementById('aiModelMaxTokensInput');
        this.aiModelBaseURLInput = document.getElementById('aiModelBaseURLInput');
        this.aiModelAPIKeyInput = document.getElementById('aiModelAPIKeyInput');
        this.aiModelLegacyModeInput = document.getElementById('aiModelLegacyModeInput');
        this.aiModelProviderNote = document.getElementById('aiModelProviderNote');
        this.aiModelCheckStatus = document.getElementById('aiModelCheckStatus');
        this.checkAIModelButton = document.getElementById('checkAIModelButton');
        this.deleteAIModelButton = document.getElementById('deleteAIModelButton');

        this.extractorSettingsForm = document.getElementById('extractorSettingsForm');
        this.extractorURLInput = document.getElementById('extractorURLInput');
        this.extractorTokenInput = document.getElementById('extractorTokenInput');
        this.extractorFileFieldInput = document.getElementById('extractorFileFieldInput');
        this.extractorTimeoutInput = document.getElementById('extractorTimeoutInput');
        this.extractorPollIntervalInput = document.getElementById('extractorPollIntervalInput');
        this.extractorSummary = document.getElementById('extractorSummary');

        this.exportDbButton = document.getElementById('exportDbButton');
        this.importDbForm = document.getElementById('importDbForm');
        this.importDbFile = document.getElementById('importDbFile');
        this.purgeDbButton = document.getElementById('purgeDbButton');
        this.changePasswordForm = document.getElementById('changePasswordForm');
        this.currentPasswordInput = document.getElementById('currentPassword');
        this.newPasswordInput = document.getElementById('newPassword');
        this.confirmPasswordInput = document.getElementById('confirmPassword');
        this.logoutButton = document.getElementById('logoutButton');

        this.bindEvents();
        this.bootstrap();
    },

    bindEvents() {
        this.aiSettingsForm.addEventListener('submit', async (event) => {
            event.preventDefault();
            await this.saveAISettings();
        });
        this.addAIModelButton.addEventListener('click', () => this.addAIModel());
        this.restoreAIPromptsButton.addEventListener('click', async () => {
            await this.restoreRecommendedPrompts();
        });
        this.aiModelList.addEventListener('click', (event) => {
            const button = event.target.closest('[data-model-id]');
            if (!button) return;
            this.openAIModelEditor(button.dataset.modelId);
        });
        this.aiModelEditorForm.addEventListener('submit', async (event) => {
            event.preventDefault();
            await this.saveEditedAIModel();
        });
        this.checkAIModelButton.addEventListener('click', async () => {
            await this.checkActiveAIModel();
        });
        this.deleteAIModelButton.addEventListener('click', () => this.deleteCurrentAIModel());
        this.aiModelProviderInput.addEventListener('change', () => this.updateAIModelModalUI());
        this.closeAIModelModalButton.addEventListener('click', () => this.closeAIModelModal());
        this.aiModelModal.addEventListener('click', (event) => {
            if (event.target === this.aiModelModal) {
                this.closeAIModelModal();
            }
        });
        this.extractorSettingsForm.addEventListener('submit', async (event) => {
            event.preventDefault();
            await this.saveExtractorSettings();
        });

        this.exportDbButton.addEventListener('click', () => this.exportDatabase());
        this.importDbForm.addEventListener('submit', async (event) => {
            event.preventDefault();
            await this.importDatabase();
        });
        this.purgeDbButton.addEventListener('click', () => this.purgeDatabase());
        this.changePasswordForm.addEventListener('submit', async (event) => {
            event.preventDefault();
            await this.changePassword();
        });
        this.logoutButton.addEventListener('click', () => this.logout());
        document.addEventListener('keydown', (event) => {
            if (event.key === 'Escape' && this.aiModelModal && !this.aiModelModal.classList.contains('hidden')) {
                this.closeAIModelModal();
            }
        });
    },

    async bootstrap() {
        try {
            await Promise.all([this.loadAISettings(), this.loadExtractorSettings()]);
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async loadAISettings() {
        const settings = await API.getAISettings();

        this.aiModelDraft = Array.isArray(settings.models) && settings.models.length
            ? settings.models.map((item) => ({ ...item }))
            : [this.createAIModelDraft()];
        this.temperatureInput.value = settings.temperature ?? 0.2;
        this.maxFiguresInput.value = settings.max_figures ?? 0;
        this.systemPromptInput.value = settings.system_prompt || '';
        this.qaPromptInput.value = settings.qa_prompt || '';
        this.figurePromptInput.value = settings.figure_prompt || '';
        this.tagPromptInput.value = settings.tag_prompt || '';
        this.groupPromptInput.value = settings.group_prompt || '';

        this.renderAIModels();
        this.renderSceneModelSelectors(settings.scene_models || {});
    },

    buildAISettingsPayload(options = {}) {
        const { models = this.aiModelDraft, sceneModels = this.readSceneModelSelections() } = options;
        return {
            models: this.getAIPayloadModels(models),
            scene_models: sceneModels,
            temperature: this.temperatureInput.value === '' ? 0.2 : Number(this.temperatureInput.value),
            max_figures: Number(this.maxFiguresInput.value || 0),
            system_prompt: this.systemPromptInput.value.trim(),
            qa_prompt: this.qaPromptInput.value.trim(),
            figure_prompt: this.figurePromptInput.value.trim(),
            tag_prompt: this.tagPromptInput.value.trim(),
            group_prompt: this.groupPromptInput.value.trim()
        };
    },

    async persistAISettings(payload, successMessage) {
        await API.updateAISettings(payload);
        await this.loadAISettings();
        if (successMessage) {
            Utils.showToast(successMessage);
        }
    },

    async saveAISettings() {
        const payload = this.buildAISettingsPayload();

        await this.persistAISettings(payload, 'AI 配置已保存');
    },

    async restoreRecommendedPrompts() {
        const button = this.restoreAIPromptsButton;
        const originalLabel = button?.textContent || '';
        if (button) {
            button.disabled = true;
            button.textContent = '载入中...';
        }

        try {
            const defaults = await API.getDefaultAISettings();
            this.systemPromptInput.value = defaults.system_prompt || '';
            this.qaPromptInput.value = defaults.qa_prompt || '';
            this.figurePromptInput.value = defaults.figure_prompt || '';
            this.tagPromptInput.value = defaults.tag_prompt || '';
            this.groupPromptInput.value = defaults.group_prompt || '';
            Utils.showToast('已恢复推荐 Prompt，记得点击“保存 AI 配置”');
        } catch (error) {
            Utils.showToast(error.message, 'error');
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = originalLabel || '恢复推荐 Prompt';
            }
        }
    },

    async loadExtractorSettings() {
        const settings = await API.getExtractorSettings();
        const extractorURLValue = this.extractorAddressValue(settings.extractor_url || '');

        this.extractorURLInput.value = extractorURLValue;
        this.extractorTokenInput.value = settings.extractor_token || '';
        this.extractorFileFieldInput.value = settings.extractor_file_field || 'file';
        this.extractorTimeoutInput.value = settings.timeout_seconds ?? 300;
        this.extractorPollIntervalInput.value = settings.poll_interval_seconds ?? 2;

        this.renderExtractorSummary(settings);
    },

    async saveExtractorSettings() {
        const payload = {
            extractor_url: this.extractorURLInput.value.trim(),
            extractor_jobs_url: '',
            extractor_token: this.extractorTokenInput.value.trim(),
            extractor_file_field: this.extractorFileFieldInput.value.trim(),
            timeout_seconds: Number(this.extractorTimeoutInput.value || 300),
            poll_interval_seconds: Number(this.extractorPollIntervalInput.value || 2)
        };

        const response = await API.updateExtractorSettings(payload);
        this.renderExtractorSummary(response.settings);
        Utils.showToast('PDF 提取服务配置已保存');
    },

    providerNoteText(provider) {
        const notes = {
            openai: 'OpenAI 默认使用 Responses API。勾选传统模式后会切到 Chat Completions，以兼容多数 OpenAI 风格网关。',
            anthropic: 'Anthropic 使用原生 Messages API，请填写兼容的 Base URL 和模型名。',
            gemini: 'Gemini 使用 generateContent 接口，API Key 会通过 query 参数发送。'
        };
        return notes[provider] || '';
    },

    createAIModelDraft() {
        const suffix = `${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
        return {
            id: `model_${suffix}`,
            name: '',
            provider: 'openai',
            model: '',
            base_url: '',
            api_key: '',
            max_output_tokens: 1200,
            openai_legacy_mode: false,
            check_status: ''
        };
    },

    renderAIModels() {
        if (!this.aiModelDraft.length) {
            this.aiModelList.innerHTML = '<p class="muted">还没有模型，先新增一个。</p>';
            return;
        }

        this.aiModelList.innerHTML = this.aiModelDraft.map((item) => `
            <button class="ai-model-button ${item.id === this.activeAIModelID ? 'active' : ''}" type="button" data-model-id="${Utils.escapeHTML(item.id)}">
                <strong>${Utils.escapeHTML(this.aiModelButtonTitle(item))}</strong>
                <span>${Utils.escapeHTML(this.aiModelButtonMeta(item))}</span>
            </button>
        `).join('');
    },

    aiModelButtonTitle(model) {
        return model.name || model.model || '未命名模型';
    },

    aiModelButtonMeta(model) {
        const provider = model.provider || 'openai';
        const modelName = model.model || '未填写模型名';
        return `${provider} / ${modelName}`;
    },

    addAIModel() {
        this.openAIModelEditor(this.createAIModelDraft(), { isNew: true });
    },

    openAIModelEditor(target, options = {}) {
        const isNew = Boolean(options.isNew);
        const model = typeof target === 'string'
            ? this.aiModelDraft.find((item) => item.id === target)
            : target;
        if (!model) return;

        this.activeAIModelID = model.id;
        this.renderAIModels();

        this.editingAIModel = { ...model };
        this.editingAIModelIsNew = isNew;
        this.aiModelModalTitle.textContent = isNew ? '新增模型' : `编辑模型 · ${this.aiModelButtonTitle(model)}`;
        this.aiModelNameInput.value = model.name || '';
        this.aiModelProviderInput.value = model.provider || 'openai';
        this.aiModelIdentifierInput.value = model.model || '';
        this.aiModelMaxTokensInput.value = Number(model.max_output_tokens || 1200);
        this.aiModelBaseURLInput.value = model.base_url || '';
        this.aiModelAPIKeyInput.value = model.api_key || '';
        this.aiModelLegacyModeInput.checked = Boolean(model.openai_legacy_mode);
        this.aiModelCheckStatus.textContent = model.check_status || '尚未检查';
        this.deleteAIModelButton.disabled = isNew || this.aiModelDraft.length <= 1;
        this.updateAIModelModalUI();
        this.aiModelModal.classList.remove('hidden');
        document.body.classList.add('modal-open');
    },

    closeAIModelModal() {
        if (!this.aiModelModal) return;
        this.aiModelModal.classList.add('hidden');
        document.body.classList.remove('modal-open');
        this.editingAIModel = null;
        this.editingAIModelIsNew = false;
        this.activeAIModelID = '';
        this.renderAIModels();
    },

    updateAIModelModalUI() {
        const provider = this.aiModelProviderInput.value || 'openai';
        this.aiModelProviderNote.textContent = this.providerNoteText(provider);
        const legacyEnabled = provider === 'openai';
        this.aiModelLegacyModeInput.disabled = !legacyEnabled;
        if (!legacyEnabled) {
            this.aiModelLegacyModeInput.checked = false;
        }
    },

    readAIModelFromModal() {
        const model = {
            ...(this.editingAIModel || this.createAIModelDraft()),
            name: this.aiModelNameInput.value.trim(),
            provider: this.aiModelProviderInput.value,
            model: this.aiModelIdentifierInput.value.trim(),
            max_output_tokens: Number(this.aiModelMaxTokensInput.value || 1200),
            base_url: this.aiModelBaseURLInput.value.trim(),
            api_key: this.aiModelAPIKeyInput.value.trim(),
            openai_legacy_mode: this.aiModelLegacyModeInput.checked,
            check_status: this.aiModelCheckStatus.textContent.trim()
        };
        if (model.provider !== 'openai') {
            model.openai_legacy_mode = false;
        }
        return model;
    },

    async saveEditedAIModel() {
        const model = this.readAIModelFromModal();
        const nextModels = this.editingAIModelIsNew
            ? [...this.aiModelDraft, model]
            : this.aiModelDraft.map((item) => item.id === model.id ? model : item);
        const selection = this.readSceneModelSelections();
        const payload = this.buildAISettingsPayload({
            models: nextModels,
            sceneModels: selection
        });

        await this.persistAISettings(payload, '模型已保存');
        if (!this.aiModelModal.classList.contains('hidden')) {
            this.closeAIModelModal();
        }
    },

    async deleteAIModel(modelID) {
        if (this.aiModelDraft.length <= 1) {
            Utils.showToast('至少需要保留一个 AI 模型', 'error');
            return;
        }
        const selection = this.readSceneModelSelections();
        const nextModels = this.aiModelDraft.filter((item) => item.id !== modelID);
        const payload = this.buildAISettingsPayload({
            models: nextModels,
            sceneModels: selection
        });

        await this.persistAISettings(payload, '模型已删除');
    },

    async deleteCurrentAIModel() {
        if (!this.editingAIModel?.id) return;
        if (this.aiModelDraft.length <= 1) {
            Utils.showToast('至少需要保留一个 AI 模型', 'error');
            return;
        }

        await this.deleteAIModel(this.editingAIModel.id);
        if (!this.aiModelModal.classList.contains('hidden')) {
            this.closeAIModelModal();
        }
    },

    renderSceneModelSelectors(selection = {}) {
        const safeSelection = {
            default_model_id: selection.default_model_id || this.defaultModelSelect?.value || '',
            qa_model_id: selection.qa_model_id || this.qaModelSelect?.value || '',
            figure_model_id: selection.figure_model_id || this.figureModelSelect?.value || '',
            tag_model_id: selection.tag_model_id || this.tagModelSelect?.value || '',
            group_model_id: selection.group_model_id || this.groupModelSelect?.value || ''
        };
        const options = this.aiModelDraft.map((item) => {
            const label = `${item.name || '未命名模型'} · ${item.provider || 'openai'} / ${item.model || '未填写模型名'}`;
            return `<option value="${Utils.escapeHTML(item.id)}">${Utils.escapeHTML(label)}</option>`;
        }).join('');

        [
            [this.defaultModelSelect, safeSelection.default_model_id],
            [this.qaModelSelect, safeSelection.qa_model_id],
            [this.figureModelSelect, safeSelection.figure_model_id],
            [this.tagModelSelect, safeSelection.tag_model_id],
            [this.groupModelSelect, safeSelection.group_model_id]
        ].forEach(([element, selectedValue], index) => {
            if (!element) return;
            element.innerHTML = options;
            const fallback = this.aiModelDraft[0]?.id || '';
            element.value = selectedValue && this.aiModelDraft.some((item) => item.id === selectedValue)
                ? selectedValue
                : (index === 0 ? fallback : (this.defaultModelSelect?.value || fallback));
        });
    },

    readSceneModelSelections() {
        const fallback = this.aiModelDraft[0]?.id || '';
        const defaultModelID = this.defaultModelSelect?.value || fallback;
        return {
            default_model_id: defaultModelID,
            qa_model_id: this.qaModelSelect?.value || defaultModelID,
            figure_model_id: this.figureModelSelect?.value || defaultModelID,
            tag_model_id: this.tagModelSelect?.value || defaultModelID,
            group_model_id: this.groupModelSelect?.value || defaultModelID
        };
    },

    getAIPayloadModels(models = this.aiModelDraft) {
        return models.map((item) => ({
            id: item.id,
            name: item.name || '',
            provider: item.provider,
            model: item.model || '',
            max_output_tokens: Number(item.max_output_tokens || 1200),
            base_url: item.base_url || '',
            api_key: item.api_key || '',
            openai_legacy_mode: Boolean(item.openai_legacy_mode)
        }));
    },

    async checkActiveAIModel() {
        const originalLabel = this.checkAIModelButton.textContent;
        this.checkAIModelButton.disabled = true;
        this.checkAIModelButton.textContent = '检查中...';

        try {
            const model = this.readAIModelFromModal();
            const result = await API.checkAIModel({
                id: model.id,
                name: model.name,
                provider: model.provider,
                model: model.model,
                max_output_tokens: Number(model.max_output_tokens || 1200),
                base_url: model.base_url,
                api_key: model.api_key,
                openai_legacy_mode: model.openai_legacy_mode
            });
            const statusText = `${result.message} · ${result.provider} / ${result.model} / ${result.mode}`;
            this.aiModelCheckStatus.textContent = statusText;
            Utils.showToast('模型检查通过');
        } catch (error) {
            this.aiModelCheckStatus.textContent = `检查失败：${error.message}`;
            Utils.showToast(error.message, 'error');
        } finally {
            this.checkAIModelButton.disabled = false;
            this.checkAIModelButton.textContent = originalLabel;
        }
    },

    renderExtractorSummary(settings) {
        const extractURL = settings.effective_extractor_url || '未配置';
        const jobsURL = settings.effective_jobs_url || '未配置';
        const tokenLabel = settings.extractor_token ? '已配置' : '未配置';

        this.extractorSummary.innerHTML = `
            <div><span>生效的提取接口</span><strong class="settings-url-value">${Utils.escapeHTML(extractURL)}</strong></div>
            <div><span>生效的任务接口</span><strong class="settings-url-value">${Utils.escapeHTML(jobsURL)}</strong></div>
            <div><span>上传字段名</span><strong>${Utils.escapeHTML(settings.extractor_file_field || 'file')}</strong></div>
            <div><span>鉴权 Token</span><strong>${Utils.escapeHTML(tokenLabel)}</strong></div>
        `;
    },

    extractorAddressValue(rawURL) {
        const value = (rawURL || '').trim();
        if (!value) return '';

        if (value.endsWith('/api/v1/extract')) {
            return value.slice(0, -'/api/v1/extract'.length).replace(/\/$/, '');
        }

        return value;
    },

    exportDatabase() {
        const link = document.createElement('a');
        link.href = '/api/database/export';
        link.download = `library_backup_${new Date().toISOString().slice(0, 10)}.db`;
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        Utils.showToast('数据库导出开始');
    },

    async importDatabase() {
        const file = this.importDbFile.files[0];
        if (!file) {
            Utils.showToast('请选择要导入的数据库文件', 'error');
            return;
        }

        const confirmed = await Utils.confirmTypedAction({
            title: '覆盖导入数据库',
            badge: 'Import Override',
            message: '导入数据库会用备份文件覆盖当前所有文献、图片、分组和标签。确认后将立即开始恢复。',
            keyword: 'IMPORT',
            hint: '请输入 IMPORT 继续导入',
            confirmLabel: '开始导入'
        });
        if (!confirmed) return;

        try {
            const formData = new FormData();
            formData.append('database', file);
            await API.importDatabase(formData);
            Utils.showToast('数据库导入成功，页面将刷新');
            setTimeout(() => window.location.reload(), 1500);
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async purgeDatabase() {
        const confirmed = await Utils.confirmTypedAction({
            title: '清空数据库',
            badge: 'Danger Zone',
            message: '这会删除所有文献、提取图片、分组和标签，并且不可恢复。该操作只适合在你明确要重置整个库时使用。',
            keyword: 'CLEAR',
            hint: '请输入 CLEAR 继续清空数据库',
            confirmLabel: '确认清空'
        });
        if (!confirmed) return;

        try {
            await API.purgeLibrary();
            Utils.showToast('数据库已清空');
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async changePassword() {
        const currentPassword = this.currentPasswordInput.value.trim();
        const newPassword = this.newPasswordInput.value.trim();
        const confirmPassword = this.confirmPasswordInput.value.trim();

        if (!currentPassword || !newPassword || !confirmPassword) {
            Utils.showToast('请填写所有密码字段', 'error');
            return;
        }

        if (newPassword.length < 6) {
            Utils.showToast('新密码长度不能少于 6 位', 'error');
            return;
        }

        if (newPassword !== confirmPassword) {
            Utils.showToast('两次输入的新密码不一致', 'error');
            return;
        }

        try {
            await API.changePassword({
                current_password: currentPassword,
                new_password: newPassword
            });
            Utils.showToast('密码修改成功，请使用新密码重新登录');
            // 清空表单
            this.currentPasswordInput.value = '';
            this.newPasswordInput.value = '';
            this.confirmPasswordInput.value = '';
            // 延迟后跳转到登录页
            setTimeout(() => {
                window.location.href = '/login';
            }, 2000);
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async logout() {
        const confirmed = await Utils.confirm('确定要登出吗？');
        if (!confirmed) return;

        try {
            await API.logout();
        } catch (error) {
            Utils.showToast(error.message, 'error');
            return;
        }

        sessionStorage.removeItem('citebox_auth');
        localStorage.removeItem('citebox_auth');
        localStorage.removeItem('citebox_logged_in');
        localStorage.removeItem('citebox_username');
        localStorage.removeItem('citebox_password');

        Utils.showToast('已登出');
        setTimeout(() => {
            window.location.href = '/login';
        }, 1000);
    }
};
