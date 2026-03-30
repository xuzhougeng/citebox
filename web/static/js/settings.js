const SettingsPage = {
    init() {
        if (this.initialized) return;
        this.initialized = true;
        if (typeof t !== 'function') { window.t = function(k, f) { return f || k; }; }

        this.aiSettingsForm = document.getElementById('aiSettingsForm');
        this.aiModelList = document.getElementById('aiModelList');
        this.addAIModelButton = document.getElementById('addAIModelButton');
        this.defaultModelSelect = document.getElementById('aiDefaultModelSelect');
        this.qaModelSelect = document.getElementById('aiQAModelSelect');
        this.imIntentModelSelect = document.getElementById('aiIMIntentModelSelect');
        this.figureModelSelect = document.getElementById('aiFigureModelSelect');
        this.tagModelSelect = document.getElementById('aiTagModelSelect');
        this.groupModelSelect = document.getElementById('aiGroupModelSelect');
        this.translateModelSelect = document.getElementById('aiTranslateModelSelect');
        this.ttsModelSelect = document.getElementById('aiTTSModelSelect');
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
        this.ttsPromptInput = document.getElementById('aiTTSPromptInput');
        this.aiModelAutosaveStatus = document.getElementById('aiModelAutosaveStatus');
        this.aiPromptSaveStatus = document.getElementById('aiPromptSaveStatus');
        this.translatePromptSaveStatus = document.getElementById('translatePromptSaveStatus');
        this.saveTranslatePromptButton = document.getElementById('saveTranslatePromptButton');
        this.saveAIPromptsButton = document.getElementById('saveAIPromptsButton');
        this.saveFigureGroupPromptsButton = document.getElementById('saveFigureGroupPromptsButton');
        this.figureGroupPromptSaveStatus = document.getElementById('figureGroupPromptSaveStatus');
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
        this.extractorProfileSelect = document.getElementById('extractorProfileSelect');
        this.extractorFigureModelField = document.getElementById('extractorFigureModelField');
        this.extractorURLField = document.getElementById('extractorURLField');
        this.extractorURLInput = document.getElementById('extractorURLInput');
        this.extractorTokenField = document.getElementById('extractorTokenField');
        this.extractorTokenInput = document.getElementById('extractorTokenInput');
        this.extractorFileFieldField = document.getElementById('extractorFileFieldField');
        this.extractorFileFieldInput = document.getElementById('extractorFileFieldInput');
        this.extractorTimeoutField = document.getElementById('extractorTimeoutField');
        this.extractorTimeoutInput = document.getElementById('extractorTimeoutInput');
        this.extractorPollIntervalField = document.getElementById('extractorPollIntervalField');
        this.extractorPollIntervalInput = document.getElementById('extractorPollIntervalInput');
        this.extractorPDFFigXHint = document.getElementById('extractorPDFFigXHint');
        this.extractorManualHint = document.getElementById('extractorManualHint');
        this.extractorBuiltInHint = document.getElementById('extractorBuiltInHint');
        this.extractorSummary = document.getElementById('extractorSummary');
        this.wolaiSettingsForm = document.getElementById('wolaiSettingsForm');
        this.wolaiTokenInput = document.getElementById('wolaiTokenInput');
        this.wolaiParentBlockIDInput = document.getElementById('wolaiParentBlockIDInput');
        this.wolaiBaseURLInput = document.getElementById('wolaiBaseURLInput');
        this.wolaiSummary = document.getElementById('wolaiSummary');
        this.wolaiStatus = document.getElementById('wolaiStatus');
        this.wolaiResultLink = document.getElementById('wolaiResultLink');
        this.testWolaiButton = document.getElementById('testWolaiButton');
        this.testWolaiInsertButton = document.getElementById('testWolaiInsertButton');
        this.desktopCloseSettingsSection = document.getElementById('desktopCloseSettingsSection');
        this.desktopCloseActionSelect = document.getElementById('desktopCloseActionSelect');
        this.desktopCloseSummary = document.getElementById('desktopCloseSummary');
        this.desktopCloseStatus = document.getElementById('desktopCloseStatus');
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
        this.rememberLoginEnabledInput = document.getElementById('rememberLoginEnabledInput');
        this.rememberLoginStatus = document.getElementById('rememberLoginStatus');
        this.logoutButton = document.getElementById('logoutButton');
        this.weixinBindingSummary = document.getElementById('weixinBindingSummary');
        this.weixinQRCodePlaceholder = document.getElementById('weixinQRCodePlaceholder');
        this.weixinQRCodePanel = document.getElementById('weixinQRCodePanel');
        this.weixinQRCodeImage = document.getElementById('weixinQRCodeImage');
        this.weixinQRCodeLink = document.getElementById('weixinQRCodeLink');
        this.weixinBindingStatus = document.getElementById('weixinBindingStatus');
        this.weixinBridgeEnabledInput = document.getElementById('weixinBridgeEnabledInput');
        this.weixinDailyRecommendationEnabledInput = document.getElementById('weixinDailyRecommendationEnabledInput');
        this.weixinDailyRecommendationTimeInput = document.getElementById('weixinDailyRecommendationTimeInput');
        this.weixinDailyRecommendationSaveStatus = document.getElementById('weixinDailyRecommendationSaveStatus');
        this.weixinDailyRecommendationTestStatus = document.getElementById('weixinDailyRecommendationTestStatus');
        this.saveWeixinDailyRecommendationButton = document.getElementById('saveWeixinDailyRecommendationButton');
        this.testWeixinDailyRecommendationButton = document.getElementById('testWeixinDailyRecommendationButton');
        this.ttsSettingsForm = document.getElementById('ttsSettingsForm');
        this.ttsAppIDInput = document.getElementById('ttsAppIDInput');
        this.ttsAccessKeyInput = document.getElementById('ttsAccessKeyInput');
        this.ttsResourceIDInput = document.getElementById('ttsResourceIDInput');
        this.ttsSpeakerInput = document.getElementById('ttsSpeakerInput');
        this.weixinBridgeSaveStatus = document.getElementById('weixinBridgeSaveStatus');
        this.saveWeixinBridgeButton = document.getElementById('saveWeixinBridgeButton');
        this.ttsSaveStatus = document.getElementById('ttsSaveStatus');
        this.ttsTestStatus = document.getElementById('ttsTestStatus');
        this.ttsAudioPreview = document.getElementById('ttsAudioPreview');
        this.ttsAudioPlayer = document.getElementById('ttsAudioPlayer');
        this.saveTTSButton = document.getElementById('saveTTSButton');
        this.testTTSButton = document.getElementById('testTTSButton');
        this.startWeixinBindingButton = document.getElementById('startWeixinBindingButton');
        this.unbindWeixinButton = document.getElementById('unbindWeixinButton');
        this.ttsAudioObjectURL = '';

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
            this.qaPromptInput
        ].forEach((element) => {
            element?.addEventListener('input', () => {
                if (this.isHydratingAISettings) return;
                this.setAIPromptSaveStatus(t('settings.ai.prompt_modified', '提示词已修改，点击”保存 Prompt 配置”后生效。'), 'saving');
            });
        });
        [
            this.figurePromptInput,
            this.tagPromptInput,
            this.groupPromptInput
        ].forEach((element) => {
            element?.addEventListener('input', () => {
                if (this.isHydratingAISettings) return;
                this.setFigureGroupPromptSaveStatus(t('settings.ai.figure_group_prompt_modified', '图片与分组 Prompt 已修改，点击”保存”后生效。'), 'saving');
            });
        });
        this.translatePromptInput?.addEventListener('input', () => {
            if (this.isHydratingAISettings) return;
            this.setTranslatePromptSaveStatus(t('settings.ai.translate_prompt_modified', '翻译 Prompt 已修改，点击”保存翻译 Prompt”后生效。'), 'saving');
        });
        this.ttsPromptInput?.addEventListener('input', () => {
            if (this.isHydratingAISettings) return;
            this.setTTSSaveStatus(t('settings.tts.modified', 'TTS 配置已修改，点击”保存 TTS 配置”后生效。'), 'saving');
        });
        this.saveTranslatePromptButton?.addEventListener('click', async () => {
            await this.saveTranslatePromptSettings();
        });
        this.saveFigureGroupPromptsButton?.addEventListener('click', async () => {
            await this.saveFigureGroupPromptSettings();
        });
        this.extractorSettingsForm.addEventListener('submit', async (event) => {
            event.preventDefault();
            await this.saveExtractorSettings();
        });
        this.extractorProfileSelect?.addEventListener('change', () => {
            this.syncExtractorProfileFormState();
        });
        this.wolaiSettingsForm?.addEventListener('submit', async (event) => {
            event.preventDefault();
            await this.saveWolaiSettings();
        });
        this.testWolaiButton?.addEventListener('click', async () => {
            await this.testWolaiSettings();
        });
        this.testWolaiInsertButton?.addEventListener('click', async () => {
            await this.insertWolaiTestPage();
        });
        this.desktopCloseActionSelect?.addEventListener('change', async () => {
            await this.saveDesktopCloseSettings();
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
        this.rememberLoginEnabledInput?.addEventListener('change', async () => {
            await this.updateRememberLogin();
        });
        this.logoutButton.addEventListener('click', () => this.logout());
        this.weixinBridgeEnabledInput?.addEventListener('change', () => {
            this.setWeixinBridgeSaveStatus(t('settings.weixin.bridge_modified', '配置已修改，点击”保存微信桥接配置”后生效。'), 'saving');
        });
        this.weixinDailyRecommendationEnabledInput?.addEventListener('change', () => {
            this.setWeixinDailyRecommendationSaveStatus(t('settings.weixin.daily_modified', '配置已修改，点击“保存今日推荐配置”后生效。'), 'saving');
        });
        this.weixinDailyRecommendationTimeInput?.addEventListener('input', () => {
            this.setWeixinDailyRecommendationSaveStatus(t('settings.weixin.daily_modified', '配置已修改，点击“保存今日推荐配置”后生效。'), 'saving');
        });
        [
            this.ttsAppIDInput,
            this.ttsAccessKeyInput,
            this.ttsResourceIDInput,
            this.ttsSpeakerInput
        ].forEach((element) => {
            element?.addEventListener('input', () => {
                this.setTTSSaveStatus(t('settings.tts.modified', 'TTS 配置已修改，点击“保存 TTS 配置”后生效。'), 'saving');
                this.clearTTSAudioPreview();
                this.setTTSTestStatus(t('settings.tts.test_hint', '测试会直接使用当前表单配置，成功后可在下方试听。'));
            });
        });
        this.saveWeixinBridgeButton?.addEventListener('click', async () => {
            await this.saveWeixinBridgeSettings();
        });
        this.saveWeixinDailyRecommendationButton?.addEventListener('click', async () => {
            await this.saveWeixinDailyRecommendationSettings();
        });
        this.testWeixinDailyRecommendationButton?.addEventListener('click', async () => {
            await this.testWeixinDailyRecommendation();
        });
        this.ttsSettingsForm?.addEventListener('submit', async (event) => {
            event.preventDefault();
            await this.saveTTSSettings();
        });
        this.testTTSButton?.addEventListener('click', async () => {
            await this.testTTSSettings();
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
        window.addEventListener('beforeunload', () => {
            this.clearTTSAudioPreview();
        });
    },

    bindAIModelAutoSaveInputs() {
        [
            this.defaultModelSelect,
            this.qaModelSelect,
            this.imIntentModelSelect,
            this.figureModelSelect,
            this.tagModelSelect,
            this.groupModelSelect,
            this.translateModelSelect
        ].forEach((element) => {
            element?.addEventListener('change', () => {
                this.scheduleAIModelAutosave({ immediate: true });
            });
        });
        this.ttsModelSelect?.addEventListener('change', () => {
            if (this.isHydratingAISettings) return;
            this.setTTSSaveStatus(t('settings.tts.modified', 'TTS 配置已修改，点击"保存 TTS 配置"后生效。'), 'saving');
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
        if (this.desktopCloseSettingsSection) {
            this.desktopCloseSettingsSection.classList.toggle('hidden', !this.supportsDesktopCloseSettings());
        }

        try {
            await Promise.all([
                this.loadAISettings(),
                this.loadExtractorSettings(),
                this.loadWolaiSettings(),
                this.loadDesktopCloseSettings(),
                this.loadVersionStatus(),
                this.loadAuthSettings(),
                this.loadTTSSettings()
            ]);
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async loadAuthSettings() {
        const settings = await API.getAuthSettings();
        this.renderAuthSettings(settings || {});
    },

    async loadTTSSettings() {
        const settings = await API.getTTSSettings();
        this.renderTTSSettings(settings || {});
    },

    async loadAISettings() {
        const settings = await API.getAISettings();
        this.applyAISettings(settings, {
            overwritePromptInputs: true,
            overwriteRolePrompts: true
        });
        this.setAIModelAutosaveStatus(t('settings.ai.autosave_hint', '模型配置修改后会自动保存。'));
        this.setAIPromptSaveStatus(t('settings.ai.prompt_save_hint', '提示词修改后需点击保存。'));
        this.setFigureGroupPromptSaveStatus(t('settings.ai.figure_group_prompt_save_hint', '图片与分组 Prompt 修改后需点击保存。'));
        this.setTranslatePromptSaveStatus(t('settings.ai.translate_prompt_save_hint', '翻译 Prompt 修改后需点击保存。'));
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
        this.translationPrimaryLanguageInput.value = settings.translation?.primary_language || t('settings.ai.primary_language_default', '中文');
        this.translationTargetLanguageInput.value = settings.translation?.target_language || t('settings.ai.target_language_default', '英文');

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
        if (this.extractorSettings) {
            this.renderExtractorSummary(this.extractorSettings);
        }
        if (this.authSettings) {
            this.renderWeixinBindingSummary(this.authSettings.weixin_binding || {});
        }
        this.isHydratingAISettings = false;
    },

    applyPromptSettingsToInputs(settings = {}) {
        this.systemPromptInput.value = settings.system_prompt || '';
        this.qaPromptInput.value = settings.qa_prompt || '';
        this.figurePromptInput.value = settings.figure_prompt || '';
        this.tagPromptInput.value = settings.tag_prompt || '';
        this.groupPromptInput.value = settings.group_prompt || '';
        this.translatePromptInput.value = settings.translate_prompt || '';
        this.ttsPromptInput.value = settings.tts_prompt || '';
    },

    extractPromptSettings(settings = {}) {
        return {
            system_prompt: settings.system_prompt || '',
            qa_prompt: settings.qa_prompt || '',
            figure_prompt: settings.figure_prompt || '',
            tag_prompt: settings.tag_prompt || '',
            group_prompt: settings.group_prompt || '',
            translate_prompt: settings.translate_prompt || '',
            tts_prompt: settings.tts_prompt || ''
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
            translate_prompt: this.translatePromptInput.value.trim(),
            tts_prompt: this.ttsPromptInput.value.trim()
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

    setTranslatePromptSaveStatus(message, tone = '') {
        this.setInlineStatus(this.translatePromptSaveStatus, message, tone);
    },

    setFigureGroupPromptSaveStatus(message, tone = '') {
        this.setInlineStatus(this.figureGroupPromptSaveStatus, message, tone);
    },

    setAIModelEditorStatus(message, tone = '') {
        this.setInlineStatus(this.aiModelEditorStatus, message, tone);
    },

    supportsDesktopCloseSettings() {
        return Boolean(window.__CITEBOX_DESKTOP__)
            && typeof window.citeboxDesktopMinimizeToTray === 'function'
            && typeof window.citeboxDesktopExitApp === 'function';
    },

    normalizeDesktopCloseAction(value) {
        const normalized = String(value || '').trim().toLowerCase();
        return normalized === 'minimize' || normalized === 'exit' ? normalized : 'ask';
    },

    setDesktopCloseStatus(message, tone = '') {
        this.setInlineStatus(this.desktopCloseStatus, message, tone);
    },

    desktopCloseActionLabel(action) {
        const normalized = this.normalizeDesktopCloseAction(action);
        const labels = {
            ask: t('settings.desktop_close.option_ask', '每次询问'),
            minimize: t('settings.desktop_close.option_minimize', '最小化到托盘'),
            exit: t('settings.desktop_close.option_exit', '直接退出')
        };
        return labels[normalized] || labels.ask;
    },

    desktopCloseActionEffect(action) {
        const normalized = this.normalizeDesktopCloseAction(action);
        const effects = {
            ask: t('settings.desktop_close.effect_ask', '关闭窗口时每次都先询问。'),
            minimize: t('settings.desktop_close.effect_minimize', '关闭窗口时会直接最小化到托盘，不再重复询问。'),
            exit: t('settings.desktop_close.effect_exit', '关闭窗口时会直接退出桌面应用，不再重复询问。')
        };
        return effects[normalized] || effects.ask;
    },

    scheduleAIModelAutosave(options = {}) {
        if (this.isHydratingAISettings || this.isHydratingAIModelEditor) return;

        const { immediate = false } = options;
        window.clearTimeout(this.aiModelAutosaveTimer);
        this.setAIModelAutosaveStatus(immediate ? t('settings.ai.model_config_saving', '模型配置保存中...') : t('settings.ai.model_config_preparing', '检测到修改，正在准备自动保存...'), 'saving');
        this.setAIModelEditorStatus(t('settings.ai.model_editor_saving', '当前模型修改后会自动保存。'), 'saving');

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
        this.setAIModelAutosaveStatus(t('settings.ai.model_config_saving', '模型配置保存中...'), 'saving');
        this.setAIModelEditorStatus(t('settings.ai.model_config_saving', '模型配置保存中...'), 'saving');

        try {
            const response = await API.updateAIModelSettings(this.buildAIModelSettingsPayload());
            if (requestID !== this.aiModelAutosaveRequestID) return;
            this.applyAISettings(response.settings || {}, {
                overwritePromptInputs: false,
                overwriteRolePrompts: false
            });
            this.setAIModelAutosaveStatus(t('settings.ai.model_config_saved', '模型配置已自动保存。'), 'success');
            this.setAIModelEditorStatus(t('settings.ai.model_editor_saved', '模型配置已自动保存。'), 'success');
            if (successMessage) {
                Utils.showToast(successMessage);
            }
        } catch (error) {
            if (requestID !== this.aiModelAutosaveRequestID) return;
            this.setAIModelAutosaveStatus(t('settings.ai.model_config_save_failed', '自动保存失败：{0}').replace('{0}', error.message), 'error');
            this.setAIModelEditorStatus(t('settings.ai.model_config_save_failed', '自动保存失败：{0}').replace('{0}', error.message), 'error');
            Utils.showToast(error.message, 'error');
        }
    },

    async savePromptSettings() {
        const button = this.saveAIPromptsButton;
        const originalLabel = button?.textContent || '';
        if (button) {
            button.disabled = true;
            button.textContent = t('settings.ai.saving_btn', '保存中...');
        }

        try {
            const response = await API.updateAIPromptSettings(this.buildAIPromptSettingsPayload());
            this.applyAISettings(response.settings || {}, {
                overwritePromptInputs: true,
                overwriteRolePrompts: false
            });
            this.setAIPromptSaveStatus(t('settings.ai.prompt_saved', 'Prompt 配置已保存。'), 'success');
            Utils.showToast(t('settings.ai.prompt_saved_toast', 'Prompt 配置已保存'));
        } catch (error) {
            this.setAIPromptSaveStatus(t('settings.ai.prompt_save_failed', '保存失败：{0}').replace('{0}', error.message), 'error');
            Utils.showToast(error.message, 'error');
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = originalLabel || t('settings.ai.save_prompts', '保存 Prompt 配置');
            }
        }
    },

    async saveTranslatePromptSettings() {
        const button = this.saveTranslatePromptButton;
        const originalLabel = button?.textContent || '';
        if (button) {
            button.disabled = true;
            button.textContent = t('settings.ai.saving_btn', '保存中...');
        }

        try {
            const response = await API.updateAIPromptSettings(this.buildAIPromptSettingsPayload());
            this.applyAISettings(response.settings || {}, {
                overwritePromptInputs: true,
                overwriteRolePrompts: false
            });
            this.setTranslatePromptSaveStatus(t('settings.ai.translate_prompt_saved', '翻译 Prompt 已保存。'), 'success');
            Utils.showToast(t('settings.ai.translate_prompt_saved_toast', '翻译 Prompt 已保存'));
        } catch (error) {
            this.setTranslatePromptSaveStatus(t('settings.ai.translate_prompt_save_failed', '保存失败：{0}').replace('{0}', error.message), 'error');
            Utils.showToast(error.message, 'error');
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = originalLabel || t('settings.ai.save_translate_prompt', '保存翻译 Prompt');
            }
        }
    },

    async saveFigureGroupPromptSettings() {
        const button = this.saveFigureGroupPromptsButton;
        const originalLabel = button?.textContent || '';
        if (button) {
            button.disabled = true;
            button.textContent = t('settings.ai.saving_btn', '保存中...');
        }

        try {
            const response = await API.updateAIPromptSettings(this.buildAIPromptSettingsPayload());
            this.applyAISettings(response.settings || {}, {
                overwritePromptInputs: true,
                overwriteRolePrompts: false
            });
            this.setFigureGroupPromptSaveStatus(t('settings.ai.figure_group_prompt_saved', '图片与分组 Prompt 已保存。'), 'success');
            Utils.showToast(t('settings.ai.figure_group_prompt_saved_toast', '图片与分组 Prompt 已保存'));
        } catch (error) {
            this.setFigureGroupPromptSaveStatus(t('settings.ai.figure_group_prompt_save_failed', '保存失败：{0}').replace('{0}', error.message), 'error');
            Utils.showToast(error.message, 'error');
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = originalLabel || t('settings.ai.save_figure_group_prompts', '保存图片与分组 Prompt');
            }
        }
    },

    async restoreRecommendedPrompts() {
        const button = this.restoreAIPromptsButton;
        const originalLabel = button?.textContent || '';
        if (button) {
            button.disabled = true;
            button.textContent = t('settings.ai.loading_btn', '载入中...');
        }

        try {
            const defaults = await API.getDefaultAISettings();
            this.systemPromptInput.value = defaults?.system_prompt || '';
            this.qaPromptInput.value = defaults?.qa_prompt || '';
            this.setAIPromptSaveStatus(t('settings.ai.restore_done', '已恢复推荐 Prompt，点击”保存 Prompt 配置”后生效。'), 'saving');
            Utils.showToast(t('settings.ai.restore_done_toast', '已恢复推荐 Prompt，记得点击”保存 Prompt 配置”'));
        } catch (error) {
            Utils.showToast(error.message, 'error');
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = originalLabel || t('settings.ai.restore_prompts', '恢复推荐 Prompt');
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
            this.rolePromptList.innerHTML = `<div class="prompt-preset-empty">${t('settings.ai.role_prompt_empty', '还没有保存角色 Prompt。新增一个角色后，就可以在 AI 伴读聊天框里通过 <code>@角色名</code> 直接调用。')}</div>`;
            return;
        }

        this.rolePromptList.innerHTML = rolePrompts.map((rolePrompt, index) => {
            return `
                <article class="prompt-preset-card">
                    <div class="prompt-preset-card-head">
                        <div>
                            <strong>${Utils.escapeHTML(rolePrompt.name || t('settings.ai.role_prompt_unnamed', '未命名角色'))}</strong>
                            <span>${t('settings.ai.role_prompt_call_hint', '聊天时输入 {0} 即可调用').replace('{0}', Utils.escapeHTML(`@${rolePrompt.name || t('settings.ai.role_editor_name_label', '角色名')}`))}</span>
                        </div>
                    </div>
                    <div class="prompt-preset-preview">
                        <div class="prompt-preset-preview-item">
                            <span>${t('settings.ai.role_prompt_label', '角色 Prompt')}</span>
                            <p>${Utils.escapeHTML(this.rolePromptExcerpt(rolePrompt.prompt, t('settings.ai.role_prompt_not_filled', '未填写角色 Prompt')))}</p>
                        </div>
                    </div>
                    <div class="prompt-preset-card-actions">
                        <button class="btn btn-outline btn-small" type="button" data-role-prompt-action="edit" data-role-prompt-index="${index}">${t('btn.edit', '编辑')}</button>
                        <button class="btn btn-outline btn-small danger" type="button" data-role-prompt-action="delete" data-role-prompt-index="${index}">${t('btn.delete', '删除')}</button>
                    </div>
                </article>
            `;
        }).join('');
    },

    async openAIRolePromptEditor(index = -1) {
        const current = index >= 0 ? this.rolePromptDraft?.[index] : null;
        const values = await Utils.promptFields({
            title: current ? t('settings.ai.role_editor_title_edit', '编辑角色 Prompt · {0}').replace('{0}', current.name) : t('settings.ai.role_editor_title_new', '新建角色 Prompt'),
            description: t('settings.ai.role_editor_desc', '角色 Prompt 只保存角色信息。保存后可在 AI 伴读聊天框中输入 @角色名 直接调用。'),
            confirmLabel: current ? t('settings.ai.role_editor_confirm_edit', '保存角色') : t('settings.ai.role_editor_confirm_new', '创建角色'),
            fields: [
                {
                    name: 'name',
                    label: t('settings.ai.role_editor_name_label', '角色名称'),
                    placeholder: t('settings.ai.role_editor_name_placeholder', '例如：严格证据模式'),
                    value: current?.name || '',
                    required: true
                },
                {
                    name: 'prompt',
                    label: t('settings.ai.role_editor_prompt_label', '角色 Prompt'),
                    type: 'textarea',
                    rows: 8,
                    placeholder: t('settings.ai.role_editor_prompt_placeholder', '例如：你是一名严格的论文审稿人，优先指出证据链、方法缺口和结论边界。'),
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
        let successMessage = editingCurrent ? t('settings.ai.role_updated', '已更新角色：{0}').replace('{0}', roleName) : t('settings.ai.role_saved', '已保存角色：{0}').replace('{0}', roleName);

        if (existingIndex >= 0 && existingIndex !== index) {
            const confirmed = await Utils.confirm(t('settings.ai.role_overwrite_confirm', '已存在同名角色”{0}”，是否覆盖？').replace('{0}', Utils.escapeHTML(roleName)), t('settings.ai.role_overwrite_title', '覆盖角色 Prompt'));
            if (!confirmed) return;
            nextRolePrompts.splice(existingIndex, 1, nextRolePrompt);
            if (editingCurrent && index >= 0 && index > existingIndex) {
                nextRolePrompts.splice(index, 1);
            }
            successMessage = t('settings.ai.role_updated', '已更新角色：{0}').replace('{0}', roleName);
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

        const confirmed = await Utils.confirm(t('settings.ai.role_delete_confirm', '确定要删除角色”{0}”吗？').replace('{0}', Utils.escapeHTML(rolePrompt.name)), t('settings.ai.role_delete_title', '删除角色 Prompt'));
        if (!confirmed) return;

        const nextRolePrompts = this.rolePromptDraft.filter((_, itemIndex) => itemIndex !== index);
        await this.persistAIRolePrompts(nextRolePrompts, t('settings.ai.role_deleted', '已删除角色：{0}').replace('{0}', rolePrompt.name));
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
        this.extractorSettings = settings;

        if (this.extractorProfileSelect) {
            this.extractorProfileSelect.value = settings.extractor_profile || 'pdffigx_v1';
        }
        this.extractorURLInput.value = extractorURLValue;
        this.extractorTokenInput.value = settings.extractor_token || '';
        this.extractorFileFieldInput.value = settings.extractor_file_field || 'file';
        this.extractorTimeoutInput.value = settings.timeout_seconds ?? 300;
        this.extractorPollIntervalInput.value = settings.poll_interval_seconds ?? 2;

        this.syncExtractorProfileFormState();
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
        this.renderWolaiResultLink('');
    },

    async loadVersionStatus(forceRefresh = false) {
        const button = this.checkVersionButton;
        const originalLabel = button?.textContent || '';
        if (button) {
            button.disabled = true;
            button.textContent = forceRefresh ? t('settings.version.checking_btn', '检查中...') : t('settings.version.loading_btn', '载入中...');
        }

        try {
            const status = await API.getVersionStatus(forceRefresh);
            this.renderVersionSummary(status);
            if (forceRefresh) {
                const toastMessage = status.has_update
                    ? t('settings.version.new_version_found', '发现新版本 {0}').replace('{0}', status.latest_version || '').trim()
                    : (status.message || t('settings.version.check_done', '版本检查已完成'));
                Utils.showToast(toastMessage);
            }
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = originalLabel || t('settings.version.check_update', '检查更新');
            }
        }
    },

    async loadDesktopCloseSettings() {
        if (!this.supportsDesktopCloseSettings()) {
            return;
        }

        const settings = await API.getDesktopCloseSettings();
        const action = this.normalizeDesktopCloseAction(settings?.action);
        this.desktopCloseCurrentAction = action;
        if (this.desktopCloseActionSelect) {
            this.desktopCloseActionSelect.value = action;
        }
        this.renderDesktopCloseSummary(action);
        this.setDesktopCloseStatus(t('settings.desktop_close.status_hint', '修改后立即生效，也会同步影响关闭弹窗里的“记住这次选择”。'));
    },

    async saveDesktopCloseSettings() {
        if (!this.supportsDesktopCloseSettings() || !this.desktopCloseActionSelect) {
            return;
        }

        const action = this.normalizeDesktopCloseAction(this.desktopCloseActionSelect.value);
        const previousAction = this.normalizeDesktopCloseAction(this.desktopCloseCurrentAction);
        const select = this.desktopCloseActionSelect;
        select.disabled = true;
        this.setDesktopCloseStatus(t('settings.desktop_close.saving', '正在保存关闭窗口行为...'), 'saving');

        try {
            const response = await API.updateDesktopCloseSettings({ action });
            const savedAction = this.normalizeDesktopCloseAction(response?.settings?.action);
            this.desktopCloseCurrentAction = savedAction;
            select.value = savedAction;
            this.renderDesktopCloseSummary(savedAction);
            this.setDesktopCloseStatus(t('settings.desktop_close.saved', '关闭窗口行为已更新。'), 'success');
            Utils.showToast(t('settings.desktop_close.saved_toast', '关闭窗口行为已更新'));
        } catch (error) {
            select.value = previousAction;
            this.setDesktopCloseStatus(error.message, 'error');
            Utils.showToast(error.message, 'error');
        } finally {
            select.disabled = false;
        }
    },

    async saveExtractorSettings() {
        this.syncExtractorProfileFormState();
        const extractorProfile = this.extractorProfileSelect?.value || 'pdffigx_v1';
        const payload = {
            extractor_profile: extractorProfile,
            pdf_text_source: this.extractorPDFTextSourceValue(extractorProfile),
            extractor_url: this.extractorURLInput.value.trim(),
            extractor_jobs_url: '',
            extractor_token: this.extractorTokenInput.value.trim(),
            extractor_file_field: this.extractorFileFieldInput.value.trim(),
            timeout_seconds: Number(this.extractorTimeoutInput.value || 300),
            poll_interval_seconds: Number(this.extractorPollIntervalInput.value || 2)
        };

        const response = await API.updateExtractorSettings(payload);
        this.extractorSettings = response.settings || payload;
        this.syncExtractorProfileFormState();
        this.renderExtractorSummary(response.settings);
        Utils.showToast(t('settings.extractor.saved_toast', 'PDF 提取服务配置已保存'));
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
        this.setWolaiStatus(t('settings.wolai.saved', 'Wolai 配置已保存。'), 'success');
        this.renderWolaiResultLink('');
        Utils.showToast(t('settings.wolai.saved_toast', 'Wolai 配置已保存'));
    },

    async testWolaiSettings() {
        const button = this.testWolaiButton;
        const originalLabel = button?.textContent || t('settings.wolai.test_token', '测试 Token');
        if (button) {
            button.disabled = true;
            button.textContent = t('settings.wolai.testing_btn', '测试中...');
        }

        this.setWolaiStatus(t('settings.wolai.testing_status', '正在校验 Wolai token 与目标块访问权限...'), 'saving');
        this.renderWolaiResultLink('');

        try {
            const result = await API.testWolaiSettings(this.wolaiSettingsPayload());
            this.setWolaiStatus(result.message || t('settings.wolai.token_ok', 'Wolai token 可用'), 'success');
            Utils.showToast(result.message || t('settings.wolai.token_ok', 'Wolai token 可用'));
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

    async insertWolaiTestPage() {
        const button = this.testWolaiInsertButton;
        const originalLabel = button?.textContent || t('settings.wolai.test_insert', '插入测试页面');
        if (button) {
            button.disabled = true;
            button.textContent = t('settings.wolai.inserting_btn', '插入中...');
        }

        this.setWolaiStatus(t('settings.wolai.inserting_status', '正在创建 Wolai 测试页面，并写入图片导出 TODO 说明...'), 'saving');

        try {
            const result = await API.insertWolaiTestPage(this.wolaiSettingsPayload());
            this.setWolaiStatus(result.message || t('settings.wolai.insert_ok', 'Wolai 测试页面已创建'), 'success');
            this.renderWolaiResultLink(result.target_block_url || '');
            Utils.showToast(result.message || t('settings.wolai.insert_ok', 'Wolai 测试页面已创建'));
        } catch (error) {
            this.setWolaiStatus(error.message, 'error');
            this.renderWolaiResultLink('');
            Utils.showToast(error.message, 'error');
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = originalLabel;
            }
        }
    },

    async persistWeixinBridgeSettings(options = {}) {
        const {
            button = null,
            defaultLabel = t('settings.weixin.save_bridge', '保存微信桥接配置'),
            savingLabel = t('settings.weixin.saving_btn', '保存中...'),
            toastMessage = t('settings.weixin.saved_toast', '微信配置已保存')
        } = options;
        const originalLabel = button?.textContent || defaultLabel;
        if (button) {
            button.disabled = true;
            button.textContent = savingLabel;
        }

        try {
            const response = await API.updateWeixinBridgeSettings(this.weixinBridgeSavePayload());
            const dailySettings = response.settings?.daily_recommendation || {};
            this.authSettings = {
                ...(this.authSettings || {}),
                weixin_bridge: response.settings || {}
            };
            this.renderAuthSettings(this.authSettings);
            this.setWeixinBridgeSaveStatus(
                response.settings?.enabled ? t('settings.weixin.bridge_enabled', '微信消息通知已开启。') : t('settings.weixin.bridge_disabled', '微信消息通知已关闭。'),
                'success'
            );
            this.setWeixinDailyRecommendationSaveStatus(
                this.weixinDailyRecommendationStatusMessage(dailySettings),
                dailySettings.enabled ? 'success' : ''
            );
            Utils.showToast(toastMessage);
        } catch (error) {
            this.setWeixinBridgeSaveStatus(t('settings.weixin.bridge_save_failed', '保存失败：{0}').replace('{0}', error.message), 'error');
            this.setWeixinDailyRecommendationSaveStatus(t('settings.weixin.daily_save_failed', '保存失败：{0}').replace('{0}', error.message), 'error');
            Utils.showToast(error.message, 'error');
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = originalLabel;
            }
        }
    },

    async saveWeixinBridgeSettings() {
        await this.persistWeixinBridgeSettings({
            button: this.saveWeixinBridgeButton,
            defaultLabel: t('settings.weixin.save_bridge', '保存微信桥接配置'),
            savingLabel: t('settings.weixin.saving_btn', '保存中...'),
            toastMessage: t('settings.weixin.saved_toast', '微信配置已保存')
        });
    },

    async saveWeixinDailyRecommendationSettings() {
        await this.persistWeixinBridgeSettings({
            button: this.saveWeixinDailyRecommendationButton,
            defaultLabel: t('settings.weixin.daily_save_btn', '保存今日推荐配置'),
            savingLabel: t('settings.weixin.daily_saving_btn', '保存中...'),
            toastMessage: t('settings.weixin.daily_saved_toast', '今日推荐配置已保存')
        });
    },

    async testWeixinDailyRecommendation() {
        const button = this.testWeixinDailyRecommendationButton;
        const originalLabel = button?.textContent || t('settings.weixin.daily_test_btn', '发送测试图片');
        if (button) {
            button.disabled = true;
            button.textContent = t('settings.weixin.daily_testing_btn', '发送中...');
        }

        this.setWeixinDailyRecommendationTestStatus(t('settings.weixin.daily_testing_status', '正在向绑定微信发送一张随机测试图片...'), 'saving');

        try {
            const result = await API.testWeixinDailyRecommendation(this.weixinDailyRecommendationTestPayload());
            this.setWeixinDailyRecommendationTestStatus(result.message || t('settings.weixin.daily_test_ok', '测试图片已发送到微信。'), 'success');
            Utils.showToast(result.message || t('settings.weixin.daily_test_ok', '测试图片已发送到微信。'));
        } catch (error) {
            this.setWeixinDailyRecommendationTestStatus(t('settings.weixin.daily_test_failed', '测试失败：{0}').replace('{0}', error.message), 'error');
            Utils.showToast(error.message, 'error');
        } finally {
            this.updateWeixinDailyRecommendationTestButton();
            if (button) {
                button.textContent = originalLabel;
            }
        }
    },

    async saveTTSSettings() {
        const button = this.saveTTSButton;
        const originalLabel = button?.textContent || t('settings.tts.save_btn', '保存 TTS 配置');
        if (button) {
            button.disabled = true;
            button.textContent = t('settings.tts.saving_btn', '保存中...');
        }

        try {
            const [ttsResponse] = await Promise.all([
                API.updateTTSSettings(this.ttsSavePayload()),
                API.updateAIModelSettings(this.buildAIModelSettingsPayload()),
                API.updateAIPromptSettings(this.buildAIPromptSettingsPayload())
            ]);
            this.renderTTSSettings(ttsResponse.settings || {});
            this.setTTSSaveStatus(t('settings.tts.saved', 'TTS 配置已保存。'), 'success');
            Utils.showToast(t('settings.tts.saved_toast', 'TTS 配置已保存'));
        } catch (error) {
            this.setTTSSaveStatus(t('settings.tts.save_failed', '保存失败：{0}').replace('{0}', error.message), 'error');
            Utils.showToast(error.message, 'error');
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = originalLabel;
            }
        }
    },

    async testTTSSettings() {
        const button = this.testTTSButton;
        const originalLabel = button?.textContent || t('settings.tts.test_btn', '测试 TTS');
        if (button) {
            button.disabled = true;
            button.textContent = t('settings.tts.testing_btn', '测试中...');
        }

        this.clearTTSAudioPreview();
        this.setTTSTestStatus(t('settings.tts.testing_status', '正在调用当前 TTS 配置合成测试音频...'), 'saving');

        try {
            const result = await API.testTTS(this.ttsSavePayload());
            this.showTTSAudioPreview(result?.blob, result?.filename || '');
            this.setTTSTestStatus(t('settings.tts.test_ok', 'TTS 测试成功，可直接试听。'), 'success');
            Utils.showToast(t('settings.tts.test_ok', 'TTS 测试成功，可直接试听。'));
        } catch (error) {
            this.clearTTSAudioPreview();
            this.setTTSTestStatus(t('settings.tts.test_failed', '测试失败：{0}').replace('{0}', error.message), 'error');
            Utils.showToast(error.message, 'error');
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = originalLabel;
            }
        }
    },

    currentSavedWeixinBridgeSettings() {
        const settings = this.authSettings?.weixin_bridge || {};
        return {
            enabled: Boolean(settings.enabled),
            daily_recommendation: this.currentSavedWeixinDailyRecommendationSettings()
        };
    },

    currentSavedWeixinDailyRecommendationSettings() {
        const settings = this.authSettings?.weixin_bridge?.daily_recommendation || {};
        return {
            enabled: Boolean(settings.enabled),
            send_time: String(settings.send_time || '09:00').trim() || '09:00'
        };
    },

    weixinBridgeSavePayload() {
        const saved = this.currentSavedWeixinBridgeSettings();
        return {
            ...saved,
            enabled: Boolean(this.weixinBridgeEnabledInput?.checked),
            daily_recommendation: {
                ...saved.daily_recommendation,
                enabled: Boolean(this.weixinDailyRecommendationEnabledInput?.checked),
                send_time: String(this.weixinDailyRecommendationTimeInput?.value || '').trim()
            }
        };
    },

    weixinDailyRecommendationTestPayload() {
        return {
            enabled: Boolean(this.weixinDailyRecommendationEnabledInput?.checked),
            send_time: String(this.weixinDailyRecommendationTimeInput?.value || '').trim()
        };
    },

    currentSavedTTSSettings() {
        const settings = this.ttsSettings || {};
        return {
            app_id: String(settings.app_id || '').trim(),
            access_key: String(settings.access_key || '').trim(),
            resource_id: String(settings.resource_id || '').trim(),
            speaker: String(settings.speaker || '').trim(),
            weixin_voice_output_enabled: settings.weixin_voice_output_enabled !== false
        };
    },

    ttsSavePayload() {
        const saved = this.currentSavedTTSSettings();
        return {
            ...saved,
            app_id: String(this.ttsAppIDInput?.value || '').trim(),
            access_key: String(this.ttsAccessKeyInput?.value || '').trim(),
            resource_id: String(this.ttsResourceIDInput?.value || '').trim(),
            speaker: String(this.ttsSpeakerInput?.value || '').trim()
        };
    },

    renderAuthSettings(settings = {}) {
        this.authSettings = settings;
        if (this.rememberLoginEnabledInput) {
            this.rememberLoginEnabledInput.checked = Boolean(settings.remember_login_enabled);
        }
        this.setRememberLoginStatus(Boolean(settings.remember_login_enabled));
        this.syncRememberLoginPreference(Boolean(settings.remember_login_enabled));
        this.renderWeixinBridgeControls(settings.weixin_bridge || {});
        this.renderWeixinBindingSummary(settings.weixin_binding || {});
    },

    rememberLoginPreferenceKey() {
        return 'citebox_remember_login_preference';
    },

    syncRememberLoginPreference(enabled) {
        try {
            localStorage.setItem(this.rememberLoginPreferenceKey(), enabled ? '1' : '0');
        } catch (error) {
            // Ignore storage errors and keep server-side auth state as the source of truth.
        }
    },

    setRememberLoginStatus(enabled, tone = '') {
        if (!this.rememberLoginStatus) return;

        let message = t('settings.password.remember_status_disabled', '当前未记住这台设备的登录状态。');
        let statusTone = tone;
        if (enabled) {
            message = t('settings.password.remember_status_enabled', '当前已记住这台设备的登录状态，后续访问会自动登录。');
            if (!statusTone) statusTone = 'success';
        }

        this.rememberLoginStatus.textContent = message;
        this.rememberLoginStatus.classList.remove('success', 'error', 'saving');
        if (statusTone) {
            this.rememberLoginStatus.classList.add(statusTone);
        }
    },

    renderWolaiSummary(settings = {}) {
        if (!this.wolaiSummary) return;

        const tokenConfigured = Boolean(String(settings.token || '').trim());
        const parentBlockID = String(settings.parent_block_id || '').trim();
        const baseURL = String(settings.base_url || 'https://openapi.wolai.com').trim() || 'https://openapi.wolai.com';

        this.wolaiSummary.innerHTML = `
            <div>
                <span>${t('settings.wolai.summary_token', 'Token')}</span>
                <strong>${Utils.escapeHTML(tokenConfigured ? t('settings.extractor.configured', '已配置') : t('settings.extractor.not_configured', '未配置'))}</strong>
            </div>
            <div>
                <span>${t('settings.wolai.summary_target', '目标页面')}</span>
                <strong>${Utils.escapeHTML(parentBlockID || t('settings.wolai.summary_target_none', '未填写'))}</strong>
            </div>
            <div>
                <span>${t('settings.wolai.summary_api', 'OpenAPI')}</span>
                <strong>${Utils.escapeHTML(baseURL)}</strong>
            </div>
            <div>
                <span>${t('settings.wolai.summary_export', '导出方式')}</span>
                <strong>${t('settings.wolai.summary_export_method', '新建页面并写入内容')}</strong>
            </div>
        `;
    },

    renderWeixinBridgeControls(settings = {}) {
        const enabled = Boolean(settings.enabled);
        if (this.weixinBridgeEnabledInput) {
            this.weixinBridgeEnabledInput.checked = enabled;
        }
        this.renderWeixinDailyRecommendationControls(settings.daily_recommendation || {});
        if (!this.weixinBridgeSaveStatus?.textContent) {
            this.setWeixinBridgeSaveStatus(
                enabled ? t('settings.weixin.bridge_enabled_current', '微信消息通知当前已开启。') : t('settings.weixin.bridge_disabled_current', '微信消息通知当前已关闭。'),
                enabled ? 'success' : ''
            );
        }
        if (!this.pendingWeixinQRCode) {
            this.setWeixinQRCodeVisible(false);
        }
    },

    renderWeixinDailyRecommendationControls(settings = {}) {
        const safeSettings = {
            enabled: Boolean(settings.enabled),
            send_time: String(settings.send_time || '09:00').trim() || '09:00'
        };

        if (this.weixinDailyRecommendationEnabledInput) {
            this.weixinDailyRecommendationEnabledInput.checked = safeSettings.enabled;
        }
        if (this.weixinDailyRecommendationTimeInput) {
            this.weixinDailyRecommendationTimeInput.value = safeSettings.send_time;
        }
        if (!this.weixinDailyRecommendationSaveStatus?.textContent) {
            this.setWeixinDailyRecommendationSaveStatus(this.weixinDailyRecommendationStatusMessage(safeSettings), safeSettings.enabled ? 'success' : '');
        }
        if (!this.weixinDailyRecommendationTestStatus?.textContent) {
            this.setWeixinDailyRecommendationTestStatus(
                t('settings.weixin.daily_test_hint', '点击“发送测试图片”后，会立即向绑定微信发送一张随机图片。')
            );
        }
        this.updateWeixinDailyRecommendationTestButton();
    },

    weixinDailyRecommendationStatusMessage(settings = {}) {
        const enabled = Boolean(settings.enabled);
        const sendTime = String(settings.send_time || '09:00').trim() || '09:00';
        const bridgeEnabled = Boolean(this.authSettings?.weixin_bridge?.enabled);
        const isBound = Boolean(this.authSettings?.weixin_binding?.bound);

        if (!enabled) {
            return t('settings.weixin.daily_disabled_current', '今日推荐当前已关闭。');
        }
        if (!bridgeEnabled) {
            return t('settings.weixin.daily_enabled_bridge_off', '今日推荐已开启，计划每天 {0} 发送；当前微信桥接关闭，暂不会自动发送。').replace('{0}', sendTime);
        }
        if (!isBound) {
            return t('settings.weixin.daily_enabled_not_bound', '今日推荐已开启，计划每天 {0} 发送；当前尚未绑定微信账号，暂不会自动发送。').replace('{0}', sendTime);
        }
        return t('settings.weixin.daily_enabled_current', '今日推荐当前已开启，将在每天 {0} 随机发送 1 张图片。').replace('{0}', sendTime);
    },

    updateWeixinDailyRecommendationTestButton() {
        if (!this.testWeixinDailyRecommendationButton) return;

        const isBound = Boolean(this.authSettings?.weixin_binding?.bound);
        this.testWeixinDailyRecommendationButton.disabled = !isBound;
        this.testWeixinDailyRecommendationButton.title = isBound
            ? ''
            : t('settings.weixin.daily_test_requires_binding', '请先完成微信绑定，再测试发图。');
    },

    renderTTSSettings(settings = {}) {
        this.ttsSettings = settings;
        if (this.ttsAppIDInput) {
            this.ttsAppIDInput.value = String(settings.app_id || '');
        }
        if (this.ttsAccessKeyInput) {
            this.ttsAccessKeyInput.value = String(settings.access_key || '');
        }
        if (this.ttsResourceIDInput) {
            this.ttsResourceIDInput.value = String(settings.resource_id || '');
        }
        if (this.ttsSpeakerInput) {
            this.ttsSpeakerInput.value = String(settings.speaker || '');
        }
    },

    renderWeixinBindingSummary(binding = {}) {
        if (!this.weixinBindingSummary) return;

        const isBound = Boolean(binding.bound);
        const bridgeEnabled = Boolean(this.authSettings?.weixin_bridge?.enabled);
        const accountFull = String(binding.account_id || '').trim();
        const userIDFull = String(binding.user_id || '').trim();
        const endpointFull = String(binding.base_url || '').trim();
        const bridgeStateLabel = bridgeEnabled ? t('settings.weixin.summary_bridge_on', '桥接已开启') : t('settings.weixin.summary_bridge_off', '桥接已关闭');
        const bindingStateLabel = isBound ? t('settings.weixin.summary_bound', '微信已绑定') : t('settings.weixin.summary_not_bound', '尚未绑定');
        const overviewTitle = bridgeEnabled && isBound
            ? t('settings.weixin.summary_ready', '微信入口已就绪')
            : (bridgeEnabled ? t('settings.weixin.summary_wait_scan', '等待扫码绑定账号') : t('settings.weixin.summary_bridge_inactive', '桥接当前未启用'));
        const accountLabel = isBound
            ? this.compactDisplayText(accountFull || t('settings.weixin.summary_account_label', '微信账号'), 10, 9)
            : t('settings.weixin.summary_not_bound_account', '未绑定');
        const boundAtLabel = isBound && binding.bound_at
            ? Utils.formatDate(binding.bound_at)
            : t('settings.weixin.summary_not_bound_account', '未绑定');
        const endpointLabel = endpointFull
            ? this.compactURLLabel(endpointFull)
            : t('settings.weixin.summary_endpoint_default', '使用当前桥接配置');
        const userIDLabel = userIDFull
            ? this.compactDisplayText(userIDFull, 10, 8)
            : t('settings.weixin.summary_user_id_none', '未返回');
        let detail = t('settings.weixin.detail_not_bound', '当前未绑定微信账号，可先保存桥接开关，再生成二维码完成绑定。');
        if (bridgeEnabled && isBound) {
            detail = t('settings.weixin.detail_ready', '桥接和账号绑定都已就绪，可以直接通过微信上传 PDF、发送消息并触发自动解析。');
        } else if (bridgeEnabled && !isBound) {
            detail = t('settings.weixin.detail_no_binding', '桥接已开启，但还没有绑定账号。生成二维码后扫码即可开始使用。');
        } else if (!bridgeEnabled && isBound) {
            detail = t('settings.weixin.detail_bridge_off', '账号已绑定，但桥接处于关闭状态，当前不会接收或回复微信消息。');
        }

        this.weixinBindingSummary.innerHTML = `
            <section class="weixin-summary-hero">
                <div class="weixin-status-row">
                    <span class="weixin-status-pill ${bridgeEnabled ? 'is-active' : 'is-idle'}">${Utils.escapeHTML(bridgeStateLabel)}</span>
                    <span class="weixin-status-pill ${isBound ? 'is-active' : 'is-idle'}">${Utils.escapeHTML(bindingStateLabel)}</span>
                </div>
                <strong class="weixin-summary-title">${Utils.escapeHTML(overviewTitle)}</strong>
                <p>${Utils.escapeHTML(detail)}</p>
            </section>
            <div class="weixin-summary-meta">
                ${this.renderWeixinSummaryItem(t('settings.weixin.summary_account_label', '绑定账号'), accountLabel, accountFull || t('settings.weixin.summary_not_bound_account', '未绑定'))}
                ${this.renderWeixinSummaryItem(t('settings.weixin.summary_bound_at', '绑定时间'), boundAtLabel)}
                ${this.renderWeixinSummaryItem(t('settings.weixin.summary_endpoint', '接入域名'), endpointLabel, endpointFull || t('settings.weixin.summary_endpoint_default', '使用当前桥接配置'))}
                ${this.renderWeixinSummaryItem(t('settings.weixin.summary_user_id', '用户 ID'), userIDLabel, userIDFull || t('settings.weixin.summary_user_id_none', '未返回'))}
            </div>
        `;

        if (this.startWeixinBindingButton) {
            this.startWeixinBindingButton.textContent = isBound ? t('settings.weixin.rebind', '重新绑定') : t('settings.weixin.start_binding', '开始绑定');
        }
        if (this.unbindWeixinButton) {
            this.unbindWeixinButton.classList.toggle('hidden', !isBound);
        }
        this.updateWeixinDailyRecommendationTestButton();
    },

    renderWeixinSummaryItem(label, value, title = '') {
        const titleAttr = title ? ` title="${Utils.escapeHTML(title)}"` : '';
        return `
            <div class="weixin-summary-item">
                <span>${Utils.escapeHTML(label)}</span>
                <strong${titleAttr}>${Utils.escapeHTML(value)}</strong>
            </div>
        `;
    },

    compactDisplayText(value, prefix = 12, suffix = 10) {
        const normalized = String(value || '').trim();
        if (!normalized) return '';
        if (normalized.length <= prefix + suffix + 3) {
            return normalized;
        }
        return `${normalized.slice(0, prefix)}...${normalized.slice(-suffix)}`;
    },

    compactURLLabel(value) {
        const normalized = String(value || '').trim();
        if (!normalized) return '';
        try {
            return new URL(normalized).host || normalized;
        } catch (_) {
            return this.compactDisplayText(normalized, 16, 10);
        }
    },

    setWeixinQRCodeVisible(visible) {
        this.weixinQRCodePanel?.classList.toggle('hidden', !visible);
        this.weixinQRCodePlaceholder?.classList.toggle('hidden', visible);
    },

    setWeixinBindingStatus(message, tone = '') {
        this.setInlineStatus(this.weixinBindingStatus, message, tone);
    },

    setWolaiStatus(message, tone = '') {
        this.setInlineStatus(this.wolaiStatus, message, tone);
    },

    renderDesktopCloseSummary(action) {
        if (!this.desktopCloseSummary) return;

        const normalized = this.normalizeDesktopCloseAction(action);
        this.desktopCloseSummary.innerHTML = `
            <div>
                <span>${t('settings.desktop_close.current_label', '当前模式')}</span>
                <strong>${Utils.escapeHTML(this.desktopCloseActionLabel(normalized))}</strong>
            </div>
            <div>
                <span>${t('settings.desktop_close.effect_label', '实际行为')}</span>
                <strong>${Utils.escapeHTML(this.desktopCloseActionLabel(normalized))}</strong>
                <p>${Utils.escapeHTML(this.desktopCloseActionEffect(normalized))}</p>
            </div>
            <div>
                <span>${t('settings.desktop_close.scope_label', '作用范围')}</span>
                <strong>${t('settings.desktop_close.scope_value', '桌面端关闭弹窗')}</strong>
                <p>${t('settings.desktop_close.status_hint', '修改后立即生效，也会同步影响关闭弹窗里的“记住这次选择”。')}</p>
            </div>
        `;
    },

    renderWolaiResultLink(url) {
        if (!this.wolaiResultLink) return;

        const normalizedURL = String(url || '').trim();
        if (!normalizedURL) {
            this.wolaiResultLink.textContent = '';
            return;
        }

        this.wolaiResultLink.innerHTML = `${t('settings.wolai.result_link_label', '最新测试页面：')}<a href="${Utils.escapeHTML(normalizedURL)}" target="_blank" rel="noreferrer">${Utils.escapeHTML(normalizedURL)}</a>`;
        this.wolaiResultLink.classList.remove('is-success', 'is-error', 'is-saving');
    },

    setWeixinBridgeSaveStatus(message, tone = '') {
        this.setInlineStatus(this.weixinBridgeSaveStatus, message, tone);
    },

    setWeixinDailyRecommendationSaveStatus(message, tone = '') {
        this.setInlineStatus(this.weixinDailyRecommendationSaveStatus, message, tone);
    },

    setWeixinDailyRecommendationTestStatus(message, tone = '') {
        this.setInlineStatus(this.weixinDailyRecommendationTestStatus, message, tone);
    },

    setTTSSaveStatus(message, tone = '') {
        this.setInlineStatus(this.ttsSaveStatus, message, tone);
    },

    setTTSTestStatus(message, tone = '') {
        this.setInlineStatus(this.ttsTestStatus, message, tone);
    },

    showTTSAudioPreview(blob, filename = '') {
        if (!(blob instanceof Blob) || blob.size <= 0) {
            this.clearTTSAudioPreview();
            return;
        }

        this.clearTTSAudioPreview();
        this.ttsAudioObjectURL = URL.createObjectURL(blob);

        if (this.ttsAudioPlayer) {
            this.ttsAudioPlayer.src = this.ttsAudioObjectURL;
            this.ttsAudioPlayer.title = filename || t('settings.tts.preview_title', '测试音频');
            this.ttsAudioPlayer.load();
        }
        this.ttsAudioPreview?.classList.remove('hidden');
    },

    clearTTSAudioPreview() {
        if (this.ttsAudioPlayer) {
            this.ttsAudioPlayer.pause();
            this.ttsAudioPlayer.removeAttribute('src');
            this.ttsAudioPlayer.removeAttribute('title');
            this.ttsAudioPlayer.load();
        }
        this.ttsAudioPreview?.classList.add('hidden');
        if (this.ttsAudioObjectURL) {
            URL.revokeObjectURL(this.ttsAudioObjectURL);
            this.ttsAudioObjectURL = '';
        }
    },

    async startWeixinBinding() {
        const button = this.startWeixinBindingButton;
        const originalLabel = button?.textContent || t('settings.weixin.start_binding', '开始绑定');
        if (button) {
            button.disabled = true;
            button.textContent = t('settings.weixin.qr_generating', '生成二维码中...');
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
                this.weixinQRCodeLink.textContent = result.qrcode_content || t('settings.weixin.qr_content_unavailable', '二维码内容不可用');
            }
            this.setWeixinQRCodeVisible(true);
            this.setWeixinBindingStatus(result.message || t('settings.weixin.binding_wait', '请使用微信扫码完成绑定'), 'saving');

            if (!this.pendingWeixinQRCode) {
                throw new Error(t('settings.weixin.qr_session_empty', '二维码会话为空，无法跟踪绑定状态'));
            }

            this.scheduleWeixinBindingPoll(1200);
            Utils.showToast(t('settings.weixin.qr_generated_toast', '微信二维码已生成'));
        } catch (error) {
            this.setWeixinBindingStatus(error.message, 'error');
            Utils.showToast(error.message, 'error');
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = this.authSettings?.weixin_binding?.bound ? t('settings.weixin.rebind', '重新绑定') : (originalLabel || t('settings.weixin.start_binding', '开始绑定'));
            }
        }
    },

    async unbindWeixin() {
        if (!confirm(t('settings.weixin.unbind_confirm', '确定要解除微信绑定吗？解除后将停止接收微信消息。'))) return;
        try {
            await API.unbindWeixin();
            if (this.authSettings) {
                this.authSettings.weixin_binding = {};
            }
            this.renderWeixinBindingSummary({});
            this.pendingWeixinQRCode = '';
            this.setWeixinQRCodeVisible(false);
            this.setWeixinBindingStatus('');
            Utils.showToast(t('settings.weixin.unbound_toast', '微信绑定已解除'));
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
            const message = result.message || t('settings.weixin.binding_wait', '等待微信扫码');

            if (status === 'confirmed') {
                this.stopWeixinBindingPolling();
                this.pendingWeixinQRCode = '';
                this.authSettings = {
                    ...(this.authSettings || {}),
                    weixin_binding: result.binding || {}
                };
                this.renderAuthSettings(this.authSettings);
                this.setWeixinQRCodeVisible(false);
                this.setWeixinBindingStatus(message, 'success');
                Utils.showToast(message || t('settings.weixin.binding_success_toast', '微信绑定成功'));
                return;
            }

            if (status === 'expired') {
                this.stopWeixinBindingPolling();
                this.pendingWeixinQRCode = '';
                this.setWeixinQRCodeVisible(false);
                this.setWeixinBindingStatus(message, 'error');
                return;
            }

            this.setWeixinBindingStatus(
                status === 'scaned' ? (message || t('settings.weixin.binding_scanned', '二维码已扫描，请在微信中确认登录')) : message,
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
            ? t('settings.version.build_time', '构建时间：{0}').replace('{0}', Utils.escapeHTML(Utils.formatDate(status.build_time)))
            : t('settings.version.build_time_none', '构建时间未注入');
        const latestVersion = status.latest_version || t('settings.version.not_fetched', '暂未获取');
        const latestDetail = status.published_at
            ? t('settings.version.publish_time', '发布时间：{0}').replace('{0}', Utils.escapeHTML(Utils.formatDate(status.published_at)))
            : t('settings.version.publish_time_none', '尚无发布时间信息');
        const checkedAt = status.checked_at
            ? Utils.formatDate(status.checked_at)
            : t('settings.version.not_checked', '尚未完成检查');

        this.versionSummary.innerHTML = `
            <div>
                <span>${t('settings.version.current', '当前版本')}</span>
                <strong>${Utils.escapeHTML(currentVersion)}</strong>
                <p>${currentDetail}</p>
            </div>
            <div>
                <span>${t('settings.version.check_result', '检查结果')}</span>
                <strong>${badge}</strong>
                <p>${Utils.escapeHTML(status.message || t('settings.version.no_check_message', '尚未检查最新版本'))}</p>
            </div>
            <div>
                <span>${t('settings.version.latest', '最新正式版本')}</span>
                <strong>${Utils.escapeHTML(latestVersion)}</strong>
                <p>${latestDetail}</p>
            </div>
            <div>
                <span>${t('settings.version.last_check', '最近检查')}</span>
                <strong>${Utils.escapeHTML(checkedAt)}</strong>
                <p>${status.latest_release_url ? `${t('settings.version.download_page', '下载页面：')}<a href="${Utils.escapeHTML(status.latest_release_url)}" target="_blank" rel="noreferrer">${Utils.escapeHTML(status.latest_release_url)}</a>` : t('settings.version.no_release_link', '暂无可用的 Release 链接')}</p>
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
            latest: { label: t('settings.version.badge_latest', '已是最新'), tone: 'tone-success' },
            update_available: { label: t('settings.version.badge_update', '发现更新'), tone: 'tone-error' },
            ahead: { label: t('settings.version.badge_ahead', '当前更高'), tone: 'tone-info' },
            unknown: { label: t('settings.version.badge_unknown', '无法判断'), tone: 'tone-info' }
        };
        const badge = badges[status] || badges.unknown;
        return `<span class="status-badge ${badge.tone}" title="${Utils.escapeHTML(message || badge.label)}">${Utils.escapeHTML(badge.label)}</span>`;
    },

    providerNoteText(provider) {
        const notes = {
            openai: t('settings.ai.provider_note_openai', 'OpenAI 默认使用 Responses API。勾选传统模式后会切到 Chat Completions，以兼容多数 OpenAI 风格网关。'),
            anthropic: t('settings.ai.provider_note_anthropic', 'Anthropic 使用原生 Messages API，请填写兼容的 Base URL 和模型名。'),
            gemini: t('settings.ai.provider_note_gemini', 'Gemini 使用 generateContent 接口，API Key 会通过 query 参数发送。')
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
            this.aiModelList.innerHTML = `<p class="muted">${t('settings.ai.no_models', '还没有模型，先新增一个。')}</p>`;
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
        return model.name || model.model || t('settings.ai.unnamed_model', '未命名模型');
    },

    aiModelButtonMeta(model) {
        const provider = model.provider || 'openai';
        const modelName = model.model || t('settings.ai.unnamed_model_id', '未填写模型名');
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
        this.setAIModelAutosaveStatus(t('settings.ai.model_added', '已新增模型。'), 'success');
        this.openAIModelEditor(nextModel.id);
        Utils.showToast(t('settings.ai.model_added_toast', '已新增模型'));
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
        this.aiModelModalTitle.textContent = t('settings.ai.edit_model', '编辑模型 · {0}').replace('{0}', this.aiModelButtonTitle(model));
        this.aiModelNameInput.value = model.name || '';
        this.aiModelProviderInput.value = model.provider || 'openai';
        this.aiModelIdentifierInput.value = model.model || '';
        this.aiModelMaxTokensInput.value = Number(model.max_output_tokens || 1200);
        this.aiModelBaseURLInput.value = model.base_url || '';
        this.aiModelAPIKeyInput.value = model.api_key || '';
        this.aiModelLegacyModeInput.checked = Boolean(model.openai_legacy_mode);
        this.aiModelCheckStatus.textContent = model.check_status || t('settings.ai.not_checked', '尚未检查');
        this.deleteAIModelButton.disabled = this.aiModelDraft.length <= 1;
        this.updateAIModelModalUI();
        this.setAIModelEditorStatus(t('settings.ai.model_editor_autosave', '修改后自动保存。'));
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
            Utils.showToast(t('settings.ai.model_keep_one', '至少需要保留一个 AI 模型'), 'error');
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
        this.setAIModelAutosaveStatus(t('settings.ai.model_deleted', '模型已删除。'), 'success');
        Utils.showToast(t('settings.ai.model_deleted_toast', '模型已删除'));
    },

    async deleteCurrentAIModel() {
        if (!this.editingAIModelID) return;
        if (this.aiModelDraft.length <= 1) {
            Utils.showToast(t('settings.ai.model_keep_one', '至少需要保留一个 AI 模型'), 'error');
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
            im_intent_model_id: selection.im_intent_model_id || this.imIntentModelSelect?.value || '',
            figure_model_id: selection.figure_model_id || this.figureModelSelect?.value || '',
            tag_model_id: selection.tag_model_id || this.tagModelSelect?.value || '',
            group_model_id: selection.group_model_id || this.groupModelSelect?.value || '',
            translate_model_id: selection.translate_model_id || this.translateModelSelect?.value || '',
            tts_model_id: selection.tts_model_id || this.ttsModelSelect?.value || ''
        };
        const options = this.aiModelDraft.map((item) => {
            const label = `${item.name || t('settings.ai.unnamed_model', '未命名模型')} · ${item.provider || 'openai'} / ${item.model || t('settings.ai.unnamed_model_id', '未填写模型名')}`;
            return `<option value="${Utils.escapeHTML(item.id)}">${Utils.escapeHTML(label)}</option>`;
        }).join('');

        [
            [this.defaultModelSelect, safeSelection.default_model_id],
            [this.qaModelSelect, safeSelection.qa_model_id],
            [this.imIntentModelSelect, safeSelection.im_intent_model_id],
            [this.figureModelSelect, safeSelection.figure_model_id],
            [this.tagModelSelect, safeSelection.tag_model_id],
            [this.groupModelSelect, safeSelection.group_model_id],
            [this.translateModelSelect, safeSelection.translate_model_id],
            [this.ttsModelSelect, safeSelection.tts_model_id]
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
            im_intent_model_id: this.imIntentModelSelect?.value || defaultModelID,
            figure_model_id: this.figureModelSelect?.value || defaultModelID,
            tag_model_id: this.tagModelSelect?.value || defaultModelID,
            group_model_id: this.groupModelSelect?.value || defaultModelID,
            translate_model_id: this.translateModelSelect?.value || defaultModelID,
            tts_model_id: this.ttsModelSelect?.value || defaultModelID
        };
    },

    aiModelDisplayLabel(modelID, fallbackModelID = '', emptyLabel = '未配置') {
        const normalizedModelID = String(modelID || '').trim() || String(fallbackModelID || '').trim();
        if (!normalizedModelID) {
            return emptyLabel;
        }

        const matchedModel = (this.aiModelDraft || []).find((item) => item.id === normalizedModelID);
        if (!matchedModel) {
            return emptyLabel;
        }

        return `${matchedModel.name || t('settings.ai.unnamed_model', '未命名模型')} · ${matchedModel.provider || 'openai'} / ${matchedModel.model || t('settings.ai.unnamed_model_id', '未填写模型名')}`;
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
        this.checkAIModelButton.textContent = t('settings.ai.check_btn_checking', '检查中...');

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
            this.setAIModelEditorStatus(t('settings.ai.check_passed', '模型检查通过。'), 'success');
            Utils.showToast(t('settings.ai.check_passed_toast', '模型检查通过'));
        } catch (error) {
            this.aiModelCheckStatus.textContent = t('settings.ai.check_failed', '检查失败：{0}').replace('{0}', error.message);
            this.setAIModelEditorStatus(t('settings.ai.check_failed_status', '模型检查失败：{0}').replace('{0}', error.message), 'error');
            Utils.showToast(error.message, 'error');
        } finally {
            this.checkAIModelButton.disabled = false;
            this.checkAIModelButton.textContent = originalLabel;
        }
    },

    renderExtractorSummary(settings) {
        const profile = String(settings?.extractor_profile || '').trim();
        const usesBuiltIn = profile === 'open_source_vision';
        const usesManual = profile === 'manual';
        if (usesManual) {
            this.extractorSummary.innerHTML = `
                <div><span>${t('settings.extractor.summary_profile', '提取方案')}</span><strong>${Utils.escapeHTML(this.extractorProfileLabel(settings.extractor_profile))}</strong></div>
                <div><span>${t('settings.extractor.summary_figure_extract', '图片提取')}</span><strong>${t('settings.extractor.summary_figure_off', '关闭，仅保留手工标注')}</strong></div>
                <div><span>${t('settings.extractor.summary_fulltext', '全文处理')}</span><strong>${t('settings.extractor.summary_fulltext_auto', '上传后自动提取并保存 PDF 全文')}</strong></div>
                <div><span>${t('settings.extractor.summary_external', '外部提取服务')}</span><strong>${t('settings.extractor.summary_external_hidden', '已隐藏，不使用')}</strong></div>
            `;
            return;
        }
        if (usesBuiltIn) {
            const figureModelLabel = this.aiModelDisplayLabel(
                this.figureModelSelect?.value,
                this.defaultModelSelect?.value,
                t('settings.version.loading_btn', '载入中...')
            );
            this.extractorSummary.innerHTML = `
                <div><span>${t('settings.extractor.summary_profile', '提取方案')}</span><strong>${Utils.escapeHTML(this.extractorProfileLabel(settings.extractor_profile))}</strong></div>
                <div><span>${t('settings.extractor.summary_text_source', '全文来源')}</span><strong>${Utils.escapeHTML(this.extractorPDFTextSourceLabel(settings.pdf_text_source))}</strong></div>
                <div><span>${t('settings.extractor.summary_coord_model', '坐标提取模型')}</span><strong>${Utils.escapeHTML(figureModelLabel)}</strong></div>
                <div><span>${t('settings.extractor.summary_external', '外部提取服务')}</span><strong>${t('settings.extractor.summary_external_hidden', '已隐藏，不使用')}</strong></div>
            `;
            return;
        }

        this.extractorSummary.innerHTML = `
            <div><span>${t('settings.extractor.summary_profile', '提取方案')}</span><strong>${Utils.escapeHTML(this.extractorProfileLabel(settings.extractor_profile))}</strong></div>
            <div><span>${t('settings.extractor.summary_text_source', '全文来源')}</span><strong>${Utils.escapeHTML(this.extractorPDFTextSourceLabel(settings.pdf_text_source))}</strong></div>
            <div><span>${t('settings.extractor.summary_effective_extract', '生效的提取接口')}</span><strong class="settings-url-value">${Utils.escapeHTML(settings.effective_extractor_url || t('settings.extractor.not_configured', '未配置'))}</strong></div>
            <div><span>${t('settings.extractor.summary_effective_jobs', '生效的任务接口')}</span><strong class="settings-url-value">${Utils.escapeHTML(settings.effective_jobs_url || t('settings.extractor.not_configured', '未配置'))}</strong></div>
            <div><span>${t('settings.extractor.summary_file_field', '上传字段名')}</span><strong>${Utils.escapeHTML(settings.extractor_file_field || 'file')}</strong></div>
            <div><span>${t('settings.extractor.summary_token', '鉴权 Token')}</span><strong>${Utils.escapeHTML(settings.extractor_token ? t('settings.extractor.configured', '已配置') : t('settings.extractor.not_configured', '未配置'))}</strong></div>
        `;
    },

    extractorProfileLabel(value) {
        switch (String(value || '').trim()) {
            case 'manual':
                return t('settings.extractor.profile_manual', '手工');
            case 'open_source_vision':
                return t('settings.extractor.profile_builtin', '内置 LLM 坐标提取');
            case 'pdffigx_v1':
            default:
                return t('settings.extractor.profile_pdffigx', '标准 pdffigx');
        }
    },

    extractorPDFTextSourceLabel(value) {
        switch (String(value || '').trim()) {
            case 'pdfjs':
                return t('settings.extractor.text_source_pdfjs', '浏览器 pdf.js');
            case 'extractor':
            default:
                return t('settings.extractor.text_source_extractor', '解析服务返回');
        }
    },

    extractorPDFTextSourceValue(profile) {
        return ['manual', 'open_source_vision'].includes(String(profile || '').trim()) ? 'pdfjs' : 'extractor';
    },

    syncExtractorProfileFormState() {
        const profile = String(this.extractorProfileSelect?.value || '').trim();
        const usesBuiltIn = profile === 'open_source_vision';
        const usesManual = profile === 'manual';

        [
            this.extractorFigureModelField,
            this.extractorURLField,
            this.extractorTokenField,
            this.extractorFileFieldField,
            this.extractorTimeoutField,
            this.extractorPollIntervalField,
            this.extractorPDFFigXHint
        ].forEach((element) => {
            if (!element) return;
            if (element === this.extractorFigureModelField) {
                element.classList.toggle('hidden', !usesBuiltIn);
                return;
            }
            element.classList.toggle('hidden', usesBuiltIn || usesManual);
        });
        this.extractorManualHint?.classList.toggle('hidden', !usesManual);
        this.extractorBuiltInHint?.classList.toggle('hidden', !usesBuiltIn);

        [
            this.figureModelSelect,
            this.extractorURLInput,
            this.extractorTokenInput,
            this.extractorFileFieldInput,
            this.extractorTimeoutInput,
            this.extractorPollIntervalInput
        ].forEach((element) => {
            if (element) {
                if (element === this.figureModelSelect) {
                    element.disabled = !usesBuiltIn;
                    return;
                }
                element.disabled = usesBuiltIn || usesManual;
            }
        });
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
                Utils.showToast(t('settings.db.export_done', '数据库导出完成'));
            }
        } catch (error) {
            Utils.showToast(error.message || t('settings.db.export_failed', '数据库导出失败'), 'error');
        }
    },

    async importDatabase() {
        const file = this.importDbFile.files[0];
        if (!file) {
            Utils.showToast(t('settings.db.import_no_file', '请选择要导入的数据库文件'), 'error');
            return;
        }

        const confirmed = await Utils.confirmTypedAction({
            title: t('settings.db.import_confirm_title', '覆盖导入数据库'),
            badge: 'Import Override',
            message: t('settings.db.import_confirm_message', '导入数据库会用备份文件覆盖当前所有文献、图片、分组和标签。确认后将立即开始恢复。'),
            keyword: 'IMPORT',
            hint: t('settings.db.import_confirm_hint', '请输入 IMPORT 继续导入'),
            confirmLabel: t('settings.db.import_confirm_label', '开始导入')
        });
        if (!confirmed) return;

        try {
            const formData = new FormData();
            formData.append('database', file);
            await API.importDatabase(formData);
            Utils.showToast(t('settings.db.import_done', '数据库导入成功，页面将刷新'));
            setTimeout(() => window.location.reload(), 1500);
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async purgeDatabase() {
        const confirmed = await Utils.confirmTypedAction({
            title: t('settings.db.purge_confirm_title', '清空数据库'),
            badge: 'Danger Zone',
            message: t('settings.db.purge_confirm_message', '这会删除所有文献、提取图片、分组和标签，并且不可恢复。该操作只适合在你明确要重置整个库时使用。'),
            keyword: 'CLEAR',
            hint: t('settings.db.purge_confirm_hint', '请输入 CLEAR 继续清空数据库'),
            confirmLabel: t('settings.db.purge_confirm_label', '确认清空')
        });
        if (!confirmed) return;

        try {
            await API.purgeLibrary();
            Utils.showToast(t('settings.db.purge_done', '数据库已清空'));
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async changePassword() {
        const currentPassword = this.currentPasswordInput.value.trim();
        const newPassword = this.newPasswordInput.value.trim();
        const confirmPassword = this.confirmPasswordInput.value.trim();

        if (!currentPassword || !newPassword || !confirmPassword) {
            Utils.showToast(t('settings.password.empty_fields', '请填写所有密码字段'), 'error');
            return;
        }

        if (newPassword.length < 6) {
            Utils.showToast(t('settings.password.too_short', '新密码长度不能少于 6 位'), 'error');
            return;
        }

        if (newPassword !== confirmPassword) {
            Utils.showToast(t('settings.password.mismatch', '两次输入的新密码不一致'), 'error');
            return;
        }

        try {
            await API.changePassword({
                current_password: currentPassword,
                new_password: newPassword
            });
            Utils.showToast(t('settings.password.changed_toast', '密码修改成功，请使用新密码重新登录'));
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

    async updateRememberLogin() {
        if (!this.rememberLoginEnabledInput) return;

        const enabled = Boolean(this.rememberLoginEnabledInput.checked);
        const previous = Boolean(this.authSettings?.remember_login_enabled);

        this.rememberLoginEnabledInput.disabled = true;
        this.setRememberLoginStatus(
            enabled
                ? t('settings.password.remember_status_enabling', '正在开启记住登录状态...')
                : t('settings.password.remember_status_disabling', '正在关闭记住登录状态...'),
            'saving'
        );

        try {
            const result = await API.updateRememberLogin({ enabled });
            this.authSettings = {
                ...(this.authSettings || {}),
                remember_login_enabled: Boolean(result.remember_login_enabled)
            };
            this.renderAuthSettings(this.authSettings);
            Utils.showToast(
                this.authSettings.remember_login_enabled
                    ? t('settings.password.remember_toast_enabled', '已记住这台设备的登录状态')
                    : t('settings.password.remember_toast_disabled', '已取消记住这台设备的登录状态')
            );
        } catch (error) {
            this.rememberLoginEnabledInput.checked = previous;
            this.setRememberLoginStatus(previous);
            Utils.showToast(error.message, 'error');
        } finally {
            this.rememberLoginEnabledInput.disabled = false;
        }
    },

    async logout() {
        const confirmed = await Utils.confirm(t('settings.password.logout_confirm', '确定要登出吗？'));
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

        Utils.showToast(t('settings.password.logout_toast', '已登出'));
        setTimeout(() => {
            window.location.href = '/login';
        }, 1000);
    }
};
