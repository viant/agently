// Message normalization logic for chat messages

import {deepCopy} from './utils/apiUtils';

/**
 * Checks if a message is an assistant message with elicitation
 * @param {Object} message - The message to check
 * @returns {boolean} - True if the message is an assistant message with elicitation
 */
// Determines whether the given JSON schema represents a *simple* text
// prompt that should be rendered as a regular chat bubble rather than an
// interactive form.  The heuristic intentionally stays minimalistic and
// treats a schema as “simple” when **all** of the following hold true:
//   • exactly one property is defined
//   • the property has type "string" (or no type specified → defaults to string)
//   • the property does **not** specify an enum (open text field)
// Any other schema (multiple fields, enum constraints, number/array/etc.) is
// considered *complex* and therefore rendered with the dedicated form
// component.
export function isSimpleTextSchema(schema) {
    if (!schema || typeof schema !== 'object') return false;

    const {properties} = schema;
    if (!properties || typeof properties !== 'object') return false;

    const keys = Object.keys(properties);
    if (keys.length !== 1) return false;

    const field = properties[keys[0]] || {};

    // Detect enum – any non-empty array counts.
    if (Array.isArray(field.enum) && field.enum.length > 0) {
        return false;
    }
    if (field.format) {
        return false;
    }

    // Normalize type to array for easy checking; undefined → treat as string.
    const types = Array.isArray(field.type) ? field.type : [field.type || 'string'];
    return types.every(t => t === 'string');
}

// Determines whether an assistant message should be handled as an interactive
// form elicitation as opposed to a plain text bubble.
const isAssistantElicitation = (message) => {
    if (message.role !== 'assistant') return false;
    const schema = message.elicitation?.requestedSchema;
    if (!schema) return false;
    // Skip simple single-field text questions – they don’t need the form UI.
    return !isSimpleTextSchema(schema);
};

/**
 * Classifies a message based on its content and structure
 * @param {Object} message - The message to classify
 * @returns {string} - The message classification ('form' or 'bubble')
 */
export function classifyMessage(message) {
    // ------------------------------------------------------------------
    // Summary / summarised housekeeping
    // ------------------------------------------------------------------

    // Fully ignore messages that were already skipped in normalizeMessages
    // (status=="summarized").  We keep this guard as a safety net when
    // classifyMessage is invoked on raw messages directly.
    if (message.status === 'summarized') {
        return undefined; // caller can decide to ignore undefined classes
    }

    // Render conversation *summary* messages (status=="summary") with a
    // dedicated collapsed system-note component so they don’t clutter the
    // transcript but remain accessible on demand.
    if (message.status === 'summary') {
        return 'summary';
    }
    // Domain-specific: show execution bubble when available
    if (Array.isArray(message.executions) && message.executions.length > 0) {
        return 'execution';
    }
    // Detect interactive MCP prompts that should be rendered with a dedicated
    // component (modal dialog).  We rely on message.status so that already
    // resolved prompts (status != "open") fall back to the default bubble
    // renderer and therefore disappear from the visible chat once the user
    // has responded and the backend marked them as done/declined.

    if (message.role === 'mcpelicitation' && message.status === 'open') {
        return 'mcpelicitation';
    }

    if (message.role === 'mcpuserinteraction' && message.status === 'open') {
        return 'mcpuserinteraction';
    }

    if (message.role === 'policyapproval' && message.status === 'open') {
        return 'policyapproval';
    }

    // User supplied HTML table (JSON converted) gets special renderer.
    if (message.role === 'user' && typeof message.content === 'string' && message.content.trim().startsWith('<')) {
        return 'htmltable';
    }
    // Assistant elicitations that qualify as simple text questions
    // (see isSimpleTextSchema) are downgraded to regular bubbles so they
    // appear visually like a normal question without an embedded form.
    if (message.elicitation?.requestedSchema) {
        // Always render schema-based form when elicitation is present
        return 'form';
    }

    return 'bubble';
}

/**
 * Normalizes raw messages to handle elicitation flows and form interactions
 * @param {Array} raw - Raw messages from the API
 * @returns {Array} - Normalized messages for display
 */
