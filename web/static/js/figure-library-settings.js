const FigureLibrarySettings = {
    initialized: false,

    init() {
        if (this.initialized) return;
        this.initialized = true;
        this.form = document.getElementById('figureLibrarySettingsForm');
        if (!this.form) return;
        this.dropDirInput = document.getElementById('figureLibraryDropDirInput');
        this.summary = document.getElementById('figureLibrarySummary');
        this.status = document.getElementById('figureLibraryStatus');
        this.testButton = document.getElementById('figureLibraryTestButton');

        this.form.addEventListener('submit', async (event) => {
            event.preventDefault();
            await this.saveSettings();
        });
        this.testButton?.addEventListener('click', () => this.testStatus());
        this.bootstrap();
    },

    async bootstrap() {
        try {
            const settings = await API.getFigureLibrarySettings();
            this.applySettings(settings || {});
            await this.testStatus(true);
        } catch (error) {
            this.setStatus(this.status, error.message || t('settings.figure_library.load_failed', '读取 Figure Library 设置失败'));
        }
    },

    applySettings(settings) {
        if (this.dropDirInput) {
            this.dropDirInput.value = settings.drop_dir || '';
        }
    },

    settingsPayload() {
        return { drop_dir: (this.dropDirInput?.value || '').trim() };
    },

    async saveSettings() {
        try {
            const settings = await API.updateFigureLibrarySettings(this.settingsPayload());
            this.applySettings(settings || {});
            this.setStatus(this.status, t('settings.figure_library.saved', 'Figure Library 设置已保存'));
            await this.testStatus(true);
        } catch (error) {
            this.setStatus(this.status, error.message || t('settings.figure_library.save_failed', '保存失败'));
        }
    },

    async testStatus(silent = false) {
        try {
            const status = await API.getFigureLibraryStatus();
            this.renderSummary(status || {});
            if (!silent) {
                this.setStatus(this.status, status?.message || t('settings.figure_library.status_done', '已检查接收目录'));
            }
        } catch (error) {
            this.setStatus(this.status, error.message || t('settings.figure_library.status_failed', '检查接收目录失败'));
        }
    },

    renderSummary(status) {
        if (!this.summary) return;
        const ready = status.ready ? t('settings.figure_library.ready_yes', '可用') : t('settings.figure_library.ready_no', '不可用');
        const dir = status.drop_dir || '-';
        this.summary.innerHTML = `
            <div><span>${t('settings.figure_library.summary_dir', '接收目录')}</span><strong>${this.escapeHTML(dir)}</strong></div>
            <div><span>${t('settings.figure_library.summary_ready', '状态')}</span><strong>${ready}</strong></div>
        `;
    },

    setStatus(node, message) {
        if (node) node.textContent = message;
    },

    escapeHTML(value) {
        return String(value ?? '')
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }
};

document.addEventListener('DOMContentLoaded', () => {
    if (document.getElementById('figureLibrarySettingsForm')) {
        FigureLibrarySettings.init();
    }
});
