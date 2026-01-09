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
    const traceId = String(row?.traceId || '');
    const op = String(row?.operation || '');
    return `${traceId}:${op}`;
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
            trace: row?.trace || '',
            operation: row?.operation || '',
            count: Number(row?.count || 0),
            items: normalizeItems(row?.items),
        }));
    }, [data]);

    const [pageByRow, setPageByRow] = useState({});
    const pageSize = 10;

    const grouped = useMemo(() => {
        const byTrace = new Map();
        for (const row of ops) {
            const trace = String(row.trace || '').trim() || 'no-trace';
            const list = byTrace.get(trace) || [];
            list.push(row);
            byTrace.set(trace, list);
        }
        return Array.from(byTrace.entries());
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
            {grouped.map(([trace, rows]) => (
                <div key={trace} style={{ marginBottom: 12 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
                        <Tag minimal intent="primary">Trace {trace}</Tag>
                    </div>
                    <div style={{ border: '1px solid var(--light-gray2)', borderRadius: 6, overflow: 'hidden' }}>
                        {rows.map((row) => {
                            const meta = operationMeta(row.operation);
                            const key = rowKey(row);
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
