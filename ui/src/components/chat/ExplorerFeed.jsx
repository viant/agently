import React, { useMemo, useState } from 'react';
import { Button, Icon, Tag } from '@blueprintjs/core';
import { explorerRead } from '../../services/chatService.js';

function operationMeta(operation) {
    const op = String(operation || '').toLowerCase().trim();
    switch (op) {
        case 'list':
            return { icon: 'folder-open', label: 'List' };
        case 'read':
            return { icon: 'document-open', label: 'Read' };
        case 'search':
            return { icon: 'search', label: 'Search' };
        default:
            return { icon: 'dot', label: op || 'Op' };
    }
}

function rowKey(row) {
    const traceId = String(row?.traceGroupId || row?.traceId || '');
    const op = String(row?.operation || '');
    return `${traceId}:${op}`;
}

function traceLabelForRow(row) {
    const trace = String(row?.trace || '').trim();
    const traceId = String(row?.traceId || '').trim();
    const traceShortId = String(row?.traceShortId || '').trim();

    const looksLikeRawTraceId = (v) => {
        const s = String(v || '').trim();
        if (!s) return false;
        if (s === traceId || s === traceShortId) return true;
        if (/^resp_[0-9a-f]{10,}$/i.test(s)) return true;
        return false;
    };

    if (trace && !looksLikeRawTraceId(trace)) return trace;
    // No human-readable trace label for this trace id.
    if (traceId || traceShortId) return '…';
    return 'no-trace';
}

function normalizeItems(items) {
    const arr = Array.isArray(items) ? items : [];
    const out = [];
    const seen = new Set();
    for (const it of arr) {
        const name = String(it?.name || '').trim();
        const uri = String(it?.uri || '').trim();
        const hits = (it && typeof it === 'object') ? it.hits : null;
        const key = `${uri}|${name}`;
        if (!name || seen.has(key)) continue;
        seen.add(key);
        out.push({ name, uri, hits });
    }
    out.sort((a, b) => a.name.localeCompare(b.name) || a.uri.localeCompare(b.uri));
    return out;
}

export default function ExplorerFeed({ data, context }) {
    const ops = useMemo(() => {
        const raw = Array.isArray(data?.ops) ? data.ops : [];
        return raw.map((row) => ({
            traceId: row?.traceId || '',
            traceGroupId: row?.traceGroupId || '',
            traceShortId: row?.traceShortId || '',
            trace: traceLabelForRow(row),
            operation: row?.operation || '',
            count: Number(row?.count || 0),
            resources: String(row?.resources || ''),
            items: normalizeItems(row?.items),
        }));
    }, [data]);

    const [pageByRow, setPageByRow] = useState({});
    const pageSize = 10;

    const grouped = useMemo(() => {
        // Keep chronological order as emitted by backend `ops` array while grouping by full traceId.
        const byTrace = new Map(); // traceGroupId -> {label, rows}
        for (const row of ops) {
            const traceKey = String(row.traceGroupId || row.traceId || '').trim() || 'no-trace';
            const existing = byTrace.get(traceKey);
            if (!existing) {
                byTrace.set(traceKey, { label: row.trace || traceLabelForRow(row), rows: [row] });
            } else {
                existing.rows.push(row);
            }
        }
        return Array.from(byTrace.entries()).map(([traceId, v]) => ({ traceId, label: v.label, rows: v.rows }));
    }, [ops]);

    const openURI = async (uri) => {
        const u = String(uri || '').trim();
        if (!u) return;
        await explorerRead({ context, uri: u });
    };

    if (!ops.length) {
        return <div style={{ padding: 8, color: 'var(--gray2)' }}>No resources.</div>;
    }

    return (
        <div style={{ padding: 8 }}>
            {grouped.map((g) => (
                <div key={g.traceId} style={{ marginBottom: 12 }}>
                    <div style={{
                        marginBottom: 6,
                        padding: '6px 8px',
                        borderRadius: 6,
                        border: '1px solid var(--light-gray2)',
                        background: 'var(--light-gray5)',
                        color: 'var(--blue2)',
                        display: 'block',
                        width: '100%',
                        maxWidth: 'none',
                        overflow: 'visible',
                        textOverflow: 'unset',
                        whiteSpace: 'pre-wrap',
                        wordBreak: 'break-word',
                        overflowWrap: 'anywhere'
                    }}
                    title={g.label || ''}
                    >
                        <pre style={{
                            margin: 0,
                            padding: 0,
                            fontFamily: 'inherit',
                            fontSize: 'inherit',
                            color: 'inherit',
                            whiteSpace: 'pre-wrap',
                            wordBreak: 'break-word',
                            overflowWrap: 'anywhere',
                            overflow: 'visible',
                            textOverflow: 'unset',
                            maxWidth: 'none',
                        }}>{g.label || 'no-trace'}</pre>
                    </div>
                    <div style={{ border: '1px solid var(--light-gray2)', borderRadius: 6, overflow: 'hidden' }}>
                        {g.rows.map((row, idx) => {
                            const meta = operationMeta(row.operation);
                            const key = rowKey(row) || `${g.traceId}:${idx}`;
                            const total = row.items.length;
                            const totalPages = Math.max(1, Math.ceil(total / pageSize));
                            const page = Math.min(Math.max(0, Number(pageByRow[key] || 0)), totalPages - 1);
                            const start = page * pageSize;
                            const end = Math.min(total, start + pageSize);
                            const slice = row.items.slice(start, end);

                            const setPage = (next) => setPageByRow((prev) => ({ ...prev, [key]: next }));

                            return (
                                <div key={key} style={{ padding: 10, borderTop: '1px solid var(--light-gray2)' }}>
                                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10 }}>
                                        <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 160 }}>
                                            <Icon icon={meta.icon} />
                                            <div style={{ fontWeight: 600 }}>{meta.label}</div>
                                            <Tag minimal>{row.count || total}</Tag>
                                        </div>
                                        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                                            <Button small minimal icon="chevron-left" disabled={page <= 0} onClick={() => setPage(page - 1)} />
                                            <span style={{ fontSize: 12, color: 'var(--gray2)' }}>{page + 1}/{totalPages}</span>
                                            <Button small minimal icon="chevron-right" disabled={page >= totalPages - 1} onClick={() => setPage(page + 1)} />
                                        </div>
                                    </div>

                                    <div style={{ marginTop: 8, display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                                        {slice.map((it) => (
                                            <a
                                                key={`${it.uri}|${it.name}`}
                                                href="#"
                                                onClick={(e) => { e.preventDefault(); if (it.uri) openURI(it.uri); }}
                                                style={{
                                                    display: 'inline-flex',
                                                    alignItems: 'center',
                                                    gap: 6,
                                                    padding: '4px 8px',
                                                    borderRadius: 999,
                                                    border: '1px solid var(--light-gray2)',
                                                    background: 'var(--light-gray5)',
                                                    color: 'var(--blue2)',
                                                    textDecoration: 'none',
                                                    fontSize: 12
                                                }}
                                                title={it.uri || it.name}
                                            >
                                                <Icon icon="document" size={12} />
                                                <span>{it.name}</span>
                                                {Number.isFinite(Number(it.hits)) && Number(it.hits) > 0 && (
                                                    <Tag minimal round>{Number(it.hits)}</Tag>
                                                )}
                                            </a>
                                        ))}
                                        {total === 0 && (
                                            <span style={{ fontSize: 12, color: 'var(--gray2)' }}>No items.</span>
                                        )}
                                    </div>
                                </div>
                            );
                        })}
                    </div>
                </div>
            ))}
        </div>
    );
}
