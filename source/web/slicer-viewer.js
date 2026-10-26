import * as THREE from './vendor/three/three.module.min.js';
import { OrbitControls } from './vendor/three/OrbitControls.js';
import { STLLoader } from './vendor/three/STLLoader.js';
import {createGizmo} from './slicer-gizmo.js';
import {projectedBounds,attachBoxSelection} from './slicer-box.js';
import {facePlacement} from './slicer-face.js';

export function createViewer(host,{onBounds,onTransform,onSelect,onSelectMany,onGesture,onFace}={}){
 const scene=new THREE.Scene();scene.background=new THREE.Color('#f1f4fa');
 const renderer=new THREE.WebGLRenderer({antialias:true,alpha:false});renderer.setPixelRatio(Math.min(devicePixelRatio,2));renderer.shadowMap.enabled=true;renderer.outputColorSpace=THREE.SRGBColorSpace;host.append(renderer.domElement);
 const camera=new THREE.PerspectiveCamera(37,1,.1,3000);camera.up.set(0,0,1);camera.position.set(425,-300,340);
 const controls=new OrbitControls(camera,renderer.domElement);controls.target.set(128,128,16);controls.enableDamping=true;controls.minDistance=45;controls.maxDistance=1000;controls.maxPolarAngle=Math.PI*.49;
 scene.add(new THREE.HemisphereLight(0xffffff,0x9bacc5,3));const light=new THREE.DirectionalLight(0xffffff,3.3);light.position.set(90,-80,350);light.castShadow=true;light.shadow.mapSize.set(1024,1024);light.shadow.camera.left=-300;light.shadow.camera.right=300;light.shadow.camera.top=300;light.shadow.camera.bottom=-300;scene.add(light);
 const plate=new THREE.Mesh(new THREE.BoxGeometry(256,256,2),new THREE.MeshStandardMaterial({color:'#dce3ef',roughness:.8}));plate.position.set(128,128,-1.5);plate.receiveShadow=true;scene.add(plate);
 const grid=new THREE.GridHelper(256,16,0x9aaac4,0xc4cfdf);grid.rotation.x=Math.PI/2;grid.position.set(128,128,.05);scene.add(grid);
 const excluded=new THREE.Mesh(new THREE.PlaneGeometry(20.5,256),new THREE.MeshBasicMaterial({color:'#c4cddb',transparent:true,opacity:.65,side:THREE.DoubleSide}));excluded.position.set(10.25,128,.1);scene.add(excluded);
 const axes=new THREE.AxesHelper(22);axes.position.set(5,5,.4);scene.add(axes);
 let pathGroup=null,faceGhost=null,faceHighlight=null;
 let mesh=null,sources=new Map(),objects=[],active=0,options=null,selection=[],frame=0,closed=false,editing=true,toolMode='select';
 const orbitLeft=controls.mouseButtons.LEFT;
 const orbitRight=controls.mouseButtons.RIGHT;
 const raycaster=new THREE.Raycaster(),pointer=new THREE.Vector2();
 const gizmo=createGizmo(scene,camera,renderer.domElement,{onChange:patch=>onTransform?.(patch),onGesture:dragging=>{controls.enabled=!dragging;onGesture?.(dragging)}});
 function resize(){if(closed)return;const w=Math.max(host.clientWidth,100),h=Math.max(host.clientHeight,250);renderer.setSize(w,h,false);camera.aspect=w/h;camera.updateProjectionMatrix()}
 const resizeObserver=new ResizeObserver(resize);resizeObserver.observe(host);resize();
 function tick(){if(closed)return;frame=requestAnimationFrame(tick);controls.update();renderer.render(scene,camera)}tick();
 function clearFace(){if(faceGhost){scene.remove(faceGhost);faceGhost.material.dispose();faceGhost=null}if(faceHighlight){scene.remove(faceHighlight);faceHighlight.geometry.dispose();faceHighlight.material.dispose();faceHighlight=null}}
 function clearCopies(){clearFace();gizmo.sync(null,null);if(mesh)scene.remove(mesh);for(const m of objects){m.material.dispose();m.children[0].material.dispose()}objects=[];mesh=null}
 function freeModel(){clearCopies();for(const source of sources.values()){source.geometry.dispose();source.edges.dispose()}sources.clear()}
 function set(o,selected=active,indices=[selected]){clearFace();selection=[...indices];
  clearPaths();options={...o,rotation:[...o.rotation]};active=selected;
  if(Array.isArray(o.copies)&&!o.copies.length){clearCopies();onBounds?.({size:[0,0,0],valid:false,issues:[],reason:(globalThis.I18n?.t('空打印板 · 导入 STL 或从其他盘移入对象。')??'空打印板 · 导入 STL 或从其他盘移入对象。')});return}
  const copies=o.copies?.length?o.copies:[o],assets=copies.map(p=>sources.get(Number(p.modelId)||0));
  if(assets.some(s=>!s)){clearCopies();onBounds?.({size:[0,0,0],valid:false,issues:[],reason:(globalThis.I18n?.t('正在读取盘内模型，未就绪前不能切片。')??'正在读取盘内模型，未就绪前不能切片。')});return}
  if(objects.length!==copies.length||objects.some((m,i)=>m.geometry!==assets[i].geometry)){
   clearCopies();mesh=new THREE.Group();scene.add(mesh);
   objects=copies.map((p,i)=>{const source=assets[i],m=new THREE.Mesh(source.geometry,new THREE.MeshStandardMaterial({color:o.color,metalness:.05,roughness:.66}));m.userData.index=i;m.castShadow=true;m.receiveShadow=true;m.add(new THREE.LineSegments(source.edges,new THREE.LineBasicMaterial({color:'#94a5bf'})));mesh.add(m);return m})
  }
  const boxes=[];
  for(let i=0;i<objects.length;i++){const m=objects[i],p=copies[i];m.position.set(0,0,0);m.scale.setScalar(p.scale);m.rotation.set(...p.rotation.map(v=>v*Math.PI/180));m.updateMatrixWorld(true);let b=new THREE.Box3().setFromObject(m,true);m.position.set(p.x-(b.min.x+b.max.x)/2,p.y-(b.min.y+b.max.y)/2,-b.min.z);m.updateMatrixWorld(true);b=new THREE.Box3().setFromObject(m,true);boxes.push({min:b.min.toArray(),max:b.max.toArray()})}
  const issues=SlicerCopies.inspectBounds(boxes,o),bad=new Set(issues.map(item=>item.index));
  objects.forEach((m,i)=>{m.material.color.set(bad.has(i)?'#e58983':o.color);m.children[0].material.color.set(bad.has(i)?'#cf5148':(selection.includes(i)?'#3159ed':'#94a5bf'))});
  if(selection.length>1&&toolMode==='translate'){
   const chosen=selection.filter(i=>objects[i]),box=new THREE.Box3();for(const i of chosen)box.union(new THREE.Box3().setFromObject(objects[i],true));
   const center={x:chosen.reduce((n,i)=>n+copies[i].x,0)/chosen.length,y:chosen.reduce((n,i)=>n+copies[i].y,0)/chosen.length,scale:1,rotation:[0,0,0]};gizmo.sync(objects[active],center,box);
  }else gizmo.sync(selection.length===1?objects[active]:null,selection.length===1?copies[active]:null,null);
  const box=new THREE.Box3().setFromObject(mesh,true);onBounds?.({min:box.min.toArray(),max:box.max.toArray(),size:box.getSize(new THREE.Vector3()).toArray(),valid:issues.length===0,issues,reason:(globalThis.I18n?.t('对象越界或间距不足，请调整位置或自动排版。')??'对象越界或间距不足，请调整位置或自动排版。')})
 }
 function loadModels(entries){
  const next=new Map();
  try{for(const {id,buffer}of entries){const geometry=new STLLoader().parse(buffer);geometry.computeBoundingBox();const center=geometry.boundingBox.getCenter(new THREE.Vector3());geometry.translate(-center.x,-center.y,-center.z);geometry.computeBoundingBox();geometry.computeVertexNormals();next.set(id,{geometry,edges:new THREE.EdgesGeometry(geometry,32)})}}
  catch(e){for(const source of next.values()){source.geometry.dispose();source.edges.dispose()}throw e}
  freeModel();sources=next;if(options)set(options,active,selection)
 }
 function load(buffer){loadModels([{id:0,buffer}])}
 const pickAt=e=>{if(!editing||pathGroup||!mesh||gizmo.dragging||e.button!==0)return;const r=renderer.domElement.getBoundingClientRect();pointer.set((e.clientX-r.left)/r.width*2-1,-(e.clientY-r.top)/r.height*2+1);raycaster.setFromCamera(pointer,camera);const hits=raycaster.intersectObjects(objects,false);if(hits.length){active=hits[0].object.userData.index;onSelect?.(active,{ctrlKey:e.ctrlKey,metaKey:e.metaKey,shiftKey:e.shiftKey})}};
 function pickFace(e){if(!editing||pathGroup||e.button!==0||selection.length!==1)return;const r=renderer.domElement.getBoundingClientRect();pointer.set((e.clientX-r.left)/r.width*2-1,-(e.clientY-r.top)/r.height*2+1);raycaster.setFromCamera(pointer,camera);const hit=raycaster.intersectObjects(objects,false)[0];if(!hit)return;if(hit.object!==objects[active]){clearFace();onFace?.({error:(globalThis.I18n?.t('请点击当前选中对象的表面；其他对象保持')??'请点击当前选中对象的表面；其他对象保持')});return}clearFace();try{const m=hit.object,p=options.copies?.[active]||options,attribute=m.geometry.attributes.position,indices=m.geometry.index?[0,1,2].map(i=>m.geometry.index.getX(hit.faceIndex*3+i)):[0,1,2].map(i=>hit.faceIndex*3+i),candidate=facePlacement(attribute.array,indices,p);faceGhost=new THREE.Mesh(m.geometry,new THREE.MeshStandardMaterial({color:'#16a079',transparent:true,opacity:.38,depthWrite:false,side:THREE.DoubleSide}));faceGhost.scale.setScalar(p.scale);faceGhost.rotation.set(...candidate.rotation.map(THREE.MathUtils.degToRad));faceGhost.updateMatrixWorld(true);const b=new THREE.Box3().setFromObject(faceGhost,true);faceGhost.position.set(p.x-(b.min.x+b.max.x)/2,p.y-(b.min.y+b.max.y)/2,-b.min.z);scene.add(faceGhost);const g=new THREE.BufferGeometry();g.setAttribute('position',new THREE.Float32BufferAttribute(candidate.triangle.flat(),3));g.applyMatrix4(m.matrixWorld);faceHighlight=new THREE.Mesh(g,new THREE.MeshBasicMaterial({color:'#2563eb',side:THREE.DoubleSide,depthTest:false,transparent:true,opacity:.8}));faceHighlight.renderOrder=5;scene.add(faceHighlight);onFace?.(candidate)}catch(error){onFace?.({error:error.message})}}
 const down=e=>{if(toolMode==='face')pickFace(e);else if(toolMode!=='box')pickAt(e)};renderer.domElement.addEventListener('pointerdown',down);
 const boxSelection=attachBoxSelection(renderer.domElement,host,{enabled:()=>editing&&toolMode==='box'&&!pathGroup&&!!mesh,project:()=>{const r=renderer.domElement.getBoundingClientRect();camera.updateMatrixWorld();return objects.map(m=>projectedBounds(new THREE.Box3().setFromObject(m,true),camera,r.width,r.height))},onSelect:(indices,event)=>onSelectMany?.(indices,event),onGesture,onClick:pickAt});
 function view(name){const target=controls.target,subject=name==='selected'?objects[active]:pathGroup||mesh;if((name==='fit'||name==='selected')&&subject){const box=new THREE.Box3().setFromObject(subject,true);if(box.isEmpty())return;const c=box.getCenter(new THREE.Vector3()),d=Math.max(...box.getSize(new THREE.Vector3()).toArray(),15);controls.target.copy(c);camera.position.set(c.x+d*1.8,c.y-d*2,c.z+d*1.6);controls.update();return}if(name==='iso')controls.target.set(128,128,16);if(name==='top')camera.position.set(target.x,target.y-0.01,420);else if(name==='front')camera.position.set(target.x,-360,100);else camera.position.set(425,-300,340);controls.update()}
 function clearPaths(){if(pathGroup){scene.remove(pathGroup);pathGroup.traverse(o=>{o.geometry?.dispose();o.material?.dispose()});pathGroup=null}if(mesh)mesh.visible=true}
 function paths(segments,hidden,colors){clearPaths();gizmo.enable(false);if(mesh)mesh.visible=false;pathGroup=new THREE.Group();scene.add(pathGroup);for(let kind=0;kind<colors.length;kind++){if(hidden.has(kind))continue;const points=[];for(const p of segments){if(p[6]===kind)points.push(...p.slice(0,6))}if(!points.length)continue;const g=new THREE.BufferGeometry();g.setAttribute('position',new THREE.Float32BufferAttribute(points,3));pathGroup.add(new THREE.LineSegments(g,new THREE.LineBasicMaterial({color:colors[kind],transparent:kind===0,opacity:kind===0?.3:1})))}}
 return {load,loadModels,set,view,paths,clearPaths,tool(mode){boxSelection.cancel();toolMode=mode;controls.mouseButtons.LEFT=editing&&['box','face'].includes(mode)?null:orbitLeft;controls.mouseButtons.RIGHT=editing&&mode==='face'?THREE.MOUSE.ROTATE:orbitRight;gizmo.mode(['box','face'].includes(mode)?'select':mode);if(options)set(options,active,selection)},editing(value){if(!value){boxSelection.cancel();clearFace()}editing=value;controls.mouseButtons.LEFT=value&&['box','face'].includes(toolMode)?null:orbitLeft;controls.mouseButtons.RIGHT=value&&toolMode==='face'?THREE.MOUSE.ROTATE:orbitRight;gizmo.enable(value&&!pathGroup)},finish(){boxSelection.cancel();gizmo.finish()},dispose(){boxSelection.dispose();closed=true;cancelAnimationFrame(frame);resizeObserver.disconnect();controls.dispose();clearPaths();freeModel();gizmo.dispose();renderer.domElement.removeEventListener('pointerdown',down);scene.traverse(o=>{o.geometry?.dispose();if(o.material)o.material.dispose?.()});renderer.dispose();renderer.domElement.remove()}};
}
