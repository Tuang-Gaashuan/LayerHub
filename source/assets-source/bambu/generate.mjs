import fs from 'node:fs';
import {amsLiteCorrected,liteTubePaths} from './ams-lite.mjs';
import {amsCorrected,rearTube} from './ams-direction.mjs';
import path from 'node:path';


import sharp from 'sharp';
const ROOT=path.dirname(new URL(import.meta.url).pathname.replace(/^\/(\w:)/,'$1'));
fs.mkdirSync(path.join(ROOT,'assets'),{recursive:true});
fs.mkdirSync(path.join(ROOT,'boards'),{recursive:true});

const sources={
 a2l:'https://blog.bambulab.com/bambu-lab-a2l-creative-playground-extra-large/',
 x2d:'https://blog.bambulab.com/xcellence-made-simple-bambu-lab-presents-the-x2d/',
 p2s:'https://blog.bambulab.com/the-icon-redefined-meet-the-p2s-a-completely-reengineered-version-of-the-ultra-productive-p1-series/',
 h2c:'https://blog.bambulab.com/bambu-lab-h2c-where-multi-material-vortek-system-meets-engineering-precision/',
 h2dpro:'https://blog.bambulab.com/bambu-lab-launches-h2d-pro-for-enterprise-manufacturing/',
 h2d:'https://blog.bambulab.com/bambu-lab-launches-h2d-to-rethink-personal-manufacturing/',
 p1:'https://bambulab.com/en-us/p1p',
 x1eol:'https://blog.bambulab.com/the-x1-series-is-eol-the-standard-it-set-will-remain-forever/',
 p1peol:'https://blog.bambulab.com/a-farewell-to-p1p/'
};
const models=[
 {id:'a1-mini',name:'A1 mini',series:'A',kind:'mini',ams:'AMS lite',subtitle:'单悬臂 · 紧凑开放式',status:'current'},
 {id:'a1',name:'A1',series:'A',kind:'a1',ams:'AMS lite',subtitle:'双立柱 · 开放式',status:'current'},
 {id:'a2l',name:'A2L',series:'A',kind:'a2l',ams:'AMS lite',subtitle:'大幅面 · 开放式',status:'current',source:sources.a2l},
 {id:'p1p',name:'P1P',series:'P',kind:'p1p',ams:'AMS',subtitle:'开放式 CoreXY 框架',status:'legacy',source:sources.p1peol},
 {id:'p1s',name:'P1S',series:'P',kind:'p1s',ams:'AMS',subtitle:'深色封闭机箱 · 小型按键屏',status:'current',source:sources.p1},
 {id:'p2s',name:'P2S',series:'P',kind:'p2s',ams:'AMS 2 Pro',subtitle:'封闭机箱 · 大触控屏',status:'current',source:sources.p2s},
 {id:'x1',name:'X1',series:'X',kind:'x1',ams:'AMS',subtitle:'浅色机身 · 触控屏',status:'legacy',source:sources.x1eol},
 {id:'x1-carbon',name:'X1 Carbon',series:'X',kind:'x1c',ams:'AMS',subtitle:'银灰机身 · 深色顶盖',status:'legacy',source:sources.x1eol},
 {id:'x1e',name:'X1E',series:'X',kind:'x1e',ams:'AMS',subtitle:'企业机型 · 银灰外壳',status:'legacy',source:sources.x1eol},
 {id:'x2d',name:'X2D',series:'X',kind:'x2d',ams:'AMS 2 Pro',subtitle:'新一代 X 系列 · 双喷嘴',status:'current',source:sources.x2d},
 {id:'h2s',name:'H2S',series:'H',kind:'h2s',ams:'AMS 2 Pro',subtitle:'宽体封闭机箱 · 单喷嘴',status:'current'},
 {id:'h2d',name:'H2D',series:'H',kind:'h2d',ams:'AMS 2 Pro',subtitle:'宽体封闭机箱 · 双喷嘴',status:'current',source:sources.h2d},
 {id:'h2d-pro',name:'H2D Pro',series:'H',kind:'h2dpro',ams:'AMS 2 Pro + AMS HT',subtitle:'企业机型 · 双喷嘴',status:'current',source:sources.h2dpro},
 {id:'h2c',name:'H2C',series:'H',kind:'h2c',ams:'AMS 2 Pro',subtitle:'Vortek 换热端系统',status:'current',source:sources.h2c}
];
const esc=s=>String(s).replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('"','&quot;');
const n=v=>Number(v.toFixed(2));
const colors=['#b4c9b8','#d7b899','#b7b5cf','#dce0d5'];

