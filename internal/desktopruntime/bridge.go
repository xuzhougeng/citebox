package desktopruntime

import (
	"encoding/json"
	"fmt"

	webview "github.com/webview/webview_go"
)

var desktopInternalRoutes = []string{
	"/",
	"/index.html",
	"/library",
	"/library.html",
	"/guide",
	"/guide.html",
	"/upload",
	"/upload.html",
	"/manual",
	"/manual.html",
	"/viewer",
	"/viewer.html",
	"/figures",
	"/figures.html",
	"/groups",
	"/groups.html",
	"/tags",
	"/tags.html",
	"/notes",
	"/notes.html",
	"/ai",
	"/ai.html",
	"/settings",
	"/settings.html",
	"/login",
	"/login.html",
}

const desktopBridgeScript = `(function() {
    window.__CITEBOX_DESKTOP__ = true;

    const normalizePath = (path = '') => {
        const normalized = String(path || '').trim();
        if (!normalized || normalized === '/index.html') {
            return '/';
        }
        return normalized.replace(/\.html$/, '');
    };

    const internalRoutes = new Set(%s.map(normalizePath));
    const resolveURL = (value) => {
        try {
            return new URL(value, window.location.href);
        } catch (error) {
            return null;
        }
    };

    const isInternalRoute = (value) => {
        const url = resolveURL(value);
        if (!url) {
            return false;
        }
        return url.origin === window.location.origin && internalRoutes.has(normalizePath(url.pathname));
    };

    const isSameOrigin = (value) => {
        const url = resolveURL(value);
        if (!url) {
            return false;
        }
        return url.origin === window.location.origin;
    };

    const navigateInPlace = (value) => {
        const url = resolveURL(value);
        if (!url) {
            return;
        }
        window.location.assign(url.href);
    };

    const openExternal = (value) => {
        const url = resolveURL(value);
        if (!url || typeof window.citeboxDesktopOpenExternal !== 'function') {
            return;
        }
        void window.citeboxDesktopOpenExternal(url.href).catch(() => {});
    };

    const prefersChinese = () => String(document.documentElement.lang || '').toLowerCase().startsWith('zh');
    const desktopTranslate = (key, zh, en) => {
        if (typeof window.t === 'function') {
            return window.t(key, prefersChinese() ? zh : en);
        }
        return prefersChinese() ? zh : en;
    };

    const closePromptStyleId = 'citeboxDesktopClosePromptStyle';
    const closePromptOverlayId = 'citeboxDesktopClosePrompt';

    const ensureClosePromptStyles = () => {
        if (document.getElementById(closePromptStyleId)) {
            return;
        }

        const style = document.createElement('style');
        style.id = closePromptStyleId;
        style.textContent = [
            '.citebox-desktop-close-overlay {',
            '    position: fixed;',
            '    inset: 0;',
            '    z-index: 2147483646;',
            '    display: flex;',
            '    align-items: center;',
            '    justify-content: center;',
            '    padding: 1.5rem;',
            '    background: rgba(17, 24, 39, 0.42);',
            '    backdrop-filter: blur(18px);',
            '    -webkit-backdrop-filter: blur(18px);',
            '    opacity: 0;',
            '    transition: opacity 180ms ease;',
            '}',
            '.citebox-desktop-close-overlay.active {',
            '    opacity: 1;',
            '}',
            '.citebox-desktop-close-card {',
            '    width: min(30rem, 100%%);',
            '    display: grid;',
            '    gap: 1rem;',
            '    padding: 1.35rem;',
            '    border-radius: 1.6rem;',
            '    color: var(--ink, #1f2937);',
            '    background:',
            '        radial-gradient(circle at top right, rgba(var(--accent-rgb, 193, 127, 89), 0.16), transparent 12rem),',
            '        linear-gradient(180deg, rgba(var(--surface-rgb, 255, 255, 255), 0.985), rgba(var(--surface-rgb, 255, 255, 255), 0.955));',
            '    border: 1px solid rgba(var(--shadow-deep-rgb, 15, 23, 42), 0.08);',
            '    box-shadow: 0 28px 80px rgba(var(--shadow-deep-rgb, 15, 23, 42), 0.26);',
            '    transform: translateY(0.75rem) scale(0.985);',
            '    transition: transform 180ms ease;',
            '}',
            '.citebox-desktop-close-overlay.active .citebox-desktop-close-card {',
            '    transform: translateY(0) scale(1);',
            '}',
            '.citebox-desktop-close-badge {',
            '    display: inline-flex;',
            '    width: fit-content;',
            '    align-items: center;',
            '    justify-content: center;',
            '    padding: 0.38rem 0.75rem;',
            '    border-radius: 999px;',
            '    background: rgba(var(--accent-rgb, 193, 127, 89), 0.12);',
            '    color: var(--accent-deep, var(--accent, #a66a48));',
            '    font-size: 0.76rem;',
            '    font-weight: 800;',
            '    letter-spacing: 0.08em;',
            '    text-transform: uppercase;',
            '}',
            '.citebox-desktop-close-head {',
            '    display: grid;',
            '    gap: 0.6rem;',
            '}',
            '.citebox-desktop-close-head h3 {',
            '    margin: 0;',
            '    font-size: 1.35rem;',
            '    line-height: 1.2;',
            '}',
            '.citebox-desktop-close-head p,',
            '.citebox-desktop-close-note,',
            '.citebox-desktop-close-error {',
            '    margin: 0;',
            '    line-height: 1.65;',
            '}',
            '.citebox-desktop-close-head p,',
            '.citebox-desktop-close-note {',
            '    color: var(--muted, #6b7280);',
            '}',
            '.citebox-desktop-close-note {',
            '    padding: 0.9rem 1rem;',
            '    border-radius: 1rem;',
            '    background: rgba(var(--ink-rgb, 31, 41, 55), 0.05);',
            '    border: 1px solid rgba(var(--ink-rgb, 31, 41, 55), 0.08);',
            '}',
            '.citebox-desktop-close-error {',
            '    color: var(--error, #c9544d);',
            '    font-size: 0.92rem;',
            '    min-height: 1.4rem;',
            '}',
            '.citebox-desktop-close-actions {',
            '    display: flex;',
            '    justify-content: flex-end;',
            '    gap: 0.7rem;',
            '    flex-wrap: wrap;',
            '}',
            '.citebox-desktop-close-button {',
            '    appearance: none;',
            '    border: none;',
            '    border-radius: 999px;',
            '    padding: 0.78rem 1.12rem;',
            '    font: inherit;',
            '    font-weight: 700;',
            '    cursor: pointer;',
            '    transition: transform 140ms ease, box-shadow 140ms ease, background 140ms ease, color 140ms ease;',
            '}',
            '.citebox-desktop-close-button:disabled {',
            '    opacity: 0.6;',
            '    cursor: wait;',
            '    transform: none;',
            '    box-shadow: none;',
            '}',
            '.citebox-desktop-close-button:hover:not(:disabled) {',
            '    transform: translateY(-1px);',
            '}',
            '.citebox-desktop-close-button.subtle {',
            '    background: rgba(var(--ink-rgb, 31, 41, 55), 0.08);',
            '    color: var(--ink, #1f2937);',
            '}',
            '.citebox-desktop-close-button.danger {',
            '    background: rgba(var(--error-rgb, 201, 84, 77), 0.1);',
            '    color: var(--error, #c9544d);',
            '    box-shadow: inset 0 0 0 1px rgba(var(--error-rgb, 201, 84, 77), 0.14);',
            '}',
            '.citebox-desktop-close-button.primary {',
            '    background: linear-gradient(135deg, var(--accent, #c17f59), var(--accent-dark, #a66a48));',
            '    color: #fff;',
            '    box-shadow: 0 14px 28px rgba(var(--accent-rgb, 193, 127, 89), 0.26);',
            '}',
            '@media (max-width: 640px) {',
            '    .citebox-desktop-close-card {',
            '        width: min(100%%, 24rem);',
            '        padding: 1.15rem;',
            '        border-radius: 1.3rem;',
            '    }',
            '    .citebox-desktop-close-actions {',
            '        flex-direction: column-reverse;',
            '    }',
            '    .citebox-desktop-close-button {',
            '        width: 100%%;',
            '        justify-content: center;',
            '    }',
            '}'
        ].join('\n');

        document.head.appendChild(style);
    };

    const createClosePromptCopy = () => ({
        badge: desktopTranslate('shared.desktop_close.badge', '桌面端', 'Desktop'),
        title: desktopTranslate('shared.desktop_close.title', '关闭 CiteBox', 'Close CiteBox'),
        message: desktopTranslate('shared.desktop_close.message', '选择关闭窗口时的操作。', 'Choose what to do when closing the window.'),
        hint: desktopTranslate('shared.desktop_close.hint', '你可以将 CiteBox 最小化到托盘，稍后从托盘图标重新打开。', 'Keep CiteBox running in the tray so you can reopen it later.'),
        cancel: desktopTranslate('shared.desktop_close.cancel', '取消', 'Cancel'),
        exit: desktopTranslate('shared.desktop_close.exit', '直接退出', 'Exit'),
        minimize: desktopTranslate('shared.desktop_close.minimize', '最小化到托盘', 'Minimize to tray'),
        actionFailed: desktopTranslate('shared.desktop_close.action_failed', '操作未完成，请重试。', 'The requested action could not be completed.')
    });

    window.__citeboxDesktopOpenClosePrompt = () => {
        if (!document.body) {
            window.addEventListener('DOMContentLoaded', () => {
                window.__citeboxDesktopOpenClosePrompt();
            }, { once: true });
            return;
        }

        ensureClosePromptStyles();

        const existing = document.getElementById(closePromptOverlayId);
        if (existing) {
            const preferredButton = existing.querySelector('[data-close-action="minimize"]');
            if (preferredButton && typeof preferredButton.focus === 'function') {
                preferredButton.focus();
            }
            return;
        }

        const copy = createClosePromptCopy();
        const overlay = document.createElement('div');
        overlay.id = closePromptOverlayId;
        overlay.className = 'citebox-desktop-close-overlay';
        overlay.innerHTML = [
            '<div class="citebox-desktop-close-card" role="dialog" aria-modal="true" aria-labelledby="citeboxDesktopCloseTitle">',
            '<div class="citebox-desktop-close-badge">' + copy.badge + '</div>',
            '<div class="citebox-desktop-close-head">',
            '<h3 id="citeboxDesktopCloseTitle">' + copy.title + '</h3>',
            '<p>' + copy.message + '</p>',
            '</div>',
            '<p class="citebox-desktop-close-note">' + copy.hint + '</p>',
            '<p class="citebox-desktop-close-error" data-close-error hidden></p>',
            '<div class="citebox-desktop-close-actions">',
            '<button type="button" class="citebox-desktop-close-button subtle" data-close-action="cancel">' + copy.cancel + '</button>',
            '<button type="button" class="citebox-desktop-close-button danger" data-close-action="exit">' + copy.exit + '</button>',
            '<button type="button" class="citebox-desktop-close-button primary" data-close-action="minimize">' + copy.minimize + '</button>',
            '</div>',
            '</div>'
        ].join('');

        const errorMessage = overlay.querySelector('[data-close-error]');
        const actionButtons = Array.from(overlay.querySelectorAll('[data-close-action]'));

        const setBusy = (busy) => {
            actionButtons.forEach((button) => {
                button.disabled = busy;
            });
        };

        const showError = (message) => {
            if (!errorMessage) {
                return;
            }
            if (!message) {
                errorMessage.hidden = true;
                errorMessage.textContent = '';
                return;
            }
            errorMessage.hidden = false;
            errorMessage.textContent = message;
        };

        const closePrompt = () => {
            document.removeEventListener('keydown', onKeydown, true);
            overlay.classList.remove('active');
            window.setTimeout(() => {
                overlay.remove();
            }, 180);
        };

        const runAction = async (action) => {
            if (action === 'cancel') {
                closePrompt();
                return;
            }

            const actionName = action === 'minimize'
                ? 'citeboxDesktopMinimizeToTray'
                : 'citeboxDesktopExitApp';

            if (typeof window[actionName] !== 'function') {
                showError(copy.actionFailed);
                return;
            }

            setBusy(true);
            showError('');

            try {
                await window[actionName]();
                closePrompt();
            } catch (error) {
                setBusy(false);
                showError(error && error.message ? error.message : copy.actionFailed);
            }
        };

        const onKeydown = (event) => {
            if (event.key === 'Escape') {
                event.preventDefault();
                event.stopPropagation();
                closePrompt();
            }
        };

        overlay.addEventListener('click', (event) => {
            if (event.target === overlay) {
                closePrompt();
            }
        });

        actionButtons.forEach((button) => {
            button.addEventListener('click', () => {
                runAction(String(button.getAttribute('data-close-action') || 'cancel'));
            });
        });

        document.body.appendChild(overlay);
        document.addEventListener('keydown', onKeydown, true);
        window.requestAnimationFrame(() => {
            overlay.classList.add('active');
            const preferredButton = overlay.querySelector('[data-close-action="minimize"]');
            if (preferredButton && typeof preferredButton.focus === 'function') {
                preferredButton.focus();
            }
        });
    };

    const textInputTypes = new Set(['', 'text', 'search', 'url', 'tel', 'password', 'email']);
    const isTextInput = (element) => {
        if (!(element instanceof HTMLInputElement) || element.disabled) {
            return false;
        }
        return textInputTypes.has(String(element.type || '').toLowerCase());
    };

    const isTextControl = (element) => {
        if (element instanceof HTMLTextAreaElement) {
            return !element.disabled;
        }
        return isTextInput(element);
    };

    const resolveTextControl = (event) => {
        const candidates = [];
        const pushCandidate = (candidate) => {
            if (!(candidate instanceof Element)) {
                return;
            }
            if (!candidates.includes(candidate)) {
                candidates.push(candidate);
            }
        };

        pushCandidate(event.target);
        if (typeof event.composedPath === 'function') {
            event.composedPath().forEach(pushCandidate);
        }
        pushCandidate(document.activeElement);

        for (const candidate of candidates) {
            if (isTextControl(candidate)) {
                return candidate;
            }
            const closest = typeof candidate.closest === 'function'
                ? candidate.closest('textarea, input')
                : null;
            if (isTextControl(closest)) {
                return closest;
            }
        }
        return null;
    };

    const currentSelectionRange = (element) => {
        const fallback = String(element.value || '').length;
        const start = typeof element.selectionStart === 'number' ? element.selectionStart : fallback;
        const end = typeof element.selectionEnd === 'number' ? element.selectionEnd : start;
        return {
            start: Math.min(start, end),
            end: Math.max(start, end)
        };
    };

    const dispatchTextControlInput = (element) => {
        element.dispatchEvent(new Event('input', { bubbles: true }));
    };

    const selectAllText = (element) => {
        element.focus();
        if (typeof element.select === 'function') {
            element.select();
            return;
        }
        if (typeof element.setSelectionRange === 'function') {
            const length = String(element.value || '').length;
            element.setSelectionRange(0, length);
        }
    };

    const selectedText = (element) => {
        const range = currentSelectionRange(element);
        return String(element.value || '').slice(range.start, range.end);
    };

    const replaceSelection = (element, text) => {
        const range = currentSelectionRange(element);
        element.focus();
        if (typeof element.setRangeText === 'function') {
            element.setRangeText(String(text || ''), range.start, range.end, 'end');
        } else {
            const value = String(element.value || '');
            const next = value.slice(0, range.start) + String(text || '') + value.slice(range.end);
            element.value = next;
            const caret = range.start + String(text || '').length;
            if (typeof element.setSelectionRange === 'function') {
                element.setSelectionRange(caret, caret);
            }
        }
        dispatchTextControlInput(element);
    };

    document.addEventListener('keydown', (event) => {
        if (event.defaultPrevented || event.isComposing) {
            return;
        }
        if ((!event.metaKey && !event.ctrlKey) || event.altKey || event.shiftKey) {
            return;
        }

        const key = String(event.key || '').toLowerCase();
        if (!['a', 'c', 'x', 'v'].includes(key)) {
            return;
        }

        const control = resolveTextControl(event);
        if (!control) {
            return;
        }

        if (key === 'a') {
            event.preventDefault();
            event.stopPropagation();
            selectAllText(control);
            return;
        }

        if (key === 'c') {
            const text = selectedText(control);
            if (!text || typeof window.citeboxDesktopWriteClipboardText !== 'function') {
                return;
            }
            event.preventDefault();
            event.stopPropagation();
            void window.citeboxDesktopWriteClipboardText(text).catch(() => {});
            return;
        }

        if (key === 'x') {
            const text = selectedText(control);
            if (!text || control.readOnly || typeof window.citeboxDesktopWriteClipboardText !== 'function') {
                return;
            }
            event.preventDefault();
            event.stopPropagation();
            void window.citeboxDesktopWriteClipboardText(text)
                .then(() => {
                    replaceSelection(control, '');
                })
                .catch(() => {});
            return;
        }

        if (key === 'v') {
            if (control.readOnly || typeof window.citeboxDesktopReadClipboardText !== 'function') {
                return;
            }
            event.preventDefault();
            event.stopPropagation();
            void window.citeboxDesktopReadClipboardText()
                .then((text) => {
                    replaceSelection(control, String(text || ''));
                })
                .catch(() => {});
        }
    }, true);

    document.addEventListener('click', (event) => {
        if (event.defaultPrevented || event.button !== 0) {
            return;
        }
        if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
            return;
        }

        const anchor = event.target && typeof event.target.closest === 'function'
            ? event.target.closest('a[href]')
            : null;
        if (!anchor) {
            return;
        }

        const href = anchor.getAttribute('href') || '';
        if (!href || href.startsWith('#') || href.startsWith('javascript:')) {
            return;
        }

        if (isInternalRoute(anchor.href)) {
            event.preventDefault();
            navigateInPlace(anchor.href);
            return;
        }

        if (isSameOrigin(anchor.href)) {
            event.preventDefault();
            navigateInPlace(anchor.href);
            return;
        }

        if (String(anchor.target || '').toLowerCase() === '_blank') {
            event.preventDefault();
            openExternal(anchor.href);
        }
    }, true);

    const originalOpen = window.open;
    window.open = function(url, target, features) {
        if (!url) {
            return null;
        }

        if (isSameOrigin(String(url)) || isInternalRoute(String(url))) {
            navigateInPlace(String(url));
            return window;
        }

        const nextTarget = String(target || '').toLowerCase();
        if (!nextTarget || nextTarget === '_blank') {
            openExternal(String(url));
            return null;
        }

        return typeof originalOpen === 'function'
            ? originalOpen.call(window, url, target, features)
            : null;
    };
})();`

func initDesktopBridge(w webview.WebView) error {
	routesJSON, err := json.Marshal(desktopInternalRoutes)
	if err != nil {
		return fmt.Errorf("marshal desktop routes: %w", err)
	}

	w.Init(fmt.Sprintf(desktopBridgeScript, string(routesJSON)))
	return nil
}
