// src/components/StatusBar.jsx
import React from 'react';
import {useStage} from '../utils/stageBus';

// Subtle pulsing glow to accentuate terminated/aborted state
const pulseStyles = `
@keyframes pulse-glow {
  0%   { text-shadow: 0 0 0 rgba(255, 0, 0, 0.0); }
  50%  { text-shadow: 0 0 8px rgba(255, 0, 0, 0.65); }
  100% { text-shadow: 0 0 0 rgba(255, 0, 0, 0.0); }
}
.glow-pulse { animation: pulse-glow 1.4s ease-in-out infinite; }
`;

const phaseMap = {
    waiting:  {icon: '⏳', label: 'Waiting for input…'},
    thinking: {icon: '🤔', label: 'Assistant thinking…'},
    executing:{icon: '⚙️', label: 'Executing…'},
    elicitation:{icon: '✍️', label: 'Awaiting your input…'},
    done:     {icon: '✅', label: 'Done'},
    error:    {icon: '❌', label: 'Error'},
    ready:    {icon: '🟢', label: 'Ready'},
    terminated:{icon: '🛑', label: 'Terminated'},
    aborted:  {icon: '🛑', label: 'Terminated'}, // backward-compat mapping
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

    const isTerminated = (stage.phase === 'terminated' || stage.phase === 'aborted');
    const extraStyle = isTerminated ? { color: 'var(--red3)' } : {};
    return (
        <div className="status-bar" style={{padding: '4px 8px', fontSize: '0.9em'}}>
            {/* Inject animation CSS locally */}
            {isTerminated && <style>{pulseStyles}</style>}
            <span className={isTerminated ? 'glow-pulse' : ''} style={{marginRight: 6, ...extraStyle}}>{map.icon}</span>
            <span className={isTerminated ? 'glow-pulse' : ''} style={extraStyle}>{text}</span>
        </div>
    );
}
