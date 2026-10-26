'use strict';
const SlicerPlates=(()=>{
 const clone=s=>JSON.parse(JSON.stringify(s));
 const make=(name,options)=>{const {modelId,...process}=clone(options);return {id:crypto.randomUUID(),name,modelId:0,options:{...process,copies:[]},activeCopy:0,taskId:0}};
 function fromState(s){return s.plates?.length?clone(s.plates):[{id:crypto.randomUUID(),name:(globalThis.I18n?.t('打印板 01')??'打印板 01'),modelId:s.modelId,options:clone(s.options),activeCopy:s.activeCopy||0,taskId:0}]}
 function state(plates,activePlate){const anchor=plates.find(p=>p.modelId);return anchor?{formatVersion:4,modelId:anchor.modelId,options:clone(anchor.options),activeCopy:anchor.activeCopy,plates:clone(plates),activePlate}:null}
 function layout(o){const c=clone(o);if(c.copies?.length&&!c.copies.some(p=>Object.keys(p.process||{}).length))for(const p of c.copies){delete p.instanceId;if(p.process&&!Object.keys(p.process).length)delete p.process}if(c.copies?.length)for(const key of ['x','y','rotation','scale'])delete c[key];return JSON.stringify(c,(_,v)=>v&&typeof v==='object'&&!Array.isArray(v)?Object.keys(v).sort().reduce((r,k)=>(r[k]=v[k],r),{}):v)}
 const count=plates=>plates.reduce((n,p)=>n+(p.modelId?(p.options.copies?.length||1):0),0);
 function move(plates,source,target,index,models){return moveMany(plates,source,target,[index],models)}
 function moveMany(plates,source,target,indices,models){
  if(source===target||!plates[source]||!plates[target])throw Error((globalThis.I18n?.t('请选择另一块打印板')??'请选择另一块打印板'));
  const next=clone(plates),a=next[source],b=next[target],selected=[...new Set(indices)].sort((a,b)=>a-b),moving=selected.map(i=>a.options.copies[i]);
  if(!moving.length||selected.some(i=>!Number.isInteger(i)||i<0)||moving.some(o=>!o))throw Error((globalThis.I18n?.t('请先选择有效对象')??'请先选择有效对象'));
  const copies=[...b.options.copies,...moving.map(clone)],size=p=>{const m=models.find(m=>m.id===p.modelId);if(!m)throw Error((globalThis.I18n?.t('源模型不存在')??'源模型不存在'));return m.bounds.size};
  const triangles=copies.reduce((n,p)=>n+(models.find(m=>m.id===p.modelId)?.triangles||0),0);
  if(copies.length>64||triangles>150000)throw Error((globalThis.I18n?.t('目标盘超过 64 个对象或 150,000 个三角面')??'目标盘超过 64 个对象或 150,000 个三角面'));
  b.options.copies=SlicerCopies.arrange(copies,size,b.options);b.modelId=b.options.copies[0].modelId;b.activeCopy=copies.length-1;b.taskId=0;
  a.options.copies=a.options.copies.filter((_,i)=>!selected.includes(i));a.modelId=a.options.copies[0]?.modelId||0;a.activeCopy=Math.max(0,Math.min(a.activeCopy,a.options.copies.length-1));a.taskId=0;
  for(const p of [a,b])if(p.modelId){const {modelId,...placement}=SlicerCopies.transform(p.options.copies[p.activeCopy]);Object.assign(p.options,placement)}
  return next;
 }
 return {clone,make,fromState,state,layout,count,move,moveMany};
})();
