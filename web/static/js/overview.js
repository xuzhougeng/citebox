(function () {
    'use strict';

    function el(tag, props, children) {
        const node = document.createElement(tag);
        if (props) {
            for (const key in props) {
                if (key === 'className') node.className = props[key];
                else if (key === 'text') node.textContent = props[key];
                else if (key.startsWith('data-')) node.setAttribute(key, props[key]);
                else node[key] = props[key];
            }
        }
        if (Array.isArray(children)) {
            for (const child of children) {
                if (child == null) continue;
                if (typeof child === 'string') node.appendChild(document.createTextNode(child));
                else node.appendChild(child);
            }
        }
        return node;
    }

    function fail(panel, msg) {
        panel.replaceChildren(el('p', { className: 'overview-error', text: msg }));
    }

    async function loadRecentPapers() {
        const panel = document.querySelector('[data-bind="recent-papers"]');
        try {
            const r = await fetch('/api/overview/summary');
            if (!r.ok) throw new Error('HTTP ' + r.status);
            const body = await r.json();
            const papers = (body && body.recent_papers) || [];
            if (papers.length === 0) {
                panel.replaceChildren(el('p', { className: 'overview-empty', text: '尚未导入文献。' }));
                return;
            }
            const list = el('ul', { className: 'overview-list' },
                papers.map(p => el('li', null, [
                    el('span', { className: 'overview-paper-id', text: '#' + p.id }),
                    ' ',
                    el('a', { className: 'overview-paper-title', href: '/?paper=' + p.id, text: p.title || '(无标题)' }),
                ])));
            panel.replaceChildren(list);
        } catch (err) {
            fail(panel, '加载最近文献失败：' + err.message);
        }
    }

    async function loadDailyFigure() {
        const panel = document.querySelector('[data-bind="daily-figure"]');
        try {
            const r = await fetch('/api/overview/daily-figure');
            if (!r.ok) {
                if (r.status === 412) {
                    panel.replaceChildren(el('p', { className: 'overview-empty', text: '图片库为空，导入文献后再来。' }));
                    return;
                }
                throw new Error('HTTP ' + r.status);
            }
            const fig = await r.json();
            const img = el('img', {
                className: 'overview-figure-img',
                src: '/api/figures/' + fig.id + '/image',
                alt: fig.caption || fig.label || '',
            });
            const caption = (fig.caption || '').trim();
            const label = (fig.label || '').trim();
            const meta = [];
            if (label) meta.push(label);
            if (fig.paper) meta.push(fig.paper);
            if (fig.page) meta.push('p.' + fig.page);
            panel.replaceChildren(
                img,
                meta.length ? el('p', { className: 'overview-figure-meta', text: meta.join(' · ') }) : null,
                caption ? el('p', { className: 'overview-figure-caption', text: caption }) : null,
            );
        } catch (err) {
            fail(panel, '加载今日图片失败：' + err.message);
        }
    }

    async function loadStatus() {
        const panel = document.querySelector('[data-bind="status"]');
        try {
            const r = await fetch('/api/overview/status');
            if (!r.ok) throw new Error('HTTP ' + r.status);
            const status = await r.json();
            const rows = [];
            rows.push(['服务时间', status.server_time || '?']);
            const wb = status.weixin_bridge || {};
            rows.push(['微信桥接', wb.enabled ? '已启用' : '未启用']);
            if (wb.enabled) {
                rows.push(['每日推送', wb.daily_recommendation_on ? ('开启 · ' + (wb.daily_recommendation_at || '默认时间')) : '关闭']);
            }
            const dl = el('dl', { className: 'overview-status-list' });
            for (const [k, v] of rows) {
                dl.appendChild(el('dt', { text: k }));
                dl.appendChild(el('dd', { text: String(v) }));
            }
            panel.replaceChildren(dl);
        } catch (err) {
            fail(panel, '加载状态失败：' + err.message);
        }
    }

    document.addEventListener('DOMContentLoaded', () => {
        loadRecentPapers();
        loadDailyFigure();
        loadStatus();
    });
})();
