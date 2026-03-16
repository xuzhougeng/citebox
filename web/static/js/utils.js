// 工具函数模块
const Utils = {
    formatFileSize(bytes) {
        if (bytes === 0) return '0 Bytes';
        const k = 1024;
        const sizes = ['Bytes', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    },

    formatDate(dateString) {
        const date = new Date(dateString);
        return date.toLocaleDateString('zh-CN', {
            year: 'numeric',
            month: '2-digit',
            day: '2-digit',
            hour: '2-digit',
            minute: '2-digit'
        });
    },

    showToast(message, type = 'success') {
        const toast = document.createElement('div');
        toast.className = `toast toast-${type}`;
        toast.textContent = message;
        document.body.appendChild(toast);
        setTimeout(() => {
            toast.remove();
        }, 3000);
    },

    debounce(func, wait) {
        let timeout;
        return function executedFunction(...args) {
            const later = () => {
                clearTimeout(timeout);
                func(...args);
            };
            clearTimeout(timeout);
            timeout = setTimeout(later, wait);
        };
    },

    confirm(message, title = '确认') {
        return new Promise((resolve) => {
            const overlay = document.createElement('div');
            overlay.className = 'dialog-overlay';
            overlay.innerHTML = `
                <div class="dialog-box">
                    <div class="dialog-header">
                        <h3>${title}</h3>
                    </div>
                    <div class="dialog-body">
                        <p>${message}</p>
                    </div>
                    <div class="dialog-footer">
                        <button class="btn btn-outline dialog-cancel">取消</button>
                        <button class="btn btn-danger dialog-confirm">确定</button>
                    </div>
                </div>
            `;
            document.body.appendChild(overlay);
            
            // 动画显示
            requestAnimationFrame(() => overlay.classList.add('active'));

            const close = (result) => {
                overlay.classList.remove('active');
                setTimeout(() => overlay.remove(), 200);
                resolve(result);
            };

            overlay.querySelector('.dialog-cancel').onclick = () => close(false);
            overlay.querySelector('.dialog-confirm').onclick = () => close(true);
            overlay.onclick = (e) => { if (e.target === overlay) close(false); };
        });
    },

    confirmTypedAction(options = {}) {
        const {
            title = '危险操作确认',
            message = '',
            keyword = 'CLEAR',
            confirmLabel = '确认继续',
            hint = `请输入 ${keyword} 继续`,
            badge = 'Danger Zone'
        } = options;

        return new Promise((resolve) => {
            const normalizedKeyword = String(keyword || '').trim();
            const overlay = document.createElement('div');
            overlay.className = 'dialog-overlay';
            overlay.innerHTML = `
                <div class="dialog-box dialog-box-danger">
                    <div class="dialog-danger-head">
                        <span class="dialog-danger-badge">${Utils.escapeHTML(badge)}</span>
                        <div class="dialog-header">
                            <h3>${Utils.escapeHTML(title)}</h3>
                        </div>
                    </div>
                    <div class="dialog-body dialog-danger-body">
                        <p class="dialog-danger-message">${Utils.escapeHTML(message)}</p>
                        <div class="dialog-danger-instruction">
                            <span>确认口令</span>
                            <strong>${Utils.escapeHTML(normalizedKeyword)}</strong>
                        </div>
                        <label class="dialog-danger-field">
                            <span>${Utils.escapeHTML(hint)}</span>
                            <input class="form-input dialog-confirm-input" type="text" autocomplete="off" spellcheck="false" placeholder="${Utils.escapeHTML(normalizedKeyword)}">
                        </label>
                    </div>
                    <div class="dialog-footer">
                        <button class="btn btn-outline dialog-cancel">取消</button>
                        <button class="btn btn-outline danger dialog-confirm" type="button" disabled>${Utils.escapeHTML(confirmLabel)}</button>
                    </div>
                </div>
            `;
            document.body.appendChild(overlay);
            requestAnimationFrame(() => overlay.classList.add('active'));

            const input = overlay.querySelector('.dialog-confirm-input');
            const confirmButton = overlay.querySelector('.dialog-confirm');

            const close = (result) => {
                document.removeEventListener('keydown', onKeydown);
                overlay.classList.remove('active');
                setTimeout(() => overlay.remove(), 200);
                resolve(result);
            };

            const syncState = () => {
                confirmButton.disabled = input.value.trim() !== normalizedKeyword;
            };

            const onKeydown = (event) => {
                if (event.key === 'Escape') {
                    close(false);
                }
            };

            input.addEventListener('input', syncState);
            input.addEventListener('keydown', (event) => {
                if (event.key === 'Enter' && !confirmButton.disabled) {
                    event.preventDefault();
                    close(true);
                }
            });

            overlay.querySelector('.dialog-cancel').onclick = () => close(false);
            confirmButton.onclick = () => close(true);
            overlay.onclick = (event) => {
                if (event.target === overlay) close(false);
            };
            document.addEventListener('keydown', onKeydown);
            setTimeout(() => input.focus(), 0);
        });
    },

    alert(message, title = '提示') {
        return new Promise((resolve) => {
            const overlay = document.createElement('div');
            overlay.className = 'dialog-overlay';
            overlay.innerHTML = `
                <div class="dialog-box">
                    <div class="dialog-header">
                        <h3>${title}</h3>
                    </div>
                    <div class="dialog-body">
                        <p>${message}</p>
                    </div>
                    <div class="dialog-footer">
                        <button class="btn btn-primary dialog-ok">确定</button>
                    </div>
                </div>
            `;
            document.body.appendChild(overlay);
            
            requestAnimationFrame(() => overlay.classList.add('active'));

            const close = () => {
                overlay.classList.remove('active');
                setTimeout(() => overlay.remove(), 200);
                resolve();
            };

            overlay.querySelector('.dialog-ok').onclick = close;
            overlay.onclick = (e) => { if (e.target === overlay) close(); };
        });
    },

    escapeHTML(value = '') {
        return String(value)
            .replaceAll('&', '&amp;')
            .replaceAll('<', '&lt;')
            .replaceAll('>', '&gt;')
            .replaceAll('"', '&quot;')
            .replaceAll("'", '&#39;');
    },

    splitTags(value = '') {
        return value
            .split(',')
            .map((item) => item.trim())
            .filter(Boolean);
    },

    joinTags(tags = []) {
        return tags.map((tag) => tag.name || tag).join(', ');
    },

    isProcessingStatus(status = '') {
        return status === 'queued' || status === 'running';
    },

    statusTone(status = '') {
        if (status === 'completed') return 'success';
        if (status === 'failed' || status === 'cancelled') return 'error';
        if (status === 'queued' || status === 'running' || status === 'manual_pending') return 'info';
        return 'info';
    },

    statusLabel(status = '') {
        if (status === 'queued') return '等待解析';
        if (status === 'running') return '解析中';
        if (status === 'manual_pending') return '待人工处理';
        if (status === 'completed') return '解析完成';
        if (status === 'failed') return '解析失败';
        if (status === 'cancelled') return '已取消';
        return status || '未知状态';
    }
};
