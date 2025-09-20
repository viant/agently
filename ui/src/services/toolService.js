// Tool service – maintains the nested `tool` array inside an agent record.
// It operates purely on client-side signals; the data are persisted when the
// user eventually hits “Save Agent”.

/* eslint no-console: ["error", { allow: ["warn", "error", "log"] }] */

function ensureArray(holder, key) {
    if (!Array.isArray(holder[key])) {
        // eslint-disable-next-line no-param-reassign
        holder[key] = [];
    }
    return holder[key];
}

// -------------------------------------------------------------------------
// ADD TOOL
// -------------------------------------------------------------------------
export function addTool({ context }) {
    const toolsCtx  = context;             // DS bound to agentTools
    const agentsCtx = context?.Context('agents');

    if (!toolsCtx || !agentsCtx) {
        log.error('toolService.addTool – missing dataSource context');
        return false;
    }
    const agentsHandlers = agentsCtx.handlers.dataSource;
    const agentSel = agentsHandlers.peekSelection();
    const rowIndex = agentSel.rowIndex;
    const agent      = agentSel.selected;
    if (rowIndex === -1 || !agentSel.selected) {
        log.warn('toolService.addTool – no agent selected');
        return false;
    }
    const agentCollection = agentsHandlers.peekCollection();

    const toolsHandlers = toolsCtx.handlers.dataSource;
    const formData      = toolsHandlers.getFormData();

    const pattern   = formData?.pattern?.trim() || '';
    const name      = formData?.definition?.name?.trim() || '';
    if (!pattern && !name) {
        log.warn('toolService.addTool – pattern or name required');
        return false;
    }

    const toolsArray = ensureArray(agent, 'tool');
    const idx = toolsArray.findIndex(
        (t) => (t.pattern === pattern && pattern !== "")|| ((t.definition?.name || '') === name && name !=="")
    );

    let newTools;
    if (idx !== -1) {
        newTools = toolsArray.map((t, i) => (i === idx ? { ...t, ...formData } : t));
    } else {
        newTools = [...toolsArray, { ...formData }];
    }

    // Assign new array reference
    agent.tool = newTools;
    agentCollection[rowIndex] = {...agent};
    agentsHandlers.setCollection([...agentCollection]);
    agentsCtx.handlers.dataSource.setSelection({args:{rowIndex: rowIndex}})

    return true;
}


// -------------------------------------------------------------------------
// DELETE TOOL (removes current selection)
// -------------------------------------------------------------------------
export function deleteTool({ context }) {
    const toolsCtx  = context;
    const agentsCtx = context?.Context('agents');

    if (!toolsCtx || !agentsCtx) {
        log.error('toolService.deleteTool – missing dataSource context');
        return false;
    }

    const agentsHandlers = agentsCtx.handlers.dataSource;
    const toolsHandlers  = toolsCtx.handlers.dataSource;
    const sel            = toolsHandlers.peekSelection();
    const rowIndex       = sel?.rowIndex ?? -1;
    if (rowIndex === -1) {
        log.warn('toolService.deleteTool – nothing selected');
        return false;
    }
    const agentSel = agentsHandlers.peekSelection();
    const agentRowIndex = agentSel.rowIndex;
    const agent      = agentSel.selected;
    if (agentRowIndex === -1 || !agentSel.selected) {
        log.warn('toolService.addTool – no agent selected');
        return false;
    }
    const agentCollection = agentsHandlers.peekCollection();
    const toolsArrOrig  = ensureArray(agent, 'tool');
    agent.tool = toolsArrOrig.filter((_, idx) => idx !== rowIndex);
    agentCollection[agentRowIndex] = {...agent};
    agentsHandlers.setCollection([...agentCollection]);
    agentsCtx.handlers.dataSource.setSelection({args:{rowIndex: agentRowIndex}})

    return true;
}


export const toolService = { addTool, deleteTool };
import { getLogger, ForgeLog } from 'forge/utils/logger';
const log = getLogger('agently');
