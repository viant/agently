// Message normalization logic for chat messages

import { deepCopy } from './utils/apiUtils';

/**
 * Checks if a message is an assistant message with elicitation
 * @param {Object} message - The message to check
 * @returns {boolean} - True if the message is an assistant message with elicitation
 */
const isAssistantElicitation = (message) =>
    message.role === "assistant" && message.elicitation?.requestedSchema;

/**
 * Classifies a message based on its content and structure
 * @param {Object} message - The message to classify
 * @returns {string} - The message classification ('form' or 'bubble')
 */
export function classifyMessage(message) {
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
    return message.elicitation?.requestedSchema ? 'form' : 'bubble';
}

/**
 * Normalizes raw messages to handle elicitation flows and form interactions
 * @param {Array} raw - Raw messages from the API
 * @returns {Array} - Normalized messages for display
 */
export function normalizeMessages(raw = []) {
    const out = [];

    let pending = null;          // waiting assistant-elicitation
    const openStack = [];        // past elicitations still answerable

    const flushPending = () => {
        if (pending) {
            out.push(pending);
            openStack.push(pending);
            pending = null;
        }
    };

    for (let i = 0; i < raw.length; i++) {
        const message = raw[i];

        // 1. Assistant message with elicitation
        if (isAssistantElicitation(message)) {
            flushPending();                     // avoid doublets
            pending = deepCopy(message);
            pending.elicitation.userInputs = [];
            continue;                           // don't output yet
        }

        // 2. User message with potential JSON payload
        if (message.role === "user") {
            const payload = tryParseJSON(message.content);

            if (payload && typeof payload === "object") {
                const matchedElicitation = findMatchingElicitation(payload, pending, openStack);
                
                if (matchedElicitation) {
                    // If best match is still pending, emit only the populated version
                    const isPendingBest = matchedElicitation === pending;
                    if (isPendingBest) pending = null;

                    const populatedMessage = createPopulatedElicitationMessage(message, matchedElicitation, payload);
                    
                    out.push(populatedMessage);
                    openStack.push(populatedMessage);
                    continue;                      // consume user message
                }
            }
        }

        // 3. Everything else
        flushPending();
        out.push(deepCopy(message));
    }

    flushPending(); // tail flush

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

/**
 * Finds the best matching elicitation for a payload
 * @param {Object} payload - The payload to match
 * @param {Object} pending - The pending elicitation
 * @param {Array} openStack - Stack of open elicitations
 * @returns {Object|null} - The best matching elicitation or null
 */
function findMatchingElicitation(payload, pending, openStack) {
    // Build search list: pending (if any) first, then openStack (newest→oldest)
    const candidates = [
        ...(pending ? [pending] : []),
        ...openStack.slice().reverse(),
    ];

    let best = null;
    for (const elicitation of candidates) {
        const { properties, required = [] } = elicitation.elicitation.requestedSchema;
        const sharesKey = Object.keys(payload).some((k) => k in properties);
        if (!sharesKey) continue;

        best = elicitation;
        const hasAllRequired = required.every((k) => k in payload);
        if (hasAllRequired) break;           // perfect match
    }

    return best;
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