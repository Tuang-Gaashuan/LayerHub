'use strict';
// File preparation is a draft. Only an explicit submission can create jobs;
// reading a receipt or restoring a page never dispatches a print.
const UploadPreparation=(()=>{
 const storeKey=uid=>'layerhub.pendingImport.'+uid;
 const model=v=>String(v||'').toLowerCase().replace(/^bambu\s*(lab)?/,'').replace(/[\s_-]/g,'');
 const color=v=>/^#?[0-9a-f]{6}([0-9a-f]{2})?$/i.test(v||'')?'#'+v.replace('#','').slice(0,6):'#b7c0ce';
 const identifier=()=>{const b=new Uint8Array(20);crypto.getRandomValues(b);return Array.from(b,n=>n.toString(16).padStart(2,'0')).join('')};
 function prior(uid){try{return JSON.parse(sessionStorage.getItem(storeKey(uid))||'null')}catch{return null}}
 function exit(file){FileIntake.close();if(picked===file)picked=null;BoardUI.resetUpload();$('#fileInput').value='';$('#modal').close();navigate('queues')}
 function open(file){
  const review=FileIntake.forFile(file);if(!review||!data.user)return;
  const uid=data.user.id,all=review.sliced.info.plates,slice=review.sliced.info;
  let pending=prior(uid),ids=FileIntake.selection(file).map(p=>p.id),active=ids[0]||all[0].id;
  const preferred=Number($('#pickPrinter')?.value)||0,printers=data.printers.filter(p=>p.enabled&&model(printerModel(p)?.name||p.model)===model(slice.model));
  let pid=printers.some(p=>p.id===preferred)?preferred:printers[0]?.id||0,choices={},busy=false,controller=null,mayRetry=false;
  if(pending?.sha256===review.sha256){ids=pending.items.map(i=>i.plate);pid=pending.items[0].printer;choices=Object.fromEntries(pending.items.map(i=>[i.plate,i.spoolId]))}
  modal(I18n.t('确认这一盘，接着开始'),I18n.html`<div id="uploadPreparation"><div class="up-file"><span class="file-icon">${icon('files')}</span><div><strong>${esc(file.name)}</strong><p>${esc(slice.model||I18n.t('机型未识别'))} · ${I18n.msg`共 ${all.length} 个打印盘，全部打印需 ${all.length} 次`}</p></div></div><div class="up-layout"><section class="up-preview"><div class="up-preview-top"><strong>本次打印内容</strong><button class="text-link" id="upAll">选择全部</button></div><div id="upArt"></div><div id="upPlates" class="up-plates"></div><p class="form-help">每盘独立成为任务；下一盘开始前，仍需取件并确认打印板已清空。</p></section><section class="up-settings"><label><span class="field-label">目标打印机</span><select class="input" id="upPrinter">${printers.map(p=>`<option value="${p.id}" ${p.id===pid?'selected':''}>${esc(p.name)}</option>`).join('')||I18n.html('<option value="0">没有匹配机型的设备</option>')}</select></label><p id="upPrinterNote" class="form-help"></p><div id="upMaterials"></div><details><summary>打印选项</summary><label class="check-row"><input id="upLevel" type="checkbox" checked>打印前自动调平</label><p class="form-help">复杂供料可使用原有的高级准备流程。</p></details><p id="upChecks" class="form-error" role="status"></p><label id="upClearRow" class="up-clear"><input id="upClear" type="checkbox">已清空打印板，并确认打印机可以开始</label><p id="upStatus" class="form-help" role="status"></p><button id="upRead" class="button" hidden>核对上次提交结果</button><button id="upRetry" class="button" hidden>用原编号重试上传</button></section></div></div>`,I18n.html`<button class="button" id="upCancel">取消本次上传</button><div class="up-submit"><span id="upTotal"></span><button class="button" id="upAdvanced">高级准备</button><button class="button secondary" id="upQueue">仅加入队列</button><button class="button primary" id="upStart">确认并开始打印</button></div>`,{onDismiss:leave});
  $('#modal').classList.add('upload-preparation-modal');const root=$('#uploadPreparation');
  const current=()=>root.isConnected&&$('#modal').open&&data.user?.id===uid&&picked===file;
  const ps=()=>all.filter(p=>ids.includes(p.id)),printer=()=>data.printers.find(p=>p.id===pid),spool=id=>data.spools.find(s=>s.id===id);
  const candidates=p=>data.spools.filter(s=>Workspace.visible(s,data)&&s.state==='available'&&s.mount?.printerId===pid&&s.material.toLowerCase()===String(p.filaments[0]?.material).toLowerCase()&&(printer()?.configuration==='ams'?['1','2','3','4'].includes(s.mount.slot):s.mount.slot==='external'));
  const direct=()=>ps().length===1&&printer()?.state==='idle'&&Remote.printerFresh(printer())&&!Workspace.queue(data,pid).rows.length&&!printer()?.warning&&!Remote.deviceBlocked(pid)&&data.capabilities?.printStart;
  const options=p=>{const sp=spool(choices[p.id]),slot=Number(sp?.mount?.slot)-1;return {plate:p.id,useAMS:printer()?.configuration==='ams',mapping:[{filament:p.filaments[0].index,amsId:Number.isFinite(slot)?Math.floor(slot/4):0,slotId:Number.isFinite(slot)?slot%4:0,spoolId:sp?.id||'',grams:p.filaments[0].grams}],bedLeveling:$('#upLevel').checked,flowCalibration:false,timelapse:false}};
  function errors(){
   if(!ps().length)return [I18n.t('请先选择要打印的盘')];
   if(ps().length>16)return [I18n.t('每批最多选择 16 个打印盘')];
   if(!printer()?.enabled||model(printerModel(printer())?.name||printer().model)!==model(slice.model))return [I18n.t('切片机型与目标设备不匹配')];
   const problems=[],sums={};
   for(const p of ps()){
    if(p.multiNozzle||p.filaments.length!==1){problems.push(I18n.t('多耗材或多喷嘴盘请使用高级准备'));continue}
    if(!Number.isFinite(p.seconds)||p.seconds<=0||Math.ceil(p.seconds/60)>100000)problems.push(I18n.t('切片未提供有效时长，请使用高级准备核对'));
    const sp=candidates(p).find(s=>s.id===choices[p.id]),g=p.filaments[0].grams;
    if(!sp){problems.push(I18n.t('请为每盘选择本机已装载的实际料卷'));continue}
    if(!Number.isInteger(g)||g<1||g>10000)problems.push(I18n.t('切片未提供有效用量，请使用高级准备核对'));
    sums[sp.id]=(sums[sp.id]||0)+g;
    problems.push(...PrintPrepare.checks({id:0},printer(),slice,p,printer().configuration==='ams',{[p.filaments[0].index]:{spoolId:sp.id,grams:g}}).filter(r=>r.blocking).map(r=>r.title));
   }
   if(Object.entries(sums).some(([id,g])=>g>Workspace.available(data,spool(id))))problems.push(I18n.t('同一料卷的合计预留超过可用余量'));
   return [...new Set(problems)];
  }
  function paint(){
   if(!current())return;
   const selected=ps(),p=all.find(p=>p.id===active)||all[0],image=review.sliced.previews?.[p.id],safe=typeof image==='string'&&/^data:image\/png;base64,[A-Za-z0-9+/=]+$/.test(image)&&image.length<140000;
   $('#upArt').innerHTML=`${safe?`<img src="${image}" alt="${esc(I18n.msg`打印盘 ${p.id}`)}">`:`<div class="up-no-image">${icon('box')}<p>${I18n.t('暂无预览')}</p></div>`}<span>PLATE ${String(p.id).padStart(2,'0')}</span><div>${p.seconds?Math.ceil(p.seconds/60)+' '+I18n.t('分钟'):I18n.t('用时未提供')} · ${p.filaments.reduce((n,f)=>n+Number(f.usedGrams||f.grams||0),0).toFixed(2)} g</div>`;
   const lock=busy||!!pending;
   $('#upPlates').innerHTML=all.map(p=>`<button class="up-plate ${ids.includes(p.id)?'selected':''}" data-up-plate="${p.id}" aria-pressed="${ids.includes(p.id)}" ${lock?'disabled':''}><strong>${ids.includes(p.id)?'✓ ':''}${esc(I18n.msg`打印盘 ${p.id}`)}</strong><small>${p.seconds?Math.ceil(p.seconds/60)+' '+I18n.t('分钟'):I18n.t('用时未提供')} · ${p.filaments.reduce((n,f)=>n+Number(f.usedGrams||f.grams||0),0).toFixed(2)} g</small></button>`).join('');
   $('#upMaterials').innerHTML=selected.map(p=>{const list=candidates(p);if(!list.some(s=>s.id===choices[p.id]))choices[p.id]=list.find(s=>color(s.color).toLowerCase()===color(p.filaments[0]?.color).toLowerCase())?.id||'';return `<div class="up-material"><strong>${esc(I18n.msg`打印盘 ${p.id}`)} · ${p.filaments.reduce((n,f)=>n+(f.usedGrams||f.grams||0),0).toFixed(2)} g</strong><p class="form-help">${I18n.t('文件原色')} ${p.filaments.map(f=>`<i class="up-color" style="background:${color(f.color)}"></i> ${esc(f.material)}`).join(' / ')}</p>${p.filaments.length!==1?`<p>${I18n.t('多耗材或多喷嘴盘请使用高级准备')}</p>`:list.map(sp=>`<label class="up-spool"><input type="radio" name="up-spool-${p.id}" data-up-spool="${p.id}" value="${esc(sp.id)}" ${choices[p.id]===sp.id?'checked':''} ${lock?'disabled':''}><i class="up-color" style="background:${color(sp.color)}"></i><span><strong>${esc(sp.colorName)} · ${esc(sp.material)}</strong><small>${esc(sp.label)} · ${esc(sp.mount.slot==='external'?I18n.t('外挂'):'AMS '+sp.mount.slot)} · ${Workspace.available(data,sp)} g ${I18n.t('可分配')}</small></span></label>`).join('')||`<p>${I18n.t('没有匹配的已装载料卷，请到耗材库登记或确认装载。')}</p>`}</div>`}).join('');
   const problems=errors(),start=direct();$('#upChecks').textContent=problems[0]||I18n.t('设备与供料检查通过，提交时会再次检查。');
   $('#upPrinterNote').textContent=printer()?printer().name+' · '+(start?I18n.t('当前可以准备开始'):I18n.t('本次将加入设备队列，不会自动打印')):I18n.t('没有匹配机型的设备');
   $('#upAll').disabled=lock;$('#upPrinter').disabled=lock;$('#upLevel').disabled=lock;$('#upClear').disabled=lock;
   $('#upClearRow').hidden=!start||!!pending;$('#upQueue').hidden=!start;$('#upQueue').disabled=lock||!!problems.length;
   $('#upAdvanced').disabled=lock;$('#upStart').disabled=lock||!!problems.length||start&&!$('#upClear').checked;
   $('#upStart').textContent=busy?I18n.t('正在提交…'):start?I18n.t('确认并开始打印'):I18n.t('确认加入队列');
   $('#upTotal').textContent=I18n.msg`本次 ${selected.length} 盘 · 预留 ${selected.reduce((n,p)=>n+p.filaments.reduce((n,f)=>n+(f.grams||0),0),0)} g`;
   $('#upCancel').textContent=pending?I18n.t('返回任务列表'):I18n.t('取消本次上传');$('#upRead').hidden=!pending;$('#upRead').disabled=busy;$('#upRetry').hidden=!mayRetry;$('#upRetry').disabled=busy;
   if(pending&&!busy)$('#upStatus').textContent=I18n.t('上次提交结果待核对。读取结果不会再次启动打印；重新打开页面也不会继续执行。');
  }
  function persist(){sessionStorage.setItem(storeKey(uid),JSON.stringify(pending))}
  function leave(){controller?.abort();controller=null;uploading=false;exit(file);if(pending)toast(I18n.t('已返回队列；本次提交结果仍需核对，不会自动重试。'))}
  async function read(){
   if(busy||!pending)return;busy=true;paint();try{const r=await Remote.request('/api/jobs/import-receipt?requestId='+encodeURIComponent(pending.requestId));if(!current())return;
    if(r.found){sessionStorage.removeItem(storeKey(uid));pending=null;exit(file);await Remote.refresh({renderPage:!$('#modal').open});toast(I18n.t('已找到上次保存的任务，请在队列查看；没有重新启动打印。'))}
    else{mayRetry=pending.sha256===review.sha256&&(!pending.stage||pending.stage==='upload');$('#upStatus').textContent=I18n.t('尚未找到导入记录；可沿用原编号重试，不会重复建任务。')}
   }catch(e){if(current())$('#upStatus').textContent=I18n.system(e.message)}finally{busy=false;if(current()){paint();if(mayRetry)$('#upStatus').textContent=I18n.t('尚未找到导入记录；可沿用原编号重试，不会重复建任务。')}}
  }
  async function submit(startRequested,retry=false){
   if(busy||!current()||pending&&!retry)return;
   if(!retry&&errors().length)return;
   if(startRequested&&(!direct()||!$('#upClear').checked))return;
   busy=true;mayRetry=false;paint();let deadline;
   try{
    await Remote.refresh({renderPage:false});if(!current())return;
    if(!retry){const problem=errors()[0];if(problem)throw Error(problem);if(startRequested&&!direct())throw Error(I18n.t('设备状态或队列已变化，请重新确认安排'));
     pending={requestId:identifier(),sha256:review.sha256,filename:file.name,items:ps().map(p=>({plate:p.id,printer:pid,estimate:Math.ceil(p.seconds/60),grams:p.filaments[0].grams,spoolId:choices[p.id]})),options:ps().map(options),note:$('#note')?.value.trim()||'',stage:'upload'};persist();
    }
    if(pending.sha256!==review.sha256)throw Error(I18n.t('请先核对上一批导入结果'));
    const body=new FormData();body.append('file',file);body.append('batch',JSON.stringify({requestId:pending.requestId,items:pending.items}));body.append('note',pending.note||'');
    controller=new AbortController();deadline=setTimeout(()=>controller.abort(),600000);uploading=true;$('#upStatus').textContent=I18n.t('正在上传并创建独立任务…');
    const response=await fetch('/api/upload/batch',{method:'POST',credentials:'same-origin',headers:{'X-CSRF-Token':Remote.csrfToken(),'Accept-Language':I18n.language()},body,signal:controller.signal});
    const result=await response.json();if(!current())return;
    if(!response.ok||result?.ok!==true||!Array.isArray(result.receipt?.jobIds)||result.receipt.jobIds.length!==pending.items.length)throw Error(result.error||I18n.t('上传结果未确认，请先核对结果；重试会沿用原编号。'));
    // A replay is an existing batch, never a fresh permission to start.
    pending.stage='prepare';pending.jobIds=result.receipt.jobIds;persist();await Remote.refresh({renderPage:false});if(!current())return;
    for(let n=0;n<pending.jobIds.length;n++){
     const j=data.jobs.find(j=>j.id===pending.jobIds[n]);if(!j||j.user!==data.user.name||j.state!=='queued'||j.printer!==pid||j.sha256&&j.sha256!==review.sha256)throw Error(I18n.t('已创建任务的状态已变化，请在队列中核对'));
     if(pending.options?.[n])await Remote.request('/api/jobs/options',{jobId:j.id,options:pending.options[n]});if(!current())return;
    }
    if(startRequested&&!retry&&!result.receipt.replayed){
     await Remote.refresh({renderPage:false});if(!current())return;const id=pending.jobIds[0],j=data.jobs.find(j=>j.id===id),p=printer(),q=Workspace.queue(data,pid);
     if(!j||j.state!=='queued'||p?.state!=='idle'||!Remote.printerFresh(p)||Remote.deviceBlocked(pid)||q.rows[0]?.job.id!==id||!$('#upClear').checked)throw Error(I18n.t('任务已入队；设备或队首变化，请在队列重新确认开始'));
     pending.stage='dispatch';persist();$('#upStatus').textContent=I18n.t('正在请求传输与启动；请等待设备状态确认。');await Remote.request('/api/jobs/dispatch',{jobId:id});if(!current())return;
     sessionStorage.removeItem(storeKey(uid));pending=null;picked=null;BoardUI.resetUpload();uploading=false;$('#modal').close();await Remote.refresh();PrinterControl.open(pid);toast(I18n.t('已提交传输与启动，请在设备控制台查看真实进度。'));
    }else{sessionStorage.removeItem(storeKey(uid));pending=null;uploading=false;exit(file);await Remote.refresh();toast(I18n.t('任务已分别加入队列，没有自动开始打印。'))}
   }catch(e){if(current())$('#upStatus').textContent=(pending?I18n.t('提交结果需要核对；不会自动重试或再次启动。')+' ':'')+I18n.system(e.message)}
   finally{clearTimeout(deadline);controller=null;uploading=false;busy=false;if(current())paint()}
  }
  $('#upPrinter').onchange=e=>{pid=Number(e.target.value);choices={};$('#upClear').checked=false;paint()};
  $('#upPlates').onclick=e=>{const b=e.target.closest('[data-up-plate]');if(!b||busy||pending)return;const id=Number(b.dataset.upPlate);active=id;ids=ids.includes(id)?ids.filter(x=>x!==id):[...ids,id];FileIntake.select(file,ids);$('#upClear').checked=false;paint()};
  $('#upAll').onclick=()=>{if(busy||pending)return;ids=ids.length===all.length?[]:all.slice(0,16).map(p=>p.id);FileIntake.select(file,ids);$('#upClear').checked=false;paint()};
  $('#upMaterials').onchange=e=>{if(!e.target.dataset.upSpool||busy||pending)return;choices[e.target.dataset.upSpool]=e.target.value;$('#upClear').checked=false;paint()};$('#upClear').onchange=paint;
  $('#upCancel').onclick=leave;$('#upRead').onclick=read;$('#upRetry').onclick=()=>submit(false,true);$('#upQueue').onclick=()=>submit(false);$('#upStart').onclick=()=>submit(direct());
  $('#upAdvanced').onclick=()=>{if(busy||pending)return;$('#modal').close();Remote.upload()};paint();
 }
 return {open,exit};
})();
