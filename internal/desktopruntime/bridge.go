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
