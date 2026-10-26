import * as THREE from './vendor/three/three.module.min.js';

export function projectedBounds(box,camera,width,height){
 const points=[];for(const x of [box.min.x,box.max.x])for(const y of [box.min.y,box.max.y])for(const z of [box.min.z,box.max.z])points.push(new THREE.Vector3(x,y,z).project(camera));
 if(points.some(p=>![p.x,p.y,p.z].every(Number.isFinite)||p.z< -1||p.z>1))return null;
 return {left:Math.min(...points.map(p=>(p.x+1)*width/2)),right:Math.max(...points.map(p=>(p.x+1)*width/2)),top:Math.min(...points.map(p=>(1-p.y)*height/2)),bottom:Math.max(...points.map(p=>(1-p.y)*height/2))};
}
export function withinRectangle(boxes,a,b){
 const left=Math.min(a.x,b.x),right=Math.max(a.x,b.x),top=Math.min(a.y,b.y),bottom=Math.max(a.y,b.y);
 return boxes.flatMap((box,i)=>box&&[box.left,box.right,box.top,box.bottom].every(Number.isFinite)&&box.left>=left&&box.right<=right&&box.top>=top&&box.bottom<=bottom?[i]:[]);
}
export function attachBoxSelection(canvas,host,{enabled,project,onSelect,onGesture,onClick}){
 let drag=null;const frame=host.ownerDocument.createElement('div');frame.className='slice-box-selection';frame.hidden=true;frame.setAttribute('aria-hidden','true');host.append(frame);
 const point=e=>{const r=canvas.getBoundingClientRect();return {x:Math.max(0,Math.min(r.width,e.clientX-r.left)),y:Math.max(0,Math.min(r.height,e.clientY-r.top))}};
 const draw=end=>{const r=canvas.getBoundingClientRect(),h=host.getBoundingClientRect();Object.assign(frame.style,{left:(r.left-h.left+Math.min(drag.start.x,end.x))+'px',top:(r.top-h.top+Math.min(drag.start.y,end.y))+'px',width:Math.abs(end.x-drag.start.x)+'px',height:Math.abs(end.y-drag.start.y)+'px'});frame.hidden=false};
 function cancel(){if(!drag)return;const id=drag.id;drag=null;frame.hidden=true;try{if(canvas.hasPointerCapture?.(id))canvas.releasePointerCapture(id)}catch{}onGesture?.(false)}
 const down=e=>{if(!enabled()||e.button!==0||e.isPrimary===false||drag)return;e.preventDefault();e.stopImmediatePropagation();drag={id:e.pointerId,start:point(e),additive:e.ctrlKey||e.metaKey||e.shiftKey};try{canvas.setPointerCapture?.(e.pointerId)}catch{}onGesture?.(true)};
 const move=e=>{if(!drag||e.pointerId!==drag.id)return;e.preventDefault();e.stopImmediatePropagation();draw(point(e))};
 const up=e=>{if(!drag||e.pointerId!==drag.id)return;e.preventDefault();e.stopImmediatePropagation();const state=drag,end=point(e),moved=Math.hypot(end.x-state.start.x,end.y-state.start.y)>=5;cancel();if(!enabled())return;if(moved)onSelect(withinRectangle(project(),state.start,end),{additive:state.additive});else onClick?.(e)};
 canvas.addEventListener('pointerdown',down,true);canvas.addEventListener('pointermove',move,true);canvas.addEventListener('pointerup',up,true);canvas.addEventListener('pointercancel',cancel);canvas.addEventListener('lostpointercapture',cancel);
 return {cancel,dispose(){cancel();canvas.removeEventListener('pointerdown',down,true);canvas.removeEventListener('pointermove',move,true);canvas.removeEventListener('pointerup',up,true);canvas.removeEventListener('pointercancel',cancel);canvas.removeEventListener('lostpointercapture',cancel);frame.remove()}};
}
