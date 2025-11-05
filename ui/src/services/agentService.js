// Agent service for managing workspace agents.
import { getLogger, ForgeLog } from 'forge/utils/logger';
const log = getLogger('agently');

/**
 * Saves or updates an agent definition using the underlying DataSource
 * connector assigned to the `agents` context. The function mirrors the shape
 * and behaviour of `modelService.saveModel` so it can be used in a generic
 * way from forge toolbar definitions.
 *
 * The PUT request is routed to `/v1/workspace/agent/{name}` where the
 * `{name}` comes from the currently edited form.
 *
 * @param {Object} options - Options object provided by forge
 * @param {Object} options.context - SettingProvider context instance
 * @returns {Promise<boolean|Object>} - Response payload or false on error
 */
export async function saveAgent({ context }) {
    const agentsCtx = context?.Context('agents');
    if (!agentsCtx) {
        log.error('agentService.saveAgent: agents context not found');
        return false;
    }

    const api = agentsCtx.connector;
    const handlers = agentsCtx?.handlers?.dataSource;

    const formData = handlers?.getFormData?.() || handlers?.getSelection?.()?.selected;
    if (!formData) {
        log.warn('agentService.saveAgent: no form data');
        return false;
    }

    const name = formData?.name;
    if (!name) {
        log.error('agentService.saveAgent: name field is required');
        return false;
    }

    log.debug('agentService.saveAgent', { name });

    handlers?.setLoading?.(true);
    try {
        // Strip any UI-only helper fields before saving (e.g., edit meta)
        const { editMeta, ...toPersist } = (formData || {});
        const resp = await api.put?.({
            inputParameters: { name },
            body: { ...toPersist },
        });
        log.debug('PUT', resp);
        // Surface non-blocking warnings when present
        try {
            const data = resp && resp.data ? resp.data : resp;
            const warnings = data && data.warnings ? data.warnings : [];
            if (Array.isArray(warnings) && warnings.length) {
                warnings.forEach(w => log.warn('[agent.save] warning:', w));
            }
        } catch (_) {}
        return resp;
    } catch (err) {
        log.error('agentService.saveAgent error', err);
        handlers?.setError?.(err);
        return false;
    } finally {
        handlers?.setLoading?.(false);
    }
}

