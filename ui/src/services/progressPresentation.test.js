import {describe,it,expect} from 'vitest';
import {progressStatusPresentation} from './progressPresentation';

describe('end-user progress presentation',()=>{
 it('uses neutral attention wording without exposing technical errors',()=>{
  expect(progressStatusPresentation({phase:'error',text:'Error: tool execution failed (stack detail)'}))
   .toEqual({phase:'attention',text:'This step couldn’t be completed. You can try again.'});
 });
 it('retains exact error state and text for developer diagnostics',()=>{
  const error={phase:'error',text:'Error: tool execution failed (stack detail)'};
  expect(progressStatusPresentation(error,true)).toEqual(error);
 });
 it('preserves ordinary progress and provides a specific connection recovery message',()=>{
  expect(progressStatusPresentation({phase:'thinking',text:'Preparing the report'})).toEqual({phase:'thinking',text:'Preparing the report'});
  expect(progressStatusPresentation({phase:'error',text:'Network timeout'})).toEqual({phase:'attention',text:'The connection was interrupted. Please try again.'});
 });
});
