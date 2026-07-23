import{M as y,bp as R,l as K,a7 as Q,bc as tt,a8 as et,bd as at,ac as rt,bf as nt,e as p,aI as z,aa as it,y as st,bb as lt,aU as ot,aT as ct,L as ut,z as pt,Y as dt}from"./index---mJp4TZ.js";import{p as gt}from"./chunk-4BX2VUAB-BWvhrnrR.js";import{p as ft}from"./wardley-L42UT6IY-B0mszA01.js";import{d as _}from"./arc-DyqlDtHm.js";function ht(t,a){return a<t?-1:a>t?1:a>=t?0:NaN}function mt(t){return t}function vt(){var t=mt,a=ht,f=null,S=y(0),s=y(R),d=y(0);function l(e){var n,o=(e=K(e)).length,g,h,v=0,c=new Array(o),i=new Array(o),x=+S.apply(this,arguments),w=Math.min(R,Math.max(-R,s.apply(this,arguments)-x)),m,D=Math.min(Math.abs(w)/o,d.apply(this,arguments)),$=D*(w<0?-1:1),u;for(n=0;n<o;++n)(u=i[c[n]=n]=+t(e[n],n,e))>0&&(v+=u);for(a!=null?c.sort(function(A,C){return a(i[A],i[C])}):f!=null&&c.sort(function(A,C){return f(e[A],e[C])}),n=0,h=v?(w-o*$)/v:0;n<o;++n,x=m)g=c[n],u=i[g],m=x+(u>0?u*h:0)+$,i[g]={data:e[g],index:n,value:u,startAngle:x,endAngle:m,padAngle:D};return i}return l.value=function(e){return arguments.length?(t=typeof e=="function"?e:y(+e),l):t},l.sortValues=function(e){return arguments.length?(a=e,f=null,l):a},l.sort=function(e){return arguments.length?(f=e,a=null,l):f},l.startAngle=function(e){return arguments.length?(S=typeof e=="function"?e:y(+e),l):S},l.endAngle=function(e){return arguments.length?(s=typeof e=="function"?e:y(+e),l):s},l.padAngle=function(e){return arguments.length?(d=typeof e=="function"?e:y(+e),l):d},l}var xt=dt.pie,W={sections:new Map,showData:!1},T=W.sections,F=W.showData,yt=structuredClone(xt),St=p(()=>structuredClone(yt),"getConfig"),wt=p(()=>{T=new Map,F=W.showData,pt()},"clear"),At=p(({label:t,value:a})=>{if(a<0)throw new Error(`"${t}" has invalid value: ${a}. Negative values are not allowed in pie charts. All slice values must be >= 0.`);T.has(t)||(T.set(t,a),z.debug(`added new section: ${t}, with value: ${a}`))},"addSection"),Ct=p(()=>T,"getSections"),Dt=p(t=>{F=t},"setShowData"),$t=p(()=>F,"getShowData"),U={getConfig:St,clear:wt,setDiagramTitle:nt,getDiagramTitle:rt,setAccTitle:at,getAccTitle:et,setAccDescription:tt,getAccDescription:Q,addSection:At,getSections:Ct,setShowData:Dt,getShowData:$t},Tt=p((t,a)=>{gt(t,a),a.setShowData(t.showData),t.sections.map(a.addSection)},"populateDb"),bt={parse:p(async t=>{const a=await ft("pie",t);z.debug(a),Tt(a,U)},"parse")},Mt=p(t=>`
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
`,"getStyles"),kt=Mt,Et=p(t=>{const a=[...t.values()].reduce((s,d)=>s+d,0),f=[...t.entries()].map(([s,d])=>({label:s,value:d})).filter(s=>s.value/a*100>=1);return vt().value(s=>s.value).sort(null)(f)},"createPieArcs"),Rt=p((t,a,f,S)=>{var O;z.debug(`rendering pie chart
`+t);const s=S.db,d=it(),l=st(s.getConfig(),d.pie),e=40,n=18,o=4,g=450,h=g,v=lt(a),c=v.append("g");c.attr("transform","translate("+h/2+","+g/2+")");const{themeVariables:i}=d;let[x]=ot(i.pieOuterStrokeWidth);x??(x=2);const w=l.textPosition,m=Math.min(h,g)/2-e,D=_().innerRadius(0).outerRadius(m),$=_().innerRadius(m*w).outerRadius(m*w);c.append("circle").attr("cx",0).attr("cy",0).attr("r",m+x/2).attr("class","pieOuterCircle");const u=s.getSections(),A=Et(u),C=[i.pie1,i.pie2,i.pie3,i.pie4,i.pie5,i.pie6,i.pie7,i.pie8,i.pie9,i.pie10,i.pie11,i.pie12];let b=0;u.forEach(r=>{b+=r});const L=A.filter(r=>(r.data.value/b*100).toFixed(0)!=="0"),M=ct(C).domain([...u.keys()]);c.selectAll("mySlices").data(L).enter().append("path").attr("d",D).attr("fill",r=>M(r.data.label)).attr("class","pieCircle"),c.selectAll("mySlices").data(L).enter().append("text").text(r=>(r.data.value/b*100).toFixed(0)+"%").attr("transform",r=>"translate("+$.centroid(r)+")").style("text-anchor","middle").attr("class","slice");const V=c.append("text").text(s.getDiagramTitle()).attr("x",0).attr("y",-400/2).attr("class","pieTitleText"),G=[...u.entries()].map(([r,E])=>({label:r,value:E})),k=c.selectAll(".legend").data(G).enter().append("g").attr("class","legend").attr("transform",(r,E)=>{const P=n+o,q=P*G.length/2,H=12*n,J=E*P-q;return"translate("+H+","+J+")"});k.append("rect").attr("width",n).attr("height",n).style("fill",r=>M(r.label)).style("stroke",r=>M(r.label)),k.append("text").attr("x",n+o).attr("y",n-o).text(r=>s.getShowData()?`${r.label} [${r.value}]`:r.label);const j=Math.max(...k.selectAll("text").nodes().map(r=>(r==null?void 0:r.getBoundingClientRect().width)??0)),X=h+e+n+o+j,N=((O=V.node())==null?void 0:O.getBoundingClientRect().width)??0,Y=h/2-N/2,Z=h/2+N/2,B=Math.min(0,Y),I=Math.max(X,Z)-B;v.attr("viewBox",`${B} 0 ${I} ${g}`),ut(v,g,I,l.useMaxWidth)},"draw"),zt={draw:Rt},Bt={parser:bt,db:U,renderer:zt,styles:kt};export{Bt as diagram};