export function normalizeMessages(raw = []) {
    const out = [];

    // Safe date → ISO helper to avoid invalid time errors in Forge MessageCard
    const toISOSafe = (v) => {
        if (!v) return new Date().toISOString();
        try {
            const d = new Date(v);
            if (!isNaN(d.getTime())) return d.toISOString();
        } catch (_) { /* ignore */ }
        return new Date().toISOString();
    };

    for (const msg of raw) {
        // ------------------------------------------------------------------
        // 1. Skip messages that have been explicitly marked as *summarized* –
        //    those represent the older turns that have already been merged
        //    into an aggregate summary. They should never reach the visible
        //    chat transcript.
        // ------------------------------------------------------------------
        if (msg.status === 'summarized') {
            continue; // drop entirely
        }

        const copy = deepCopy(msg);

        // Normalize timestamp for Forge MessageCard (expects valid date)
        copy.createdAt = toISOSafe(copy.createdAt || copy.CreatedAt);
        // Normalize role casing for consistent renderer classification
        if (copy.role) {
            copy.role = String(copy.role).toLowerCase();
        }

        // Assistant elicitation: when content is a JSON string/object with
        // { elicitationId, message, requestedSchema }
        if (copy.role === 'assistant') {
            const maybeObj = typeof copy.content === 'string' ? tryParseJSON(copy.content) : copy.content;
            if (maybeObj && typeof maybeObj === 'object' && (maybeObj.requestedSchema || maybeObj.elicitationId)) {
                copy.elicitation = {
                    elicitationId: maybeObj.elicitationId,
                    message: maybeObj.message || maybeObj.prompt || '',
                    requestedSchema: maybeObj.requestedSchema || {},
                };
                // Keep content as prompt text for context (not used by FormRenderer)
                copy.content = copy.elicitation.message || '';
            }
        }

        // When user supplies a JSON object, convert it to an HTML table so it renders nicely.
        if (copy.role === 'user') {
            const payload = tryParseJSON(copy.content);
            if (payload && typeof payload === 'object' && !Array.isArray(payload)) {
                const keys = Object.keys(payload);
                if (keys.length) {
                    // Start directly with <table> so HTML-based renderers pass it through
                    let html = '<table style="width:100%;border-collapse:collapse">';
                    html += '<tbody>';
                    keys.forEach(k => {
                        const cellStyle = 'word-break:break-word;white-space:pre-wrap';
                        const v = payload[k];
                        const rendered = typeof v === 'string' ? escapeHTML(v) : escapeHTML(JSON.stringify(v, null, 2));
                        html += `<tr><th style="text-align:left;padding-right:8px;white-space:nowrap">${escapeHTML(k)}</th><td style="${cellStyle}">${rendered}</td></tr>`;
                    });
                    html += '</tbody></table>';
                    copy.content = html;
                }
            }
        }

        out.push(copy);
    }
    return out;
}

/**
 * Attempts to parse a string as JSON
 * @param {string} content - String to parse
 * @returns {Object|null} - Parsed object or null if parsing failed
 */
function tryParseJSON(content) {
    try {
        return JSON.parse(content ?? "");
    } catch {
        return null;
    }
}

// Simple HTML escaper
function escapeHTML(str = '') {
    return String(str)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
}


/**
 * Creates a populated elicitation message from a user message and matching elicitation
 * @param {Object} userMessage - The user message
 * @param {Object} matchingElicitation - The matching elicitation
 * @param {Object} payload - The parsed payload
 * @returns {Object} - The populated elicitation message
 */
function createPopulatedElicitationMessage(userMessage, matchingElicitation, payload) {
    const schema = deepCopy(matchingElicitation.elicitation.requestedSchema);

    // Set default values in schema based on payload
    for (const [key, value] of Object.entries(payload)) {
        if (schema.properties?.[key]) {
            schema.properties[key].default = value;
        }
    }

    return {
        id: userMessage.id,                    // reuse user id
        role: "assistant",
        content: matchingElicitation.content,  // keep prompt content
        createdAt: userMessage.createdAt,
        elicitation: {
            message: matchingElicitation.content,
            requestedSchema: schema,
            userInputs: [payload],
        },
    };
}
