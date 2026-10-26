'use strict';
const SlicerSelection=(()=>{
 function pick(current,index,event,length,anchor=index){
  if(!Number.isInteger(index)||index<0||index>=length)return current;
  const additive=event.ctrlKey||event.metaKey,set=new Set(additive?current:[]);
  if(event.shiftKey){for(let i=Math.min(anchor,index);i<=Math.max(anchor,index);i++)if(i>=0&&i<length)set.add(i)}
  else if(additive&&set.has(index)){if(set.size>1)set.delete(index)}else set.add(index);
  return [...set].sort((a,b)=>a-b);
 }
 function quarter(copies,indices,axis){
  if(!Number.isInteger(axis)||axis<0||axis>2)throw Error((globalThis.I18n?.t('旋转轴无效')??'旋转轴无效'));
  const selected=new Set(indices);return copies.map((p,i)=>{const next=SlicerCopies.clone(p);if(selected.has(i)){const value=next.rotation[axis]+90;next.rotation[axis]=Math.round((((value+180)%360+360)%360-180)*1000)/1000}return next});
 }
 function translate(copies,indices,x,y){
  if(![x,y].every(Number.isFinite))throw Error((globalThis.I18n?.t('移动位置无效')??'移动位置无效'));
  const selected=new Set(indices),items=copies.filter((_,i)=>selected.has(i));if(!items.length)throw Error((globalThis.I18n?.t('请先选择模型')??'请先选择模型'));
  const cx=items.reduce((n,p)=>n+p.x,0)/items.length,cy=items.reduce((n,p)=>n+p.y,0)/items.length,dx=x-cx,dy=y-cy;
  const next=copies.map((p,i)=>{const value=SlicerCopies.clone(p);if(selected.has(i)){value.x=Math.round((p.x+dx)*1000)/1000;value.y=Math.round((p.y+dy)*1000)/1000;if(Math.abs(value.x)>1000||Math.abs(value.y)>1000)throw Error((globalThis.I18n?.t('整组移动超出编辑位置范围')??'整组移动超出编辑位置范围'))}return value});return next;
 }
 return {pick,quarter,translate};
})();
