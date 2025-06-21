// Utility functions for API interactions

/**
 * Polls a request function until a condition is met or max attempts are reached
 * @param {Function} requestFn - Function that returns a promise with the data to check
 * @param {Function} conditionFn - Function that checks if the condition is met
 * @param {Object} options - Polling options
 * @param {number} options.maxAttempts - Maximum number of polling attempts
 * @param {number} options.intervalMs - Interval between polling attempts in milliseconds
 * @param {Function} options.cancelled - Function that returns true if polling should be cancelled
 * @returns {Promise<*>} - The data when condition is met or null if max attempts reached
 */
export async function poll(requestFn, conditionFn, {
    maxAttempts = 900,
    intervalMs = 1000,
    cancelled = () => false,
} = {}) {
    const sleep = (ms) => new Promise((res) => setTimeout(res, ms));

    for (let attempt = 0; attempt < maxAttempts && !cancelled(); attempt++) {
        try {
            const data = await requestFn();
            if (conditionFn(data)) {
                return data;
            }
        } catch (err) {
            console.warn('Poll error:', err);
        }

        await sleep(intervalMs);
    }
    return null;
}

/**
 * Lightweight helper to GET & parse JSON with minimal ceremony
 * @param {string} url - URL to fetch
 * @param {Object} options - Fetch options
 * @returns {Promise<*>} - Parsed JSON response or null if request failed
 */
export async function fetchJSON(url, options) {
    const resp = await fetch(url, options);
    return resp.ok ? resp.json() : null;
}

/**
 * Creates a deep copy of an object
 * @param {Object} obj - Object to copy
 * @returns {Object} - Deep copy of the object
 */
export const deepCopy = (obj) => JSON.parse(JSON.stringify(obj));