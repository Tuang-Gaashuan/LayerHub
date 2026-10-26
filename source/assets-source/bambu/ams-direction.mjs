// Shared isometric AMS component. World X follows the printer's width;
// world Y points toward the rear. Spool axes follow X, so circular faces
// lie in the YZ plane instead of facing the printer's front door.
const COLOR=['#b4c9b8','#d7b899','#b7b5cf','#dce0d5'];
const points=(cx,cy,cz,r)=>Array.from({length:64},(_,i)=>{const t=i*Math.PI/32;return [cx,cy+r*Math.cos(t),cz+r*Math.sin(t)]});
function disc(a,x,y,z,r,fill,stroke='none',width=.4,opacity=1){a.poly(points(x,y,z,r),fill,stroke,width,opacity)}
function cylinder(a,x,y,z,r,width,fill){
 disc(a,x+width,y,z,r,fill);
 const facets=[];
 for(let i=0;i<64;i++){
  const t=(i+.5)*Math.PI/32;
  if(-Math.cos(t)+.85*Math.sin(t)<0)continue;
  const t0=i*Math.PI/32,t1=(i+1)*Math.PI/32;
  const v=[[x,y+r*Math.cos(t0),z+r*Math.sin(t0)],[x+width,y+r*Math.cos(t0),z+r*Math.sin(t0)],[x+width,y+r*Math.cos(t1),z+r*Math.sin(t1)],[x,y+r*Math.cos(t1),z+r*Math.sin(t1)]];
  facets.push({v,depth:Math.cos(t)-Math.sin(t),dark:Math.max(0,.18-.13*Math.sin(t))});
 }
 facets.sort((p,q)=>q.depth-p.depth);
 for(const f of facets){a.poly(f.v,fill);if(f.dark>.01)a.poly(f.v,'#000000','none',0,f.dark)}
 disc(a,x,y,z,r,fill);
}
function spool(a,x,y,z,width,r,color){
 // Flanges surround a wound filament cylinder. Faces are side-on to
 // the viewer; the colored curved winding is seen from the front/top.
 cylinder(a,x+width-1,y,z,r,1,'#b8bfc0');
 cylinder(a,x+2,y,z,r*.87,width-4,color);
 for(let i=2;i<width-2;i+=1.4){
  const v=[];for(let k=0;k<=40;k++){const t=.35+k*Math.PI/40;v.push([x+i,y+r*.88*Math.cos(t),z+r*.88*Math.sin(t)])}a.line(v,'#ffffff',.35,.18);
 }
 cylinder(a,x,y,z,r,1.5,'#e1e4df');
 disc(a,x-.05,y,z,r*.91,'#d4d9d5','#a9b4ad',.35);
 for(let i=0;i<12;i++){
  const t=i*Math.PI/6,v=[];
  for(const [rr,dt] of [[r*.35,-.085],[r*.79,-.055],[r*.79,.055],[r*.35,.085]])v.push([x-.1,y+rr*Math.cos(t+dt),z+rr*Math.sin(t+dt)]);
  a.poly(v,'#a6b2ad');
 }
 disc(a,x-.2,y,z,r*.25,'#e9ece5','#a9b4ad',.4);
 disc(a,x-.3,y,z,r*.115,'#788a83');
 disc(a,x-.4,y,z,r*.045,'#c1cbc1');
}
function smoothPath(a,profile,x){const v=profile.map(([y,z])=>a.p(x,y,z));let s='M'+v[0].join(',');for(let i=1;i<v.length;i++){const p=v[i-1],q=v[i];if(i===v.length-1)s+=' L'+q.join(',');else{const mid=[(p[0]+q[0])/2,(p[1]+q[1])/2];s+=' Q'+p.join(',')+' '+mid.join(',')}}return s+' L'+v.at(-1).join(',')}

