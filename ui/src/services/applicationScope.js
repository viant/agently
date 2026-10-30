import {activeWindows, removeWindow} from 'forge/core';
import {createApplicationScope} from 'forge/utils/applicationScope';
export {createApplicationScope} from 'forge/utils/applicationScope';
export const applicationScope = createApplicationScope({windows: () => activeWindows.peek(), closeWindow: id => removeWindow(id)});
