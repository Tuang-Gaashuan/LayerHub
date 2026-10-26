import {sideSpool,sideCylinder,sideDisc} from './ams-direction.mjs';

const COLORS=['#b4c9b8','#b7b5cf','#dce0d5','#d7b899'];
// Two coaxial pairs are displaced fore/aft, not four disks in one
// camera-facing plane. X is the spool axis; Y is the machine depth.
function beam(a,x0,x1,profile,front='#d3dbd1',side='#9fb3a5',top='#ecf0e4'){
 a.poly(profile.map(([y,z])=>[x0,y,z]),side);
 a.poly(profile.map(([y,z])=>[x1,y,z]),front);
 for(let i=0;i<profile.length;i++){const j=(i+1)%profile.length,[y,z]=profile[i],[yy,zz]=profile[j];a.poly([[x0,y,z],[x1,y,z],[x1,yy,zz],[x0,yy,zz]],i===0?top:front)}
}
function verticalCylinder(a,x,y,z,r,h,fill){
 const circle=zz=>Array.from({length:40},(_,i)=>{const t=i*Math.PI/20;return [x+r*Math.cos(t),y+r*Math.sin(t),zz]});
 const b=circle(z),t=circle(z+h);
 for(let i=0;i<40;i++){const j=(i+1)%40;a.poly([b[i],b[j],t[j],t[i]],fill);const dark=Math.max(0,Math.cos(i*Math.PI/20)*.15);if(dark)a.poly([b[i],b[j],t[j],t[i]],'#000000','none',0,dark)}
 a.poly(t,'#718276','#a7b59d',.4);
}
function claw(a,x,y,z,outer=false,mark='#c9a85f'){
 sideDisc(a,x,y,z,6.9,'#899c8b','#c1ccba',.4);sideDisc(a,x-.2,y,z,4.9,'#b1bfa9');
 for(let k=0;k<3;k++){
  const t=k*2*Math.PI/3,p=[];
  for(const [r,dt] of [[3,-.31],[9,-.13],[8.7,.18],[3.8,.38]])p.push([x-.5,y+r*Math.cos(t+dt),z+r*Math.sin(t+dt)]);
  a.poly(p,'#687e6c','#c1cdb8',.3);
 }
 sideDisc(a,x-.8,y,z,2.1,'#7e9580');
 if(outer)a.line([[x-1,y-6.5,z-1],[x-1,y-4,z+1.5]],mark,1.1);
}
function feeder(a,x,y,z,slot){
 // Dark motor above a translucent lower feeder; release button in yellow.
 verticalCylinder(a,x,y,z,5.2,12,'#5d7462');
 verticalCylinder(a,x,y,z+12,5.2,9,'#425b48');
 const ring=Array.from({length:24},(_,i)=>{let t=i*Math.PI/12;return [x+4.4*Math.cos(t),y+4.4*Math.sin(t),z+21.15]});a.poly(ring,'#687f6a');
 a.label(String(slot),x-1.2,y-3.9,z+20.5,2.6,'#a8b99e',600);
 a.box(x-1.1,y-6.7,z+8.1,2.2,2.6,5,'#c9a85f','#b49757','#dab753');
 a.disk(x,y-7,z+5.8,2.5,'#dab753','#b29b62',.4);
 a.front(x-.8,y-7.1,z+11,1.6,1.1,'#334a38');
 // PTFE outlet above the release collar.
 a.box(x-.65,y-5.5,z+13.2,1.3,1.3,1.8,'#3e5540','#324b36','#74886b');
}

