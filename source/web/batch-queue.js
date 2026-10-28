'use strict';
const BatchQueue=(()=>{
 const key=uid=>'layerhub.pendingImport.'+uid;
 const model=v=>String(v||'').toLowerCase().replace(/^bambu\s*(lab)?/,'').replace(/[\s_-]/g,'');
 const freshID=()=>{const bytes=new Uint8Array(20);crypto.getRandomValues(bytes);return [...bytes].map(n=>n.toString(16).padStart(2,'0')).join('')};
 function read(uid){try{const p=JSON.parse(sessionStorage.getItem(key(uid))||'null');return p&&/^[a-zA-Z0-9-]{16,80}$/.test(p.requestId)&&/^[a-f0-9]{64}$/i.test(p.sha256)&&Array.isArray(p.items)&&p.items.length>=1&&p.items.length<=16?p:null}catch{return null}}
 const forget=uid=>{try{sessionStorage.removeItem(key(uid))}catch{}};
 async function open(file,plates){
  const review=FileIntake.forFile(file);if(!review||!data.user)return;
  const uid=data.user.id,prior=read(uid),sha=review.sha256;
  if(prior&&prior.sha256!==sha){
   modal(I18n.t('上一批导入结果待核对'),`<p>${esc(prior.filename||'')}</p><p>${I18n.t('请先核对上一批结果；如需重试，请重新选择原文件，系统会沿用原导入编号。')}</p>`,`<button class="button" data-cancel>${I18n.t('返回')}</button><button class="button primary" id="readPendingBatch">${I18n.t('核对结果')}</button>`);
   $('#readPendingBatch').onclick=async()=>{try{const r=await Remote.request('/api/jobs/import-receipt?requestId='+encodeURIComponent(prior.requestId));if(data.user?.id!==uid)return;if(r.found){forget(uid);$('#modal').close();await Remote.refresh();toast(I18n.t('已找到上次导入记录，请查看我的任务'))}else toast(I18n.t('尚未找到记录，请选择原文件后使用相同编号重试'))}catch(e){toast(I18n.system(e.message))}};
   return;
  }
  if(prior)plates=prior.items.map(x=>review.sliced.info.plates.find(p=>p.id===x.plate));
  if(!plates.length||plates.length>16||plates.some(p=>!p||p.multiNozzle||p.filaments.length!==1))return toast(I18n.t('批量入队仅接受单耗材盘；多耗材盘请单独创建并完成供料准备'));
  const selectedPrinter=Number($('#pickPrinter')?.value)||0,printers=data.printers.filter(p=>p.enabled&&model(p.model||printerModel(p)?.name)===model(review.sliced.info.model));
  const initialPrinter=prior?.items[0]?.printer||(printers.some(p=>p.id===selectedPrinter)?selectedPrinter:0);
  let items=plates.map(p=>prior?.items.find(x=>x.plate===p.id)||{plate:p.id,printer:initialPrinter,estimate:p.seconds?Math.ceil(p.seconds/60):null,grams:p.filaments[0].grams||null,spoolId:''});
  let pending=prior?.editable?null:prior,busy=false,requestId=prior?.requestId||freshID();
  modal(I18n.t('逐盘确认并加入队列'),`<div id="batchQueueRoot"><p class="scheduling-filename">${esc(file.name)}</p><p>${I18n.t('每个盘独立成为任务；这里只预留库存，不传输或启动打印。')}</p><p class="batch-pending" role="status" ${prior?'':'hidden'}>${I18n.t('上次结果待核对，已保留原安排；重试使用同一编号，不会重复创建任务。')}</p><form id="batchQueueForm"><label><span class="field-label">${I18n.t('本批次目标设备')}</span><select class="input" name="printer"><option value="0">${I18n.t('稍后分配')}</option>${printers.map(p=>`<option value="${p.id}" ${p.id===initialPrinter?'selected':''}>${esc(p.name)}</option>`).join('')}</select></label><div class="batch-plates">${plates.map(p=>{const item=items.find(x=>x.plate===p.id),preview=review.sliced.previews?.[p.id];return `<section class="batch-plate" data-batch-plate="${p.id}"><div class="batch-plate-title">${typeof preview==='string'&&/^data:image\/png;base64,[A-Za-z0-9+/=]+$/.test(preview)?`<img src="${preview}" alt="${esc(I18n.msg`打印盘 ${p.id}`)}">`:icon('box')}<div><strong>${I18n.msg`打印盘 ${p.id}`}</strong><small>${esc(p.filaments[0].material)} · ${esc(p.filaments[0].color||I18n.t('颜色未提供'))}</small></div></div><div class="batch-numbers"><label>${I18n.t('预计时长（分钟）')}<input class="input" name="estimate-${p.id}" type="number" min="1" max="100000" required value="${item.estimate??''}"></label><label>${I18n.t('预留耗材（g）')}<input class="input" name="grams-${p.id}" type="number" min="${Math.max(1,p.filaments[0].grams)}" max="10000" required value="${item.grams??''}"></label></div><button type="button" class="batch-spool button" data-batch-spool="${p.id}"></button></section>`}).join('')}</div><p class="form-help" id="batchTotals"></p><p class="form-error" id="batchError" role="status"></p><div class="batch-actions"><button type="button" class="button" data-cancel>${I18n.t('返回')}</button><button type="button" class="button" id="batchRead" ${prior?'':'hidden'}>${I18n.t('核对结果')}</button><button type="submit" class="button primary" id="batchSubmit">${prior?I18n.t('用原编号重试上传'):I18n.t('确认批量入队')}</button></div></form></div>`);
  const root=$('#batchQueueRoot'),form=$('#batchQueueForm'),error=$('#batchError'),button=$('#batchSubmit');
  root.querySelector('.scheduling-filename').insertAdjacentHTML('afterend',`<p class="batch-run-summary">${I18n.msg`本批次包含 ${plates.length} 个独立打印任务，需分别确认启动。`}</p>`);
  const current=()=>root.isConnected&&data.user?.id===uid;
  const readInputs=()=>{if(pending)return;items=items.map(item=>({...item,printer:Number(form.elements.printer.value),estimate:Number(form.elements['estimate-'+item.plate].value),grams:Number(form.elements['grams-'+item.plate].value)}))};
  function totals(exclude){const sums={};for(const item of items)if(item.plate!==exclude&&item.spoolId)sums[item.spoolId]=(sums[item.spoolId]||0)+item.grams;return sums}
  function validate(){
   if(!form.reportValidity())return I18n.t('请补齐每盘的时长和用量');
   const sums=totals();
   for(const item of items){const plate=plates.find(p=>p.id===item.plate),sp=data.spools.find(s=>s.id===item.spoolId&&Workspace.visible(s,data));if(!sp||sp.state!=='available'||sp.material.toLowerCase()!==plate.filaments[0].material.toLowerCase())return I18n.t('请为每盘选择材质一致的可用料卷');if(!Number.isInteger(item.estimate)||!Number.isInteger(item.grams)||item.estimate<1||item.estimate>100000||item.grams>10000||item.grams<Math.max(1,plate.filaments[0].grams))return I18n.t('时长或预留用量无效');if(sums[sp.id]>Workspace.available(data,sp))return I18n.t('同一料卷的合计预留超过可用余量')}
   return '';
  }
  function paint(){
   for(const item of items){const sp=data.spools.find(s=>s.id===item.spoolId&&Workspace.visible(s,data)),target=root.querySelector(`[data-batch-spool="${item.plate}"]`);target.innerHTML=sp?`<i style="background:${/^#[0-9a-f]{6}$/i.test(sp.color)?sp.color:'#b7c0ce'}"></i><span>${esc(sp.colorName)} · ${esc(sp.material)}<small>${esc(sp.label)}</small></span>`:`${icon('plus')} ${I18n.t('选择这盘使用的料卷颜色')}`;target.disabled=busy||!!pending}
   root.querySelector('#batchTotals').textContent=I18n.msg`共 ${items.length} 盘，预计 ${items.reduce((n,x)=>n+(Number(x.estimate)||0),0)} 分钟，预留 ${items.reduce((n,x)=>n+(Number(x.grams)||0),0)} g`;
   form.querySelectorAll('input,select').forEach(input=>input.disabled=busy||!!pending);button.disabled=busy;button.textContent=busy?I18n.t('正在上传并保存…'):pending?I18n.t('用原编号重试上传'):I18n.t('确认批量入队');root.querySelector('#batchRead').hidden=!pending;root.querySelector('#batchRead').disabled=busy;root.querySelector('.batch-pending').hidden=!pending;
  }
  function pickSpool(id){
   if(pending||busy)return;readInputs();const item=items.find(x=>x.plate===id),plate=plates.find(p=>p.id===id),dialog=document.createElement('dialog');dialog.className='batch-spool-dialog';
   dialog.innerHTML=`<form id="batchSpoolForm"><h2>${I18n.msg`打印盘 ${id}`}</h2><input type="hidden" name="printer" value="${item.printer}"><input type="hidden" name="grams" value="${item.grams||1}">${SpoolPicker.markup()}<p class="form-error" role="status"></p><div class="batch-actions"><button type="button" class="button" data-back>${I18n.t('取消')}</button><button class="button primary">${I18n.t('使用这卷耗材')}</button></div></form>`;document.body.append(dialog);
   const close=()=>{observer.disconnect();if(dialog.open)dialog.close();dialog.remove()};const observer=new MutationObserver(()=>{if(!current()||!$('#modal').open)close()});observer.observe($('#modal'),{childList:true,subtree:true,attributes:true,attributeFilter:['open']});
   dialog.querySelector('[data-back]').onclick=close;dialog.oncancel=e=>{e.preventDefault();close()};dialog.onclick=e=>{if(e.target===dialog)close()};
   SpoolPicker.bind(dialog.querySelector('form'),{spoolId:item.spoolId},{material:plate.filaments[0].material,reserved:totals(id)});
   dialog.querySelector('form').onsubmit=e=>{e.preventDefault();const chosen=new FormData(e.target).get('spoolId');if(!chosen)return;item.spoolId=String(chosen);close();error.textContent='';paint()};dialog.showModal();
  }
  form.oninput=()=>{readInputs();paint()};form.onchange=()=>{readInputs();paint()};root.querySelectorAll('[data-batch-spool]').forEach(b=>b.onclick=()=>pickSpool(Number(b.dataset.batchSpool)));
  form.querySelector('[data-cancel]').onclick=()=>$('#modal').close();
  async function accepted(receipt,original=false){
   if(data.user?.id!==uid)return;uploading=false;forget(uid);pending=null;if(picked===file){picked=null;BoardUI.resetUpload()}if(current())$('#modal').close();await Remote.refresh({renderPage:!$('#modal').open});toast(original?I18n.t('已找到先前保存的批次，请到我的任务核对原安排'):I18n.msg`已确认批次，共 ${receipt.jobIds.length} 个任务`);
  }
  root.querySelector('#batchRead').onclick=async()=>{if(!pending||busy)return;try{const result=await Remote.request('/api/jobs/import-receipt?requestId='+encodeURIComponent(pending.requestId));if(!current())return;if(result.found)await accepted(result.receipt);else error.textContent=I18n.t('尚未找到记录；可用原编号重试，不会重复创建任务。')}catch(e){if(current())error.textContent=I18n.system(e.message)}};
  form.onsubmit=async e=>{
   e.preventDefault();if(busy||!current())return;readInputs();error.textContent='';
   if(!pending){const problem=validate();if(problem){error.textContent=problem;return}}
   busy=true;paint();let deadline,controller;
   try{
    await Remote.refresh({renderPage:false});if(!current())return;
    if(!pending){const problem=validate();if(problem)throw Error(problem);pending={requestId,sha256:sha,filename:file.name,items:items.map(x=>({...x})),note:$('#note')?.value.trim()||''};try{sessionStorage.setItem(key(uid),JSON.stringify(pending))}catch{pending=null;throw Error(I18n.t('浏览器无法保存导入恢复编号，请允许站点存储后重试。'))}}
    const body=new FormData();body.append('file',file);body.append('batch',JSON.stringify({requestId:pending.requestId,items:pending.items}));body.append('note',pending.note);
    controller=new AbortController();deadline=setTimeout(()=>controller.abort(),600000);uploading=true;
    const response=await fetch('/api/upload/batch',{method:'POST',credentials:'same-origin',headers:{'X-CSRF-Token':Remote.csrfToken(),'Accept-Language':I18n.language()},body,signal:controller.signal});
    const result=await response.json();if(!current())return;
    if(response.ok&&result?.ok===true&&Array.isArray(result.receipt?.jobIds))await accepted(result.receipt);
    else if(response.status>=400&&response.status<500&&![401,403].includes(response.status)&&result?.ok===false){
     const check=await Remote.request('/api/jobs/import-receipt?requestId='+encodeURIComponent(pending.requestId));if(!current())return;
     if(check.found){await accepted(check.receipt,true);return}
     // Keep the same key even when correcting a definitively rejected draft.
     // An older slow request can then never create a second batch later.
     const draft={...pending,editable:true};try{sessionStorage.setItem(key(uid),JSON.stringify(draft))}catch{}pending=null;throw Error(I18n.system(result.error));
    }
    else throw Error(I18n.t('上传结果未确认，请先核对结果；重试会沿用原编号。'));
   }catch(e){if(current())error.textContent=pending?I18n.t('上传结果未确认，请先核对结果；重试会沿用原编号。'):I18n.system(e.message)}
   finally{clearTimeout(deadline);uploading=false;busy=false;if(current())paint()}
  };
  paint();
 }
 return {open};
})();
