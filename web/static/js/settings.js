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
        this.translateModelSelect = document.getElementById('aiTranslateModelSelect');
        this.temperatureInput = document.getElementById('aiTemperatureInput');
        this.maxFiguresInput = document.getElementById('aiMaxFiguresInput');
        this.translationPrimaryLanguageInput = document.getElementById('aiTranslationPrimaryLanguageInput');
        this.translationTargetLanguageInput = document.getElementById('aiTranslationTargetLanguageInput');
        this.systemPromptInput = document.getElementById('aiSystemPromptInput');
        this.qaPromptInput = document.getElementById('aiQAPromptInput');
        this.figurePromptInput = document.getElementById('aiFigurePromptInput');
        this.tagPromptInput = document.getElementById('aiTagPromptInput');
        this.groupPromptInput = document.getElementById('aiGroupPromptInput');
        this.translatePromptInput = document.getElementById('aiTranslatePromptInput');
        this.aiModelAutosaveStatus = document.getElementById('aiModelAutosaveStatus');
        this.aiPromptSaveStatus = document.getElementById('aiPromptSaveStatus');
        this.saveAIPromptsButton = document.getElementById('saveAIPromptsButton');
        this.rolePromptList = document.getElementById('aiRolePromptList');
        this.addAIRolePromptButton = document.getElementById('addAIRolePromptButton');
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
        this.aiModelEditorStatus = document.getElementById('aiModelEditorStatus');
        this.checkAIModelButton = document.getElementById('checkAIModelButton');
        this.deleteAIModelButton = document.getElementById('deleteAIModelButton');

        this.extractorSettingsForm = document.getElementById('extractorSettingsForm');
        this.extractorURLInput = document.getElementById('extractorURLInput');
        this.extractorTokenInput = document.getElementById('extractorTokenInput');
        this.extractorFileFieldInput = document.getElementById('extractorFileFieldInput');
        this.extractorTimeoutInput = document.getElementById('extractorTimeoutInput');
        this.extractorPollIntervalInput = document.getElementById('extractorPollIntervalInput');
        this.extractorSummary = document.getElementById('extractorSummary');
        this.wolaiSettingsForm = document.getElementById('wolaiSettingsForm');
        this.wolaiTokenInput = document.getElementById('wolaiTokenInput');
        this.wolaiParentBlockIDInput = document.getElementById('wolaiParentBlockIDInput');
        this.wolaiBaseURLInput = document.getElementById('wolaiBaseURLInput');
        this.wolaiSummary = document.getElementById('wolaiSummary');
        this.wolaiStatus = document.getElementById('wolaiStatus');
        this.testWolaiButton = document.getElementById('testWolaiButton');
        this.versionSummary = document.getElementById('versionSummary');
        this.checkVersionButton = document.getElementById('checkVersionButton');
        this.versionReleaseLink = document.getElementById('versionReleaseLink');

        this.exportDbButton = document.getElementById('exportDbButton');
        this.importDbForm = document.getElementById('importDbForm');
        this.importDbFile = document.getElementById('importDbFile');
        this.purgeDbButton = document.getElementById('purgeDbButton');
        this.changePasswordForm = document.getElementById('changePasswordForm');
        this.currentPasswordInput = document.getElementById('currentPassword');
        this.newPasswordInput = document.getElementById('newPassword');
        this.confirmPasswordInput = document.getElementById('confirmPassword');
        this.logoutButton = document.getElementById('logoutButton');
        this.weixinBindingSummary = document.getElementById('weixinBindingSummary');
        this.weixinQRCodePanel = document.getElementById('weixinQRCodePanel');
        this.weixinQRCodeImage = document.getElementById('weixinQRCodeImage');
        this.weixinQRCodeLink = document.getElementById('weixinQRCodeLink');
        this.weixinBindingStatus = document.getElementById('weixinBindingStatus');
        this.weixinBridgeEnabledInput = document.getElementById('weixinBridgeEnabledInput');
        this.weixinBridgeSaveStatus = document.getElementById('weixinBridgeSaveStatus');
        this.saveWeixinBridgeButton = document.getElementById('saveWeixinBridgeButton');
        this.startWeixinBindingButton = document.getElementById('startWeixinBindingButton');
        this.unbindWeixinButton = document.getElementById('unbindWeixinButton');

        this.bindEvents();
        this.bootstrap();
    },

    bindEvents() {
        this.aiSettingsForm.addEventListener('submit', (event) => {
            event.preventDefault();
        });
        this.addAIModelButton.addEventListener('click', async () => {
            await this.addAIModel();
        });
        this.restoreAIPromptsButton.addEventListener('click', async () => {
            await this.restoreRecommendedPrompts();
        });
        this.saveAIPromptsButton.addEventListener('click', async () => {
            await this.savePromptSettings();
        });
        this.addAIRolePromptButton.addEventListener('click', async () => {
            await this.openAIRolePromptEditor();
        });
        this.rolePromptList.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-role-prompt-action]');
            if (!button) return;

            const rolePromptIndex = Number(button.dataset.rolePromptIndex);
            if (!Number.isInteger(rolePromptIndex) || rolePromptIndex < 0) return;

            if (button.dataset.rolePromptAction === 'edit') {
                await this.openAIRolePromptEditor(rolePromptIndex);
                return;
            }
            if (button.dataset.rolePromptAction === 'delete') {
                await this.deleteAIRolePrompt(rolePromptIndex);
            }
        });
        this.aiModelList.addEventListener('click', (event) => {
            const button = event.target.closest('[data-model-id]');
            if (!button) return;
            this.openAIModelEditor(button.dataset.modelId);
        });
        this.aiModelEditorForm.addEventListener('submit', async (event) => {
            event.preventDefault();
            await this.flushAIModelAutosave();
        });
        this.checkAIModelButton.addEventListener('click', async () => {
            await this.checkActiveAIModel();
        });
        this.deleteAIModelButton.addEventListener('click', async () => {
            await this.deleteCurrentAIModel();
        });
        this.aiModelProviderInput.addEventListener('change', () => {
            this.updateAIModelModalUI();
            this.scheduleAIModelAutosave({ immediate: true });
        });
        this.closeAIModelModalButton.addEventListener('click', async () => {
            await this.closeAIModelModal();
        });
        this.aiModelModal.addEventListener('click', (event) => {
            if (event.target === this.aiModelModal) {
                this.closeAIModelModal();
            }
        });
        this.bindAIModelAutoSaveInputs();
        this.bindAIModelEditorAutoSaveInputs();
        [
            this.systemPromptInput,
            this.qaPromptInput,
            this.figurePromptInput,
            this.tagPromptInput,
            this.groupPromptInput,
            this.translatePromptInput
        ].forEach((element) => {
            element?.addEventListener('input', () => {
                if (this.isHydratingAISettings) return;
                this.setAIPromptSaveStatus('Prompt 已修改，点击“保存 Prompt 配置”后生效。', 'saving');
            });
        });
        this.extractorSettingsForm.addEventListener('submit', async (event) => {
            event.preventDefault();
            await this.saveExtractorSettings();
        });
        this.wolaiSettingsForm?.addEventListener('submit', async (event) => {
            event.preventDefault();
            await this.saveWolaiSettings();
        });
        this.testWolaiButton?.addEventListener('click', async () => {
            await this.testWolaiSettings();
        });
        this.checkVersionButton.addEventListener('click', async () => {
            await this.loadVersionStatus(true);
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
        this.weixinBridgeEnabledInput?.addEventListener('change', () => {
            this.setWeixinBridgeSaveStatus('桥接配置已修改，点击“保存微信桥接配置”后生效。', 'saving');
        });
        this.saveWeixinBridgeButton?.addEventListener('click', async () => {
            await this.saveWeixinBridgeSettings();
        });
        this.startWeixinBindingButton.addEventListener('click', async () => {
            await this.startWeixinBinding();
        });
        this.unbindWeixinButton?.addEventListener('click', async () => {
            await this.unbindWeixin();
        });
        document.addEventListener('keydown', (event) => {
            if (event.key === 'Escape' && this.aiModelModal && !this.aiModelModal.classList.contains('hidden')) {
                event.preventDefault();
                event.stopPropagation();
                this.closeAIModelModal();
            }
        });
    },

    bindAIModelAutoSaveInputs() {
        [
            this.defaultModelSelect,
            this.qaModelSelect,
            this.figureModelSelect,
            this.tagModelSelect,
            this.groupModelSelect,
            this.translateModelSelect
        ].forEach((element) => {
            element?.addEventListener('change', () => {
                this.scheduleAIModelAutosave({ immediate: true });
            });
        });

        [
            this.temperatureInput,
            this.maxFiguresInput,
            this.translationPrimaryLanguageInput,
            this.translationTargetLanguageInput
        ].forEach((element) => {
            element?.addEventListener('input', () => {
                this.scheduleAIModelAutosave();
            });
            element?.addEventListener('change', () => {
                this.scheduleAIModelAutosave({ immediate: true });
            });
        });
    },

    bindAIModelEditorAutoSaveInputs() {
        [
            this.aiModelNameInput,
            this.aiModelProviderInput,
            this.aiModelIdentifierInput,
            this.aiModelMaxTokensInput,
            this.aiModelBaseURLInput,
            this.aiModelAPIKeyInput,
            this.aiModelLegacyModeInput
        ].forEach((element) => {
            if (!element) return;
            element.addEventListener('input', () => {
                if (this.isHydratingAIModelEditor) return;
                this.scheduleAIModelAutosave();
            });
            element.addEventListener('change', () => {
                if (this.isHydratingAIModelEditor) return;
                this.scheduleAIModelAutosave({ immediate: true });
            });
        });
    },

    async bootstrap() {
        try {
            await Promise.all([
                this.loadAISettings(),
                this.loadExtractorSettings(),
                this.loadWolaiSettings(),
                this.loadVersionStatus(),
                this.loadAuthSettings()
            ]);
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async loadAuthSettings() {
        const settings = await API.getAuthSettings();
        this.renderAuthSettings(settings || {});
    },

    async loadAISettings() {
        const settings = await API.getAISettings();
        this.applyAISettings(settings, {
            overwritePromptInputs: true,
            overwriteRolePrompts: true
        });
        this.setAIModelAutosaveStatus('模型配置修改后会自动保存。');
        this.setAIPromptSaveStatus('Prompt 修改后需要单独点击保存。');
    },

    applyAISettings(settings = {}, options = {}) {
        const {
            overwritePromptInputs = true,
            overwriteRolePrompts = true
        } = options;

        this.isHydratingAISettings = true;
        this.aiModelDraft = Array.isArray(settings.models) && settings.models.length
            ? settings.models.map((item) => ({ ...item }))
            : [this.createAIModelDraft()];
        this.temperatureInput.value = settings.temperature ?? 0.2;
        this.maxFiguresInput.value = settings.max_figures ?? 0;
        this.translationPrimaryLanguageInput.value = settings.translation?.primary_language || '中文';
        this.translationTargetLanguageInput.value = settings.translation?.target_language || '英文';

        if (overwritePromptInputs) {
            this.applyPromptSettingsToInputs(settings);
        }
        this.savedPromptSettings = this.extractPromptSettings(settings);
        if (overwriteRolePrompts) {
            this.rolePromptDraft = Array.isArray(settings.role_prompts)
                ? settings.role_prompts.map((item) => ({ ...item }))
                : [];
        }

        this.renderAIModels();
        this.renderSceneModelSelectors(settings.scene_models || {});
        this.renderRolePromptList();
        this.isHydratingAISettings = false;
    },

    applyPromptSettingsToInputs(settings = {}) {
        this.systemPromptInput.value = settings.system_prompt || '';
        this.qaPromptInput.value = settings.qa_prompt || '';
        this.figurePromptInput.value = settings.figure_prompt || '';
        this.tagPromptInput.value = settings.tag_prompt || '';
        this.groupPromptInput.value = settings.group_prompt || '';
        this.translatePromptInput.value = settings.translate_prompt || '';
    },

    extractPromptSettings(settings = {}) {
        return {
            system_prompt: settings.system_prompt || '',
            qa_prompt: settings.qa_prompt || '',
            figure_prompt: settings.figure_prompt || '',
            tag_prompt: settings.tag_prompt || '',
            group_prompt: settings.group_prompt || '',
            translate_prompt: settings.translate_prompt || ''
        };
    },

    buildAIModelSettingsPayload(options = {}) {
        const { models = this.currentAIModelsForSave(), sceneModels = this.readSceneModelSelections() } = options;
        return {
            models: this.getAIPayloadModels(models),
            scene_models: sceneModels,
            temperature: this.temperatureInput.value === '' ? 0.2 : Number(this.temperatureInput.value),
            max_figures: Number(this.maxFiguresInput.value || 0),
            translation: {
                primary_language: this.translationPrimaryLanguageInput.value.trim(),
                target_language: this.translationTargetLanguageInput.value.trim()
            }
        };
    },

    buildAIPromptSettingsPayload() {
        return {
            system_prompt: this.systemPromptInput.value.trim(),
            qa_prompt: this.qaPromptInput.value.trim(),
            figure_prompt: this.figurePromptInput.value.trim(),
            tag_prompt: this.tagPromptInput.value.trim(),
            group_prompt: this.groupPromptInput.value.trim(),
            translate_prompt: this.translatePromptInput.value.trim()
        };
    },

    setInlineStatus(element, message, tone = '') {
        if (!element) return;
        element.textContent = message || '';
        element.classList.remove('is-success', 'is-error', 'is-saving');
        if (tone) {
            element.classList.add(`is-${tone}`);
        }
    },

    setAIModelAutosaveStatus(message, tone = '') {
        this.setInlineStatus(this.aiModelAutosaveStatus, message, tone);
    },

    setAIPromptSaveStatus(message, tone = '') {
        this.setInlineStatus(this.aiPromptSaveStatus, message, tone);
    },

    setAIModelEditorStatus(message, tone = '') {
        this.setInlineStatus(this.aiModelEditorStatus, message, tone);
    },

    scheduleAIModelAutosave(options = {}) {
        if (this.isHydratingAISettings || this.isHydratingAIModelEditor) return;

        const { immediate = false } = options;
        window.clearTimeout(this.aiModelAutosaveTimer);
        this.setAIModelAutosaveStatus(immediate ? '模型配置保存中...' : '检测到修改，正在准备自动保存...', 'saving');
        this.setAIModelEditorStatus('当前模型修改后会自动保存。', 'saving');

        if (immediate) {
            void this.persistAIModelSettings();
            return;
        }

        this.aiModelAutosaveTimer = window.setTimeout(() => {
            this.aiModelAutosaveTimer = 0;
            void this.persistAIModelSettings();
        }, 450);
    },

    async flushAIModelAutosave() {
        window.clearTimeout(this.aiModelAutosaveTimer);
        this.aiModelAutosaveTimer = 0;
        await this.persistAIModelSettings();
    },

    async persistAIModelSettings(options = {}) {
        if (this.isHydratingAISettings) return;

        const { successMessage = '' } = options;
        const requestID = (this.aiModelAutosaveRequestID || 0) + 1;
        this.aiModelAutosaveRequestID = requestID;
        this.setAIModelAutosaveStatus('模型配置保存中...', 'saving');
        this.setAIModelEditorStatus('模型配置保存中...', 'saving');

        try {
            const response = await API.updateAIModelSettings(this.buildAIModelSettingsPayload());
            if (requestID !== this.aiModelAutosaveRequestID) return;
            this.applyAISettings(response.settings || {}, {
                overwritePromptInputs: false,
                overwriteRolePrompts: false
            });
            this.setAIModelAutosaveStatus('模型配置已自动保存。', 'success');
            this.setAIModelEditorStatus('模型配置已自动保存。', 'success');
            if (successMessage) {
                Utils.showToast(successMessage);
            }
        } catch (error) {
            if (requestID !== this.aiModelAutosaveRequestID) return;
            this.setAIModelAutosaveStatus(`自动保存失败：${error.message}`, 'error');
            this.setAIModelEditorStatus(`自动保存失败：${error.message}`, 'error');
            Utils.showToast(error.message, 'error');
        }
    },

    async savePromptSettings() {
        const button = this.saveAIPromptsButton;
        const originalLabel = button?.textContent || '';
        if (button) {
            button.disabled = true;
            button.textContent = '保存中...';
        }

        try {
            const response = await API.updateAIPromptSettings(this.buildAIPromptSettingsPayload());
            this.applyAISettings(response.settings || {}, {
                overwritePromptInputs: true,
                overwriteRolePrompts: false
            });
            this.setAIPromptSaveStatus('Prompt 配置已保存。', 'success');
            Utils.showToast('Prompt 配置已保存');
        } catch (error) {
            this.setAIPromptSaveStatus(`保存失败：${error.message}`, 'error');
            Utils.showToast(error.message, 'error');
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = originalLabel || '保存 Prompt 配置';
            }
        }
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
            this.applyPromptSettingsToInputs(defaults || {});
            this.setAIPromptSaveStatus('已恢复推荐 Prompt，点击“保存 Prompt 配置”后生效。', 'saving');
            Utils.showToast('已恢复推荐 Prompt，记得点击“保存 Prompt 配置”');
        } catch (error) {
            Utils.showToast(error.message, 'error');
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = originalLabel || '恢复推荐 Prompt';
            }
        }
    },

    readCurrentPromptFields() {
        return this.buildAIPromptSettingsPayload();
    },

    rolePromptPayloadFromValues(name, prompt = '') {
        return {
            name: String(name || '').trim(),
            prompt: String(prompt || '').trim()
        };
    },

    rolePromptExcerpt(value, fallback) {
        const normalized = String(value || '').replace(/\s+/g, ' ').trim();
        if (!normalized) return fallback;
        return normalized.length > 72 ? `${normalized.slice(0, 72)}...` : normalized;
    },

    renderRolePromptList() {
        if (!this.rolePromptList) return;

        const rolePrompts = Array.isArray(this.rolePromptDraft) ? this.rolePromptDraft : [];
        if (!rolePrompts.length) {
            this.rolePromptList.innerHTML = '<div class="prompt-preset-empty">还没有保存角色 Prompt。新增一个角色后，就可以在 AI 伴读聊天框里通过 <code>@角色名</code> 直接调用。</div>';
            return;
        }

        this.rolePromptList.innerHTML = rolePrompts.map((rolePrompt, index) => {
            return `
                <article class="prompt-preset-card">
                    <div class="prompt-preset-card-head">
                        <div>
                            <strong>${Utils.escapeHTML(rolePrompt.name || '未命名角色')}</strong>
                            <span>聊天时输入 ${Utils.escapeHTML(`@${rolePrompt.name || '角色名'}`)} 即可调用</span>
                        </div>
                    </div>
                    <div class="prompt-preset-preview">
                        <div class="prompt-preset-preview-item">
                            <span>角色 Prompt</span>
                            <p>${Utils.escapeHTML(this.rolePromptExcerpt(rolePrompt.prompt, '未填写角色 Prompt'))}</p>
                        </div>
                    </div>
                    <div class="prompt-preset-card-actions">
                        <button class="btn btn-outline btn-small" type="button" data-role-prompt-action="edit" data-role-prompt-index="${index}">编辑</button>
                        <button class="btn btn-outline btn-small danger" type="button" data-role-prompt-action="delete" data-role-prompt-index="${index}">删除</button>
                    </div>
                </article>
            `;
        }).join('');
    },

    async openAIRolePromptEditor(index = -1) {
        const current = index >= 0 ? this.rolePromptDraft?.[index] : null;
        const values = await Utils.promptFields({
            title: current ? `编辑角色 Prompt · ${current.name}` : '新建角色 Prompt',
            description: '角色 Prompt 只保存角色信息。保存后可在 AI 伴读聊天框中输入 @角色名 直接调用。',
            confirmLabel: current ? '保存角色' : '创建角色',
            fields: [
                {
                    name: 'name',
                    label: '角色名称',
                    placeholder: '例如：严格证据模式',
                    value: current?.name || '',
                    required: true
                },
                {
                    name: 'prompt',
                    label: '角色 Prompt',
                    type: 'textarea',
                    rows: 8,
                    placeholder: '例如：你是一名严格的论文审稿人，优先指出证据链、方法缺口和结论边界。',
                    value: current?.prompt || '',
                    required: true
                }
            ]
        });
        if (!values) return;

        const roleName = String(values.name || '').trim();
        const rolePrompt = String(values.prompt || '').trim();
        if (!roleName || !rolePrompt) return;

        const nextRolePrompt = this.rolePromptPayloadFromValues(roleName, rolePrompt);
        const existingIndex = this.findAIRolePromptIndexByName(roleName);
        const editingCurrent = index >= 0;
        let nextRolePrompts = [...(this.rolePromptDraft || [])];
        let successMessage = editingCurrent ? `已更新角色：${roleName}` : `已保存角色：${roleName}`;

        if (existingIndex >= 0 && existingIndex !== index) {
            const confirmed = await Utils.confirm(`已存在同名角色“${Utils.escapeHTML(roleName)}”，是否覆盖？`, '覆盖角色 Prompt');
            if (!confirmed) return;
            nextRolePrompts.splice(existingIndex, 1, nextRolePrompt);
            if (editingCurrent && index >= 0 && index > existingIndex) {
                nextRolePrompts.splice(index, 1);
            }
            successMessage = `已更新角色：${roleName}`;
        } else if (editingCurrent && index >= 0) {
            nextRolePrompts.splice(index, 1, nextRolePrompt);
        } else {
            nextRolePrompts = [nextRolePrompt, ...nextRolePrompts];
        }

        await this.persistAIRolePrompts(nextRolePrompts, successMessage);
    },

    async deleteAIRolePrompt(index) {
        const rolePrompt = this.rolePromptDraft?.[index];
        if (!rolePrompt) return;

        const confirmed = await Utils.confirm(`确定要删除角色“${Utils.escapeHTML(rolePrompt.name)}”吗？`, '删除角色 Prompt');
        if (!confirmed) return;

        const nextRolePrompts = this.rolePromptDraft.filter((_, itemIndex) => itemIndex !== index);
        await this.persistAIRolePrompts(nextRolePrompts, `已删除角色：${rolePrompt.name}`);
    },

    findAIRolePromptIndexByName(name) {
        const normalized = String(name || '').trim().toLowerCase();
        if (!normalized) return -1;
        return (this.rolePromptDraft || []).findIndex((item) => String(item.name || '').trim().toLowerCase() === normalized);
    },

    async persistAIRolePrompts(nextRolePrompts, successMessage) {
        const response = await API.updateAIRolePrompts({
            role_prompts: nextRolePrompts.map((item) => this.rolePromptPayloadFromValues(item.name, item.prompt))
        });
        this.rolePromptDraft = Array.isArray(response.role_prompts)
            ? response.role_prompts.map((item) => ({ ...item }))
            : [];
        this.renderRolePromptList();
        Utils.showToast(successMessage);
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

    async loadWolaiSettings() {
        const settings = await API.getWolaiSettings();

        if (this.wolaiTokenInput) {
            this.wolaiTokenInput.value = settings.token || '';
        }
        if (this.wolaiParentBlockIDInput) {
            this.wolaiParentBlockIDInput.value = settings.parent_block_id || '';
        }
        if (this.wolaiBaseURLInput) {
            this.wolaiBaseURLInput.value = settings.base_url || 'https://openapi.wolai.com';
        }

        this.renderWolaiSummary(settings);
        this.setWolaiStatus('');
    },

    async loadVersionStatus(forceRefresh = false) {
        const button = this.checkVersionButton;
        const originalLabel = button?.textContent || '';
        if (button) {
            button.disabled = true;
            button.textContent = forceRefresh ? '检查中...' : '载入中...';
        }

        try {
            const status = await API.getVersionStatus(forceRefresh);
            this.renderVersionSummary(status);
            if (forceRefresh) {
                const toastMessage = status.has_update
                    ? `发现新版本 ${status.latest_version || ''}`.trim()
                    : (status.message || '版本检查已完成');
                Utils.showToast(toastMessage);
            }
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = originalLabel || '检查更新';
            }
        }
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

    wolaiSettingsPayload() {
        return {
            token: this.wolaiTokenInput?.value.trim() || '',
            parent_block_id: this.wolaiParentBlockIDInput?.value.trim() || '',
            base_url: this.wolaiBaseURLInput?.value.trim() || 'https://openapi.wolai.com'
        };
    },

    async saveWolaiSettings() {
        const payload = this.wolaiSettingsPayload();
        const response = await API.updateWolaiSettings(payload);
        this.renderWolaiSummary(response.settings || payload);
        this.setWolaiStatus('Wolai 配置已保存。', 'success');
        Utils.showToast('Wolai 配置已保存');
    },

    async testWolaiSettings() {
        const button = this.testWolaiButton;
        const originalLabel = button?.textContent || '测试 Token';
        if (button) {
            button.disabled = true;
            button.textContent = '测试中...';
        }

        this.setWolaiStatus('正在校验 Wolai token 与目标块访问权限...', 'saving');

        try {
            const result = await API.testWolaiSettings(this.wolaiSettingsPayload());
            this.setWolaiStatus(result.message || 'Wolai token 可用', 'success');
            Utils.showToast(result.message || 'Wolai token 可用');
        } catch (error) {
            this.setWolaiStatus(error.message, 'error');
            Utils.showToast(error.message, 'error');
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = originalLabel;
            }
        }
    },

    async saveWeixinBridgeSettings() {
        const button = this.saveWeixinBridgeButton;
        const originalLabel = button?.textContent || '保存微信桥接配置';
        if (button) {
            button.disabled = true;
            button.textContent = '保存中...';
        }

        try {
            const response = await API.updateWeixinBridgeSettings({
                enabled: Boolean(this.weixinBridgeEnabledInput?.checked)
            });
            this.authSettings = {
                ...(this.authSettings || {}),
                weixin_bridge: response.settings || {}
            };
            this.renderAuthSettings(this.authSettings);
            this.setWeixinBridgeSaveStatus(
                response.settings?.enabled ? '微信 IM 桥接已启用。' : '微信 IM 桥接已关闭。',
                'success'
            );
            Utils.showToast(response.settings?.enabled ? '微信 IM 桥接已启用' : '微信 IM 桥接已关闭');
        } catch (error) {
            this.setWeixinBridgeSaveStatus(`保存失败：${error.message}`, 'error');
            Utils.showToast(error.message, 'error');
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = originalLabel;
            }
        }
    },

    renderAuthSettings(settings = {}) {
        this.authSettings = settings;
        this.renderWeixinBridgeControls(settings.weixin_bridge || {});
        this.renderWeixinBindingSummary(settings.weixin_binding || {});
    },

    renderWolaiSummary(settings = {}) {
        if (!this.wolaiSummary) return;

        const tokenConfigured = Boolean(String(settings.token || '').trim());
        const parentBlockID = String(settings.parent_block_id || '').trim();
        const baseURL = String(settings.base_url || 'https://openapi.wolai.com').trim() || 'https://openapi.wolai.com';

        this.wolaiSummary.innerHTML = `
            <div>
                <span>Token</span>
                <strong>${Utils.escapeHTML(tokenConfigured ? '已配置' : '未配置')}</strong>
            </div>
            <div>
                <span>目标块 ID</span>
                <strong>${Utils.escapeHTML(parentBlockID || '未填写')}</strong>
            </div>
            <div>
                <span>OpenAPI</span>
                <strong>${Utils.escapeHTML(baseURL)}</strong>
            </div>
            <div>
                <span>导出行为</span>
                <strong>追加为新文本块</strong>
            </div>
        `;
    },

    renderWeixinBridgeControls(settings = {}) {
        const enabled = Boolean(settings.enabled);
        if (this.weixinBridgeEnabledInput) {
            this.weixinBridgeEnabledInput.checked = enabled;
        }
        if (!this.weixinBridgeSaveStatus?.textContent) {
            this.setWeixinBridgeSaveStatus(
                enabled ? '微信 IM 桥接当前已启用。' : '微信 IM 桥接当前已关闭。',
                enabled ? 'success' : ''
            );
        }
    },

    renderWeixinBindingSummary(binding = {}) {
        if (!this.weixinBindingSummary) return;

        const isBound = Boolean(binding.bound);
        const bridgeEnabled = Boolean(this.authSettings?.weixin_bridge?.enabled);
        const title = bridgeEnabled ? '微信 IM 已启用' : '微信 IM 已关闭';
        const detail = [
            bridgeEnabled ? '桥接状态：后台会轮询微信消息并尝试自动回复。' : '桥接状态：当前不会接收或回复微信消息。',
            isBound
                ? `绑定账号：${Utils.escapeHTML(binding.account_id || '微信账号')}`
                : '当前未绑定微信，可先保存桥接开关，再扫码完成绑定。',
            binding.user_id ? `用户 ID：${Utils.escapeHTML(binding.user_id)}` : '',
            binding.bound_at ? `绑定时间：${Utils.escapeHTML(Utils.formatDate(binding.bound_at))}` : '',
            binding.base_url ? `接入域名：${Utils.escapeHTML(binding.base_url)}` : ''
        ].filter(Boolean).join('<br>');

        this.weixinBindingSummary.innerHTML = `
            <span>WeChat Bridge</span>
            <strong>${title}</strong>
            <p>${detail}</p>
        `;

        if (this.startWeixinBindingButton) {
            this.startWeixinBindingButton.textContent = isBound ? '重新绑定' : '开始绑定';
        }
        if (this.unbindWeixinButton) {
            this.unbindWeixinButton.classList.toggle('hidden', !isBound);
        }
    },

    setWeixinBindingStatus(message, tone = '') {
        this.setInlineStatus(this.weixinBindingStatus, message, tone);
    },

    setWolaiStatus(message, tone = '') {
        this.setInlineStatus(this.wolaiStatus, message, tone);
    },

    setWeixinBridgeSaveStatus(message, tone = '') {
        this.setInlineStatus(this.weixinBridgeSaveStatus, message, tone);
    },

    async startWeixinBinding() {
        const button = this.startWeixinBindingButton;
        const originalLabel = button?.textContent || '开始绑定';
        if (button) {
            button.disabled = true;
            button.textContent = '生成二维码中...';
        }

        this.stopWeixinBindingPolling();

        try {
            const result = await API.startWeixinBinding();
            this.pendingWeixinQRCode = result.qrcode || '';

            if (this.weixinQRCodeImage) {
                this.weixinQRCodeImage.src = result.qrcode_data_url || '';
            }
            if (this.weixinQRCodeLink) {
                this.weixinQRCodeLink.href = result.qrcode_content || '#';
                this.weixinQRCodeLink.textContent = result.qrcode_content || '二维码内容不可用';
            }
            this.weixinQRCodePanel?.classList.remove('hidden');
            this.setWeixinBindingStatus(result.message || '请使用微信扫码完成绑定', 'saving');

            if (!this.pendingWeixinQRCode) {
                throw new Error('二维码会话为空，无法跟踪绑定状态');
            }

            this.scheduleWeixinBindingPoll(1200);
            Utils.showToast('微信二维码已生成');
        } catch (error) {
            this.setWeixinBindingStatus(error.message, 'error');
            Utils.showToast(error.message, 'error');
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = this.authSettings?.weixin_binding?.bound ? '重新绑定' : (originalLabel || '开始绑定');
            }
        }
    },

    async unbindWeixin() {
        if (!confirm('确定要解除微信绑定吗？解除后将停止接收微信消息。')) return;
        try {
            await API.unbindWeixin();
            if (this.authSettings) {
                this.authSettings.weixin_binding = {};
            }
            this.renderWeixinBindingSummary({});
            this.weixinQRCodePanel?.classList.add('hidden');
            this.setWeixinBindingStatus('');
            Utils.showToast('微信绑定已解除');
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    scheduleWeixinBindingPoll(delay = 1500) {
        this.stopWeixinBindingPolling();
        this.weixinBindingPollTimer = window.setTimeout(async () => {
            await this.pollWeixinBindingStatus();
        }, delay);
    },

    stopWeixinBindingPolling() {
        window.clearTimeout(this.weixinBindingPollTimer);
        this.weixinBindingPollTimer = 0;
    },

    async pollWeixinBindingStatus() {
        if (!this.pendingWeixinQRCode) return;

        try {
            const result = await API.getWeixinBindingStatus(this.pendingWeixinQRCode);
            const status = result.status || 'wait';
            const message = result.message || '等待微信扫码';

            if (status === 'confirmed') {
                this.stopWeixinBindingPolling();
                this.pendingWeixinQRCode = '';
                this.authSettings = {
                    ...(this.authSettings || {}),
                    weixin_binding: result.binding || {}
                };
                this.renderAuthSettings(this.authSettings);
                this.weixinQRCodePanel?.classList.add('hidden');
                this.setWeixinBindingStatus(message, 'success');
                Utils.showToast(message || '微信绑定成功');
                return;
            }

            if (status === 'expired') {
                this.stopWeixinBindingPolling();
                this.pendingWeixinQRCode = '';
                this.setWeixinBindingStatus(message, 'error');
                return;
            }

            this.setWeixinBindingStatus(
                status === 'scaned' ? (message || '二维码已扫描，请在微信中确认登录') : message,
                'saving'
            );
            this.scheduleWeixinBindingPoll(status === 'scaned' ? 900 : 1500);
        } catch (error) {
            this.stopWeixinBindingPolling();
            this.setWeixinBindingStatus(error.message, 'error');
            Utils.showToast(error.message, 'error');
        }
    },

    renderVersionSummary(status = {}) {
        const badge = this.versionStatusBadge(status.status, status.message);
        const currentVersion = status.current_version || 'dev';
        const currentDetail = status.build_time
            ? `构建时间：${Utils.escapeHTML(Utils.formatDate(status.build_time))}`
            : '构建时间未注入';
        const latestVersion = status.latest_version || '暂未获取';
        const latestDetail = status.published_at
            ? `发布时间：${Utils.escapeHTML(Utils.formatDate(status.published_at))}`
            : '尚无发布时间信息';
        const checkedAt = status.checked_at
            ? Utils.formatDate(status.checked_at)
            : '尚未完成检查';

        this.versionSummary.innerHTML = `
            <div>
                <span>当前版本</span>
                <strong>${Utils.escapeHTML(currentVersion)}</strong>
                <p>${currentDetail}</p>
            </div>
            <div>
                <span>检查结果</span>
                <strong>${badge}</strong>
                <p>${Utils.escapeHTML(status.message || '尚未检查最新版本')}</p>
            </div>
            <div>
                <span>最新正式版本</span>
                <strong>${Utils.escapeHTML(latestVersion)}</strong>
                <p>${latestDetail}</p>
            </div>
            <div>
                <span>最近检查</span>
                <strong>${Utils.escapeHTML(checkedAt)}</strong>
                <p>${status.latest_release_url ? `下载页面：<a href="${Utils.escapeHTML(status.latest_release_url)}" target="_blank" rel="noreferrer">${Utils.escapeHTML(status.latest_release_url)}</a>` : '暂无可用的 Release 链接'}</p>
            </div>
        `;

        if (status.latest_release_url) {
            this.versionReleaseLink.href = status.latest_release_url;
            this.versionReleaseLink.classList.remove('hidden');
        } else {
            this.versionReleaseLink.href = '#';
            this.versionReleaseLink.classList.add('hidden');
        }
    },

    versionStatusBadge(status, message) {
        const badges = {
            latest: { label: '已是最新', tone: 'tone-success' },
            update_available: { label: '发现更新', tone: 'tone-error' },
            ahead: { label: '当前更高', tone: 'tone-info' },
            unknown: { label: '无法判断', tone: 'tone-info' }
        };
        const badge = badges[status] || badges.unknown;
        return `<span class="status-badge ${badge.tone}" title="${Utils.escapeHTML(message || badge.label)}">${Utils.escapeHTML(badge.label)}</span>`;
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

    async addAIModel() {
        const nextModel = this.createAIModelDraft();
        const nextModels = [...(this.aiModelDraft || []), nextModel];
        const response = await API.updateAIModelSettings(this.buildAIModelSettingsPayload({
            models: nextModels,
            sceneModels: this.readSceneModelSelections()
        }));
        this.applyAISettings(response.settings || {}, {
            overwritePromptInputs: false,
            overwriteRolePrompts: false
        });
        this.setAIModelAutosaveStatus('已新增模型。', 'success');
        this.openAIModelEditor(nextModel.id);
        Utils.showToast('已新增模型');
    },

    openAIModelEditor(target) {
        const model = typeof target === 'string'
            ? this.aiModelDraft.find((item) => item.id === target)
            : (target || null);
        if (!model) return;

        this.activeAIModelID = model.id;
        this.renderAIModels();

        this.editingAIModelID = model.id;
        this.isHydratingAIModelEditor = true;
        this.aiModelModalTitle.textContent = `编辑模型 · ${this.aiModelButtonTitle(model)}`;
        this.aiModelNameInput.value = model.name || '';
        this.aiModelProviderInput.value = model.provider || 'openai';
        this.aiModelIdentifierInput.value = model.model || '';
        this.aiModelMaxTokensInput.value = Number(model.max_output_tokens || 1200);
        this.aiModelBaseURLInput.value = model.base_url || '';
        this.aiModelAPIKeyInput.value = model.api_key || '';
        this.aiModelLegacyModeInput.checked = Boolean(model.openai_legacy_mode);
        this.aiModelCheckStatus.textContent = model.check_status || '尚未检查';
        this.deleteAIModelButton.disabled = this.aiModelDraft.length <= 1;
        this.updateAIModelModalUI();
        this.setAIModelEditorStatus('修改后自动保存。');
        this.aiModelModal.classList.remove('hidden');
        document.body.classList.add('modal-open');
        this.isHydratingAIModelEditor = false;
    },

    async closeAIModelModal() {
        if (!this.aiModelModal) return;
        await this.flushAIModelAutosave();
        this.aiModelModal.classList.add('hidden');
        document.body.classList.remove('modal-open');
        this.editingAIModelID = '';
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
            ...(this.aiModelDraft.find((item) => item.id === this.editingAIModelID) || this.createAIModelDraft()),
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

    currentAIModelsForSave() {
        if (!this.editingAIModelID || this.aiModelModal?.classList.contains('hidden')) {
            return this.aiModelDraft || [];
        }
        const model = this.readAIModelFromModal();
        return (this.aiModelDraft || []).map((item) => item.id === model.id ? model : item);
    },

    async deleteAIModel(modelID) {
        if (this.aiModelDraft.length <= 1) {
            Utils.showToast('至少需要保留一个 AI 模型', 'error');
            return;
        }
        const nextModels = this.aiModelDraft.filter((item) => item.id !== modelID);
        const response = await API.updateAIModelSettings(this.buildAIModelSettingsPayload({
            models: nextModels,
            sceneModels: this.readSceneModelSelections()
        }));
        this.applyAISettings(response.settings || {}, {
            overwritePromptInputs: false,
            overwriteRolePrompts: false
        });
        this.setAIModelAutosaveStatus('模型已删除。', 'success');
        Utils.showToast('模型已删除');
    },

    async deleteCurrentAIModel() {
        if (!this.editingAIModelID) return;
        if (this.aiModelDraft.length <= 1) {
            Utils.showToast('至少需要保留一个 AI 模型', 'error');
            return;
        }

        await this.deleteAIModel(this.editingAIModelID);
        if (!this.aiModelModal.classList.contains('hidden')) {
            await this.closeAIModelModal();
        }
    },

    renderSceneModelSelectors(selection = {}) {
        const safeSelection = {
            default_model_id: selection.default_model_id || this.defaultModelSelect?.value || '',
            qa_model_id: selection.qa_model_id || this.qaModelSelect?.value || '',
            figure_model_id: selection.figure_model_id || this.figureModelSelect?.value || '',
            tag_model_id: selection.tag_model_id || this.tagModelSelect?.value || '',
            group_model_id: selection.group_model_id || this.groupModelSelect?.value || '',
            translate_model_id: selection.translate_model_id || this.translateModelSelect?.value || ''
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
            [this.groupModelSelect, safeSelection.group_model_id],
            [this.translateModelSelect, safeSelection.translate_model_id]
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
            group_model_id: this.groupModelSelect?.value || defaultModelID,
            translate_model_id: this.translateModelSelect?.value || defaultModelID
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
            this.setAIModelEditorStatus('模型检查通过。', 'success');
            Utils.showToast('模型检查通过');
        } catch (error) {
            this.aiModelCheckStatus.textContent = `检查失败：${error.message}`;
            this.setAIModelEditorStatus(`模型检查失败：${error.message}`, 'error');
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

    async exportDatabase() {
        try {
            const fallbackName = `library_backup_${new Date().toISOString().slice(0, 10)}.db`;
            const result = await requestBlob('/api/database/export');
            const saved = await Utils.saveBlobDownload(result.blob, result.filename || fallbackName);
            if (saved) {
                Utils.showToast('数据库导出完成');
            }
        } catch (error) {
            Utils.showToast(error.message || '数据库导出失败', 'error');
        }
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