export function amsLiteCorrected(a,x,y,z=0,{loaded=true,scale=1}={}){
 const prior=a.p.bind(a);a.p=(px,py,pz)=>prior(x+px*scale,y+py*scale,z+pz*scale);
 a.shadow(36,35,69);
 // Broad dark Y-shaped desk stand, with three separated contact pads.
 a.poly([[3,-27,0],[10,-28,0],[35,25,0],[63,-24,0],[72,-22,0],[44,35,0],[43,83,0],[34,86,0],[28,36,0]],'#354d3a');
 a.poly([[3,-27,0],[10,-28,0],[35,25,5],[63,-24,5],[72,-22,0],[44,35,8],[43,83,4],[34,86,4],[28,36,8]],'#4f6250');
 a.line([[11,-23,5],[35,27,8],[64,-19,6]],'#bac8ae',.6,.55);
 a.box(2,-28,-1,10,10,2,'#3d5741','#314a35','#7b9478');a.box(62,-24,-1,10,10,2,'#3d5741','#314a35','#7b9478');a.box(33,77,-1,12,11,2,'#3d5741','#314a35','#7b9478');
 // Central stand has a tapered inclined stem rather than a straight pole.
 beam(a,29,43,[[24,6],[46,6],[46,30],[33,67],[26,61],[32,25]],'#c6d2c0','#95ae99','#e0e9d6');
 // Far pair sits higher and farther back; its two spools share one X axis.
 if(loaded){sideSpool(a,47,58,87,21,27.5,COLORS[3]);sideSpool(a,3,58,87,21,27.5,COLORS[2])}
 // Main white body is a diagonal capsule in the Y/Z plane.
 beam(a,25,45,[[-8,46],[-8,55],[57,104],[72,102],[72,92],[7,43]],'#d4ded0','#a9bda8','#edf2e3');
 beam(a,29,41,[[-5,53],[-1,58],[64,107],[71,103],[63,99]],'#e4ebdb','#bacbb5','#f3f5e8');
 // The exposed rotary holders point outward from both sides of the body.
 for(const [cy,cz] of [[2,49],[58,87]]){
  sideCylinder(a,14,cy,cz,5,42,'#b6c6ad');
  if(!loaded){sideCylinder(a,3,cy,cz,5,22,'#8ca48c');sideCylinder(a,45,cy,cz,5,23,'#8ca48c');claw(a,2.8,cy,cz,true);claw(a,67,cy,cz,false)}
 }
 // Joined twin-lobed motor pods sit above the rear of the spine.
 // Their cylindrical end caps remain visible instead of looking like
 // four unrelated vertical posts.
 for(const fx of [25,45])beam(a,fx-5.2,fx+5.2,[[51,96],[68,100],[68,121],[51,117]],'#435b48','#344d3b','#6b806b');
 // Four feeders sit above the rear of the spine; two are partially occluded.
 feeder(a,25,68,100,3);feeder(a,45,68,100,4);feeder(a,25,51,96,1);feeder(a,45,51,96,2);
 a.box(32,51,98,7,17,19,'#c8d7bf','#a1baa0','#e0e9d5');
 a.front(33.2,50.8,105,4.5,5,'#dce6d3');
 // The near pair masks the lower body and is visibly in front of the rear pair.
 if(loaded){sideSpool(a,47,2,49,21,27.5,COLORS[1]);sideSpool(a,3,2,49,21,27.5,COLORS[0]);claw(a,2.5,2,49,true);claw(a,2.5,58,87,true)}
 a.label('AMS lite',29,-9,44,2.6,'#7b947b',500);
 a.p=prior;
 return {width:75,depth:116,height:122};
}

export function liteTubePaths(a,x,y,h,printerW,printerY,railZ){
 // Four distinct outlets, gathered into a restrained bundle leading to
 // the toolhead. Drawn behind the physical feeder and spool assembly.
 const offsets=[[25,51,109],[45,51,109],[25,68,113],[45,68,113]];
 offsets.forEach(([dx,dy,dz],i)=>a.curve([[x+dx,y+dy,dz],[x-8-i*2,y+dy+5,h+22+i*2],[printerW*.68,printerY+26,h+22+i*2],[printerW*.58,printerY-5,railZ+8]],'#b6c7b3',.8,.8));
}
