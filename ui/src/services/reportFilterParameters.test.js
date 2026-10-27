import {expect, it} from 'vitest';
import {initializeReportFilterParameters} from './reportFilterParameters';
const definition={builderDefinition:{reportBuilder:{predicates:[{id:'period',kind:'dateRange',prefill:{start:'from',end:'to'}}]}},state:{scopeParams:{period:{startExpression:'7 days ago',endExpression:'yesterday'}}}};
it('shows the declared bounded range and retains explicit date edits',()=>{
 const now=new Date(2026,9,27,12);
 expect(initializeReportFilterParameters(definition,{},now)).toEqual({from:'2026-10-20',to:'2026-10-26'});
 expect(initializeReportFilterParameters(definition,{from:'',to:'2026-10-24'},now)).toEqual({from:'',to:'2026-10-24'});
});
it('does not invent defaults when the producer has supplied none or invalid state',()=>{
 expect(initializeReportFilterParameters({})).toEqual({});
 expect(initializeReportFilterParameters({...definition,state:{scopeParams:{period:{startExpression:'unavailable',endExpression:'yesterday'}}}})).toEqual({});
});
