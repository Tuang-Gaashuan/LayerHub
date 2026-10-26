import {test} from 'node:test';
import assert from 'node:assert/strict';
import * as THREE from '../web/vendor/three/three.module.min.js';
import {gizmoPatch} from '../web/slicer-gizmo.js';

test('gizmo translation preserves an editable invalid position without adding unsupported Z',()=>{
 const proxy=new THREE.Object3D();proxy.position.set(18.12345,190.98765,12);
 assert.deepEqual(gizmoPatch('translate','XY',proxy),{x:18.123,y:190.988});
 proxy.position.x=4000;assert.equal(gizmoPatch('translate','X',proxy).x,1000);
});
test('gizmo rotations export the same Euler XYZ contract as the mesh and slicer',()=>{
 const proxy=new THREE.Object3D();proxy.rotation.set(Math.PI/2,-Math.PI/4,Math.PI/3);
 assert.deepEqual(gizmoPatch('rotate','X',proxy),{rotation:[90,-45,60]});
});
test('each scale handle produces a positive uniform scale within the saved-state limits',()=>{
 const proxy=new THREE.Object3D();proxy.scale.set(1.2,1.8,2.4);
 assert.deepEqual(gizmoPatch('scale','X',proxy),{scale:1.2});assert.deepEqual(gizmoPatch('scale','Y',proxy),{scale:1.8});assert.deepEqual(gizmoPatch('scale','Z',proxy),{scale:2.4});
 proxy.scale.set(-2,0,30);assert.equal(gizmoPatch('scale','X',proxy).scale,.01);assert.equal(gizmoPatch('scale','Z',proxy).scale,10);
});
