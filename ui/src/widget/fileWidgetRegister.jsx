// File widget registration – maps schema format `uri-reference` or explicit
// `x-ui-widget: file` to a custom widget that combines a text input with a
// native file-picker browse button.

import { registerWidget } from 'forge';
import { registerEventAdapter } from 'forge/runtime/binding';

// ------------------------------------------------------------------
// Widget implementation
// ------------------------------------------------------------------

 function FileInputWidget({ context, value = '', onChange, readOnly, placeholder, ...rest }) {
    const handleBrowse = async () => {
        if (readOnly) return;

        // Prime the file-browser data source so the dialog opens at
        // the location that is currently typed into the input field.
        // We assume a dataSource with ref "fs" exists and is used by
        // the fileBrowser view.
        try {
            const fsContext = context.Context('fs');
            fsContext.handlers.dataSource.setFilter({ filter: { uri: value } });
            fsContext.handlers.dataSource.getCollection();
        } catch (e) {
            /* eslint-disable-next-line no-console */
            log.warn('file widget – unable to prime fs dataSource', e);
        }

        const { window } = context.handlers;

        // ------------------------------------------------------------------
        // Open the dialog and wait for the user to pick a file.
        // ------------------------------------------------------------------

        const execArgs = [
            'fileBrowser',            // dialog id
            'Browse files…',          // optional title
            { awaitResult: true },    // options object recognised by runtime
        ];
        try {
           const result = await window.openDialog({ execution: { args: execArgs } });

            // Prefer explicit dialog result when provided
            const deriveValue = (item) => {
                if (!item) return '';
                if (typeof item === 'string') return item;
                // Prefer URI/URL style fields returned by file-browser
                return (
                    item.uri ||
                    item.url ||
                    item.path ||
                    item.id ||
                    item.name ||
                    ''
                );
            };

            let picked = deriveValue(result);

            // Fallback to fs DS selection if dialog didn't return a value
            if (!picked) {
                const fsContext = context.Context('fs');
                const selection = fsContext?.handlers?.dataSource?.getSelection?.();
                const selected = selection?.selected ?? selection;
                picked = deriveValue(selected);
            }

            if (picked) onChange?.(picked);
        } catch (e) {
            /* eslint-disable-next-line no-console */
            log.error('file widget – dialog failed', e);
        }
    };

    return (
        <div style={{ display: 'flex', gap: 4 }}>
            <input
                {...rest}
                type="text"
                className="bp4-input"
                style={{ flex: 1 }}
                readOnly={readOnly}
                value={value ?? ''}
                placeholder={placeholder || 'Select file…'}
                onChange={(e) => onChange?.(e.target.value)}
            />
            <button
                type="button"
                className="bp4-button"
                onClick={handleBrowse}
                disabled={readOnly}
            >Browse…
            </button>
        </div>
    );
}


// ------------------------------------------------------------------
// Runtime registration
// ------------------------------------------------------------------

// 1️⃣ Register the widget under key "file".
registerWidget('file', FileInputWidget, { framework: 'blueprint' });


// 4️⃣ Event adapter so WidgetRenderer wires onChange → state.set.
registerEventAdapter('file', {
    onChange: ({ adapter }) => (val) => {
        const v = val?.target?.value ?? val;
        adapter.set(v);
    },
});
import { getLogger, ForgeLog } from 'forge/utils/logger';
const log = getLogger('agently');
