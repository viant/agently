import{ak as y,ac as R,aM as Q,g as Y,c as tt,d as et,e as at,z as rt,y as nt,f as p,l as z,h as it,M as st,P as lt,X as ot,aN as ct,j as ut,E as pt,N as dt}from"./index-CgRtrsGI.js";import{p as gt}from"./chunk-4BX2VUAB-DIZFarfT.js";import{p as ft}from"./wardley-RL74JXVD-WX5Ep1CF.js";import{d as _}from"./arc-D6Wg9-Hw.js";import"./min-QFJqlJr3.js";import"./_baseUniq-B9zYPYiF.js";function ht(t,a){return a<t?-1:a>t?1:a>=t?0:NaN}function mt(t){return t}function vt(){var t=mt,a=ht,f=null,S=y(0),s=y(R),d=y(0);function l(e){var n,o=(e=Q(e)).length,g,h,v=0,c=new Array(o),i=new Array(o),x=+S.apply(this,arguments),w=Math.min(R,Math.max(-R,s.apply(this,arguments)-x)),m,D=Math.min(Math.abs(w)/o,d.apply(this,arguments)),$=D*(w<0?-1:1),u;for(n=0;n<o;++n)(u=i[c[n]=n]=+t(e[n],n,e))>0&&(v+=u);for(a!=null?c.sort(function(A,C){return a(i[A],i[C])}):f!=null&&c.sort(function(A,C){return f(e[A],e[C])}),n=0,h=v?(w-o*$)/v:0;n<o;++n,x=m)g=c[n],u=i[g],m=x+(u>0?u*h:0)+$,i[g]={data:e[g],index:n,value:u,startAngle:x,endAngle:m,padAngle:D};return i}return l.value=function(e){return arguments.length?(t=typeof e=="function"?e:y(+e),l):t},l.sortValues=function(e){return arguments.length?(a=e,f=null,l):a},l.sort=function(e){return arguments.length?(f=e,a=null,l):f},l.startAngle=function(e){return arguments.length?(S=typeof e=="function"?e:y(+e),l):S},l.endAngle=function(e){return arguments.length?(s=typeof e=="function"?e:y(+e),l):s},l.padAngle=function(e){return arguments.length?(d=typeof e=="function"?e:y(+e),l):d},l}var xt=dt.pie,N={sections:new Map,showData:!1},T=N.sections,W=N.showData,yt=structuredClone(xt),St=p(()=>structuredClone(yt),"getConfig"),wt=p(()=>{T=new Map,W=N.showData,pt()},"clear"),At=p(({label:t,value:a})=>{if(a<0)throw new Error(`"${t}" has invalid value: ${a}. Negative values are not allowed in pie charts. All slice values must be >= 0.`);T.has(t)||(T.set(t,a),z.debug(`added new section: ${t}, with value: ${a}`))},"addSection"),Ct=p(()=>T,"getSections"),Dt=p(t=>{W=t},"setShowData"),$t=p(()=>W,"getShowData"),V={getConfig:St,clear:wt,setDiagramTitle:nt,getDiagramTitle:rt,setAccTitle:at,getAccTitle:et,setAccDescription:tt,getAccDescription:Y,addSection:At,getSections:Ct,setShowData:Dt,getShowData:$t},Tt=p((t,a)=>{gt(t,a),a.setShowData(t.showData),t.sections.map(a.addSection)},"populateDb"),Mt={parse:p(async t=>{const a=await ft("pie",t);z.debug(a),Tt(a,V)},"parse")},kt=p(t=>`
  .pieCircle{
    stroke: ${t.pieStrokeColor};
    stroke-width : ${t.pieStrokeWidth};
    opacity : ${t.pieOpacity};
  }
  .pieOuterCircle{
    stroke: ${t.pieOuterStrokeColor};
    stroke-width: ${t.pieOuterStrokeWidth};
    fill: none;
  }
  .pieTitleText {
    text-anchor: middle;
    font-size: ${t.pieTitleTextSize};
    fill: ${t.pieTitleTextColor};
    font-family: ${t.fontFamily};
  }
  .slice {
    font-family: ${t.fontFamily};
    fill: ${t.pieSectionTextColor};
    font-size:${t.pieSectionTextSize};
    // fill: white;
  }
  .legend text {
    fill: ${t.pieLegendTextColor};
    font-family: ${t.fontFamily};
    font-size: ${t.pieLegendTextSize};
  }
`,"getStyles"),Et=kt,bt=p(t=>{const a=[...t.values()].reduce((s,d)=>s+d,0),f=[...t.entries()].map(([s,d])=>({label:s,value:d})).filter(s=>s.value/a*100>=1);return vt().value(s=>s.value).sort(null)(f)},"createPieArcs"),Rt=p((t,a,f,S)=>{var O;z.debug(`rendering pie chart
`+t);const s=S.db,d=it(),l=st(s.getConfig(),d.pie),e=40,n=18,o=4,g=450,h=g,v=lt(a),c=v.append("g");c.attr("transform","translate("+h/2+","+g/2+")");const{themeVariables:i}=d;let[x]=ot(i.pieOuterStrokeWidth);x??(x=2);const w=l.textPosition,m=Math.min(h,g)/2-e,D=_().innerRadius(0).outerRadius(m),$=_().innerRadius(m*w).outerRadius(m*w);c.append("circle").attr("cx",0).attr("cy",0).attr("r",m+x/2).attr("class","pieOuterCircle");const u=s.getSections(),A=bt(u),C=[i.pie1,i.pie2,i.pie3,i.pie4,i.pie5,i.pie6,i.pie7,i.pie8,i.pie9,i.pie10,i.pie11,i.pie12];let M=0;u.forEach(r=>{M+=r});const F=A.filter(r=>(r.data.value/M*100).toFixed(0)!=="0"),k=ct(C).domain([...u.keys()]);c.selectAll("mySlices").data(F).enter().append("path").attr("d",D).attr("fill",r=>k(r.data.label)).attr("class","pieCircle"),c.selectAll("mySlices").data(F).enter().append("text").text(r=>(r.data.value/M*100).toFixed(0)+"%").attr("transform",r=>"translate("+$.centroid(r)+")").style("text-anchor","middle").attr("class","slice");const j=c.append("text").text(s.getDiagramTitle()).attr("x",0).attr("y",-400/2).attr("class","pieTitleText"),G=[...u.entries()].map(([r,b])=>({label:r,value:b})),E=c.selectAll(".legend").data(G).enter().append("g").attr("class","legend").attr("transform",(r,b)=>{const I=n+o,H=I*G.length/2,J=12*n,K=b*I-H;return"translate("+J+","+K+")"});E.append("rect").attr("width",n).attr("height",n).style("fill",r=>k(r.label)).style("stroke",r=>k(r.label)),E.append("text").attr("x",n+o).attr("y",n-o).text(r=>s.getShowData()?`${r.label} [${r.value}]`:r.label);const U=Math.max(...E.selectAll("text").nodes().map(r=>(r==null?void 0:r.getBoundingClientRect().width)??0)),X=h+e+n+o+U,L=((O=j.node())==null?void 0:O.getBoundingClientRect().width)??0,Z=h/2-L/2,q=h/2+L/2,P=Math.min(0,Z),B=Math.max(X,q)-P;v.attr("viewBox",`${P} 0 ${B} ${g}`),ut(v,g,B,l.useMaxWidth)},"draw"),zt={draw:Rt},Ot={parser:Mt,db:V,renderer:zt,styles:Et};export{Ot as diagram};
