'use strict';
const SlicerCopies=(()=>{
 const transform=p=>({x:p.x,y:p.y,scale:p.scale,rotation:[...p.rotation],...(p.modelId?{modelId:p.modelId}:{})});
 const clone=p=>({...transform(p),...(p.instanceId?{instanceId:p.instanceId}:{}),...(p.process?{process:{...p.process}}:{})});
 const duplicate=p=>({...clone(p),instanceId:crypto.randomUUID()});
 const identify=list=>list.map(p=>p.instanceId?clone(p):duplicate(p));
 function extent(size,p){const [cx,sx,cy,sy,cz,sz]=[Math.cos(p.rotation[0]*Math.PI/180),Math.sin(p.rotation[0]*Math.PI/180),Math.cos(p.rotation[1]*Math.PI/180),Math.sin(p.rotation[1]*Math.PI/180),Math.cos(p.rotation[2]*Math.PI/180),Math.sin(p.rotation[2]*Math.PI/180)];const points=[];for(const a of [-1,1])for(const b of [-1,1])for(const c of [-1,1]){let [x,y,z]=[a*size[0]*p.scale/2,b*size[1]*p.scale/2,c*size[2]*p.scale/2];[x,y]=[cz*x-sz*y,sz*x+cz*y];[x,z]=[cy*x+sy*z,-sy*x+cy*z];[y,z]=[cx*y-sx*z,sx*y+cx*z];points.push([x,y,z])}return [0,1,2].map(k=>Math.max(...points.map(p=>p[k]))-Math.min(...points.map(p=>p[k])))}
 function arrange(copies,size,{brim=false,support=false}={}){const sizeOf=p=>typeof size==='function'?size(p):size;const margin=(brim?5:0)+(support?5:0),gap=5+2*margin,left=20.5+margin,right=256-margin,bottom=margin,top=256-margin;let x=left,y=bottom,row=0;const result=[];for(const p of copies){const [w,h,z]=extent(sizeOf(p),p);if(![w,h,z].every(Number.isFinite)||w>right-left||h>top-bottom||z>256)throw Error((globalThis.I18n?.t('模型尺寸超出可用区域，请缩小或旋转。')??'模型尺寸超出可用区域，请缩小或旋转。'));if(x+w>right+.001){x=left;y+=row+gap;row=0}if(y+h>top+.001)throw Error((globalThis.I18n?.t('这一盘放不下所需数量，请减少副本或调整尺寸。')??'这一盘放不下所需数量，请减少副本或调整尺寸。'));result.push({...clone(p),x:Math.round((x+w/2)*1000)/1000,y:Math.round((y+h/2)*1000)/1000});x+=w+gap;row=Math.max(row,h)}if(result.length){const dx=(right-Math.max(...result.map((p,i)=>p.x+extent(sizeOf(copies[i]),copies[i])[0]/2)))/2,dy=(top-Math.max(...result.map((p,i)=>p.y+extent(sizeOf(copies[i]),copies[i])[1]/2)))/2;result.forEach(p=>{p.x=Math.round((p.x+dx)*1000)/1000;p.y=Math.round((p.y+dy)*1000)/1000})}return result}
 // Actual transformed mesh bounds are used by the viewer. Mark both members
 // of every close pair, including the object encountered first.
 function inspectBounds(boxes,{brim=false,support=false}={}){
  const margin=(brim?5:0)+(support?5:0),gap=5+2*margin;
  const issues=boxes.map(()=>[]);
  boxes.forEach((b,i)=>{if(![...b.min,...b.max].every(Number.isFinite)||b.min[0]<20.5+margin-.001||b.min[1]<margin-.001||b.max[0]>256-margin+.001||b.max[1]>256-margin+.001||b.max[2]>256.001||b.min[2]<-.001)issues[i].push({kind:'bounds',text:(globalThis.I18n?.t('超出可打印区域')??'超出可打印区域')});for(let j=0;j<i;j++){const a=boxes[j];if(b.min[0]<a.max[0]+gap-.001&&b.max[0]>a.min[0]-gap+.001&&b.min[1]<a.max[1]+gap-.001&&b.max[1]>a.min[1]-gap+.001){issues[i].push({kind:'gap',peer:j,text:(globalThis.I18n?.t('与对象 ')??'与对象 ')+(j+1)+(globalThis.I18n?.t(' 间距不足')??' 间距不足')});issues[j].push({kind:'gap',peer:i,text:(globalThis.I18n?.t('与对象 ')??'与对象 ')+(i+1)+(globalThis.I18n?.t(' 间距不足')??' 间距不足')})}}});
  return issues.map((reasons,index)=>({index,reasons})).filter(item=>item.reasons.length);
 }
 return {clone,transform,duplicate,identify,extent,arrange,inspectBounds};
})();
