import React,{lazy,Suspense} from 'react';
import {Typography} from 'forge/components';
const Settings=lazy(()=>import('./MyWorkspaceSettings'));
export {WORKSPACE_SETTINGS_WINDOW_KEY,isWorkspaceSettingsWindow} from '../services/workspaceSettingsIdentity';
export default function WorkspaceSettingsDestination(){return <Suspense fallback={<Typography as="p" typeRole="body">Loading workspace settings…</Typography>}><Settings embedded/></Suspense>;}
