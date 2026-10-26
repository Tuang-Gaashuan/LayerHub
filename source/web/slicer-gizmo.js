import * as THREE from './vendor/three/three.module.min.js';
import {TransformControls} from './vendor/three/TransformControls.js';

const rounded=n=>Math.round(n*1000)/1000;
export function gizmoPatch(mode,axis,object){
 if(mode==='translate')return {x:rounded(Math.max(-1000,Math.min(1000,object.position.x))),y:rounded(Math.max(-1000,Math.min(1000,object.position.y)))};
 if(mode==='rotate')return {rotation:[object.rotation.x,object.rotation.y,object.rotation.z].map(v=>rounded(THREE.MathUtils.radToDeg(v)))};
 const raw=axis==='Y'?object.scale.y:axis==='Z'?object.scale.z:object.scale.x;
 return {scale:rounded(Math.max(.01,Math.min(10,raw)))};
}

// A persistent proxy prevents grounding the mesh from moving the drag origin.
// The application keeps X/Y, Euler XYZ and a uniform scale as its saved state.
export function createGizmo(scene,camera,canvas,{onChange,onGesture}={}){
 const proxy=new THREE.Object3D();scene.add(proxy);
 const control=new TransformControls(camera,canvas),helper=control.getHelper();scene.add(helper);control.setSize(.85);
 let tool='select',enabled=true,model=null,placement=null,bounds=null;
 function sync(nextModel=model,nextPlacement=placement,nextBounds=bounds){
  model=nextModel;placement=nextPlacement;bounds=nextBounds;
  if(!enabled||tool==='select'||!model||!placement){control.detach();control.enabled=false;return}
  control.enabled=true;
  if(!control.dragging){
   const box=bounds||new THREE.Box3().setFromObject(model,true);
   proxy.position.set(placement.x,placement.y,(box.min.z+box.max.z)/2);
   proxy.rotation.copy(model.rotation);proxy.scale.setScalar(placement.scale);proxy.updateMatrixWorld(true);
  }
  if(control.object!==proxy)control.attach(proxy);
 }
 const gesture=e=>{onGesture?.(e.value);if(!e.value)sync()};
 const change=()=>{
  const patch=gizmoPatch(control.mode,control.axis,proxy);
  if(patch.scale!==undefined)proxy.scale.setScalar(patch.scale);
  const values=Object.values(patch).flat();if(values.every(Number.isFinite))onChange?.(patch);
 };
 control.addEventListener('dragging-changed',gesture);control.addEventListener('objectChange',change);
 const cancel=()=>{if(control.dragging){control.pointerUp({button:0});sync()}};
 canvas.addEventListener('pointercancel',cancel);canvas.addEventListener('lostpointercapture',cancel);
 return {
  sync,
  get dragging(){return control.dragging},
  mode(next){cancel();tool=next;control.setMode(next==='select'?'translate':next);control.setSpace(next==='translate'?'world':'local');control.showX=true;control.showY=true;control.showZ=next!=='translate';sync()},
  enable(value){if(enabled!==value&&!value)cancel();enabled=value;sync()},
  finish:cancel,
  dispose(){cancel();canvas.removeEventListener('pointercancel',cancel);canvas.removeEventListener('lostpointercapture',cancel);control.removeEventListener('dragging-changed',gesture);control.removeEventListener('objectChange',change);control.dispose();scene.remove(helper,proxy)}
 };
}
