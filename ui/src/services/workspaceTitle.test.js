import {describe,it,expect} from 'vitest';
import {distinctWorkspaceTitle} from '../../../../forge/src/core/context/WorkspacePresentation.jsx';
describe('hosted report title ownership',()=>{
 it('suppresses only the matching title and preserves standalone or different identities',()=>{
  expect(distinctWorkspaceTitle('Performance Report',' performance   report ')).toBe('');
  expect(distinctWorkspaceTitle('Performance Report',null)).toBe('Performance Report');
  expect(distinctWorkspaceTitle('Performance Report','Resource')).toBe('Performance Report');
 });
});
