const SettingsPage = {
    init() {
        if (this.initialized) return;
        this.initialized = true;

        this.aiSettingsForm = document.getElementById('aiSettingsForm');
        this.providerInput = document.getElementById('aiProviderInput');
        this.modelInput = document.getElementById('aiModelInput');
        this.baseURLInput = document.getElementById('aiBaseURLInput');
        this.apiKeyInput = document.getElementById('aiAPIKeyInput');
        this.temperatureInput = document.getElementById('aiTemperatureInput');
        this.maxTokensInput = document.getElementById('aiMaxTokensInput');
        this.maxFiguresInput = document.getElementById('aiMaxFiguresInput');
        this.openAILegacyInput = document.getElementById('aiOpenAILegacyInput');
        this.systemPromptInput = document.getElementById('aiSystemPromptInput');
        this.qaPromptInput = document.getElementById('aiQAPromptInput');
        this.figurePromptInput = document.getElementById('aiFigurePromptInput');
        this.tagPromptInput = document.getElementById('aiTagPromptInput');
        this.providerNote = document.getElementById('aiProviderNote');

        this.extractorSettingsForm = document.getElementById('extractorSettingsForm');
        this.extractorURLInput = document.getElementById('extractorURLInput');
        this.extractorJobsURLInput = document.getElementById('extractorJobsURLInput');
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
        this.extractorSettingsForm.addEventListener('submit', async (event) => {
            event.preventDefault();
            await this.saveExtractorSettings();
        });
        this.providerInput.addEventListener('change', () => this.updateProviderUI());

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

        this.providerInput.value = settings.provider || 'openai';
        this.modelInput.value = settings.model || '';
        this.baseURLInput.value = settings.base_url || '';
        this.apiKeyInput.value = settings.api_key || '';
        this.temperatureInput.value = settings.temperature ?? 0.2;
        this.maxTokensInput.value = settings.max_output_tokens ?? 1200;
        this.maxFiguresInput.value = settings.max_figures ?? 0;
        this.openAILegacyInput.checked = Boolean(settings.openai_legacy_mode);
        this.systemPromptInput.value = settings.system_prompt || '';
        this.qaPromptInput.value = settings.qa_prompt || '';
        this.figurePromptInput.value = settings.figure_prompt || '';
        this.tagPromptInput.value = settings.tag_prompt || '';

        this.updateProviderUI();
    },

    async saveAISettings() {
        const payload = {
            provider: this.providerInput.value,
            model: this.modelInput.value.trim(),
            base_url: this.baseURLInput.value.trim(),
            api_key: this.apiKeyInput.value.trim(),
            temperature: this.temperatureInput.value === '' ? 0.2 : Number(this.temperatureInput.value),
            max_output_tokens: this.maxTokensInput.value === '' ? 1200 : Number(this.maxTokensInput.value),
            max_figures: Number(this.maxFiguresInput.value || 0),
            openai_legacy_mode: this.openAILegacyInput.checked,
            system_prompt: this.systemPromptInput.value.trim(),
            qa_prompt: this.qaPromptInput.value.trim(),
            figure_prompt: this.figurePromptInput.value.trim(),
            tag_prompt: this.tagPromptInput.value.trim()
        };

        await API.updateAISettings(payload);
        await this.loadAISettings();
        Utils.showToast('AI 配置已保存');
    },

    async loadExtractorSettings() {
        const settings = await API.getExtractorSettings();

        this.extractorURLInput.value = settings.extractor_url || '';
        this.extractorJobsURLInput.value = settings.extractor_jobs_url || '';
        this.extractorTokenInput.value = settings.extractor_token || '';
        this.extractorFileFieldInput.value = settings.extractor_file_field || 'file';
        this.extractorTimeoutInput.value = settings.timeout_seconds ?? 300;
        this.extractorPollIntervalInput.value = settings.poll_interval_seconds ?? 2;

        this.renderExtractorSummary(settings);
    },

    async saveExtractorSettings() {
        const payload = {
            extractor_url: this.extractorURLInput.value.trim(),
            extractor_jobs_url: this.extractorJobsURLInput.value.trim(),
            extractor_token: this.extractorTokenInput.value.trim(),
            extractor_file_field: this.extractorFileFieldInput.value.trim(),
            timeout_seconds: Number(this.extractorTimeoutInput.value || 300),
            poll_interval_seconds: Number(this.extractorPollIntervalInput.value || 2)
        };

        const response = await API.updateExtractorSettings(payload);
        this.renderExtractorSummary(response.settings);
        Utils.showToast('PDF 提取服务配置已保存');
    },

    updateProviderUI() {
        const provider = this.providerInput.value;
        const legacyEnabled = provider === 'openai';
        this.openAILegacyInput.disabled = !legacyEnabled;
        if (!legacyEnabled) {
            this.openAILegacyInput.checked = false;
        }

        const notes = {
            openai: 'OpenAI 默认使用 Responses API。勾选传统模式后会切到 Chat Completions，以兼容多数 OpenAI 风格网关。',
            anthropic: 'Anthropic 使用原生 Messages API，请填写兼容的 Base URL 和模型名。',
            gemini: 'Gemini 使用 generateContent 接口，API Key 会通过 query 参数发送。'
        };
        this.providerNote.textContent = notes[provider] || '';
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

        const confirmed = await Utils.confirm(
            '导入数据库将覆盖现有所有数据（文献、图片、分组、标签），且不可恢复。确定要继续吗？'
        );
        if (!confirmed) return;

        const token = window.prompt('为避免误操作，请输入 IMPORT 继续');
        if (token !== 'IMPORT') {
            Utils.showToast('未完成导入确认', 'info');
            return;
        }

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
        const confirmed = await Utils.confirm('这会清空所有文献、提取图片、分组和标签，且不可恢复。');
        if (!confirmed) return;

        const token = window.prompt('为避免误操作，请输入 CLEAR 继续');
        if (token !== 'CLEAR') {
            Utils.showToast('未完成清库确认', 'info');
            return;
        }

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
