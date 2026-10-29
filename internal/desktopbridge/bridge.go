// Package desktopbridge holds the JavaScript the desktop shells inject into
// every page. It has no cgo or webview dependency so any shell can reuse it.
package desktopbridge

import (
	"encoding/json"
	"fmt"
)

var internalRoutes = []string{
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

const scriptTemplate = `(function() {
    window.__CITEBOX_DESKTOP__ = true;

    // This script runs at the start of <head>, before the page's own scripts
    // and styles, so the shell styles below apply from the first paint.
    const rootElement = document.documentElement;
    rootElement.classList.add('citebox-desktop');
    const shellStyleRules = [
        // The desktop webview is always signed in (the shell's in-process
        // handler is trusted), so sign-in controls have nothing to do.
        '.citebox-desktop .db-action-item:has(#logoutButton),',
        '.citebox-desktop .db-action-item:has(#rememberLoginEnabledInput) {',
        '    display: none !important;',
        '}'
    ];
    if (window.__CITEBOX_DESKTOP_INSET_TITLEBAR__) {
        // macOS hidden-inset title bar: the page header sits under the traffic
        // lights, so it needs room on the left and becomes the drag area. The
        // Wails runtime reads --wails-draggable from the clicked element.
        rootElement.classList.add('desktop-mac');
        const headers = ['.navbar', '.viewer-toolbar', '.overview-topbar'];
        const inHeaders = (selector) => headers.map((header) => 'html.desktop-mac ' + header + ' ' + selector).join(',\n');
        shellStyleRules.push(
            headers.map((header) => 'html.desktop-mac ' + header).join(',\n') + ' {',
            '    --wails-draggable: drag;',
            '    -webkit-user-select: none;',
            '    user-select: none;',
            '}',
            'html.desktop-mac .navbar {',
            '    padding-left: 80px;',
            '}',
            'html.desktop-mac .viewer-toolbar,',
            'html.desktop-mac .overview-topbar {',
            '    padding-left: 88px;',
            '}',
            inHeaders(':is(a, button, input, select, textarea, label, summary, [role="button"], [role="menu"], [role="menuitem"], [tabindex], [contenteditable=""], [contenteditable="true"], .nav-dropdown-menu)') + ' {',
            '    --wails-draggable: no-drag;',
            '}',
            inHeaders(':is(input, textarea, [contenteditable=""], [contenteditable="true"])') + ' {',
            '    -webkit-user-select: text;',
            '    user-select: text;',
            '}'
        );
    }
    const shellStyle = document.createElement('style');
    shellStyle.id = 'citeboxDesktopShellStyle';
    shellStyle.textContent = shellStyleRules.join('\n');
    (document.head || rootElement).appendChild(shellStyle);

    // Report theme and UI language changes so the shell can match the native
    // window appearance and menu language. theme.js sets data-theme and
    // i18n.js sets lang on <html> (on every page load, after the locale is
    // resolved), so watching those attributes is enough.
    // The static lang attribute in the HTML is only a placeholder, so the
    // language is reported once i18n.js has set it on this page.
    let reportedAppearance = '';
    let appearanceTimer = 0;
    let langResolved = false;
    const reportAppearance = () => {
        appearanceTimer = 0;
        if (typeof window.citeboxDesktopSyncAppearance !== 'function') {
            return;
        }
        const theme = rootElement.getAttribute('data-theme') || 'warm';
        const lang = langResolved ? String(rootElement.lang || '') : '';
        const key = theme + '|' + lang;
        if (key === reportedAppearance) {
            return;
        }
        reportedAppearance = key;
        void Promise.resolve(window.citeboxDesktopSyncAppearance(theme, lang)).catch(() => {});
    };
    if (typeof MutationObserver === 'function') {
        new MutationObserver((records) => {
            if (records.some((record) => record.attributeName === 'lang')) {
                langResolved = true;
            }
            if (!appearanceTimer) {
                appearanceTimer = window.setTimeout(reportAppearance, 50);
            }
        }).observe(rootElement, { attributes: true, attributeFilter: ['data-theme', 'lang'] });
    }

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
	const normalizeCloseAction = (value) => {
		const normalized = String(value || '').trim().toLowerCase();
		return normalized === 'minimize' || normalized === 'exit' ? normalized : 'ask';
	};
	const readStoredCloseAction = async () => {
		if (typeof window.citeboxDesktopGetClosePreference !== 'function') {
			return 'ask';
		}
		try {
			return normalizeCloseAction(await window.citeboxDesktopGetClosePreference());
		} catch (error) {
			return 'ask';
		}
	};
	const writeStoredCloseAction = async (value) => {
		const normalized = normalizeCloseAction(value);
		if (typeof window.citeboxDesktopSetClosePreference !== 'function') {
			return normalized;
		}
		try {
			return normalizeCloseAction(await window.citeboxDesktopSetClosePreference(normalized));
		} catch (error) {
			return 'ask';
		}
	};
	const closeActionFunctionName = (action) => {
		if (action === 'minimize') {
			return 'citeboxDesktopMinimizeToTray';
		}
		if (action === 'exit') {
			return 'citeboxDesktopExitApp';
		}
		return '';
	};

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
            '    background: rgba(var(--shadow-deep-rgb, 20, 14, 11), 0.32);',
            '    backdrop-filter: blur(6px);',
            '    -webkit-backdrop-filter: blur(6px);',
            '    opacity: 0;',
            '    transition: opacity 160ms ease;',
            '}',
            '.citebox-desktop-close-overlay.active {',
            '    opacity: 1;',
            '}',
            '.citebox-desktop-close-card {',
            '    width: min(27rem, 100%%);',
            '    display: grid;',
            '    gap: 1.25rem;',
            '    padding: 1.5rem;',
            '    border-radius: var(--radius-panel, 20px);',
            '    color: var(--ink, #241b16);',
            '    background: var(--panel-strong, #fffdf9);',
            '    border: 1px solid var(--line, rgba(36, 27, 22, 0.12));',
            '    box-shadow: var(--shadow-floating, 0 16px 44px -12px rgba(20, 14, 11, 0.24));',
            '    transform: translateY(0.5rem);',
            '    transition: transform 160ms ease;',
            '}',
            '.citebox-desktop-close-overlay.active .citebox-desktop-close-card {',
            '    transform: none;',
            '}',
            '.citebox-desktop-close-head {',
            '    display: grid;',
            '    gap: 0.5rem;',
            '}',
            '.citebox-desktop-close-head h3 {',
            '    margin: 0;',
            '    font-size: var(--text-lg, 1.25rem);',
            '    font-weight: 700;',
            '    line-height: 1.3;',
            '}',
            '.citebox-desktop-close-head p,',
            '.citebox-desktop-close-error {',
            '    margin: 0;',
            '    font-size: var(--text-ui, 0.95rem);',
            '    line-height: 1.6;',
            '}',
            '.citebox-desktop-close-head p {',
            '    color: var(--muted, #7a6a5f);',
            '}',
            '.citebox-desktop-close-preference {',
            '    display: grid;',
            '    grid-template-columns: auto 1fr;',
            '    align-items: start;',
            '    gap: 0.6rem;',
            '    cursor: pointer;',
            '}',
            '.citebox-desktop-close-checkbox {',
            '    width: 1rem;',
            '    height: 1rem;',
            '    margin: 0.2rem 0 0;',
            '    accent-color: var(--accent-deep, #a66a48);',
            '}',
            '.citebox-desktop-close-preference-text {',
            '    display: grid;',
            '    gap: 0.1rem;',
            '}',
            '.citebox-desktop-close-preference-text strong {',
            '    font-size: var(--text-ui, 0.95rem);',
            '    font-weight: 600;',
            '    line-height: 1.5;',
            '}',
            '.citebox-desktop-close-preference-text span {',
            '    color: var(--muted, #7a6a5f);',
            '    font-size: var(--text-sm, 0.875rem);',
            '    line-height: 1.5;',
            '}',
            '.citebox-desktop-close-error {',
            '    color: var(--error, #c9544d);',
            '}',
            '.citebox-desktop-close-error[hidden] {',
            '    display: none;',
            '}',
            '.citebox-desktop-close-actions {',
            '    display: flex;',
            '    align-items: center;',
            '    gap: 0.6rem;',
            '}',
            '.citebox-desktop-close-button {',
            '    appearance: none;',
            '    display: inline-flex;',
            '    align-items: center;',
            '    justify-content: center;',
            '    min-height: var(--control-height-sm, 2.25rem);',
            '    padding: 0.45rem 1rem;',
            '    border: 1px solid transparent;',
            '    border-radius: var(--radius-control, 10px);',
            '    font: inherit;',
            '    font-size: var(--text-ui, 0.95rem);',
            '    font-weight: 600;',
            '    line-height: 1.5;',
            '    white-space: nowrap;',
            '    cursor: pointer;',
            '    transition: background-color 140ms ease, border-color 140ms ease;',
            '}',
            '.citebox-desktop-close-button:disabled {',
            '    opacity: 0.45;',
            '    cursor: wait;',
            '}',
            '.citebox-desktop-close-button:focus {',
            '    outline: none;',
            '}',
            '.citebox-desktop-close-button:focus-visible {',
            '    outline: 2px solid var(--accent-deep, #a66a48);',
            '    outline-offset: 2px;',
            '}',
            '.citebox-desktop-close-button.danger {',
            '    margin-right: auto;',
            '    background: none;',
            '    color: var(--error, #c9544d);',
            '}',
            '.citebox-desktop-close-button.danger:hover:not(:disabled) {',
            '    background: rgba(var(--error-rgb, 201, 84, 77), 0.08);',
            '}',
            '.citebox-desktop-close-button.subtle {',
            '    background: var(--panel-strong, #fffdf9);',
            '    border-color: var(--line, rgba(36, 27, 22, 0.12));',
            '    color: var(--ink, #241b16);',
            '}',
            '.citebox-desktop-close-button.subtle:hover:not(:disabled) {',
            '    border-color: rgba(var(--accent-rgb, 193, 127, 89), 0.4);',
            '}',
            '.citebox-desktop-close-button.primary {',
            '    background: var(--accent-deep, #a66a48);',
            '    color: var(--on-accent, #fff8f2);',
            '}',
            '.citebox-desktop-close-button.primary:hover:not(:disabled) {',
            '    background: var(--accent, #c17f59);',
            '}',
            '@media (max-width: 560px) {',
            '    .citebox-desktop-close-card {',
            '        padding: 1.25rem;',
            '    }',
            '    .citebox-desktop-close-actions {',
            '        flex-direction: column-reverse;',
            '        align-items: stretch;',
            '    }',
            '    .citebox-desktop-close-button.danger {',
            '        margin-right: 0;',
            '    }',
            '}'
        ].join('\n');

        document.head.appendChild(style);
    };

    const createClosePromptCopy = () => ({
		title: desktopTranslate('shared.desktop_close.title', '关闭 CiteBox', 'Close CiteBox'),
		message: desktopTranslate('shared.desktop_close.message', '最小化后 CiteBox 会继续在托盘中运行，可随时从托盘图标重新打开。', 'Minimizing keeps CiteBox running in the tray, so you can reopen it anytime.'),
		remember: desktopTranslate('shared.desktop_close.remember', '不再询问', "Don't ask again"),
		rememberHint: desktopTranslate('shared.desktop_close.remember_hint', '可随时在“设置 → 关闭窗口行为”中修改。', 'You can change this anytime in Settings → Close Behavior.'),
		cancel: desktopTranslate('shared.desktop_close.cancel', '取消', 'Cancel'),
		exit: desktopTranslate('shared.desktop_close.exit', '直接退出', 'Exit'),
		minimize: desktopTranslate('shared.desktop_close.minimize', '最小化到托盘', 'Minimize to tray'),
		actionFailed: desktopTranslate('shared.desktop_close.action_failed', '操作未完成，请重试。', 'The requested action could not be completed.')
	});

    window.__citeboxDesktopOpenClosePrompt = async () => {
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
		const rememberedAction = await readStoredCloseAction();
		if (rememberedAction !== 'ask') {
			const actionName = closeActionFunctionName(rememberedAction);
			if (actionName && typeof window[actionName] === 'function') {
				void Promise.resolve(window[actionName]()).catch(async () => {
					await writeStoredCloseAction('ask');
					window.__citeboxDesktopOpenClosePrompt();
				});
				return;
			}
			await writeStoredCloseAction('ask');
		}

		const overlay = document.createElement('div');
		overlay.id = closePromptOverlayId;
		overlay.className = 'citebox-desktop-close-overlay';
		overlay.innerHTML = [
			'<div class="citebox-desktop-close-card" role="dialog" aria-modal="true" aria-labelledby="citeboxDesktopCloseTitle" aria-describedby="citeboxDesktopCloseMessage">',
            '<div class="citebox-desktop-close-head">',
			'<h3 id="citeboxDesktopCloseTitle">' + copy.title + '</h3>',
			'<p id="citeboxDesktopCloseMessage">' + copy.message + '</p>',
			'</div>',
			'<label class="citebox-desktop-close-preference">',
			'<input class="citebox-desktop-close-checkbox" type="checkbox" data-close-remember>',
			'<span class="citebox-desktop-close-preference-text"><strong>' + copy.remember + '</strong><span>' + copy.rememberHint + '</span></span>',
			'</label>',
			'<p class="citebox-desktop-close-error" data-close-error hidden></p>',
			'<div class="citebox-desktop-close-actions">',
			'<button type="button" class="citebox-desktop-close-button danger" data-close-action="exit">' + copy.exit + '</button>',
			'<button type="button" class="citebox-desktop-close-button subtle" data-close-action="cancel">' + copy.cancel + '</button>',
			'<button type="button" class="citebox-desktop-close-button primary" data-close-action="minimize">' + copy.minimize + '</button>',
            '</div>',
            '</div>'
		].join('');

		const errorMessage = overlay.querySelector('[data-close-error]');
		const rememberToggle = overlay.querySelector('[data-close-remember]');
		const actionButtons = Array.from(overlay.querySelectorAll('[data-close-action]'));

		const setBusy = (busy) => {
			actionButtons.forEach((button) => {
				button.disabled = busy;
			});
			if (rememberToggle) {
				rememberToggle.disabled = busy;
			}
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
			const shouldRemember = Boolean(rememberToggle?.checked);

			if (typeof window[actionName] !== 'function') {
				showError(copy.actionFailed);
				return;
			}

			setBusy(true);
			showError('');
			if (shouldRemember) {
				await writeStoredCloseAction(action);
			}

			try {
				await window[actionName]();
				closePrompt();
			} catch (error) {
				if (shouldRemember) {
					await writeStoredCloseAction('ask');
				}
				setBusy(false);
				showError(error && error.message ? error.message : copy.actionFailed);
			}
		};

        const onKeydown = (event) => {
            if (event.key === 'Escape') {
                event.preventDefault();
                event.stopPropagation();
                closePrompt();
                return;
            }
            if (event.key === 'Tab') {
                const focusable = [rememberToggle].concat(actionButtons).filter((element) => element && !element.disabled);
                if (!focusable.length) {
                    return;
                }
                const index = focusable.indexOf(document.activeElement);
                const nextIndex = event.shiftKey
                    ? (index <= 0 ? focusable.length - 1 : index - 1)
                    : (index === -1 || index === focusable.length - 1 ? 0 : index + 1);
                event.preventDefault();
                focusable[nextIndex].focus();
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

// Script returns the bridge script with the internal route list filled in.
func Script() (string, error) {
	routesJSON, err := json.Marshal(internalRoutes)
	if err != nil {
		return "", fmt.Errorf("marshal desktop routes: %w", err)
	}
	return fmt.Sprintf(scriptTemplate, string(routesJSON)), nil
}
