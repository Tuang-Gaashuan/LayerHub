'use strict';
const SlicerBatch=(()=>{
 // A preview is a new value. Never mutate the source project or its results.
 function plan(state,models,amount){
  if(!Number.isInteger(amount)||amount<1||amount>255)throw Error('新增数量须为 1–255 的整数');
  const {plates,activePlate}=state,origin=plates[activePlate],object=origin?.options.copies?.[origin.activeCopy];
  if(!object)throw Error('请先选择一个对象');
  if(SlicerPlates.count(plates)+amount>256)throw Error('工程最多 256 个对象，请减少新增数量');
  if(plates.length>=8)throw Error('工程已有 8 块打印板，请先整理或移除空盘');
  const model=models.find(m=>m.id===object.modelId);
  if(!model||!Number.isInteger(model.triangles)||model.triangles<1||!model.bounds?.size?.every(n=>Number.isFinite(n)&&n>0))throw Error('源模型信息不完整，请重新读取');
  const limit=Math.min(64,Math.floor(150000/model.triangles));
  if(!limit)throw Error('单个模型已超过每盘 150,000 三角面限制');
  const layout=n=>SlicerCopies.arrange(Array.from({length:n},()=>SlicerCopies.duplicate(object)),model.bounds.size,origin.options);
  let capacity=0,low=1,high=limit;
  while(low<=high){const mid=Math.floor((low+high)/2);try{layout(mid);capacity=mid;low=mid+1}catch{high=mid-1}}
  if(!capacity)throw Error('当前尺寸、角度及支撑留空下，一盘也放不下一个对象；请先检查摆放。不会自动缩小模型');
  const needed=Math.ceil(amount/capacity),free=8-plates.length;
  if(needed>free)throw Error(`需要新增 ${needed} 盘，但工程只剩 ${free} 个盘位；请减少数量或另存工程`);
  const next=SlicerPlates.clone(plates),names=new Set(next.map(p=>p.name)),added=[];
  for(let remaining=amount;remaining>0;){let n=1;while(names.has('打印板 '+String(n).padStart(2,'0')))n++;const name='打印板 '+String(n).padStart(2,'0');names.add(name);
   const copies=layout(Math.min(capacity,remaining)),plate=SlicerPlates.make(name,origin.options),{modelId,...placement}=SlicerCopies.transform(copies[0]);plate.modelId=object.modelId;plate.options.copies=copies;Object.assign(plate.options,placement);plate.activeCopy=0;next.push(plate);added.push({name,count:copies.length});remaining-=copies.length;
  }
  return {plates:next,activePlate:plates.length,added,amount,capacity,filename:model.filename,scale:object.scale,size:model.bounds.size.map(n=>n*object.scale)};
 }
 function mount({host,get,canEdit,apply}){
  let alive=true;const uid=data.user?.id,button=host.querySelector('#sliceBatch');
  const valid=()=>alive&&host.isConnected&&data.user?.id===uid;
  const signature=v=>JSON.stringify(v);
  function sync(){if(valid())button.disabled=!canEdit()||!get()?.state.plates.some(p=>p.modelId)}
  function open(){if(!valid()||!canEdit())return;const initial=get();if(!initial)return;const saved=signature(initial);let preview=null;
   modal('批量复制与自动分盘',`<form id="sliceBatchForm" class="slice-batch-root"><p class="slice-batch-intro">复制选中对象，排入新的打印板。原有打印板与切片结果保留。</p><label class="slice-batch-amount">新增副本数量<input class="input" id="sliceBatchAmount" name="amount" type="number" min="1" max="255" step="1" required value="1"></label><p class="form-help">数量不含原对象。沿用所选对象的尺寸、角度及本盘工艺；保持至少 5 mm 间距，支撑和裙边额外留空。</p><section id="sliceBatchPreview" aria-label="分盘预览" aria-live="polite"></section><p class="form-error" id="sliceBatchError" role="alert"></p><p class="form-help">应用后可一次撤销。新盘需要分别切片、检查和确认打印。</p></form>`,`<button class="button" data-cancel>取消</button><button class="button primary" type="submit" form="sliceBatchForm" id="sliceBatchApply">创建打印板</button>`);
   $('#modal').classList.add('slice-batch-modal');const form=$('#sliceBatchForm'),input=form.elements.amount,submit=$('#sliceBatchApply'),area=$('#sliceBatchPreview'),error=$('#sliceBatchError');
   function update(){preview=null;error.textContent='';try{if(!input.value.trim())throw Error('请填写新增副本数量');preview=plan(initial.state,initial.models,Number(input.value));const number=n=>Number(n.toFixed(2)).toLocaleString('zh-CN');area.innerHTML=`<div class="slice-batch-source"><strong>${esc(preview.filename)}</strong><span>${preview.size.map(number).join(' × ')} mm · ${number(preview.scale*100)}% · 尺寸保持</span></div><div class="slice-batch-total">新增 <strong>${preview.amount}</strong> 个副本，分为 <strong>${preview.added.length}</strong> 盘</div><ol>${preview.added.map(p=>`<li><span>${esc(p.name)}</span><b>${p.count} 个</b></li>`).join('')}</ol>`;submit.textContent=`创建 ${preview.added.length} 块打印板`}catch(e){area.innerHTML='';error.textContent=e.message}submit.disabled=!preview}
   input.oninput=update;update();form.onsubmit=e=>{e.preventDefault();if(!valid()||!form.isConnected||!$('#modal').open)return;if(!canEdit()||signature(get())!==saved){error.textContent='工程、选中对象或工艺已变化，请关闭后重新预览';return}update();if(!preview)return;apply(preview);$('#modal').close();toast(`已新增 ${preview.amount} 个副本、${preview.added.length} 盘，可一次撤销`)};
  }
  button.onclick=open;sync();return {sync,close(){alive=false;button.onclick=null}};
 }
 return {plan,mount};
})();
