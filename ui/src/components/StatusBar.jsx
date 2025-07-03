// src/components/StatusBar.jsx
import React from 'react';
import {useStage} from '../utils/stageBus';

const phaseMap = {
    waiting:  {icon: '⏳', label: 'Waiting for input…'},
    thinking: {icon: '🤔', label: 'Assistant thinking…'},
    executing:{icon: '⚙️', label: 'Executing…'},
    elicitation:{icon: '✍️', label: 'Awaiting your input…'},
    done:     {icon: '✅', label: 'Done'},
    error:    {icon: '❌', label: 'Error'},
    ready:    {icon: '🟢', label: 'Ready'},
};

export default function StatusBar() {
    const stage = useStage();

    if (!stage) {
        return null; // nothing to show to keep UI clean
    }

    const map = phaseMap[stage.phase] || {icon: '', label: ''};
    let text = map.label;
    if (stage.phase === 'executing') {
        if (stage.tool) {
            text = `Running ${stage.tool}…`;
        } else if (stage.task) {
            text = `Task: ${stage.task}…`;
        }
    }

    return (
        <div className="status-bar" style={{padding: '4px 8px', fontSize: '0.9em'}}>
            <span style={{marginRight: 6}}>{map.icon}</span>
            <span>{text}</span>
        </div>
    );
}
