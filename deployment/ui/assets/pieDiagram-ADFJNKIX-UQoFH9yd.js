import{aj as S,ab as N,aN as U,g as H,s as K,c as Z,d as q,y as J,x as Q,e as p,l as z,f as X,H as Y,N as ee,W as te,aO as ae,i as re,C as ne,K as ie}from"./index-DYe8F2Mv.js";import{p as se}from"./chunk-4BX2VUAB-BkTr0vQ9.js";import{p as le}from"./treemap-GDKQZRPO-ClcPdrZS.js";import{d as I}from"./arc-hwDSbVAo.js";import"./_baseUniq-CVUxF1fc.js";import"./_basePickBy-BrAeA41J.js";import"./clone-CKejjTj0.js";function oe(e,a){return a<e?-1:a>e?1:a>=e?0:NaN}function ce(e){return e}function ue(){var e=ce,a=oe,f=null,x=S(0),s=S(N),o=S(0);function l(t){var n,c=(t=U(t)).length,g,y,m=0,u=new Array(c),i=new Array(c),v=+x.apply(this,arguments),w=Math.min(N,Math.max(-N,s.apply(this,arguments)-v)),h,D=Math.min(Math.abs(w)/c,o.apply(this,arguments)),$=D*(w<0?-1:1),d;for(n=0;n<c;++n)(d=i[u[n]=n]=+e(t[n],n,t))>0&&(m+=d);for(a!=null?u.sort(function(A,C){return a(i[A],i[C])}):f!=null&&u.sort(function(A,C){return f(t[A],t[C])}),n=0,y=m?(w-c*$)/m:0;n<c;++n,v=h)g=u[n],d=i[g],h=v+(d>0?d*y:0)+$,i[g]={data:t[g],index:n,value:d,startAngle:v,endAngle:h,padAngle:D};return i}return l.value=function(t){return arguments.length?(e=typeof t=="function"?t:S(+t),l):e},l.sortValues=function(t){return arguments.length?(a=t,f=null,l):a},l.sort=function(t){return arguments.length?(f=t,a=null,l):f},l.startAngle=function(t){return arguments.length?(x=typeof t=="function"?t:S(+t),l):x},l.endAngle=function(t){return arguments.length?(s=typeof t=="function"?t:S(+t),l):s},l.padAngle=function(t){return arguments.length?(o=typeof t=="function"?t:S(+t),l):o},l}var pe=ie.pie,F={sections:new Map,showData:!1},T=F.sections,W=F.showData,ge=structuredClone(pe),de=p(()=>structuredClone(ge),"getConfig"),fe=p(()=>{T=new Map,W=F.showData,ne()},"clear"),he=p(({label:e,value:a})=>{if(a<0)throw new Error(`"${e}" has invalid value: ${a}. Negative values are not allowed in pie charts. All slice values must be >= 0.`);T.has(e)||(T.set(e,a),z.debug(`added new section: ${e}, with value: ${a}`))},"addSection"),me=p(()=>T,"getSections"),ve=p(e=>{W=e},"setShowData"),Se=p(()=>W,"getShowData"),L={getConfig:de,clear:fe,setDiagramTitle:Q,getDiagramTitle:J,setAccTitle:q,getAccTitle:Z,setAccDescription:K,getAccDescription:H,addSection:he,getSections:me,setShowData:ve,getShowData:Se},xe=p((e,a)=>{se(e,a),a.setShowData(e.showData),e.sections.map(a.addSection)},"populateDb"),ye={parse:p(async e=>{const a=await le("pie",e);z.debug(a),xe(a,L)},"parse")},we=p(e=>`
  .pieCircle{
    stroke: ${e.pieStrokeColor};
    stroke-width : ${e.pieStrokeWidth};
    opacity : ${e.pieOpacity};
  }
  .pieOuterCircle{
    stroke: ${e.pieOuterStrokeColor};
    stroke-width: ${e.pieOuterStrokeWidth};
    fill: none;
  }
  .pieTitleText {
    text-anchor: middle;
    font-size: ${e.pieTitleTextSize};
    fill: ${e.pieTitleTextColor};
    font-family: ${e.fontFamily};
  }
  .slice {
    font-family: ${e.fontFamily};
    fill: ${e.pieSectionTextColor};
    font-size:${e.pieSectionTextSize};
    // fill: white;
  }
  .legend text {
    fill: ${e.pieLegendTextColor};
    font-family: ${e.fontFamily};
    font-size: ${e.pieLegendTextSize};
  }
`,"getStyles"),Ae=we,Ce=p(e=>{const a=[...e.values()].reduce((s,o)=>s+o,0),f=[...e.entries()].map(([s,o])=>({label:s,value:o})).filter(s=>s.value/a*100>=1).sort((s,o)=>o.value-s.value);return ue().value(s=>s.value)(f)},"createPieArcs"),De=p((e,a,f,x)=>{z.debug(`rendering pie chart
`+e);const s=x.db,o=X(),l=Y(s.getConfig(),o.pie),t=40,n=18,c=4,g=450,y=g,m=ee(a),u=m.append("g");u.attr("transform","translate("+y/2+","+g/2+")");const{themeVariables:i}=o;let[v]=te(i.pieOuterStrokeWidth);v??(v=2);const w=l.textPosition,h=Math.min(y,g)/2-t,D=I().innerRadius(0).outerRadius(h),$=I().innerRadius(h*w).outerRadius(h*w);u.append("circle").attr("cx",0).attr("cy",0).attr("r",h+v/2).attr("class","pieOuterCircle");const d=s.getSections(),A=Ce(d),C=[i.pie1,i.pie2,i.pie3,i.pie4,i.pie5,i.pie6,i.pie7,i.pie8,i.pie9,i.pie10,i.pie11,i.pie12];let b=0;d.forEach(r=>{b+=r});const G=A.filter(r=>(r.data.value/b*100).toFixed(0)!=="0"),E=ae(C);u.selectAll("mySlices").data(G).enter().append("path").attr("d",D).attr("fill",r=>E(r.data.label)).attr("class","pieCircle"),u.selectAll("mySlices").data(G).enter().append("text").text(r=>(r.data.value/b*100).toFixed(0)+"%").attr("transform",r=>"translate("+$.centroid(r)+")").style("text-anchor","middle").attr("class","slice"),u.append("text").text(s.getDiagramTitle()).attr("x",0).attr("y",-400/2).attr("class","pieTitleText");const O=[...d.entries()].map(([r,M])=>({label:r,value:M})),k=u.selectAll(".legend").data(O).enter().append("g").attr("class","legend").attr("transform",(r,M)=>{const R=n+c,B=R*O.length/2,V=12*n,j=M*R-B;return"translate("+V+","+j+")"});k.append("rect").attr("width",n).attr("height",n).style("fill",r=>E(r.label)).style("stroke",r=>E(r.label)),k.append("text").attr("x",n+c).attr("y",n-c).text(r=>s.getShowData()?`${r.label} [${r.value}]`:r.label);const _=Math.max(...k.selectAll("text").nodes().map(r=>(r==null?void 0:r.getBoundingClientRect().width)??0)),P=y+t+n+c+_;m.attr("viewBox",`0 0 ${P} ${g}`),re(m,g,P,l.useMaxWidth)},"draw"),$e={draw:De},Fe={parser:ye,db:L,renderer:$e,styles:Ae};export{Fe as diagram};