export function amsCorrected(a,x,y,z,type='AMS',scale=1,{lid='closed',empty=false}={}){
 const pro=type!=='AMS',w=103*scale,d=72*scale,h=64*scale;
 const saved=a.p.bind(a);
 // Everything below is unit-local. Printer and AMS share the same
 // projection; there is deliberately no 90-degree box rotation.
 a.p=(px,py,pz)=>saved(x+px*scale,y+py*scale,z+pz*scale);
 const base=pro?['#606b66','#45524d','#9ba69b']:['#535e58','#3d4c43','#88988a'];
 // Contact feet and actual floor of the enclosure.
 for(const [fx,fy] of [[7,6],[88,6],[7,60],[88,60]])a.box(fx,fy,-2,8,7,3,'#3b4b40','#2e4036','#596c57');
 a.box(0,0,0,103,72,12,...base);
 a.top(3,5,12.1,97,62,'#293f35');
 a.box(2,65,12,99,5,28,'#687b6b','#435b4b','#94a48c');
 for(let i=0;i<4;i++){
  const sx=7+i*24;
  a.box(sx-1,11,12,21,50,2,'#3f5748','#344e40','#637e65');
  for(const sy of [17,54])a.box(sx+1,sy,14,16,2.5,2.5,'#5b7060','#445d4d','#879b7e');
 }
 // Far/right spool first so the near/left flange has correct occlusion.
 if(!empty)for(let i=3;i>=0;i--)spool(a,7+i*24,37,36,18.8,25.4,COLOR[i]);
 // Four small first-stage feeders sit at the front, not four discs.
 for(let i=0;i<4;i++){
  const fx=10+i*24;
  a.box(fx,-1,12,13.5,10,9,pro?'#747f73':'#a2aea0','#445d4c','#b7c1ab');
  a.front(fx+2.4,-1.4,14,8.6,4.8,pro?'#bc956d':'#e5e9dd');
  a.front(fx+5.4,-1.6,16,2.6,1.3,'#3a5242');
  a.label(String(i+1),fx+5,-1.7,12.8,1.8,'#d5dfca');
  a.curve([[fx+6,10,23],[fx+7,12,25],[fx+10,13,29],[fx+12,15,31]],'#bfcbb6',.65,.7);
 }
 // The cover has a deeper side profile and a rounded front slope.
 const profile=[[0,13],[1.5,26],[7,43],[16,57],[29,64],[51,65],[65,61],[71,50],[72,13]];
 if(lid==='closed'){
  // Translucent sweep across X. This leaves the winding direction visible.
  for(let i=0;i<profile.length-1;i++){
   const [y0,z0]=profile[i],[y1,z1]=profile[i+1];
   a.poly([[0,y0,z0],[103,y0,z0],[103,y1,z1],[0,y1,z1]],pro?'#a9b9ac':'#9bab9d','none',0,pro?.085:.15);
  }
  const side=profile.map(([py,pz])=>[0,py,pz]);
  a.poly(side,'#a0b1a0','none',0,pro?.14:.22);
  a.a.push(`<path d="${smoothPath(a,profile,0)}" fill="none" stroke="#aebdab" stroke-width=".8" opacity=".8"/>`);
  a.a.push(`<path d="${smoothPath(a,profile,103)}" fill="none" stroke="#bcc9b8" stroke-width=".65" opacity=".65"/>`);
  a.line([[0,0,13],[103,0,13]],'#d6ddcd',.85,.85);
  a.line([[0,29,64],[103,29,64]],'#e0e6d5',.7,.55);
  // A restrained curved reflection rather than an opaque rectangular roof.
  a.poly([[6,9,47],[7,16,57],[28,16,57],[25,9,47]],'#ffffff','none',0,.18);
  a.poly([[7,28,64],[8,33,64.3],[83,33,64.3],[82,28,64]],'#ffffff','none',0,.08);
  for(const lx of [4,94]){a.box(lx,-1.2,9,5,2.5,7,'#354e3c','#283f30','#657d5d');a.front(lx+.8,-1.4,12,3.4,1.1,'#9cae91')}
 }else if(lid==='open'){
  // A separate transparent lid raised around the rear hinge.
  const raised=profile.map(([py,pz])=>{const yy=72+(py-72)*.3+(pz-13)*.6,zz=13+(72-py)*.85+(pz-13)*.6;return [yy,zz]});
  a.poly(raised.map(([py,pz])=>[0,py,pz]),'#a8baac','none',0,.12);
  for(let i=0;i<raised.length-1;i++){const [y0,z0]=raised[i],[y1,z1]=raised[i+1];a.poly([[0,y0,z0],[103,y0,z0],[103,y1,z1],[0,y1,z1]],'#c1cfbc','none',0,.06)}
  for(const sideX of [0,103])a.line(raised.map(([py,pz])=>[sideX,py,pz]),'#b5c6b0',.7,.6);
 }
 a.label(pro?'AMS 2 PRO':'AMS',pro?37:43,-.5,4.2,pro?3.5:4.3,'#d9e2ce',600);
 // Small chassis/vent details distinguish generations without inventing a screen.
 if(pro){for(let i=0;i<6;i++)a.line([[85+i*1.3,-.6,3],[85+i*1.3,-.6,8]],'#324b37',.7,.7);a.front(8,-.5,4,3,1,'#a3c48d')}
 a.p=saved;
 return {w,d,h};
}

export function rearTube(a,x,y,z){a.curve([[x+85,y+72,z+8],[x+102,y+89,z+8],[x+105,y+90,z-12],[x+99,y+79,z-25]],'#a1b3a0',1.0,.7)}
export {spool as sideSpool,cylinder as sideCylinder,disc as sideDisc};
