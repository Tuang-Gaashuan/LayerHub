import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import * as THREE from '../web/vendor/three/three.module.min.js';
import {projectedBounds,withinRectangle,attachBoxSelection} from '../web/slicer-box.js';
const require=createRequire(import.meta.url),{JSDOM}=require('./frontend-deps/node_modules/jsdom');
test('box selection contains full projected models in either drag direction and excludes partial or clipped models',()=>{
 const camera=new THREE.PerspectiveCamera(40,1,1,500);camera.position.set(0,0,100);camera.lookAt(0,0,0);camera.updateMatrixWorld();
 const box=projectedBounds(new THREE.Box3(new THREE.Vector3(-5,-5,0),new THREE.Vector3(5,5,10)),camera,200,200);assert.ok(box.left<100&&box.right>100);assert.deepEqual(withinRectangle([box,null],{x:50,y:50},{x:150,y:150}),[0]);assert.deepEqual(withinRectangle([box],{x:150,y:150},{x:50,y:50}),[0]);assert.deepEqual(withinRectangle([box],{x:100,y:50},{x:150,y:150}),[]);assert.equal(projectedBounds(new THREE.Box3(new THREE.Vector3(0,0,110),new THREE.Vector3(10,10,120)),camera,200,200),null);
});
test('box pointer gestures do not reach orbit handlers, select only on release and cancel without selection',()=>{
 const dom=new JSDOM('<div id="host"><canvas></canvas></div>'),w=dom.window,host=w.document.querySelector('#host'),canvas=host.querySelector('canvas'),chosen=[],gestures=[];let orbit=0,enabled=true;
 host.getBoundingClientRect=canvas.getBoundingClientRect=()=>({left:0,top:0,width:200,height:200});canvas.addEventListener('pointerdown',()=>orbit++);
 const controller=attachBoxSelection(canvas,host,{enabled:()=>enabled,project:()=>[{left:20,right:70,top:20,bottom:70}],onSelect:(i,e)=>chosen.push([i,e]),onGesture:v=>gestures.push(v)});
 const send=(name,x,y,extra={})=>{const e=new w.MouseEvent(name,{bubbles:true,cancelable:true,clientX:x,clientY:y,button:0,...extra});Object.defineProperty(e,'pointerId',{value:1});canvas.dispatchEvent(e)};
 send('pointerdown',0,0,{ctrlKey:true});send('pointermove',100,100);assert.equal(chosen.length,0);assert.equal(host.querySelector('.slice-box-selection').hidden,false);send('pointerup',100,100);assert.deepEqual(chosen,[[[0],{additive:true}]]);assert.equal(orbit,0);assert.deepEqual(gestures,[true,false]);
 send('pointerdown',0,0);controller.cancel();send('pointerup',100,100);assert.equal(chosen.length,1);assert.equal(host.querySelector('.slice-box-selection').hidden,true);
 enabled=false;send('pointerdown',0,0);assert.equal(orbit,1);controller.dispose();assert.equal(host.querySelector('.slice-box-selection'),null);dom.window.close();
});
test('tiny box clicks use normal picking and pointer cancellation never applies a rectangle',()=>{
 const dom=new JSDOM('<main><canvas></canvas></main>'),w=dom.window,host=w.document.querySelector('main'),canvas=host.querySelector('canvas');let clicked=0,selected=0;
 host.getBoundingClientRect=canvas.getBoundingClientRect=()=>({left:0,top:0,width:200,height:200});const controller=attachBoxSelection(canvas,host,{enabled:()=>true,project:()=>[],onSelect:()=>selected++,onClick:()=>clicked++});
 const send=(type,x,y)=>{const e=new w.MouseEvent(type,{bubbles:true,cancelable:true,clientX:x,clientY:y,button:0});Object.defineProperty(e,'pointerId',{value:2});canvas.dispatchEvent(e)};
 send('pointerdown',20,20);send('pointerup',22,22);assert.equal(clicked,1);assert.equal(selected,0);send('pointerdown',0,0);send('pointermove',100,100);send('pointercancel',100,100);send('pointerup',100,100);assert.equal(selected,0);controller.dispose();dom.window.close();
});