function resolveAPIBase(context, agentsCtx) {
    try {
        const ep = (context?.endpoints && (context.endpoints.agentlyAPI?.baseURL || context.endpoints.appAPI?.baseURL || context.endpoints.dataAPI?.baseURL)) || '';
        if (ep && /^https?:\/\//i.test(ep)) return ep.replace(/\/+$/, '');
    } catch(_) {}
    try {
        const b = agentsCtx?.connector?.baseURL || '';
        if (b && /^https?:\/\//i.test(b)) return b.replace(/\/+$/, '');
    } catch(_) {}
    console.warn('[agent] No absolute API base; falling back to window.origin (set DATA_URL in ui/.env)');
    return (typeof window !== 'undefined' ? window.location.origin : '').replace(/\/+$/, '');
}

/**
 * Loads Agent Edit View meta for the currently selected agent and injects it
 * into the agents dataSource form under `editMeta` so UI controls can render
 * authoring hints without changing the stored YAML.
 */
export async function loadAgentEdit({ context }) {
    try {
        const agentsCtx = context?.Context('agents');
        if (!agentsCtx) return false;
        const sel = agentsCtx.handlers?.dataSource?.peekSelection?.();
        console.log('[agent.loadAgentEdit] selection', sel);
        // Prefer stable id for repo resolution; fallback to name
        const name = sel?.selected?.id || sel?.selected?.name;
        if (!name) return false;

        const base = (
            context?.endpoints?.agentlyAPI?.baseURL ||
            agentsCtx?.connector?.baseURL ||
            'http://localhost:8081/'
        ).replace(/\/+$/, '');
        const url = `${base}/v1/workspace/agent/${encodeURIComponent(name)}/edit`;

        console.log('[agent.loadAgentEdit] GET', url);
        const resp = await fetch(url, { method: 'GET' });
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        const payload = await resp.json();
        const data = payload && payload.data ? payload.data : payload; // support raw or wrapped
        if (!data) return false;

        const handlers = agentsCtx.handlers?.dataSource;
        const form = handlers?.getFormData?.() || sel?.selected || {};
        const next = { ...form, editMeta: data.meta || {} };
        // Seed editor sources when meta has preview content
        try {
            const userText = (data?.meta?.prompts?.user?.content || '').toString();
            const sysText = (data?.meta?.prompts?.system?.content || '').toString();
            const extOf = (u) => { try { const m = String(u || '').match(/\.([a-z0-9]+)$/i); return (m && m[1] || '').toLowerCase(); } catch(_) { return ''; } };
            const mapExt = (e) => {
                if (e === 'js' || e === 'jsx' || e === 'ts' || e === 'tsx') return 'js';
                if (e === 'yml' || e === 'yaml') return 'yaml';
                if (e === 'json') return 'json';
                if (e === 'html' || e === 'htm') return 'html';
                if (e === 'css') return 'css';
                if (e === 'sql') return 'sql';
                if (e === 'go') return 'go';
                if (e === 'py') return 'py';
                return 'yaml';
            };
            const userExt = mapExt(extOf(next?.prompt?.uri || data?.meta?.prompts?.user?.display));
            const sysExt = mapExt(extOf(next?.systemPrompt?.uri || data?.meta?.prompts?.system?.display));
            next.userPreviewSource = userText;
            next.userPreviewExtension = userExt;
            next.systemPreviewSource = sysText;
            next.systemPreviewExtension = sysExt;
            // Mirror resolved paths and existence for prompt labels
            next.userPromptResolved = (data?.meta?.prompts?.user?.resolved || '').toString();
            next.userPromptExists = !!data?.meta?.prompts?.user?.exists;
            next.systemPromptResolved = (data?.meta?.prompts?.system?.resolved || '').toString();
            next.systemPromptExists = !!data?.meta?.prompts?.system?.exists;
        } catch (_) {}
        try {
            console.log('[agent.loadAgentEdit] loaded meta.prompts', {
                user: data?.meta?.prompts?.user,
                system: data?.meta?.prompts?.system,
            });
        } catch(_) {}
        handlers?.setFormData?.({ values: next });
        // Auto-fetch prompt previews once meta is loaded to bypass any button wiring issues
        try {
            setTimeout(() => {
                try { previewPrompts({ context }); } catch (_) {}
            }, 0);
        } catch (_) {}
        return true;
    } catch (e) {
        try { getLogger('agently').error('loadAgentEdit failed', e); } catch (_) {}
        return false;
    }
}

export const agentService = {
    saveAgent,
    loadAgentEdit,
    previewPrompts,
    browseKnowledge,
    refreshKnowledgeBrowser,
    copyKnowledgePreview,
    openKnowledgeDialog,
};

// Preview prompts by downloading current Prompt/System Prompt URIs resolved
// against the agent baseDir (from editMeta.source.baseDir) and placing content
// into editMeta.prompts.{user,system}.content.
export async function previewPrompts({ context }) {
    try {
        console.log('[agent.previewPrompts] invoked');
        const agentsCtx = context?.Context('agents');
        if (!agentsCtx) return false;
        const handlers = agentsCtx.handlers?.dataSource;
        const form = handlers?.getFormData?.() || {};
        const meta = form?.editMeta || {};
        const baseDir = (meta?.source?.baseDir || '').toString();
        const promptURI = (form?.prompt?.uri || '').toString();
        const systemURI = (form?.systemPrompt?.uri || '').toString();
        console.log('[agent.previewPrompts] baseDir, promptURI, systemURI', baseDir, promptURI, systemURI);
        const buildAbs = (p) => {
            const raw = (p || '').trim();
            if (!raw) return '';
            if (/^[a-z]+:\/\//i.test(raw)) return raw; // has scheme
            if (!baseDir) return raw;
            const join = (a, b) => a.replace(/\/+$/, '') + '/' + b.replace(/^\/+/, '');
            return 'file://' + join(baseDir, raw);
        };
        const absUser = buildAbs(promptURI);
        const absSys = buildAbs(systemURI);
        const apiBase = (
            context?.endpoints?.agentlyAPI?.baseURL ||
            agentsCtx?.connector?.baseURL ||
            'http://localhost:8081/'
        ).replace(/\/+$/, '');
        console.log('[agent.previewPrompts] absUser, absSys, apiBase', absUser, absSys, apiBase);
        const dl = async (uri) => {
            if (!uri) return '';
            const url = `${apiBase}/v1/workspace/file-browser/download?uri=${encodeURIComponent(uri)}`;
            console.log('[agent.previewPrompts] GET', url);
            const resp = await fetch(url, { credentials: 'include' });
            const txt = await resp.text().catch(() => '');
            return txt || '';
        };
        const [userText, sysText] = await Promise.all([dl(absUser), dl(absSys)]);
        // Derive editor language from extension
        const extOf = (u) => {
            try { const m = String(u || '').match(/\.([a-z0-9]+)$/i); return (m && m[1] || '').toLowerCase(); } catch(_) { return ''; }
        };
        const mapExt = (e) => {
            if (e === 'js' || e === 'jsx') return 'js';
            if (e === 'ts' || e === 'tsx') return 'js';
            if (e === 'py') return 'py';
            if (e === 'yml' || e === 'yaml') return 'yaml';
            if (e === 'json') return 'json';
            if (e === 'html' || e === 'htm') return 'html';
            if (e === 'css') return 'css';
            if (e === 'sql') return 'sql';
            if (e === 'go') return 'go';
            // .tmpl or unknown → yaml/plaintext-ish
            return 'yaml';
        };
        const userExt = mapExt(extOf(promptURI));
        const sysExt = mapExt(extOf(systemURI));

        const next = {
            ...form,
            editMeta: {
                ...(form.editMeta || {}),
                prompts: {
                    user: { ...(form.editMeta?.prompts?.user || {}), content: userText },
                    system: { ...(form.editMeta?.prompts?.system || {}), content: sysText },
                },
            },
            userPreviewSource: userText,
            userPreviewExtension: userExt,
            systemPreviewSource: sysText,
            systemPreviewExtension: sysExt,
        };
        handlers?.setFormData?.({ values: next });
        return true;
    } catch (e) {
        try { getLogger('agently').error('previewPrompts failed', e); } catch (_) {}
        return false;
    }
}

export async function copyUserPrompt({ context }) {
    try {
        const agentsCtx = context?.Context('agents');
        const fd = agentsCtx?.handlers?.dataSource?.getFormData?.() || {};
        const text = fd.userPreviewSource || fd?.editMeta?.prompts?.user?.content || '';
        if (!text) return false;
        await navigator.clipboard.writeText(text);
        return true;
    } catch (_) { return false; }
}

export async function copySystemPrompt({ context }) {
    try {
        const agentsCtx = context?.Context('agents');
        const fd = agentsCtx?.handlers?.dataSource?.getFormData?.() || {};
        const text = fd.systemPreviewSource || fd?.editMeta?.prompts?.system?.content || '';
        if (!text) return false;
        await navigator.clipboard.writeText(text);
        return true;
    } catch (_) { return false; }
}

// Browse knowledge roots and list files under selected root.
export async function browseKnowledge({ context }) {
    try {
        const agentsCtx = context?.Context('agents');
        const agSel = agentsCtx?.handlers?.dataSource?.peekSelection?.();
        const agentName = agSel?.selected?.id || agSel?.selected?.name;
        if (!agentName) return false;

        // Detect which table triggered: user vs system
        const userCtx = context?.Context('agentKnowledge');
        const sysCtx = context?.Context('agentSystemKnowledge');
        const userSel = userCtx?.handlers?.dataSource?.getSelection?.();
        const sysSel = sysCtx?.handlers?.dataSource?.getSelection?.();
        let scope = 'user';
        let idx = 0;
        if (sysSel?.selected) {
            scope = 'system';
            idx = sysSel?.rowIndex ?? 0;
        } else if (userSel?.selected) {
            scope = 'user';
            idx = userSel?.rowIndex ?? 0;
        }

        const handlers = agentsCtx?.handlers?.dataSource;
        const form = handlers?.getFormData?.() || {};
        const editMeta = form?.editMeta || {};
        const roots = (editMeta?.knowledge?.roots || []).filter(r => r.scope === scope);
        const root = roots[idx] || null;
        const url = `/v1/workspace/agent/${encodeURIComponent(agentName)}/knowledge?scope=${encodeURIComponent(scope)}&idx=${encodeURIComponent(idx)}`;
        console.log('[agent.browseKnowledge] GET', url, 'root:', root);
        const resp = await fetch(url, { method: 'GET', credentials: 'include' });
        const payload = await resp.json().catch(() => ({}));
        const data = payload && payload.data ? payload.data : payload;
        const listText = JSON.stringify(data, null, 2);

        const next = {
            ...form,
            knowledgeBrowserScope: scope,
            knowledgeBrowserIndex: idx,
            knowledgeBrowserPath: '',
            knowledgeBrowserResolvedRoot: root?.resolved || '',
            knowledgeBrowserListSource: listText,
            knowledgeBrowserListExtension: 'json',
            knowledgePreviewSource: '',
            knowledgePreviewExtension: 'yaml',
        };
        handlers?.setFormData?.({ values: next });
        return true;
    } catch (e) {
        console.log('[agent.browseKnowledge] error', e);
        return false;
    }
}

export async function refreshKnowledgeBrowser({ context }) {
    try {
        const agentsCtx = context?.Context('agents');
        const handlers = agentsCtx?.handlers?.dataSource;
        const form = handlers?.getFormData?.() || {};
        const agentName = form?.name || form?.id || '';
        const scope = form?.knowledgeBrowserScope || 'user';
        const idx = form?.knowledgeBrowserIndex || 0;
        const subPath = (form?.knowledgeBrowserPath || '').toString().trim();
        const resolvedRoot = (form?.knowledgeBrowserResolvedRoot || '').toString();
        const base = '';

        const joinPath = (a, b) => a.replace(/\/+$/, '') + '/' + b.replace(/^\/+/, '');
        const buildFileURI = (root, rel) => {
            let p = root || '';
            if (rel) p = joinPath(p, rel);
            if (!/^([a-z]+):\/\//i.test(p)) return 'file://' + p;
            return p;
        };

        // If subPath looks like directory (empty or ends with '/') → list. Otherwise try download.
        if (subPath === '' || /\/$/.test(subPath)) {
            const params = new URLSearchParams({ scope, idx: String(idx) });
            if (subPath) params.set('path', subPath);
            const url = `/v1/workspace/agent/${encodeURIComponent(agentName)}/knowledge?${params.toString()}`;
            console.log('[agent.refreshKnowledgeBrowser] LIST GET', url);
            const resp = await fetch(url, { method: 'GET', credentials: 'include' });
            const payload = await resp.json().catch(() => ({}));
            const data = payload && payload.data ? payload.data : payload;
            const listText = JSON.stringify(data, null, 2);
            const next = { ...form, knowledgeBrowserListSource: listText, knowledgeBrowserListExtension: 'json', knowledgePreviewSource: '' };
            handlers?.setFormData?.({ values: next });
            return true;
        } else {
            const fileURI = buildFileURI(resolvedRoot, subPath);
            const url = `/v1/workspace/file-browser/download?uri=${encodeURIComponent(fileURI)}`;
            console.log('[agent.refreshKnowledgeBrowser] DOWNLOAD GET', url);
            const resp = await fetch(url, { method: 'GET', credentials: 'include' });
            const text = await resp.text();
            const ext = (subPath.match(/\.([a-z0-9]+)$/i)?.[1] || '').toLowerCase();
            const map = (e) => {
                if (e === 'yml' || e === 'yaml') return 'yaml';
                if (e === 'json') return 'json';
                if (e === 'md' || e === 'markdown') return 'markdown';
                if (e === 'sql') return 'sql';
                if (e === 'html' || e === 'htm') return 'html';
                if (e === 'css') return 'css';
                if (e === 'js' || e === 'jsx' || e === 'ts' || e === 'tsx') return 'js';
                if (e === 'go') return 'go';
                if (e === 'py') return 'py';
                return 'yaml';
            };
            const next = { ...form, knowledgePreviewSource: text, knowledgePreviewExtension: map(ext) };
            handlers?.setFormData?.({ values: next });
            return true;
        }
    } catch (e) {
        console.log('[agent.refreshKnowledgeBrowser] error', e);
        return false;
    }
}

export async function copyKnowledgePreview({ context }) {
    try {
        const agentsCtx = context?.Context('agents');
        const fd = agentsCtx?.handlers?.dataSource?.getFormData?.() || {};
        const text = fd.knowledgePreviewSource || '';
        if (!text) return false;
        await navigator.clipboard.writeText(text);
        return true;
    } catch (e) {
        console.log('[agent.copyKnowledgePreview] error', e);
        return false;
    }
}

// Open dialog with current knowledge file content using CodeMirror viewer.
export async function openKnowledgeDialog({ context }) {
    try {
        const agentsCtx = context?.Context('agents');
        if (!agentsCtx) return false;
        const handlers = agentsCtx?.handlers?.dataSource;
        const form = handlers?.getFormData?.() || {};
        const resolvedRoot = (form?.knowledgeBrowserResolvedRoot || '').toString();
        const subPath = (form?.knowledgeBrowserPath || '').toString().trim();
        if (!resolvedRoot || !subPath || /\/$/.test(subPath)) {
            console.warn('[agent.openKnowledgeDialog] select a file path first');
            return false;
        }
        const joinPath = (a, b) => a.replace(/\/+$/, '') + '/' + b.replace(/^\/+/, '');
        const buildFileURI = (root, rel) => {
            let p = root || '';
            if (rel) p = joinPath(p, rel);
            if (!/^([a-z]+):\/\//i.test(p)) return 'file://' + p;
            return p;
        };
        const apiBase = (
            context?.endpoints?.agentlyAPI?.baseURL ||
            agentsCtx?.connector?.baseURL ||
            'http://localhost:8081/'
        ).replace(/\/+$/, '');
        const fileURI = buildFileURI(resolvedRoot, subPath);
        const url = `${apiBase}/v1/workspace/file-browser/download?uri=${encodeURIComponent(fileURI)}`;
        const resp = await fetch(url, { method: 'GET', credentials: 'include' });
        const text = await resp.text();
        const ext = (subPath.match(/\.([a-z0-9]+)$/i)?.[1] || '').toLowerCase();
        const map = (e) => {
            if (e === 'yml' || e === 'yaml') return 'yaml';
            if (e === 'json') return 'json';
            if (e === 'md' || e === 'markdown') return 'markdown';
            if (e === 'sql') return 'sql';
            if (e === 'html' || e === 'htm') return 'html';
            if (e === 'css') return 'css';
            if (e === 'js' || e === 'jsx' || e === 'ts' || e === 'tsx') return 'js';
            if (e === 'go') return 'go';
            if (e === 'py') return 'py';
            return 'yaml';
        };
        const next = { ...form, knowledgeDialogSource: text, knowledgeDialogExtension: map(ext) };
        handlers?.setFormData?.({ values: next });
        try {
            const win = context?.handlers?.window || agentsCtx?.handlers?.window;
            await win?.openDialog?.({ execution: { args: ['knowledgeViewer'] }, context: agentsCtx });
        } catch (e) {
            console.warn('[agent.openKnowledgeDialog] open dialog failed', e);
        }
        return true;
    } catch (e) {
        console.log('[agent.openKnowledgeDialog] error', e);
        return false;
    }
}