class Art {
 constructor(id,{cx=220,cy=347,scale=1.47}={}){this.id=id;this.cx=cx;this.cy=cy;this.scale=scale;this.a=[];this.defs=[];}
 p(x,y,z){return [n(this.cx+this.scale*(x-.53*y)),n(this.cy+this.scale*(-.185*x-.3*y-z))]}
 poly(points,fill,stroke='none',sw=.45,opacity=1){this.a.push(`<polygon points="${points.map(p=>this.p(...p).join(',')).join(' ')}" fill="${fill}" stroke="${stroke}" stroke-width="${sw}" opacity="${opacity}" stroke-linejoin="round"/>`)}
 line(points,stroke='#7b898a',sw=.8,opacity=1){this.a.push(`<polyline points="${points.map(p=>this.p(...p).join(',')).join(' ')}" fill="none" stroke="${stroke}" stroke-width="${sw}" opacity="${opacity}" stroke-linejoin="round" stroke-linecap="round"/>`)}
 curve(points,stroke='#c3ccca',sw=1.5,opacity=1){const q=points.map(p=>this.p(...p));this.a.push(`<path d="M${q[0]} C${q.slice(1).map(p=>p.join(',')).join(' ')}" fill="none" stroke="${stroke}" stroke-width="${sw}" opacity="${opacity}" stroke-linecap="round"/>`)}
 box(x,y,z,w,d,h,front='#c8d0cd',side='#a7b4b2',top='#e1e6e1',outline='#6f80801a'){
  this.poly([[x,y+d,z],[x,y,z],[x,y,z+h],[x,y+d,z+h]],side,outline);
  this.poly([[x,y,z+h],[x+w,y,z+h],[x+w,y+d,z+h],[x,y+d,z+h]],top,outline);
  this.poly([[x,y,z],[x+w,y,z],[x+w,y,z+h],[x,y,z+h]],front,outline);
 }
 front(x,y,z,w,h,fill,stroke='none',opacity=1){this.poly([[x,y,z],[x+w,y,z],[x+w,y,z+h],[x,y,z+h]],fill,stroke,.55,opacity)}
 top(x,y,z,w,d,fill,opacity=1){this.poly([[x,y,z],[x+w,y,z],[x+w,y+d,z],[x,y+d,z]],fill,'none',.3,opacity)}
 disk(x,y,z,r,fill,stroke='none',sw=.4,opacity=1){const pts=Array.from({length:48},(_,i)=>{let a=i*Math.PI/24;return [x+r*Math.cos(a),y,z+r*Math.sin(a)]});this.poly(pts,fill,stroke,sw,opacity)}
 spool(x,y,z,r,c,depth=5){this.disk(x,y+depth,z,r,'#697c79');this.box(x-r*.5,y,z-r*.35,r,r*.3,r*.7,c,c,c,'none');this.disk(x,y,z,r,'#e2e7de','#a7b3ab',.5);this.disk(x,y-.2,z,r*.77,c);this.disk(x,y-.4,z,r*.59,'none','#ffffff40',.8);this.disk(x,y-.5,z,r*.3,'#dbe1d7');this.disk(x,y-.7,z,r*.12,'#657974');for(let i=0;i<6;i++){const a=i*Math.PI/3;this.line([[x+Math.cos(a)*r*.38,y-.7,z+Math.sin(a)*r*.38],[x+Math.cos(a)*r*.66,y-.7,z+Math.sin(a)*r*.66]],'#e0e5dc',.65,.7)}}
 label(text,x,y,z,size=3.6,color='#c3cdc7',weight=500){const p=this.p(x,y,z);this.a.push(`<text x="0" y="0" transform="matrix(${this.scale},${-.185*this.scale},0,${this.scale},${p[0]},${p[1]})" font-family="Arial,sans-serif" font-size="${size}" letter-spacing=".15" font-weight="${weight}" fill="${color}">${esc(text)}</text>`)}
 screen(x,y,z,w,h,modern=true){this.box(x,y,z,w,3,h,'#596664','#465653','#85948d');this.front(x+1.6,y-.2,z+1.7,w-3.2,h-3.4,'#192b2e');this.front(x+3,y-.3,z+h-5,w-6,1.2,modern?'#94b8ae':'#bac7a0');this.front(x+3,y-.3,z+4,w*.4,modern?4:1,'#7a9a94');if(modern){this.front(x+w*.6,y-.3,z+4,w*.22,4,'#bed1b7');this.line([[x+3,y-.4,z+h*.5],[x+w-3,y-.4,z+h*.5]],'#455f5d',.65)}else{this.disk(x+w-3,y-.4,z+3,1,'#bcc8bd')}}
 shadow(cx=55,cy=38,rx=90){const p=this.p(cx,cy,0);this.a.push(`<ellipse cx="${p[0]}" cy="${p[1]+7}" rx="${rx*this.scale}" ry="${10*this.scale}" fill="#ccd7d3" opacity=".35" filter="url(#${this.id}-shadow)"/>`)}
 end(){const svg=`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 600 400" width="1200" height="800" role="img" aria-label="${esc(this.id)}"><defs><filter id="${this.id}-shadow" x="-40%" y="-150%" width="180%" height="400%"><feGaussianBlur stdDeviation="4"/></filter><linearGradient id="${this.id}-glass" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#516662"/><stop offset=".45" stop-color="#223735"/><stop offset="1" stop-color="#3c514d"/></linearGradient>${this.defs.join('')}</defs>${this.a.join('')}</svg>`;return svg.replace(/#[0-9a-f]{8}|#[0-9a-f]{6}/gi,c=>{if(colors.includes(c)||['#c9a85f','#dab753'].includes(c))return c;const v=[1,3,5].map(i=>parseInt(c.slice(i,i+2),16)),l=v[0]*.2126+v[1]*.7152+v[2]*.0722;const rgb=[l-2,l+1,l+2].map(x=>Math.max(0,Math.min(255,Math.round(x))).toString(16).padStart(2,'0')).join('');return '#'+rgb+c.slice(7)})}
}

function plate(a,x,y,z,w,d){a.box(x,y,z,w,d,2,'#89928a','#6e7d76','#a2aca0');a.top(x+2,y+2,z+2.05,w-4,d-4,'#6e7c70');for(let k=6;k<w;k+=8)a.line([[x+k,y+3,z+2.1],[x+k,y+d-3,z+2.1]],'#bbc4b4',.35,.15)}
function printedPart(a,x,y,z){a.box(x,y,z,18,17,13,'#c9d6c6','#a6b8ab','#e2e8d9');a.box(x+4,y+4,z+13,10,9,11,'#c9d6c6','#a6b8ab','#e2e8d9');for(let i=2;i<13;i+=2)a.line([[x,y-.2,z+i],[x+18,y-.2,z+i]],'#f1f4ec',.4,.5)}
function head(a,x,y,z,{dual=false,white=false}={}){a.box(x,y,z,dual?22:18,13,20,white?'#e0e5dd':'#8c9d93',white?'#a5b3ad':'#576e66',white?'#f2f3e9':'#a7b7a8');a.front(x+3,y-.1,z+5,dual?16:12,12,'#3d5350');a.disk(x+(dual?11:9),y-.2,z+11,4,'#273d3b','#718b7e');a.front(x+6,y-.4,z+3,7,1.5,'#d1a870');a.box(x+6,y+3,z-4,4,4,4,'#bcad89','#8e937c','#d2c6a1');if(dual)a.box(x+15,y+3,z-4,4,4,4,'#b9bda3','#8e9b83','#d8ddbf')}
function feet(a,w,d){for(const [x,y] of [[5,5],[w-13,5],[5,d-12],[w-13,d-12]])a.box(x,y,0,8,7,4,'#536d66','#465c57','#6b8276')}
function amsBox(...args){return amsCorrected(...args)}
function amsHT(a,x,y,z){a.shadow(x+19,y+20,31);a.box(x,y,z,39,48,49,'#9aa99a','#6d8476','#bdc7b3');a.front(x+4,y-.3,z+9,31,33,'#39574b');a.spool(x+19.5,y-.5,z+26,13,'#c0c9b7',4);a.front(x+3,y-1,z+7,33,37,'#698373','none',.24);a.screen(x+25,y-1,z+1,11,6);a.label('AMS HT',x+4,y-.4,z+2,3,'#e4e9d9')}
function amsLite(...args){return amsLiteCorrected(...args)}

function openPrinter(a,m,combo){const mini=m.kind==='mini',large=m.kind==='a2l';const w=mini?92:large?139:119,d=mini?83:large?112:94,h=mini?105:large?143:132;const y=large?49:mini?39:45;a.shadow(w/2,d/2,90);feet(a,w,d);a.box(0,0,4,w,d,10,'#d5ddd4','#b7c6bc','#edf0e5');a.box(6,7,14,w-12,d-16,4,'#ccd8cb','#a6bbae','#dfe7d9');
 // Rear structure and Z guides.
 if(!mini){a.box(w-16,y,14,10,15,h-14,'#d6e0d5','#a6bcb0','#f2f3e6');a.front(w-13,y-.4,22,2,h-31,'#778f86');a.line([[w-10,y-.5,24],[w-10,y-.5,h-6]],'#e9ece0',1.3);if(large){a.line([[w-9,y+9,h-8],[w-9,d-5,15]],'#879e96',5);a.line([[7,y+9,h-8],[7,d-5,15]],'#879e96',5)}}
 a.box(7,y,14,mini?16:10,15,h-14,'#d3ddd2','#9fb8ac','#eef2e6');a.front(10,y-.5,22,2,h-31,'#859f92');a.line([[14,y-.7,23],[14,y-.7,h-7]],'#f1f3e9',1.5);
 if(!mini){a.box(7,y,h-5,w-14,15,9,'#d5dfd1','#b6caba','#eff2e5');a.label(large?'BAMBU LAB A2L':'BAMBU LAB',19,y-.3,h-2,2.8,'#839c8c')}
 // Moving print bed in front of the gantry.
 a.box(19,5,18,w-38,d-22,7,'#aabeb1','#839d90','#c4d5c4');plate(a,12,0,25,w-24,d-20);printedPart(a,w*.48,22,29);
 const railZ=h*.62;a.box(9,y-8,railZ,w-19,11,8,large?'#c2d0c4':'#cbd8ca','#a6beac','#e8eddd');a.line([[16,y-8.3,railZ+4],[w-12,y-8.3,railZ+4]],'#819e8d',2.1);a.line([[16,y-8.5,railZ+3],[w-12,y-8.5,railZ+3]],'#e3eadb',.6);head(a,w*.49,y-20,railZ-15,{white:true});
 // Cable loop follows the print head, not the external feeder.
 a.curve([[w*.59,y,railZ+9],[w*.74,y+3,h+15],[w-8,y+11,h+13],[w-6,y+10,h-7]],'#9aac9e',2.7);
 a.screen(w-25,-6,8,24,16,true);a.label(mini?'A1 mini':large?'A2L':'A1',7,-.2,7,3.8,'#859e8b',600);
 if(large){for(let i=0;i<4;i++){a.box(0,11+i*12,14,5,8,4,'#8da598','#728e7d','#b3c9b4');a.box(w-5,11+i*12,14,5,8,4,'#8da598','#728e7d','#b3c9b4')}}
 if(combo){const rackX=w+41,rackY=large?42:26;liteTubePaths(a,rackX,rackY,h,w,y,railZ);amsLite(a,rackX,rackY)}else{a.box(w+8,y+14,8,6,9,h*.75,'#a5bcae','#8ca898','#c4d4c4');a.spool(w+10,y+11,h*.72,18,'#d9d7c3',7);a.curve([[w+10,y+12,h*.75],[w+5,y+34,h+24],[w*.6,y+20,h+23],[w*.58,y-3,railZ+9]],'#a9c0ae',1.1)}
}

function coreFrame(a,m,combo){const w=112,d=90,h=132;a.shadow(w/2,d/2,92);feet(a,w,d);a.box(0,0,4,w,d,17,'#c7d1c8','#8ca199','#d9e1d5');a.box(0,0,21,7,d,h-21,'#647e74','#49665c','#96ae9a');a.box(w-7,0,21,7,d,h-21,'#647e74','#49665c','#96ae9a');a.box(0,d-7,21,7,7,h-21,'#49695d','#3c5a50','#829f87');a.box(w-7,d-7,21,7,7,h-21,'#49695d','#3c5a50','#829f87');a.box(0,0,h-8,w,d,8,'#6d877a','#49665a','#9eb49d');a.top(8,8,h+.2,w-16,d-16,'#304c42');a.top(12,13,h+.4,w-24,d-27,'#718b76');a.box(7,d-12,60,w-14,5,60,'#536f5f','#3f604f','#8caa8d');
 for(const x of [12,w-14])a.line([[x,12,25],[x,12,h-11]],'#cbd7bc',1.6);plate(a,12,6,38,w-24,d-19);printedPart(a,45,31,42);a.box(9,28,h-34,w-18,6,5,'#a2b4a1','#819d85','#c8d6ba');head(a,53,21,h-49);a.box(0,-1,11,w,2,13,'#dbe1d5','#aabfac','#eceddf');a.label('BAMBU LAB',26,-1.3,15,4,'#78917d');a.screen(w-23,-5,h-12,24,16,false);
 if(combo){rearTube(a,4,10,h+3);amsBox(a,4,10,h+3,'AMS')}
}

function enclosed(a,m,combo){const hseries=m.series==='H',xold=['x1','x1c','x1e'].includes(m.kind),p1=m.kind==='p1s',h=hseries?145:132,w=hseries?139:112,d=hseries?103:90;const light=xold,white=m.kind==='x1';a.shadow(w/2,d/2,hseries?110:95);feet(a,w,d);
 const c=light?(white?['#dfe5dd','#bccbc3','#f0f1e7']:['#bdc9c4','#9bafa6','#d9e1d7']):(hseries?['#61786d','#4a655b','#81988a']:['#657c70','#4e695d','#8ba18e']);
 a.box(0,0,4,w,d,h-4,c[0],c[1],c[2]);
 // Side panels, vertical metal seams and ventilation slots.
 a.poly([[0,8,19],[0,d-8,19],[0,d-8,h-14],[0,8,h-14]],light?'#b1c1b7':'#506b5f');a.line([[0,10,20],[0,10,h-13]],light?'#dce5d6':'#829b87',.8,.7);a.line([[0,d-11,20],[0,d-11,h-13]],'#344f45',.7,.4);
 if(hseries||m.kind==='x1e'||m.kind==='p2s')for(let i=0;i<10;i++)a.line([[0,d-29,26+i*2.7],[0,d-13,26+i*2.7]],light?'#839e8e':'#344f44',1.05,.9);
 // Glass top and its thin raised frame.
 a.top(4,5,h+.2,w-8,d-10,'#344e43');a.top(8,9,h+.4,w-16,d-18,'#91a28c',.55);a.line([[9,11,h+.6],[w-11,11,h+.6],[w-11,d-11,h+.6]],'#d7dfc7',.65,.55);
 const border=hseries?9:7;const doorW=hseries?w-31:w-2*border;const z0=17,zTop=h-14;
 a.front(border,-.3,z0,doorW,zTop-z0,'#28463b');a.front(border+2,-.4,z0+3,doorW-4,zTop-z0-6,`url(#${a.id}-glass)`);
 // Interior rear wall and Z rails, deliberately simplified for small cards.
 a.front(border+5,-.6,z0+11,doorW-10,zTop-z0-19,'#2d493e');for(const x of [border+10,border+doorW-12]){a.front(x,-.8,z0+13,2,zTop-z0-22,'#809c82');a.line([[x+1,-1,z0+13],[x+1,-1,zTop-10]],'#c8d7b5',.6,.5)}
 const bedZ=hseries?48:40; // project interior details in the glass plane for legibility
 a.poly([[border+8,-1,bedZ],[border+doorW-8,-1,bedZ],[border+doorW-20,-1,bedZ+9],[border+16,-1,bedZ+9]],'#9ba98b');a.line([[border+10,-1.1,bedZ+1],[border+doorW-10,-1.1,bedZ+1]],'#d4ddbc',.8);a.front(w*.42,-1.2,bedZ+3,18,15,'#b2c2a3');a.front(w*.46,-1.3,bedZ+18,10,10,'#c4d1b2');
 a.front(border+8,-1.3,h-45,doorW-16,3,'#92a88c');const dual=['x2d','h2d','h2dpro','h2c'].includes(m.kind);head(a,w*.48,-8,h-63,{dual});
 if(m.kind==='h2c'){for(let i=0;i<6;i++){a.front(w-25,-1.5,47+i*8,9,5,'#b1b997');a.front(w-23,-1.7,46+i*8,4,2,'#6a8b70')}}
 // Glass reflections retain a view of the interior.
 a.poly([[border+4,-9,zTop-4],[border+26,-9,zTop-4],[border+4,-9,zTop-42]],'#c2d3b6','none',.3,.10);a.line([[border+2,-9,z0+3],[border+2,-9,zTop-3],[border+doorW-2,-9,zTop-3]],'#b2c6aa',.75,.6);a.front(border+doorW-6,-10,h*.49,2.4,19,'#a5b99d');
 if(hseries){a.front(w-20,-.5,18,11,h-51,'#4b6757');for(let i=0;i<15;i++)a.line([[w-18,-1,26+i*4],[w-11,-1,26+i*4]],'#2e4e3d',.65);a.screen(w-20,-5,h-20,29,21,true);a.label('BAMBU LAB',12,-.8,8,3.4,'#becbb3');a.label(m.name,w-42,-.8,8,4,'#cfdbc1',600)}
 else if(p1){a.screen(w-22,-4,h-10,22,14,false);a.label('BAMBU LAB',31,-.8,8,4,'#cad5bb');a.label('P1S',w-23,-.8,8,3,'#aabc9f')}
 else{a.screen(w-25,-4,h-17,28,xold?24:20,true);a.label('BAMBU LAB',14,-.8,8,3.7,light?'#78927d':'#c0d0b6');a.label(m.name,w-47,-.8,8,3.7,light?'#6f8b77':'#c8d7bb',600)}
 if(m.kind==='x1e')a.front(8,-.6,4,17,1.2,'#93ad98');
 if(combo){const type=m.ams==='AMS'?'AMS':'AMS 2 Pro';rearTube(a,(w-103)/2,10,h+3);amsBox(a,(w-103)/2,10,h+3,type);if(m.kind==='h2dpro'){a.curve([[w+47,29,41],[w+45,68,87],[w+10,93,105],[w+1,d-3,h-23]],'#aabfa6',1.15);amsHT(a,w+27,32,3)}}
}

function artFor(m,combo){let opts={cx:218,cy:351,scale:1.45};if(m.series==='A')opts={cx:145,cy:352,scale:m.kind==='a2l'?1.22:1.37};if(m.series==='H')opts={cx:m.kind==='h2dpro'&&combo?192:214,cy:356,scale:1.35};const a=new Art(m.id+(combo?'-ams':'-bare'),opts);if(m.series==='A')openPrinter(a,m,combo);else if(m.kind==='p1p')coreFrame(a,m,combo);else enclosed(a,m,combo);return a.end()}
const font='Arial,Microsoft YaHei,Segoe UI,sans-serif';
function text(x,y,value,size=18,color='#667985',weight=400,extra=''){return `<text x="${x}" y="${y}" font-family="${font}" font-size="${size}" fill="${color}" font-weight="${weight}" ${extra}>${esc(value)}</text>`}
function inlineSVG(svg,x,y,w=600,h=400){return svg.replace(/<svg[^>]*>/,`<svg x="${x}" y="${y}" width="${w}" height="${h}" viewBox="0 0 600 400">`)}
const seriesNames={A:'开放式创作系列',P:'紧凑 CoreXY 系列',X:'X 系列与经典机型',H:'H 系列制造工作站'};
for(const m of models){for(const combo of [false,true]){const id=m.id+(combo?'-ams':'-bare'),svg=artFor(m,combo);fs.writeFileSync(path.join(ROOT,'assets',id+'.svg'),svg);await sharp(Buffer.from(svg)).png().toFile(path.join(ROOT,'assets',id+'.png'));}}

for(const [idx,series] of ['A','P','X','H'].entries()){
 const selected=models.filter(m=>m.series===series),rowH=350,width=1600,height=253+selected.length*rowH;
 let s=`<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}"><rect width="1600" height="${height}" fill="#f5f7f6"/><rect x="37" y="34" width="38" height="38" rx="11" fill="#3159ed"/><g stroke="#fff" stroke-width="1.6" fill="none"><path d="m46 48 10-6 10 6-10 6zM46 54l10 6 10-6M46 60l10 6 10-6"/></g>`;
 s+=text(91,59,'LAYERHUB / DEVICE ILLUSTRATIONS',16,'#6c8183',600,'letter-spacing="2"')+text(46,120,`${series} 系列 · ${seriesNames[series]}`,36,'#283e42',600)+text(48,155,'共印空间设备插画  /  同一机型，单机与多色供料配置对照',17,'#90a09d')+text(1539,63,`0${idx+1} / 04`,17,'#90a09d',500,'text-anchor="end"');
 s+=text(48,209,'机型',14,'#98a59f',500)+text(510,209,'单机 · 外置单卷供料',15,'#77918a',500,'text-anchor="middle"')+text(1170,209,'配备对应 AMS 供料系统',15,'#5f7e71',500,'text-anchor="middle"');
 for(const [i,m] of selected.entries()){
  const y=231+i*rowH;s+=`<rect x="38" y="${y}" width="1524" height="${rowH-15}" rx="16" fill="#fff" stroke="#e6ede8"/><line x1="830" y1="${y+23}" x2="830" y2="${y+rowH-39}" stroke="#ecf0eb"/>`;
  s+=text(63,y+48,m.name,23,'#314b46',600);
  const lines=m.subtitle.split(' · ');lines.forEach((t,j)=>s+=text(64,y+77+j*23,t,12,'#96a79d'));
  const legacy=m.status==='legacy';s+=`<rect x="63" y="${y+lines.length*23+69}" width="${legacy?92:64}" height="25" rx="6" fill="${legacy?'#f4f1e9':'#edf5ef'}"/>`+text(legacy?109:95,y+lines.length*23+86,legacy?'存量机型':'现行机型',11,legacy?'#aaa083':'#79a68a',500,'text-anchor="middle"');
  s+=inlineSVG(artFor(m,false),236,y-10,585,350)+inlineSVG(artFor(m,true),888,y-10,585,350);
  s+=text(1170,y+rowH-36,m.ams,13,'#7f9b88',500,'text-anchor="middle"');
 }
 s+=text(49,height-17,'风格化识别插画 · 非尺寸图 · 配置为展示示例，不代表所有可售套装或连接方案',12,'#a0ada4')+text(1548,height-17,'2026.10.19 / V3',12,'#a0ada4',400,'text-anchor="end"')+'</svg>';
 const basename=series.toLowerCase()+'-series-comparison';fs.writeFileSync(path.join(ROOT,'boards',basename+'.svg'),s);await sharp(Buffer.from(s)).png().toFile(path.join(ROOT,'boards',basename+'.png'));
}

const manifest={version:3,researched_at:'2026-10-19',scope:'Bambu Lab FFF 3D printers; current and legacy equipment, excluding R1 laser cutter',revision_notes:['AMS lite redrawn as two coaxial spool pairs offset fore/aft in depth and height, using side-on circular faces and a diagonal white body.','Central inclined stand, dark desk feet and four dark feeder modules with yellow release buttons are now visible.','Approved AMS and AMS 2 Pro V2 geometry preserved.','AMS/AMS 2 Pro spool axes corrected to the printer width direction (X); circular spool faces lie in the YZ plane.','Deeper enclosure and transparent curved lid; front feeder row points in the same direction as printer door.','Rear ports and contact feet revised; no invented front display.'],style:'Project-native isometric SVG, front-left three-quarter view; simplified identification illustrations, not mechanical CAD or product photography',count:models.length,variant_count:models.length*2,notes:['裸机图表现设备本体，不代表某机型一定存在裸机销售套装。','AMS 版本为一种代表性组合，不穷举兼容设备、转接器或安装方式。','A 系列本轮绘制 AMS lite；其他已支持的 AMS 配置可复用组件后续扩展。','H2D Pro 组合图同时绘制 AMS 2 Pro 与 AMS HT。','X1、X1 Carbon、X1E 和 P1P 标为存量机型，保留用于现有设备管理。','X1 族及 H2 族部分型号外壳相近，保留相似性，通过型号文字与内部结构辅助识别，避免虚构夸张外形。'],sources,models:models.map(m=>({...m,variants:{bare:`assets/${m.id}-bare.svg`,ams:`assets/${m.id}-ams.svg`}}))};
fs.writeFileSync(path.join(ROOT,'catalog.json'),JSON.stringify(manifest,null,2));

const html=`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>共印空间 · 拓竹设备插画 V3</title><style>*{box-sizing:border-box}body{margin:0;padding:40px;background:#f5f7f6;color:#2d4542;font-family:Arial,'Microsoft YaHei',sans-serif}header{max-width:1450px;margin:0 auto 30px}h1{font-size:28px}p{color:#849b90;font-size:13px;line-height:1.8}nav{display:flex;gap:10px;flex-wrap:wrap;margin:22px 0}button,a.download{border:1px solid #dce7de;border-radius:7px;padding:10px 15px;background:white;color:#708e7b;cursor:pointer;text-decoration:none;font:inherit;font-size:12px}button.active{color:white;background:#3159ed;border-color:#3159ed}main{max-width:1500px;margin:auto}figure{margin:0 0 30px}figure>img{display:block;width:100%;height:auto;border-radius:12px}dialog{max-width:calc(100vw - 30px);width:750px;border:1px solid #e3eae1;border-radius:14px;padding:25px}dialog::backdrop{background:#243f4c55}dialog img{width:100%;height:auto}#grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px}.tile{background:white;border:1px solid #e5ebe2;border-radius:12px;padding:20px;cursor:pointer;text-align:left;color:inherit}.tile img{width:100%}.tile strong{display:block;font-size:14px}.tile small{display:block;color:#8ba18e;font-size:11px;margin-top:8px}.detail-links{display:flex;gap:10px;justify-content:center}#close{float:right}@media(max-width:750px){body{padding:20px}#grid{grid-template-columns:1fr 1fr}h1{font-size:23px}}</style></head><body><header><p>LAYERHUB / HARDWARE STUDIES</p><h1>为每一台创作伙伴，准备自己的模样。</h1><p>14 款拓竹机型 · 28 张立体插画 · 单机 / AMS 配置 · 可直接复用的透明 SVG 与 PNG</p><p>保留现有设备卡片的柔和立体风格；本轮为图片方案预览，未改动线上工作台。</p><nav><button class="active" data-view="boards">系列对照图</button><button data-view="bare">单机图集</button><button data-view="ams">AMS 图集</button></nav></header><main><section id="boards">${['a','p','x','h'].map(s=>`<figure><img src="boards/${s}-series-comparison.png" alt="${s.toUpperCase()} 系列单机与 AMS 对照图"></figure>`).join('')}</section><section id="grid" hidden></section></main><dialog id="detail"><button id="close">关闭</button><h2 id="detailName"></h2><img id="detailImage" alt="设备插画大图"><div class="detail-links"><a id="svgLink" class="download" download>下载 SVG</a><a id="pngLink" class="download" download>下载 PNG</a></div></dialog><script>const models=${JSON.stringify(models)},grid=document.querySelector('#grid'),boards=document.querySelector('#boards');function render(view){boards.hidden=view!=='boards';grid.hidden=view==='boards';document.querySelectorAll('[data-view]').forEach(b=>b.classList.toggle('active',b.dataset.view===view));if(view!=='boards'){grid.innerHTML=models.map(m=>'<button class="tile" data-model="'+m.id+'"><strong>'+m.name+'</strong><small>'+(view==='ams'?m.ams:'单机')+' · '+(m.status==='legacy'?'存量机型':'现行机型')+'</small><img src="assets/'+m.id+'-'+view+'.png" alt="'+m.name+' '+view+'"></button>').join('');grid.querySelectorAll('[data-model]').forEach(b=>b.onclick=()=>{const m=models.find(m=>m.id===b.dataset.model),base='assets/'+m.id+'-'+view;document.querySelector('#detailName').textContent=m.name+' · '+(view==='ams'?m.ams:'单机');document.querySelector('#detailImage').src=base+'.png';document.querySelector('#svgLink').href=base+'.svg';document.querySelector('#pngLink').href=base+'.png';document.querySelector('#detail').showModal()})}}document.querySelectorAll('[data-view]').forEach(b=>b.onclick=()=>render(b.dataset.view));document.querySelector('#close').onclick=()=>document.querySelector('#detail').close();</script></body></html>`;
fs.writeFileSync(path.join(ROOT,'index.html'),html);
fs.writeFileSync(path.join(ROOT,'README.md'),`# 拓竹设备立体插画 V3\n\n用于共印空间设备卡片的 SVG 原生插画扩展，未改动线上站点。\n\n- 14 款机型，每款单机与 AMS 两种版本：共 28 张独立 SVG 和 28 张透明 PNG。\n- boards/：A、P、X、H 四组单机/AMS 对照图。\n- assets/：可复用的独立设备图。\n- catalog.json：机型、配置、状态和研究来源。\n- index.html：可直接双击打开的本地图集，支持原图查看与下载。\n- generate.mjs：插画生成源文件。\n\n${manifest.notes.map(n=>'- '+n).join('\n')}\n\n## 核对来源\n\n${Object.entries(sources).map(([k,v])=>'- '+k+': '+v).join('\n')}\n`);
console.log(JSON.stringify({models:models.length,variants:models.length*2,boards:4,root:ROOT}));

export {Art,models,artFor,text,inlineSVG,ROOT,sharp};
