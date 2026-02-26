import{R as ue,O as Ie,P as de,Q as fe,T as he,U as Mt,V as Le,W as Rt,c as Fe,s as Ye,G as We,H as Ve,d as Pe,e as ze,g as ot,f as pt,X as Oe,Y as Ne,Z as Re,h as Be,$ as He,a0 as G,l as Ct,a1 as $t,a2 as Jt,a3 as qe,a4 as Xe,a5 as Ge,a6 as Ze,a7 as Ue,a8 as je,a9 as Qe,aa as Kt,ab as te,ac as ee,ad as ne,ae as ie,n as $e,m as Je,J as Ke,F as tn}from"./main-DctFIa8R.js";function en(t){return t}var vt=1,At=2,Wt=3,bt=4,re=1e-6;function nn(t){return"translate("+t+",0)"}function rn(t){return"translate(0,"+t+")"}function sn(t){return e=>+t(e)}function an(t,e){return e=Math.max(0,t.bandwidth()-e*2)/2,t.round()&&(e=Math.round(e)),n=>+t(n)+e}function cn(){return!this.__axis}function me(t,e){var n=[],i=null,a=null,h=6,o=6,x=3,C=typeof window<"u"&&window.devicePixelRatio>1?0:.5,D=t===vt||t===bt?-1:1,p=t===bt||t===At?"x":"y",M=t===vt||t===Wt?nn:rn;function _(b){var B=i??(e.ticks?e.ticks.apply(e,n):e.domain()),A=a??(e.tickFormat?e.tickFormat.apply(e,n):en),T=Math.max(h,0)+x,S=e.range(),L=+S[0]+C,F=+S[S.length-1]+C,N=(e.bandwidth?an:sn)(e.copy(),C),O=b.selection?b.selection():b,q=O.selectAll(".domain").data([null]),z=O.selectAll(".tick").data(B,e).order(),m=z.exit(),w=z.enter().append("g").attr("class","tick"),v=z.select("line"),k=z.select("text");q=q.merge(q.enter().insert("path",".tick").attr("class","domain").attr("stroke","currentColor")),z=z.merge(w),v=v.merge(w.append("line").attr("stroke","currentColor").attr(p+"2",D*h)),k=k.merge(w.append("text").attr("fill","currentColor").attr(p,D*T).attr("dy",t===vt?"0em":t===Wt?"0.71em":"0.32em")),b!==O&&(q=q.transition(b),z=z.transition(b),v=v.transition(b),k=k.transition(b),m=m.transition(b).attr("opacity",re).attr("transform",function(r){return isFinite(r=N(r))?M(r+C):this.getAttribute("transform")}),w.attr("opacity",re).attr("transform",function(r){var l=this.parentNode.__axis;return M((l&&isFinite(l=l(r))?l:N(r))+C)})),m.remove(),q.attr("d",t===bt||t===At?o?"M"+D*o+","+L+"H"+C+"V"+F+"H"+D*o:"M"+C+","+L+"V"+F:o?"M"+L+","+D*o+"V"+C+"H"+F+"V"+D*o:"M"+L+","+C+"H"+F),z.attr("opacity",1).attr("transform",function(r){return M(N(r)+C)}),v.attr(p+"2",D*h),k.attr(p,D*T).text(A),O.filter(cn).attr("fill","none").attr("font-size",10).attr("font-family","sans-serif").attr("text-anchor",t===At?"start":t===bt?"end":"middle"),O.each(function(){this.__axis=N})}return _.scale=function(b){return arguments.length?(e=b,_):e},_.ticks=function(){return n=Array.from(arguments),_},_.tickArguments=function(b){return arguments.length?(n=b==null?[]:Array.from(b),_):n.slice()},_.tickValues=function(b){return arguments.length?(i=b==null?null:Array.from(b),_):i&&i.slice()},_.tickFormat=function(b){return arguments.length?(a=b,_):a},_.tickSize=function(b){return arguments.length?(h=o=+b,_):h},_.tickSizeInner=function(b){return arguments.length?(h=+b,_):h},_.tickSizeOuter=function(b){return arguments.length?(o=+b,_):o},_.tickPadding=function(b){return arguments.length?(x=+b,_):x},_.offset=function(b){return arguments.length?(C=+b,_):C},_}function on(t){return me(vt,t)}function ln(t){return me(Wt,t)}const un=Math.PI/180,dn=180/Math.PI,St=18,ke=.96422,ye=1,ge=.82521,pe=4/29,lt=6/29,be=3*lt*lt,fn=lt*lt*lt;function ve(t){if(t instanceof $)return new $(t.l,t.a,t.b,t.opacity);if(t instanceof tt)return xe(t);t instanceof ue||(t=Ie(t));var e=Yt(t.r),n=Yt(t.g),i=Yt(t.b),a=It((.2225045*e+.7168786*n+.0606169*i)/ye),h,o;return e===n&&n===i?h=o=a:(h=It((.4360747*e+.3850649*n+.1430804*i)/ke),o=It((.0139322*e+.0971045*n+.7141733*i)/ge)),new $(116*a-16,500*(h-a),200*(a-o),t.opacity)}function hn(t,e,n,i){return arguments.length===1?ve(t):new $(t,e,n,i??1)}function $(t,e,n,i){this.l=+t,this.a=+e,this.b=+n,this.opacity=+i}de($,hn,fe(he,{brighter(t){return new $(this.l+St*(t??1),this.a,this.b,this.opacity)},darker(t){return new $(this.l-St*(t??1),this.a,this.b,this.opacity)},rgb(){var t=(this.l+16)/116,e=isNaN(this.a)?t:t+this.a/500,n=isNaN(this.b)?t:t-this.b/200;return e=ke*Lt(e),t=ye*Lt(t),n=ge*Lt(n),new ue(Ft(3.1338561*e-1.6168667*t-.4906146*n),Ft(-.9787684*e+1.9161415*t+.033454*n),Ft(.0719453*e-.2289914*t+1.4052427*n),this.opacity)}}));function It(t){return t>fn?Math.pow(t,1/3):t/be+pe}function Lt(t){return t>lt?t*t*t:be*(t-pe)}function Ft(t){return 255*(t<=.0031308?12.92*t:1.055*Math.pow(t,1/2.4)-.055)}function Yt(t){return(t/=255)<=.04045?t/12.92:Math.pow((t+.055)/1.055,2.4)}function mn(t){if(t instanceof tt)return new tt(t.h,t.c,t.l,t.opacity);if(t instanceof $||(t=ve(t)),t.a===0&&t.b===0)return new tt(NaN,0<t.l&&t.l<100?0:NaN,t.l,t.opacity);var e=Math.atan2(t.b,t.a)*dn;return new tt(e<0?e+360:e,Math.sqrt(t.a*t.a+t.b*t.b),t.l,t.opacity)}function Vt(t,e,n,i){return arguments.length===1?mn(t):new tt(t,e,n,i??1)}function tt(t,e,n,i){this.h=+t,this.c=+e,this.l=+n,this.opacity=+i}function xe(t){if(isNaN(t.h))return new $(t.l,0,0,t.opacity);var e=t.h*un;return new $(t.l,Math.cos(e)*t.c,Math.sin(e)*t.c,t.opacity)}de(tt,Vt,fe(he,{brighter(t){return new tt(this.h,this.c,this.l+St*(t??1),this.opacity)},darker(t){return new tt(this.h,this.c,this.l-St*(t??1),this.opacity)},rgb(){return xe(this).rgb()}}));function kn(t){return function(e,n){var i=t((e=Vt(e)).h,(n=Vt(n)).h),a=Mt(e.c,n.c),h=Mt(e.l,n.l),o=Mt(e.opacity,n.opacity);return function(x){return e.h=i(x),e.c=a(x),e.l=h(x),e.opacity=o(x),e+""}}}const yn=kn(Le);var xt={exports:{}},gn=xt.exports,se;function pn(){return se||(se=1,(function(t,e){(function(n,i){t.exports=i()})(gn,(function(){var n="day";return function(i,a,h){var o=function(D){return D.add(4-D.isoWeekday(),n)},x=a.prototype;x.isoWeekYear=function(){return o(this).year()},x.isoWeek=function(D){if(!this.$utils().u(D))return this.add(7*(D-this.isoWeek()),n);var p,M,_,b,B=o(this),A=(p=this.isoWeekYear(),M=this.$u,_=(M?h.utc:h)().year(p).startOf("year"),b=4-_.isoWeekday(),_.isoWeekday()>4&&(b+=7),_.add(b,n));return B.diff(A,"week")+1},x.isoWeekday=function(D){return this.$utils().u(D)?this.day()||7:this.day(this.day()%7?D:D-7)};var C=x.startOf;x.startOf=function(D,p){var M=this.$utils(),_=!!M.u(p)||p;return M.p(D)==="isoweek"?_?this.date(this.date()-(this.isoWeekday()-1)).startOf("day"):this.date(this.date()-1-(this.isoWeekday()-1)+7).endOf("day"):C.bind(this)(D,p)}}}))})(xt)),xt.exports}var bn=pn();const vn=Rt(bn);var Tt={exports:{}},xn=Tt.exports,ae;function Tn(){return ae||(ae=1,(function(t,e){(function(n,i){t.exports=i()})(xn,(function(){var n={LTS:"h:mm:ss A",LT:"h:mm A",L:"MM/DD/YYYY",LL:"MMMM D, YYYY",LLL:"MMMM D, YYYY h:mm A",LLLL:"dddd, MMMM D, YYYY h:mm A"},i=/(\[[^[]*\])|([-_:/.,()\s]+)|(A|a|Q|YYYY|YY?|ww?|MM?M?M?|Do|DD?|hh?|HH?|mm?|ss?|S{1,3}|z|ZZ?)/g,a=/\d/,h=/\d\d/,o=/\d\d?/,x=/\d*[^-_:/,()\s\d]+/,C={},D=function(T){return(T=+T)+(T>68?1900:2e3)},p=function(T){return function(S){this[T]=+S}},M=[/[+-]\d\d:?(\d\d)?|Z/,function(T){(this.zone||(this.zone={})).offset=(function(S){if(!S||S==="Z")return 0;var L=S.match(/([+-]|\d\d)/g),F=60*L[1]+(+L[2]||0);return F===0?0:L[0]==="+"?-F:F})(T)}],_=function(T){var S=C[T];return S&&(S.indexOf?S:S.s.concat(S.f))},b=function(T,S){var L,F=C.meridiem;if(F){for(var N=1;N<=24;N+=1)if(T.indexOf(F(N,0,S))>-1){L=N>12;break}}else L=T===(S?"pm":"PM");return L},B={A:[x,function(T){this.afternoon=b(T,!1)}],a:[x,function(T){this.afternoon=b(T,!0)}],Q:[a,function(T){this.month=3*(T-1)+1}],S:[a,function(T){this.milliseconds=100*+T}],SS:[h,function(T){this.milliseconds=10*+T}],SSS:[/\d{3}/,function(T){this.milliseconds=+T}],s:[o,p("seconds")],ss:[o,p("seconds")],m:[o,p("minutes")],mm:[o,p("minutes")],H:[o,p("hours")],h:[o,p("hours")],HH:[o,p("hours")],hh:[o,p("hours")],D:[o,p("day")],DD:[h,p("day")],Do:[x,function(T){var S=C.ordinal,L=T.match(/\d+/);if(this.day=L[0],S)for(var F=1;F<=31;F+=1)S(F).replace(/\[|\]/g,"")===T&&(this.day=F)}],w:[o,p("week")],ww:[h,p("week")],M:[o,p("month")],MM:[h,p("month")],MMM:[x,function(T){var S=_("months"),L=(_("monthsShort")||S.map((function(F){return F.slice(0,3)}))).indexOf(T)+1;if(L<1)throw new Error;this.month=L%12||L}],MMMM:[x,function(T){var S=_("months").indexOf(T)+1;if(S<1)throw new Error;this.month=S%12||S}],Y:[/[+-]?\d+/,p("year")],YY:[h,function(T){this.year=D(T)}],YYYY:[/\d{4}/,p("year")],Z:M,ZZ:M};function A(T){var S,L;S=T,L=C&&C.formats;for(var F=(T=S.replace(/(\[[^\]]+])|(LTS?|l{1,4}|L{1,4})/g,(function(v,k,r){var l=r&&r.toUpperCase();return k||L[r]||n[r]||L[l].replace(/(\[[^\]]+])|(MMMM|MM|DD|dddd)/g,(function(f,c,g){return c||g.slice(1)}))}))).match(i),N=F.length,O=0;O<N;O+=1){var q=F[O],z=B[q],m=z&&z[0],w=z&&z[1];F[O]=w?{regex:m,parser:w}:q.replace(/^\[|\]$/g,"")}return function(v){for(var k={},r=0,l=0;r<N;r+=1){var f=F[r];if(typeof f=="string")l+=f.length;else{var c=f.regex,g=f.parser,s=v.slice(l),P=c.exec(s)[0];g.call(k,P),v=v.replace(P,"")}}return(function(d){var u=d.afternoon;if(u!==void 0){var y=d.hours;u?y<12&&(d.hours+=12):y===12&&(d.hours=0),delete d.afternoon}})(k),k}}return function(T,S,L){L.p.customParseFormat=!0,T&&T.parseTwoDigitYear&&(D=T.parseTwoDigitYear);var F=S.prototype,N=F.parse;F.parse=function(O){var q=O.date,z=O.utc,m=O.args;this.$u=z;var w=m[1];if(typeof w=="string"){var v=m[2]===!0,k=m[3]===!0,r=v||k,l=m[2];k&&(l=m[2]),C=this.$locale(),!v&&l&&(C=L.Ls[l]),this.$d=(function(s,P,d,u){try{if(["x","X"].indexOf(P)>-1)return new Date((P==="X"?1e3:1)*s);var y=A(P)(s),V=y.year,E=y.month,Y=y.day,I=y.hours,W=y.minutes,rt=y.seconds,st=y.milliseconds,kt=y.zone,yt=y.week,H=new Date,U=Y||(V||E?1:H.getDate()),X=V||H.getFullYear(),et=0;V&&!E||(et=E>0?E-1:H.getMonth());var j,nt=I||0,Z=W||0,ct=rt||0,it=st||0;return kt?new Date(Date.UTC(X,et,U,nt,Z,ct,it+60*kt.offset*1e3)):d?new Date(Date.UTC(X,et,U,nt,Z,ct,it)):(j=new Date(X,et,U,nt,Z,ct,it),yt&&(j=u(j).week(yt).toDate()),j)}catch{return new Date("")}})(q,w,z,L),this.init(),l&&l!==!0&&(this.$L=this.locale(l).$L),r&&q!=this.format(w)&&(this.$d=new Date("")),C={}}else if(w instanceof Array)for(var f=w.length,c=1;c<=f;c+=1){m[1]=w[c-1];var g=L.apply(this,m);if(g.isValid()){this.$d=g.$d,this.$L=g.$L,this.init();break}c===f&&(this.$d=new Date(""))}else N.call(this,O)}}}))})(Tt)),Tt.exports}var wn=Tn();const _n=Rt(wn);var wt={exports:{}},Dn=wt.exports,ce;function Cn(){return ce||(ce=1,(function(t,e){(function(n,i){t.exports=i()})(Dn,(function(){return function(n,i){var a=i.prototype,h=a.format;a.format=function(o){var x=this,C=this.$locale();if(!this.isValid())return h.bind(this)(o);var D=this.$utils(),p=(o||"YYYY-MM-DDTHH:mm:ssZ").replace(/\[([^\]]+)]|Q|wo|ww|w|WW|W|zzz|z|gggg|GGGG|Do|X|x|k{1,2}|S/g,(function(M){switch(M){case"Q":return Math.ceil((x.$M+1)/3);case"Do":return C.ordinal(x.$D);case"gggg":return x.weekYear();case"GGGG":return x.isoWeekYear();case"wo":return C.ordinal(x.week(),"W");case"w":case"ww":return D.s(x.week(),M==="w"?1:2,"0");case"W":case"WW":return D.s(x.isoWeek(),M==="W"?1:2,"0");case"k":case"kk":return D.s(String(x.$H===0?24:x.$H),M==="k"?1:2,"0");case"X":return Math.floor(x.$d.getTime()/1e3);case"x":return x.$d.getTime();case"z":return"["+x.offsetName()+"]";case"zzz":return"["+x.offsetName("long")+"]";default:return M}}));return h.bind(this)(p)}}}))})(wt)),wt.exports}var Sn=Cn();const En=Rt(Sn);var Pt=(function(){var t=function(k,r,l,f){for(l=l||{},f=k.length;f--;l[k[f]]=r);return l},e=[6,8,10,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,30,32,33,35,37],n=[1,25],i=[1,26],a=[1,27],h=[1,28],o=[1,29],x=[1,30],C=[1,31],D=[1,9],p=[1,10],M=[1,11],_=[1,12],b=[1,13],B=[1,14],A=[1,15],T=[1,16],S=[1,18],L=[1,19],F=[1,20],N=[1,21],O=[1,22],q=[1,24],z=[1,32],m={trace:function(){},yy:{},symbols_:{error:2,start:3,gantt:4,document:5,EOF:6,line:7,SPACE:8,statement:9,NL:10,weekday:11,weekday_monday:12,weekday_tuesday:13,weekday_wednesday:14,weekday_thursday:15,weekday_friday:16,weekday_saturday:17,weekday_sunday:18,dateFormat:19,inclusiveEndDates:20,topAxis:21,axisFormat:22,tickInterval:23,excludes:24,includes:25,todayMarker:26,title:27,acc_title:28,acc_title_value:29,acc_descr:30,acc_descr_value:31,acc_descr_multiline_value:32,section:33,clickStatement:34,taskTxt:35,taskData:36,click:37,callbackname:38,callbackargs:39,href:40,clickStatementDebug:41,$accept:0,$end:1},terminals_:{2:"error",4:"gantt",6:"EOF",8:"SPACE",10:"NL",12:"weekday_monday",13:"weekday_tuesday",14:"weekday_wednesday",15:"weekday_thursday",16:"weekday_friday",17:"weekday_saturday",18:"weekday_sunday",19:"dateFormat",20:"inclusiveEndDates",21:"topAxis",22:"axisFormat",23:"tickInterval",24:"excludes",25:"includes",26:"todayMarker",27:"title",28:"acc_title",29:"acc_title_value",30:"acc_descr",31:"acc_descr_value",32:"acc_descr_multiline_value",33:"section",35:"taskTxt",36:"taskData",37:"click",38:"callbackname",39:"callbackargs",40:"href"},productions_:[0,[3,3],[5,0],[5,2],[7,2],[7,1],[7,1],[7,1],[11,1],[11,1],[11,1],[11,1],[11,1],[11,1],[11,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,2],[9,2],[9,1],[9,1],[9,1],[9,2],[34,2],[34,3],[34,3],[34,4],[34,3],[34,4],[34,2],[41,2],[41,3],[41,3],[41,4],[41,3],[41,4],[41,2]],performAction:function(r,l,f,c,g,s,P){var d=s.length-1;switch(g){case 1:return s[d-1];case 2:this.$=[];break;case 3:s[d-1].push(s[d]),this.$=s[d-1];break;case 4:case 5:this.$=s[d];break;case 6:case 7:this.$=[];break;case 8:c.setWeekday("monday");break;case 9:c.setWeekday("tuesday");break;case 10:c.setWeekday("wednesday");break;case 11:c.setWeekday("thursday");break;case 12:c.setWeekday("friday");break;case 13:c.setWeekday("saturday");break;case 14:c.setWeekday("sunday");break;case 15:c.setDateFormat(s[d].substr(11)),this.$=s[d].substr(11);break;case 16:c.enableInclusiveEndDates(),this.$=s[d].substr(18);break;case 17:c.TopAxis(),this.$=s[d].substr(8);break;case 18:c.setAxisFormat(s[d].substr(11)),this.$=s[d].substr(11);break;case 19:c.setTickInterval(s[d].substr(13)),this.$=s[d].substr(13);break;case 20:c.setExcludes(s[d].substr(9)),this.$=s[d].substr(9);break;case 21:c.setIncludes(s[d].substr(9)),this.$=s[d].substr(9);break;case 22:c.setTodayMarker(s[d].substr(12)),this.$=s[d].substr(12);break;case 24:c.setDiagramTitle(s[d].substr(6)),this.$=s[d].substr(6);break;case 25:this.$=s[d].trim(),c.setAccTitle(this.$);break;case 26:case 27:this.$=s[d].trim(),c.setAccDescription(this.$);break;case 28:c.addSection(s[d].substr(8)),this.$=s[d].substr(8);break;case 30:c.addTask(s[d-1],s[d]),this.$="task";break;case 31:this.$=s[d-1],c.setClickEvent(s[d-1],s[d],null);break;case 32:this.$=s[d-2],c.setClickEvent(s[d-2],s[d-1],s[d]);break;case 33:this.$=s[d-2],c.setClickEvent(s[d-2],s[d-1],null),c.setLink(s[d-2],s[d]);break;case 34:this.$=s[d-3],c.setClickEvent(s[d-3],s[d-2],s[d-1]),c.setLink(s[d-3],s[d]);break;case 35:this.$=s[d-2],c.setClickEvent(s[d-2],s[d],null),c.setLink(s[d-2],s[d-1]);break;case 36:this.$=s[d-3],c.setClickEvent(s[d-3],s[d-1],s[d]),c.setLink(s[d-3],s[d-2]);break;case 37:this.$=s[d-1],c.setLink(s[d-1],s[d]);break;case 38:case 44:this.$=s[d-1]+" "+s[d];break;case 39:case 40:case 42:this.$=s[d-2]+" "+s[d-1]+" "+s[d];break;case 41:case 43:this.$=s[d-3]+" "+s[d-2]+" "+s[d-1]+" "+s[d];break}},table:[{3:1,4:[1,2]},{1:[3]},t(e,[2,2],{5:3}),{6:[1,4],7:5,8:[1,6],9:7,10:[1,8],11:17,12:n,13:i,14:a,15:h,16:o,17:x,18:C,19:D,20:p,21:M,22:_,23:b,24:B,25:A,26:T,27:S,28:L,30:F,32:N,33:O,34:23,35:q,37:z},t(e,[2,7],{1:[2,1]}),t(e,[2,3]),{9:33,11:17,12:n,13:i,14:a,15:h,16:o,17:x,18:C,19:D,20:p,21:M,22:_,23:b,24:B,25:A,26:T,27:S,28:L,30:F,32:N,33:O,34:23,35:q,37:z},t(e,[2,5]),t(e,[2,6]),t(e,[2,15]),t(e,[2,16]),t(e,[2,17]),t(e,[2,18]),t(e,[2,19]),t(e,[2,20]),t(e,[2,21]),t(e,[2,22]),t(e,[2,23]),t(e,[2,24]),{29:[1,34]},{31:[1,35]},t(e,[2,27]),t(e,[2,28]),t(e,[2,29]),{36:[1,36]},t(e,[2,8]),t(e,[2,9]),t(e,[2,10]),t(e,[2,11]),t(e,[2,12]),t(e,[2,13]),t(e,[2,14]),{38:[1,37],40:[1,38]},t(e,[2,4]),t(e,[2,25]),t(e,[2,26]),t(e,[2,30]),t(e,[2,31],{39:[1,39],40:[1,40]}),t(e,[2,37],{38:[1,41]}),t(e,[2,32],{40:[1,42]}),t(e,[2,33]),t(e,[2,35],{39:[1,43]}),t(e,[2,34]),t(e,[2,36])],defaultActions:{},parseError:function(r,l){if(l.recoverable)this.trace(r);else{var f=new Error(r);throw f.hash=l,f}},parse:function(r){var l=this,f=[0],c=[],g=[null],s=[],P=this.table,d="",u=0,y=0,V=2,E=1,Y=s.slice.call(arguments,1),I=Object.create(this.lexer),W={yy:{}};for(var rt in this.yy)Object.prototype.hasOwnProperty.call(this.yy,rt)&&(W.yy[rt]=this.yy[rt]);I.setInput(r,W.yy),W.yy.lexer=I,W.yy.parser=this,typeof I.yylloc>"u"&&(I.yylloc={});var st=I.yylloc;s.push(st);var kt=I.options&&I.options.ranges;typeof W.yy.parseError=="function"?this.parseError=W.yy.parseError:this.parseError=Object.getPrototypeOf(this).parseError;function yt(){var J;return J=c.pop()||I.lex()||E,typeof J!="number"&&(J instanceof Array&&(c=J,J=c.pop()),J=l.symbols_[J]||J),J}for(var H,U,X,et,j={},nt,Z,ct,it;;){if(U=f[f.length-1],this.defaultActions[U]?X=this.defaultActions[U]:((H===null||typeof H>"u")&&(H=yt()),X=P[U]&&P[U][H]),typeof X>"u"||!X.length||!X[0]){var gt="";it=[];for(nt in P[U])this.terminals_[nt]&&nt>V&&it.push("'"+this.terminals_[nt]+"'");I.showPosition?gt="Parse error on line "+(u+1)+`:
`+I.showPosition()+`
Expecting `+it.join(", ")+", got '"+(this.terminals_[H]||H)+"'":gt="Parse error on line "+(u+1)+": Unexpected "+(H==E?"end of input":"'"+(this.terminals_[H]||H)+"'"),this.parseError(gt,{text:I.match,token:this.terminals_[H]||H,line:I.yylineno,loc:st,expected:it})}if(X[0]instanceof Array&&X.length>1)throw new Error("Parse Error: multiple actions possible at state: "+U+", token: "+H);switch(X[0]){case 1:f.push(H),g.push(I.yytext),s.push(I.yylloc),f.push(X[1]),H=null,y=I.yyleng,d=I.yytext,u=I.yylineno,st=I.yylloc;break;case 2:if(Z=this.productions_[X[1]][1],j.$=g[g.length-Z],j._$={first_line:s[s.length-(Z||1)].first_line,last_line:s[s.length-1].last_line,first_column:s[s.length-(Z||1)].first_column,last_column:s[s.length-1].last_column},kt&&(j._$.range=[s[s.length-(Z||1)].range[0],s[s.length-1].range[1]]),et=this.performAction.apply(j,[d,y,u,W.yy,X[1],g,s].concat(Y)),typeof et<"u")return et;Z&&(f=f.slice(0,-1*Z*2),g=g.slice(0,-1*Z),s=s.slice(0,-1*Z)),f.push(this.productions_[X[1]][0]),g.push(j.$),s.push(j._$),ct=P[f[f.length-2]][f[f.length-1]],f.push(ct);break;case 3:return!0}}return!0}},w=(function(){var k={EOF:1,parseError:function(l,f){if(this.yy.parser)this.yy.parser.parseError(l,f);else throw new Error(l)},setInput:function(r,l){return this.yy=l||this.yy||{},this._input=r,this._more=this._backtrack=this.done=!1,this.yylineno=this.yyleng=0,this.yytext=this.matched=this.match="",this.conditionStack=["INITIAL"],this.yylloc={first_line:1,first_column:0,last_line:1,last_column:0},this.options.ranges&&(this.yylloc.range=[0,0]),this.offset=0,this},input:function(){var r=this._input[0];this.yytext+=r,this.yyleng++,this.offset++,this.match+=r,this.matched+=r;var l=r.match(/(?:\r\n?|\n).*/g);return l?(this.yylineno++,this.yylloc.last_line++):this.yylloc.last_column++,this.options.ranges&&this.yylloc.range[1]++,this._input=this._input.slice(1),r},unput:function(r){var l=r.length,f=r.split(/(?:\r\n?|\n)/g);this._input=r+this._input,this.yytext=this.yytext.substr(0,this.yytext.length-l),this.offset-=l;var c=this.match.split(/(?:\r\n?|\n)/g);this.match=this.match.substr(0,this.match.length-1),this.matched=this.matched.substr(0,this.matched.length-1),f.length-1&&(this.yylineno-=f.length-1);var g=this.yylloc.range;return this.yylloc={first_line:this.yylloc.first_line,last_line:this.yylineno+1,first_column:this.yylloc.first_column,last_column:f?(f.length===c.length?this.yylloc.first_column:0)+c[c.length-f.length].length-f[0].length:this.yylloc.first_column-l},this.options.ranges&&(this.yylloc.range=[g[0],g[0]+this.yyleng-l]),this.yyleng=this.yytext.length,this},more:function(){return this._more=!0,this},reject:function(){if(this.options.backtrack_lexer)this._backtrack=!0;else return this.parseError("Lexical error on line "+(this.yylineno+1)+`. You can only invoke reject() in the lexer when the lexer is of the backtracking persuasion (options.backtrack_lexer = true).
`+this.showPosition(),{text:"",token:null,line:this.yylineno});return this},less:function(r){this.unput(this.match.slice(r))},pastInput:function(){var r=this.matched.substr(0,this.matched.length-this.match.length);return(r.length>20?"...":"")+r.substr(-20).replace(/\n/g,"")},upcomingInput:function(){var r=this.match;return r.length<20&&(r+=this._input.substr(0,20-r.length)),(r.substr(0,20)+(r.length>20?"...":"")).replace(/\n/g,"")},showPosition:function(){var r=this.pastInput(),l=new Array(r.length+1).join("-");return r+this.upcomingInput()+`
`+l+"^"},test_match:function(r,l){var f,c,g;if(this.options.backtrack_lexer&&(g={yylineno:this.yylineno,yylloc:{first_line:this.yylloc.first_line,last_line:this.last_line,first_column:this.yylloc.first_column,last_column:this.yylloc.last_column},yytext:this.yytext,match:this.match,matches:this.matches,matched:this.matched,yyleng:this.yyleng,offset:this.offset,_more:this._more,_input:this._input,yy:this.yy,conditionStack:this.conditionStack.slice(0),done:this.done},this.options.ranges&&(g.yylloc.range=this.yylloc.range.slice(0))),c=r[0].match(/(?:\r\n?|\n).*/g),c&&(this.yylineno+=c.length),this.yylloc={first_line:this.yylloc.last_line,last_line:this.yylineno+1,first_column:this.yylloc.last_column,last_column:c?c[c.length-1].length-c[c.length-1].match(/\r?\n?/)[0].length:this.yylloc.last_column+r[0].length},this.yytext+=r[0],this.match+=r[0],this.matches=r,this.yyleng=this.yytext.length,this.options.ranges&&(this.yylloc.range=[this.offset,this.offset+=this.yyleng]),this._more=!1,this._backtrack=!1,this._input=this._input.slice(r[0].length),this.matched+=r[0],f=this.performAction.call(this,this.yy,this,l,this.conditionStack[this.conditionStack.length-1]),this.done&&this._input&&(this.done=!1),f)return f;if(this._backtrack){for(var s in g)this[s]=g[s];return!1}return!1},next:function(){if(this.done)return this.EOF;this._input||(this.done=!0);var r,l,f,c;this._more||(this.yytext="",this.match="");for(var g=this._currentRules(),s=0;s<g.length;s++)if(f=this._input.match(this.rules[g[s]]),f&&(!l||f[0].length>l[0].length)){if(l=f,c=s,this.options.backtrack_lexer){if(r=this.test_match(f,g[s]),r!==!1)return r;if(this._backtrack){l=!1;continue}else return!1}else if(!this.options.flex)break}return l?(r=this.test_match(l,g[c]),r!==!1?r:!1):this._input===""?this.EOF:this.parseError("Lexical error on line "+(this.yylineno+1)+`. Unrecognized text.
`+this.showPosition(),{text:"",token:null,line:this.yylineno})},lex:function(){var l=this.next();return l||this.lex()},begin:function(l){this.conditionStack.push(l)},popState:function(){var l=this.conditionStack.length-1;return l>0?this.conditionStack.pop():this.conditionStack[0]},_currentRules:function(){return this.conditionStack.length&&this.conditionStack[this.conditionStack.length-1]?this.conditions[this.conditionStack[this.conditionStack.length-1]].rules:this.conditions.INITIAL.rules},topState:function(l){return l=this.conditionStack.length-1-Math.abs(l||0),l>=0?this.conditionStack[l]:"INITIAL"},pushState:function(l){this.begin(l)},stateStackSize:function(){return this.conditionStack.length},options:{"case-insensitive":!0},performAction:function(l,f,c,g){switch(c){case 0:return this.begin("open_directive"),"open_directive";case 1:return this.begin("acc_title"),28;case 2:return this.popState(),"acc_title_value";case 3:return this.begin("acc_descr"),30;case 4:return this.popState(),"acc_descr_value";case 5:this.begin("acc_descr_multiline");break;case 6:this.popState();break;case 7:return"acc_descr_multiline_value";case 8:break;case 9:break;case 10:break;case 11:return 10;case 12:break;case 13:break;case 14:this.begin("href");break;case 15:this.popState();break;case 16:return 40;case 17:this.begin("callbackname");break;case 18:this.popState();break;case 19:this.popState(),this.begin("callbackargs");break;case 20:return 38;case 21:this.popState();break;case 22:return 39;case 23:this.begin("click");break;case 24:this.popState();break;case 25:return 37;case 26:return 4;case 27:return 19;case 28:return 20;case 29:return 21;case 30:return 22;case 31:return 23;case 32:return 25;case 33:return 24;case 34:return 26;case 35:return 12;case 36:return 13;case 37:return 14;case 38:return 15;case 39:return 16;case 40:return 17;case 41:return 18;case 42:return"date";case 43:return 27;case 44:return"accDescription";case 45:return 33;case 46:return 35;case 47:return 36;case 48:return":";case 49:return 6;case 50:return"INVALID"}},rules:[/^(?:%%\{)/i,/^(?:accTitle\s*:\s*)/i,/^(?:(?!\n||)*[^\n]*)/i,/^(?:accDescr\s*:\s*)/i,/^(?:(?!\n||)*[^\n]*)/i,/^(?:accDescr\s*\{\s*)/i,/^(?:[\}])/i,/^(?:[^\}]*)/i,/^(?:%%(?!\{)*[^\n]*)/i,/^(?:[^\}]%%*[^\n]*)/i,/^(?:%%*[^\n]*[\n]*)/i,/^(?:[\n]+)/i,/^(?:\s+)/i,/^(?:%[^\n]*)/i,/^(?:href[\s]+["])/i,/^(?:["])/i,/^(?:[^"]*)/i,/^(?:call[\s]+)/i,/^(?:\([\s]*\))/i,/^(?:\()/i,/^(?:[^(]*)/i,/^(?:\))/i,/^(?:[^)]*)/i,/^(?:click[\s]+)/i,/^(?:[\s\n])/i,/^(?:[^\s\n]*)/i,/^(?:gantt\b)/i,/^(?:dateFormat\s[^#\n;]+)/i,/^(?:inclusiveEndDates\b)/i,/^(?:topAxis\b)/i,/^(?:axisFormat\s[^#\n;]+)/i,/^(?:tickInterval\s[^#\n;]+)/i,/^(?:includes\s[^#\n;]+)/i,/^(?:excludes\s[^#\n;]+)/i,/^(?:todayMarker\s[^\n;]+)/i,/^(?:weekday\s+monday\b)/i,/^(?:weekday\s+tuesday\b)/i,/^(?:weekday\s+wednesday\b)/i,/^(?:weekday\s+thursday\b)/i,/^(?:weekday\s+friday\b)/i,/^(?:weekday\s+saturday\b)/i,/^(?:weekday\s+sunday\b)/i,/^(?:\d\d\d\d-\d\d-\d\d\b)/i,/^(?:title\s[^\n]+)/i,/^(?:accDescription\s[^#\n;]+)/i,/^(?:section\s[^\n]+)/i,/^(?:[^:\n]+)/i,/^(?::[^#\n;]+)/i,/^(?::)/i,/^(?:$)/i,/^(?:.)/i],conditions:{acc_descr_multiline:{rules:[6,7],inclusive:!1},acc_descr:{rules:[4],inclusive:!1},acc_title:{rules:[2],inclusive:!1},callbackargs:{rules:[21,22],inclusive:!1},callbackname:{rules:[18,19,20],inclusive:!1},href:{rules:[15,16],inclusive:!1},click:{rules:[24,25],inclusive:!1},INITIAL:{rules:[0,1,3,5,8,9,10,11,12,13,14,17,23,26,27,28,29,30,31,32,33,34,35,36,37,38,39,40,41,42,43,44,45,46,47,48,49,50],inclusive:!0}}};return k})();m.lexer=w;function v(){this.yy={}}return v.prototype=m,m.Parser=v,new v})();Pt.parser=Pt;const Mn=Pt;G.extend(vn);G.extend(_n);G.extend(En);let Q="",Bt="",Ht,qt="",ft=[],ht=[],Xt={},Gt=[],Et=[],dt="",Zt="";const Te=["active","done","crit","milestone"];let Ut=[],mt=!1,jt=!1,Qt="sunday",zt=0;const An=function(){Gt=[],Et=[],dt="",Ut=[],_t=0,Nt=void 0,Dt=void 0,R=[],Q="",Bt="",Zt="",Ht=void 0,qt="",ft=[],ht=[],mt=!1,jt=!1,zt=0,Xt={},Ke(),Qt="sunday"},In=function(t){Bt=t},Ln=function(){return Bt},Fn=function(t){Ht=t},Yn=function(){return Ht},Wn=function(t){qt=t},Vn=function(){return qt},Pn=function(t){Q=t},zn=function(){mt=!0},On=function(){return mt},Nn=function(){jt=!0},Rn=function(){return jt},Bn=function(t){Zt=t},Hn=function(){return Zt},qn=function(){return Q},Xn=function(t){ft=t.toLowerCase().split(/[\s,]+/)},Gn=function(){return ft},Zn=function(t){ht=t.toLowerCase().split(/[\s,]+/)},Un=function(){return ht},jn=function(){return Xt},Qn=function(t){dt=t,Gt.push(t)},$n=function(){return Gt},Jn=function(){let t=oe();const e=10;let n=0;for(;!t&&n<e;)t=oe(),n++;return Et=R,Et},we=function(t,e,n,i){return i.includes(t.format(e.trim()))?!1:t.isoWeekday()>=6&&n.includes("weekends")||n.includes(t.format("dddd").toLowerCase())?!0:n.includes(t.format(e.trim()))},Kn=function(t){Qt=t},ti=function(){return Qt},_e=function(t,e,n,i){if(!n.length||t.manualEndTime)return;let a;t.startTime instanceof Date?a=G(t.startTime):a=G(t.startTime,e,!0),a=a.add(1,"d");let h;t.endTime instanceof Date?h=G(t.endTime):h=G(t.endTime,e,!0);const[o,x]=ei(a,h,e,n,i);t.endTime=o.toDate(),t.renderEndTime=x},ei=function(t,e,n,i,a){let h=!1,o=null;for(;t<=e;)h||(o=e.toDate()),h=we(t,n,i,a),h&&(e=e.add(1,"d")),t=t.add(1,"d");return[e,o]},Ot=function(t,e,n){n=n.trim();const a=/^after\s+(?<ids>[\d\w- ]+)/.exec(n);if(a!==null){let o=null;for(const C of a.groups.ids.split(" ")){let D=at(C);D!==void 0&&(!o||D.endTime>o.endTime)&&(o=D)}if(o)return o.endTime;const x=new Date;return x.setHours(0,0,0,0),x}let h=G(n,e.trim(),!0);if(h.isValid())return h.toDate();{Ct.debug("Invalid date:"+n),Ct.debug("With date format:"+e.trim());const o=new Date(n);if(o===void 0||isNaN(o.getTime())||o.getFullYear()<-1e4||o.getFullYear()>1e4)throw new Error("Invalid date:"+n);return o}},De=function(t){const e=/^(\d+(?:\.\d+)?)([Mdhmswy]|ms)$/.exec(t.trim());return e!==null?[Number.parseFloat(e[1]),e[2]]:[NaN,"ms"]},Ce=function(t,e,n,i=!1){n=n.trim();const h=/^until\s+(?<ids>[\d\w- ]+)/.exec(n);if(h!==null){let p=null;for(const _ of h.groups.ids.split(" ")){let b=at(_);b!==void 0&&(!p||b.startTime<p.startTime)&&(p=b)}if(p)return p.startTime;const M=new Date;return M.setHours(0,0,0,0),M}let o=G(n,e.trim(),!0);if(o.isValid())return i&&(o=o.add(1,"d")),o.toDate();let x=G(t);const[C,D]=De(n);if(!Number.isNaN(C)){const p=x.add(C,D);p.isValid()&&(x=p)}return x.toDate()};let _t=0;const ut=function(t){return t===void 0?(_t=_t+1,"task"+_t):t},ni=function(t,e){let n;e.substr(0,1)===":"?n=e.substr(1,e.length):n=e;const i=n.split(","),a={};Ae(i,a,Te);for(let o=0;o<i.length;o++)i[o]=i[o].trim();let h="";switch(i.length){case 1:a.id=ut(),a.startTime=t.endTime,h=i[0];break;case 2:a.id=ut(),a.startTime=Ot(void 0,Q,i[0]),h=i[1];break;case 3:a.id=ut(i[0]),a.startTime=Ot(void 0,Q,i[1]),h=i[2];break}return h&&(a.endTime=Ce(a.startTime,Q,h,mt),a.manualEndTime=G(h,"YYYY-MM-DD",!0).isValid(),_e(a,Q,ht,ft)),a},ii=function(t,e){let n;e.substr(0,1)===":"?n=e.substr(1,e.length):n=e;const i=n.split(","),a={};Ae(i,a,Te);for(let h=0;h<i.length;h++)i[h]=i[h].trim();switch(i.length){case 1:a.id=ut(),a.startTime={type:"prevTaskEnd",id:t},a.endTime={data:i[0]};break;case 2:a.id=ut(),a.startTime={type:"getStartDate",startData:i[0]},a.endTime={data:i[1]};break;case 3:a.id=ut(i[0]),a.startTime={type:"getStartDate",startData:i[1]},a.endTime={data:i[2]};break}return a};let Nt,Dt,R=[];const Se={},ri=function(t,e){const n={section:dt,type:dt,processed:!1,manualEndTime:!1,renderEndTime:null,raw:{data:e},task:t,classes:[]},i=ii(Dt,e);n.raw.startTime=i.startTime,n.raw.endTime=i.endTime,n.id=i.id,n.prevTaskId=Dt,n.active=i.active,n.done=i.done,n.crit=i.crit,n.milestone=i.milestone,n.order=zt,zt++;const a=R.push(n);Dt=n.id,Se[n.id]=a-1},at=function(t){const e=Se[t];return R[e]},si=function(t,e){const n={section:dt,type:dt,description:t,task:t,classes:[]},i=ni(Nt,e);n.startTime=i.startTime,n.endTime=i.endTime,n.id=i.id,n.active=i.active,n.done=i.done,n.crit=i.crit,n.milestone=i.milestone,Nt=n,Et.push(n)},oe=function(){const t=function(n){const i=R[n];let a="";switch(R[n].raw.startTime.type){case"prevTaskEnd":{const h=at(i.prevTaskId);i.startTime=h.endTime;break}case"getStartDate":a=Ot(void 0,Q,R[n].raw.startTime.startData),a&&(R[n].startTime=a);break}return R[n].startTime&&(R[n].endTime=Ce(R[n].startTime,Q,R[n].raw.endTime.data,mt),R[n].endTime&&(R[n].processed=!0,R[n].manualEndTime=G(R[n].raw.endTime.data,"YYYY-MM-DD",!0).isValid(),_e(R[n],Q,ht,ft))),R[n].processed};let e=!0;for(const[n,i]of R.entries())t(n),e=e&&i.processed;return e},ai=function(t,e){let n=e;ot().securityLevel!=="loose"&&(n=Je.sanitizeUrl(e)),t.split(",").forEach(function(i){at(i)!==void 0&&(Me(i,()=>{window.open(n,"_self")}),Xt[i]=n)}),Ee(t,"clickable")},Ee=function(t,e){t.split(",").forEach(function(n){let i=at(n);i!==void 0&&i.classes.push(e)})},ci=function(t,e,n){if(ot().securityLevel!=="loose"||e===void 0)return;let i=[];if(typeof n=="string"){i=n.split(/,(?=(?:(?:[^"]*"){2})*[^"]*$)/);for(let h=0;h<i.length;h++){let o=i[h].trim();o.charAt(0)==='"'&&o.charAt(o.length-1)==='"'&&(o=o.substr(1,o.length-2)),i[h]=o}}i.length===0&&i.push(t),at(t)!==void 0&&Me(t,()=>{tn.runFunc(e,...i)})},Me=function(t,e){Ut.push(function(){const n=document.querySelector(`[id="${t}"]`);n!==null&&n.addEventListener("click",function(){e()})},function(){const n=document.querySelector(`[id="${t}-text"]`);n!==null&&n.addEventListener("click",function(){e()})})},oi=function(t,e,n){t.split(",").forEach(function(i){ci(i,e,n)}),Ee(t,"clickable")},li=function(t){Ut.forEach(function(e){e(t)})},ui={getConfig:()=>ot().gantt,clear:An,setDateFormat:Pn,getDateFormat:qn,enableInclusiveEndDates:zn,endDatesAreInclusive:On,enableTopAxis:Nn,topAxisEnabled:Rn,setAxisFormat:In,getAxisFormat:Ln,setTickInterval:Fn,getTickInterval:Yn,setTodayMarker:Wn,getTodayMarker:Vn,setAccTitle:ze,getAccTitle:Pe,setDiagramTitle:Ve,getDiagramTitle:We,setDisplayMode:Bn,getDisplayMode:Hn,setAccDescription:Ye,getAccDescription:Fe,addSection:Qn,getSections:$n,getTasks:Jn,addTask:ri,findTaskById:at,addTaskOrg:si,setIncludes:Xn,getIncludes:Gn,setExcludes:Zn,getExcludes:Un,setClickEvent:oi,setLink:ai,getLinks:jn,bindFunctions:li,parseDuration:De,isInvalidDate:we,setWeekday:Kn,getWeekday:ti};function Ae(t,e,n){let i=!0;for(;i;)i=!1,n.forEach(function(a){const h="^\\s*"+a+"\\s*$",o=new RegExp(h);t[0].match(o)&&(e[a]=!0,t.shift(1),i=!0)})}const di=function(){Ct.debug("Something is calling, setConf, remove the call")},le={monday:Qe,tuesday:je,wednesday:Ue,thursday:Ze,friday:Ge,saturday:Xe,sunday:qe},fi=(t,e)=>{let n=[...t].map(()=>-1/0),i=[...t].sort((h,o)=>h.startTime-o.startTime||h.order-o.order),a=0;for(const h of i)for(let o=0;o<n.length;o++)if(h.startTime>=n[o]){n[o]=h.endTime,h.order=o+e,o>a&&(a=o);break}return a};let K;const hi=function(t,e,n,i){const a=ot().gantt,h=ot().securityLevel;let o;h==="sandbox"&&(o=pt("#i"+e));const x=h==="sandbox"?pt(o.nodes()[0].contentDocument.body):pt("body"),C=h==="sandbox"?o.nodes()[0].contentDocument:document,D=C.getElementById(e);K=D.parentElement.offsetWidth,K===void 0&&(K=1200),a.useWidth!==void 0&&(K=a.useWidth);const p=i.db.getTasks();let M=[];for(const m of p)M.push(m.type);M=z(M);const _={};let b=2*a.topPadding;if(i.db.getDisplayMode()==="compact"||a.displayMode==="compact"){const m={};for(const v of p)m[v.section]===void 0?m[v.section]=[v]:m[v.section].push(v);let w=0;for(const v of Object.keys(m)){const k=fi(m[v],w)+1;w+=k,b+=k*(a.barHeight+a.barGap),_[v]=k}}else{b+=p.length*(a.barHeight+a.barGap);for(const m of M)_[m]=p.filter(w=>w.type===m).length}D.setAttribute("viewBox","0 0 "+K+" "+b);const B=x.select(`[id="${e}"]`),A=Oe().domain([Ne(p,function(m){return m.startTime}),Re(p,function(m){return m.endTime})]).rangeRound([0,K-a.leftPadding-a.rightPadding]);function T(m,w){const v=m.startTime,k=w.startTime;let r=0;return v>k?r=1:v<k&&(r=-1),r}p.sort(T),S(p,K,b),Be(B,b,K,a.useMaxWidth),B.append("text").text(i.db.getDiagramTitle()).attr("x",K/2).attr("y",a.titleTopMargin).attr("class","titleText");function S(m,w,v){const k=a.barHeight,r=k+a.barGap,l=a.topPadding,f=a.leftPadding,c=He().domain([0,M.length]).range(["#00B9FA","#F95002"]).interpolate(yn);F(r,l,f,w,v,m,i.db.getExcludes(),i.db.getIncludes()),N(f,l,w,v),L(m,r,l,f,k,c,w),O(r,l),q(f,l,w,v)}function L(m,w,v,k,r,l,f){const g=[...new Set(m.map(u=>u.order))].map(u=>m.find(y=>y.order===u));B.append("g").selectAll("rect").data(g).enter().append("rect").attr("x",0).attr("y",function(u,y){return y=u.order,y*w+v-2}).attr("width",function(){return f-a.rightPadding/2}).attr("height",w).attr("class",function(u){for(const[y,V]of M.entries())if(u.type===V)return"section section"+y%a.numberSectionStyles;return"section section0"});const s=B.append("g").selectAll("rect").data(m).enter(),P=i.db.getLinks();if(s.append("rect").attr("id",function(u){return u.id}).attr("rx",3).attr("ry",3).attr("x",function(u){return u.milestone?A(u.startTime)+k+.5*(A(u.endTime)-A(u.startTime))-.5*r:A(u.startTime)+k}).attr("y",function(u,y){return y=u.order,y*w+v}).attr("width",function(u){return u.milestone?r:A(u.renderEndTime||u.endTime)-A(u.startTime)}).attr("height",r).attr("transform-origin",function(u,y){return y=u.order,(A(u.startTime)+k+.5*(A(u.endTime)-A(u.startTime))).toString()+"px "+(y*w+v+.5*r).toString()+"px"}).attr("class",function(u){const y="task";let V="";u.classes.length>0&&(V=u.classes.join(" "));let E=0;for(const[I,W]of M.entries())u.type===W&&(E=I%a.numberSectionStyles);let Y="";return u.active?u.crit?Y+=" activeCrit":Y=" active":u.done?u.crit?Y=" doneCrit":Y=" done":u.crit&&(Y+=" crit"),Y.length===0&&(Y=" task"),u.milestone&&(Y=" milestone "+Y),Y+=E,Y+=" "+V,y+Y}),s.append("text").attr("id",function(u){return u.id+"-text"}).text(function(u){return u.task}).attr("font-size",a.fontSize).attr("x",function(u){let y=A(u.startTime),V=A(u.renderEndTime||u.endTime);u.milestone&&(y+=.5*(A(u.endTime)-A(u.startTime))-.5*r),u.milestone&&(V=y+r);const E=this.getBBox().width;return E>V-y?V+E+1.5*a.leftPadding>f?y+k-5:V+k+5:(V-y)/2+y+k}).attr("y",function(u,y){return y=u.order,y*w+a.barHeight/2+(a.fontSize/2-2)+v}).attr("text-height",r).attr("class",function(u){const y=A(u.startTime);let V=A(u.endTime);u.milestone&&(V=y+r);const E=this.getBBox().width;let Y="";u.classes.length>0&&(Y=u.classes.join(" "));let I=0;for(const[rt,st]of M.entries())u.type===st&&(I=rt%a.numberSectionStyles);let W="";return u.active&&(u.crit?W="activeCritText"+I:W="activeText"+I),u.done?u.crit?W=W+" doneCritText"+I:W=W+" doneText"+I:u.crit&&(W=W+" critText"+I),u.milestone&&(W+=" milestoneText"),E>V-y?V+E+1.5*a.leftPadding>f?Y+" taskTextOutsideLeft taskTextOutside"+I+" "+W:Y+" taskTextOutsideRight taskTextOutside"+I+" "+W+" width-"+E:Y+" taskText taskText"+I+" "+W+" width-"+E}),ot().securityLevel==="sandbox"){let u;u=pt("#i"+e);const y=u.nodes()[0].contentDocument;s.filter(function(V){return P[V.id]!==void 0}).each(function(V){var E=y.querySelector("#"+V.id),Y=y.querySelector("#"+V.id+"-text");const I=E.parentNode;var W=y.createElement("a");W.setAttribute("xlink:href",P[V.id]),W.setAttribute("target","_top"),I.appendChild(W),W.appendChild(E),W.appendChild(Y)})}}function F(m,w,v,k,r,l,f,c){if(f.length===0&&c.length===0)return;let g,s;for(const{startTime:E,endTime:Y}of l)(g===void 0||E<g)&&(g=E),(s===void 0||Y>s)&&(s=Y);if(!g||!s)return;if(G(s).diff(G(g),"year")>5){Ct.warn("The difference between the min and max time is more than 5 years. This will cause performance issues. Skipping drawing exclude days.");return}const P=i.db.getDateFormat(),d=[];let u=null,y=G(g);for(;y.valueOf()<=s;)i.db.isInvalidDate(y,P,f,c)?u?u.end=y:u={start:y,end:y}:u&&(d.push(u),u=null),y=y.add(1,"d");B.append("g").selectAll("rect").data(d).enter().append("rect").attr("id",function(E){return"exclude-"+E.start.format("YYYY-MM-DD")}).attr("x",function(E){return A(E.start)+v}).attr("y",a.gridLineStartPadding).attr("width",function(E){const Y=E.end.add(1,"day");return A(Y)-A(E.start)}).attr("height",r-w-a.gridLineStartPadding).attr("transform-origin",function(E,Y){return(A(E.start)+v+.5*(A(E.end)-A(E.start))).toString()+"px "+(Y*m+.5*r).toString()+"px"}).attr("class","exclude-range")}function N(m,w,v,k){let r=ln(A).tickSize(-k+w+a.gridLineStartPadding).tickFormat($t(i.db.getAxisFormat()||a.axisFormat||"%Y-%m-%d"));const f=/^([1-9]\d*)(millisecond|second|minute|hour|day|week|month)$/.exec(i.db.getTickInterval()||a.tickInterval);if(f!==null){const c=f[1],g=f[2],s=i.db.getWeekday()||a.weekday;switch(g){case"millisecond":r.ticks(ie.every(c));break;case"second":r.ticks(ne.every(c));break;case"minute":r.ticks(ee.every(c));break;case"hour":r.ticks(te.every(c));break;case"day":r.ticks(Kt.every(c));break;case"week":r.ticks(le[s].every(c));break;case"month":r.ticks(Jt.every(c));break}}if(B.append("g").attr("class","grid").attr("transform","translate("+m+", "+(k-50)+")").call(r).selectAll("text").style("text-anchor","middle").attr("fill","#000").attr("stroke","none").attr("font-size",10).attr("dy","1em"),i.db.topAxisEnabled()||a.topAxis){let c=on(A).tickSize(-k+w+a.gridLineStartPadding).tickFormat($t(i.db.getAxisFormat()||a.axisFormat||"%Y-%m-%d"));if(f!==null){const g=f[1],s=f[2],P=i.db.getWeekday()||a.weekday;switch(s){case"millisecond":c.ticks(ie.every(g));break;case"second":c.ticks(ne.every(g));break;case"minute":c.ticks(ee.every(g));break;case"hour":c.ticks(te.every(g));break;case"day":c.ticks(Kt.every(g));break;case"week":c.ticks(le[P].every(g));break;case"month":c.ticks(Jt.every(g));break}}B.append("g").attr("class","grid").attr("transform","translate("+m+", "+w+")").call(c).selectAll("text").style("text-anchor","middle").attr("fill","#000").attr("stroke","none").attr("font-size",10)}}function O(m,w){let v=0;const k=Object.keys(_).map(r=>[r,_[r]]);B.append("g").selectAll("text").data(k).enter().append(function(r){const l=r[0].split($e.lineBreakRegex),f=-(l.length-1)/2,c=C.createElementNS("http://www.w3.org/2000/svg","text");c.setAttribute("dy",f+"em");for(const[g,s]of l.entries()){const P=C.createElementNS("http://www.w3.org/2000/svg","tspan");P.setAttribute("alignment-baseline","central"),P.setAttribute("x","10"),g>0&&P.setAttribute("dy","1em"),P.textContent=s,c.appendChild(P)}return c}).attr("x",10).attr("y",function(r,l){if(l>0)for(let f=0;f<l;f++)return v+=k[l-1][1],r[1]*m/2+v*m+w;else return r[1]*m/2+w}).attr("font-size",a.sectionFontSize).attr("class",function(r){for(const[l,f]of M.entries())if(r[0]===f)return"sectionTitle sectionTitle"+l%a.numberSectionStyles;return"sectionTitle"})}function q(m,w,v,k){const r=i.db.getTodayMarker();if(r==="off")return;const l=B.append("g").attr("class","today"),f=new Date,c=l.append("line");c.attr("x1",A(f)+m).attr("x2",A(f)+m).attr("y1",a.titleTopMargin).attr("y2",k-a.titleTopMargin).attr("class","today"),r!==""&&c.attr("style",r.replace(/,/g,";"))}function z(m){const w={},v=[];for(let k=0,r=m.length;k<r;++k)Object.prototype.hasOwnProperty.call(w,m[k])||(w[m[k]]=!0,v.push(m[k]));return v}},mi={setConf:di,draw:hi},ki=t=>`
  .mermaid-main-font {
    font-family: var(--mermaid-font-family, "trebuchet ms", verdana, arial, sans-serif);
  }

  .exclude-range {
    fill: ${t.excludeBkgColor};
  }

  .section {
    stroke: none;
    opacity: 0.2;
  }

  .section0 {
    fill: ${t.sectionBkgColor};
  }

  .section2 {
    fill: ${t.sectionBkgColor2};
  }

  .section1,
  .section3 {
    fill: ${t.altSectionBkgColor};
    opacity: 0.2;
  }

  .sectionTitle0 {
    fill: ${t.titleColor};
  }

  .sectionTitle1 {
    fill: ${t.titleColor};
  }

  .sectionTitle2 {
    fill: ${t.titleColor};
  }

  .sectionTitle3 {
    fill: ${t.titleColor};
  }

  .sectionTitle {
    text-anchor: start;
    font-family: var(--mermaid-font-family, "trebuchet ms", verdana, arial, sans-serif);
  }


  /* Grid and axis */

  .grid .tick {
    stroke: ${t.gridColor};
    opacity: 0.8;
    shape-rendering: crispEdges;
  }

  .grid .tick text {
    font-family: ${t.fontFamily};
    fill: ${t.textColor};
  }

  .grid path {
    stroke-width: 0;
  }


  /* Today line */

  .today {
    fill: none;
    stroke: ${t.todayLineColor};
    stroke-width: 2px;
  }


  /* Task styling */

  /* Default task */

  .task {
    stroke-width: 2;
  }

  .taskText {
    text-anchor: middle;
    font-family: var(--mermaid-font-family, "trebuchet ms", verdana, arial, sans-serif);
  }

  .taskTextOutsideRight {
    fill: ${t.taskTextDarkColor};
    text-anchor: start;
    font-family: var(--mermaid-font-family, "trebuchet ms", verdana, arial, sans-serif);
  }

  .taskTextOutsideLeft {
    fill: ${t.taskTextDarkColor};
    text-anchor: end;
  }


  /* Special case clickable */

  .task.clickable {
    cursor: pointer;
  }

  .taskText.clickable {
    cursor: pointer;
    fill: ${t.taskTextClickableColor} !important;
    font-weight: bold;
  }

  .taskTextOutsideLeft.clickable {
    cursor: pointer;
    fill: ${t.taskTextClickableColor} !important;
    font-weight: bold;
  }

  .taskTextOutsideRight.clickable {
    cursor: pointer;
    fill: ${t.taskTextClickableColor} !important;
    font-weight: bold;
  }


  /* Specific task settings for the sections*/

  .taskText0,
  .taskText1,
  .taskText2,
  .taskText3 {
    fill: ${t.taskTextColor};
  }

  .task0,
  .task1,
  .task2,
  .task3 {
    fill: ${t.taskBkgColor};
    stroke: ${t.taskBorderColor};
  }

  .taskTextOutside0,
  .taskTextOutside2
  {
    fill: ${t.taskTextOutsideColor};
  }

  .taskTextOutside1,
  .taskTextOutside3 {
    fill: ${t.taskTextOutsideColor};
  }


  /* Active task */

  .active0,
  .active1,
  .active2,
  .active3 {
    fill: ${t.activeTaskBkgColor};
    stroke: ${t.activeTaskBorderColor};
  }

  .activeText0,
  .activeText1,
  .activeText2,
  .activeText3 {
    fill: ${t.taskTextDarkColor} !important;
  }


  /* Completed task */

  .done0,
  .done1,
  .done2,
  .done3 {
    stroke: ${t.doneTaskBorderColor};
    fill: ${t.doneTaskBkgColor};
    stroke-width: 2;
  }

  .doneText0,
  .doneText1,
  .doneText2,
  .doneText3 {
    fill: ${t.taskTextDarkColor} !important;
  }


  /* Tasks on the critical line */

  .crit0,
  .crit1,
  .crit2,
  .crit3 {
    stroke: ${t.critBorderColor};
    fill: ${t.critBkgColor};
    stroke-width: 2;
  }

  .activeCrit0,
  .activeCrit1,
  .activeCrit2,
  .activeCrit3 {
    stroke: ${t.critBorderColor};
    fill: ${t.activeTaskBkgColor};
    stroke-width: 2;
  }

  .doneCrit0,
  .doneCrit1,
  .doneCrit2,
  .doneCrit3 {
    stroke: ${t.critBorderColor};
    fill: ${t.doneTaskBkgColor};
    stroke-width: 2;
    cursor: pointer;
    shape-rendering: crispEdges;
  }

  .milestone {
    transform: rotate(45deg) scale(0.8,0.8);
  }

  .milestoneText {
    font-style: italic;
  }
  .doneCritText0,
  .doneCritText1,
  .doneCritText2,
  .doneCritText3 {
    fill: ${t.taskTextDarkColor} !important;
  }

  .activeCritText0,
  .activeCritText1,
  .activeCritText2,
  .activeCritText3 {
    fill: ${t.taskTextDarkColor} !important;
  }

  .titleText {
    text-anchor: middle;
    font-size: 18px;
    fill: ${t.titleColor||t.textColor};
    font-family: var(--mermaid-font-family, "trebuchet ms", verdana, arial, sans-serif);
  }
`,yi=ki,pi={parser:Mn,db:ui,renderer:mi,styles:yi};export{pi as diagram};
