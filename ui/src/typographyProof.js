import './typography.js';
import './styles/typography-proof.css';

document.documentElement.classList.add('agently-application');
document.documentElement.dataset.agentlyTheme = 'typography-proof';
document.documentElement.style.setProperty('--agently-font-workspace-primary', 'Georgia, serif');
document.documentElement.style.setProperty('--agently-theme-font-family', 'var(--agently-font-workspace-primary, system-ui, sans-serif)');
document.documentElement.style.setProperty('--app-font-family', 'var(--agently-theme-font-family, system-ui, sans-serif)');
