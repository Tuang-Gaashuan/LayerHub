import * as THREE from './vendor/three/three.module.min.js';

// Use the selected triangle's outward normal. Reject faces that would put
// another part of the mesh below that plane instead of guessing a bottom.
export function facePlacement(positions,indices,placement){
 if(!positions||positions.length%3||indices.length!==3||!indices.every(i=>Number.isInteger(i)&&i>=0&&i*3+2<positions.length))throw Error('无法读取选中表面，请换一个面');
 const vertices=indices.map(i=>new THREE.Vector3().fromArray(positions,i*3)),normal=new THREE.Vector3().crossVectors(vertices[1].clone().sub(vertices[0]),vertices[2].clone().sub(vertices[0]));
 const area=normal.length()/2*placement.scale**2;
 if(!Number.isFinite(area)||area<.001)throw Error('选中的三角面太小或退化，请选择较大的平面');
 if(!Number.isFinite(placement.scale)||placement.scale<=0||placement.rotation?.length!==3||!placement.rotation.every(Number.isFinite))throw Error('当前摆放无效，请先修正');
 const start=new THREE.Quaternion().setFromEuler(new THREE.Euler(...placement.rotation.map(THREE.MathUtils.degToRad),'XYZ'));
 normal.normalize().applyQuaternion(start);
 const delta=new THREE.Quaternion().setFromUnitVectors(normal,new THREE.Vector3(0,0,-1)),orientation=delta.multiply(start).normalize(),m=new THREE.Matrix4().makeRotationFromQuaternion(orientation).elements;
 // THREE's display-oriented Euler singularity threshold is too broad for a
 // supporting plane. Retain near-singular angles instead of snapping roll.
 const cy=Math.hypot(m[0],m[4]),angles=cy>1e-12?[Math.atan2(-m[9],m[10]),Math.atan2(m[8],cy),Math.atan2(-m[4],m[0])]:[Math.atan2(m[6],m[5]),Math.sign(m[8])*Math.PI/2,0];
 const rotation=angles.map(n=>THREE.MathUtils.radToDeg(n));
 const plane=vertices[0].clone().multiplyScalar(placement.scale).applyQuaternion(orientation).z,point=new THREE.Vector3(),box=new THREE.Box3();let min=Infinity;
 for(let i=0;i<positions.length;i+=3){point.fromArray(positions,i).multiplyScalar(placement.scale).applyQuaternion(orientation);if(![point.x,point.y,point.z].every(Number.isFinite))throw Error('模型包含无效坐标');min=Math.min(min,point.z);box.expandByPoint(point)}
 if(plane-min>.02)throw Error('其他部位会先接触打印板；请换一个外侧平面');
 return {rotation,area,size:box.getSize(new THREE.Vector3()).toArray(),triangle:vertices.map(v=>v.toArray())};
}
