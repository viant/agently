import{ah as y,ac as R,ba as Q,c as d,g as Y,d as tt,e as et,f as at,z as rt,y as nt,l as z,h as it,M as st,P as lt,X as ot,bb as ct,j as ut,E as dt,N as gt}from"./index-DVTbDsF8.js";import{p as pt}from"./chunk-4BX2VUAB-CAmvXXjs.js";import{p as ft}from"./wardley-L42UT6IY-DR8pdPvx.js";import{d as _}from"./arc-D1Y-L8Pk.js";function ht(t,a){return a<t?-1:a>t?1:a>=t?0:NaN}function mt(t){return t}function vt(){var t=mt,a=ht,f=null,S=y(0),s=y(R),g=y(0);function l(e){var n,o=(e=Q(e)).length,p,h,v=0,c=new Array(o),i=new Array(o),x=+S.apply(this,arguments),w=Math.min(R,Math.max(-R,s.apply(this,arguments)-x)),m,D=Math.min(Math.abs(w)/o,g.apply(this,arguments)),$=D*(w<0?-1:1),u;for(n=0;n<o;++n)(u=i[c[n]=n]=+t(e[n],n,e))>0&&(v+=u);for(a!=null?c.sort(function(A,C){return a(i[A],i[C])}):f!=null&&c.sort(function(A,C){return f(e[A],e[C])}),n=0,h=v?(w-o*$)/v:0;n<o;++n,x=m)p=c[n],u=i[p],m=x+(u>0?u*h:0)+$,i[p]={data:e[p],index:n,value:u,startAngle:x,endAngle:m,padAngle:D};return i}return l.value=function(e){return arguments.length?(t=typeof e=="function"?e:y(+e),l):t},l.sortValues=function(e){return arguments.length?(a=e,f=null,l):a},l.sort=function(e){return arguments.length?(f=e,a=null,l):f},l.startAngle=function(e){return arguments.length?(S=typeof e=="function"?e:y(+e),l):S},l.endAngle=function(e){return arguments.length?(s=typeof e=="function"?e:y(+e),l):s},l.padAngle=function(e){return arguments.length?(g=typeof e=="function"?e:y(+e),l):g},l}var xt=gt.pie,W={sections:new Map,showData:!1},T=W.sections,F=W.showData,yt=structuredClone(xt),St=d(()=>structuredClone(yt),"getConfig"),wt=d(()=>{T=new Map,F=W.showData,dt()},"clear"),At=d(({label:t,value:a})=>{if(a<0)throw new Error(`"${t}" has invalid value: ${a}. Negative values are not allowed in pie charts. All slice values must be >= 0.`);T.has(t)||(T.set(t,a),z.debug(`added new section: ${t}, with value: ${a}`))},"addSection"),Ct=d(()=>T,"getSections"),Dt=d(t=>{F=t},"setShowData"),$t=d(()=>F,"getShowData"),V={getConfig:St,clear:wt,setDiagramTitle:nt,getDiagramTitle:rt,setAccTitle:at,getAccTitle:et,setAccDescription:tt,getAccDescription:Y,addSection:At,getSections:Ct,setShowData:Dt,getShowData:$t},Tt=d((t,a)=>{pt(t,a),a.setShowData(t.showData),t.sections.map(a.addSection)},"populateDb"),bt={parse:d(async t=>{const a=await ft("pie",t);z.debug(a),Tt(a,V)},"parse")},Et=d(t=>`
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
`,"getStyles"),Mt=Et,kt=d(t=>{const a=[...t.values()].reduce((s,g)=>s+g,0),f=[...t.entries()].map(([s,g])=>({label:s,value:g})).filter(s=>s.value/a*100>=1);return vt().value(s=>s.value).sort(null)(f)},"createPieArcs"),Rt=d((t,a,f,S)=>{var O;z.debug(`rendering pie chart
`+t);const s=S.db,g=it(),l=st(s.getConfig(),g.pie),e=40,n=18,o=4,p=450,h=p,v=lt(a),c=v.append("g");c.attr("transform","translate("+h/2+","+p/2+")");const{themeVariables:i}=g;let[x]=ot(i.pieOuterStrokeWidth);x??(x=2);const w=l.textPosition,m=Math.min(h,p)/2-e,D=_().innerRadius(0).outerRadius(m),$=_().innerRadius(m*w).outerRadius(m*w);c.append("circle").attr("cx",0).attr("cy",0).attr("r",m+x/2).attr("class","pieOuterCircle");const u=s.getSections(),A=kt(u),C=[i.pie1,i.pie2,i.pie3,i.pie4,i.pie5,i.pie6,i.pie7,i.pie8,i.pie9,i.pie10,i.pie11,i.pie12];let b=0;u.forEach(r=>{b+=r});const N=A.filter(r=>(r.data.value/b*100).toFixed(0)!=="0"),E=ct(C).domain([...u.keys()]);c.selectAll("mySlices").data(N).enter().append("path").attr("d",D).attr("fill",r=>E(r.data.label)).attr("class","pieCircle"),c.selectAll("mySlices").data(N).enter().append("text").text(r=>(r.data.value/b*100).toFixed(0)+"%").attr("transform",r=>"translate("+$.centroid(r)+")").style("text-anchor","middle").attr("class","slice");const j=c.append("text").text(s.getDiagramTitle()).attr("x",0).attr("y",-400/2).attr("class","pieTitleText"),G=[...u.entries()].map(([r,k])=>({label:r,value:k})),M=c.selectAll(".legend").data(G).enter().append("g").attr("class","legend").attr("transform",(r,k)=>{const I=n+o,H=I*G.length/2,J=12*n,K=k*I-H;return"translate("+J+","+K+")"});M.append("rect").attr("width",n).attr("height",n).style("fill",r=>E(r.label)).style("stroke",r=>E(r.label)),M.append("text").attr("x",n+o).attr("y",n-o).text(r=>s.getShowData()?`${r.label} [${r.value}]`:r.label);const U=Math.max(...M.selectAll("text").nodes().map(r=>(r==null?void 0:r.getBoundingClientRect().width)??0)),X=h+e+n+o+U,L=((O=j.node())==null?void 0:O.getBoundingClientRect().width)??0,Z=h/2-L/2,q=h/2+L/2,P=Math.min(0,Z),B=Math.max(X,q)-P;v.attr("viewBox",`${P} 0 ${B} ${p}`),ut(v,p,B,l.useMaxWidth)},"draw"),zt={draw:Rt},Pt={parser:bt,db:V,renderer:zt,styles:Mt};export{Pt as diagram};
