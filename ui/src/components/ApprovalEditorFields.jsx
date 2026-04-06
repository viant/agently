import React from 'react';

function toggleListValue(values = [], id = '') {
  const current = Array.isArray(values) ? values : [];
  if (current.includes(id)) {
    return current.filter((entry) => entry !== id);
  }
  return [...current, id];
}

export default function ApprovalEditorFields({ meta, value = {}, onChange, disabled = false }) {
  const editors = Array.isArray(meta?.editors) ? meta.editors : [];
  if (editors.length === 0) return null;

  return (
    <div className="app-approval-editors">
      {editors.map((editor) => {
        const kind = String(editor?.kind || '').toLowerCase();
        const options = Array.isArray(editor?.options) ? editor.options : [];
        const fieldValue = value?.[editor.name];
        return (
          <div key={editor.name} className="app-approval-editor">
            <div className="app-approval-editor-label">{editor.label || editor.name}</div>
            {editor.description ? <div className="app-approval-editor-description">{editor.description}</div> : null}
            <div className="app-approval-editor-options">
              {options.map((option) => {
                const checked = kind === 'radio_list'
                  ? String(fieldValue || '') === option.id
                  : Array.isArray(fieldValue) && fieldValue.includes(option.id);
                return (
                  <label key={option.id} className="app-approval-option">
                    <input
                      type={kind === 'radio_list' ? 'radio' : 'checkbox'}
                      name={editor.name}
                      value={option.id}
                      checked={checked}
                      disabled={disabled}
                      onChange={() => {
                        if (!onChange) return;
                        if (kind === 'radio_list') {
                          onChange({ ...value, [editor.name]: option.id });
                          return;
                        }
                        onChange({ ...value, [editor.name]: toggleListValue(fieldValue, option.id) });
                      }}
                    />
                    <span className="app-approval-option-body">
                      <span className="app-approval-option-label">{option.label}</span>
                      {option.description ? <span className="app-approval-option-description">{option.description}</span> : null}
                    </span>
                  </label>
                );
              })}
            </div>
          </div>
        );
      })}
    </div>
  );
}
