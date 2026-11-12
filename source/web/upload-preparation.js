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
  let pid=printers.some(p=>p.id===preferred)?preferred:printers[0]?.id||0,choices={},sources={},busy=false,controller=null,mayRetry=false,first=ids[0],startOffered=false,statusText='',watcher=null,lastAvailability='';
  if(pending?.sha256===review.sha256){ids=pending.items.map(i=>i.plate);pid=pending.items[0].printer;choices=Object.fromEntries(pending.items.map(i=>[i.plate,i.spoolId]));for(const o of pending.options||[])if(o.mapping?.length===2)for(const m of o.mapping)choices[o.plate+':'+m.filament]=m.spoolId;first=ids[0];sources=Object.fromEntries((pending.options||[]).map(o=>[o.plate,o.useAMS?'ams':'external']))}
  modal(I18n.t('确认这一盘，接着开始'),I18n.html`<div id="uploadPreparation"><div class="up-file"><span class="file-icon">${icon('files')}</span><div><strong>${esc(file.name)}</strong><p>${esc(slice.model||I18n.t('机型未识别'))} · ${I18n.msg`共 ${all.length} 个打印盘，全部打印需 ${all.length} 次`}</p></div></div><div class="up-layout"><section class="up-preview"><div class="up-preview-top"><strong>本次打印内容</strong><button class="text-link" id="upAll">选择全部</button></div><div id="upArt"></div><div id="upPlates" class="up-plates"></div><p class="form-help">每盘独立成为任务；下一盘开始前，仍需取件并确认打印板已清空。</p></section><section class="up-settings"><label><span class="field-label">目标打印机</span><select class="input" id="upPrinter">${printers.map(p=>`<option value="${p.id}" ${p.id===pid?'selected':''}>${esc(p.name)}</option>`).join('')||I18n.html('<option value="0">没有匹配机型的设备</option>')}</select></label><p id="upPrinterNote" class="form-help"></p><div id="upMaterials"></div><label id="upFirstRow"><span class="field-label">先打印哪一盘</span><select class="input" id="upFirst"></select><small class="form-help">只启动所选首盘；其余盘入队，取件并清空打印板后再开始。</small></label><details><summary>打印选项</summary><label class="check-row"><input id="upLevel" type="checkbox" checked>打印前自动调平</label><p class="form-help">复杂供料可使用原有的高级准备流程。</p></details><p id="upChecks" class="form-error" role="status"></p><label id="upClearRow" class="up-clear"><input id="upClear" type="checkbox">已清空打印板，并确认打印机可以开始</label><p id="upStatus" class="form-help" role="status"></p><button id="upRead" class="button" hidden>核对上次提交结果</button><button id="upRetry" class="button" hidden>用原编号重试上传</button></section></div></div>`,I18n.html`<button class="button" id="upCancel">取消本次上传</button><div class="up-submit"><span id="upTotal"></span><button class="button" id="upAdvanced">高级准备</button><button class="button secondary" id="upQueue">仅加入队列</button><button class="button primary" id="upStart">确认并开始打印</button></div>`,{onDismiss:leave});
  $('#modal').classList.add('upload-preparation-modal');const root=$('#uploadPreparation');
  const current=()=>root.isConnected&&$('#modal').open&&data.user?.id===uid&&picked===file;
  const ps=()=>{const selected=all.filter(p=>ids.includes(p.id));return [...selected.filter(p=>p.id===first),...selected.filter(p=>p.id!==first)]},printer=()=>data.printers.find(p=>p.id===pid),spool=id=>data.spools.find(s=>s.id===id);
  const usesAMS=p=>(sources[p.id]||(PrintPrepare.softTPU(p)?'external':printer()?.configuration==='ams'?'ams':'external'))==='ams';
  const dual=p=>PrintPrepare.dualPlate(printer(),p),key=(p,f)=>dual(p)?p.id+':'+f.index:p.id;
  const candidates=(p,f=p.filaments[0])=>data.spools.filter(s=>Workspace.visible(s,data)&&s.state==='available'&&s.mount?.printerId===pid&&s.material.toLowerCase()===String(f?.material).toLowerCase()&&((dual(p)?PrintPrepare.dualSource(f)==='ams':usesAMS(p))?['1','2','3','4'].includes(s.mount.slot):s.mount.slot===PrintPrepare.externalSlot(printer(),p,f)));
  const direct=()=>ps().length>0&&printer()?.state==='idle'&&Remote.printerFresh(printer())&&!Workspace.queue(data,pid).rows.length&&!printer()?.warning&&!Remote.deviceBlocked(pid)&&data.capabilities?.printStart;
  const options=p=>({plate:p.id,useAMS:dual(p)||usesAMS(p),mapping:p.filaments.map(f=>{const sp=spool(choices[key(p,f)]),slot=Number(sp?.mount?.slot)-1;return {...(dual(p)?{source:PrintPrepare.dualSource(f)}:{}),filament:f.index,amsId:Number.isFinite(slot)?Math.floor(slot/4):0,slotId:Number.isFinite(slot)?slot%4:0,spoolId:sp?.id||'',grams:f.grams}}),bedLeveling:$('#upLevel').checked,flowCalibration:false,timelapse:false});
  const availability=()=>JSON.stringify([!!direct(),printer()?.state,printer()?.warning||'',Workspace.queue(data,pid).rows.map(r=>r.job.id)]);
  function errors(){
   if(!ps().length)return [I18n.t('请先选择要打印的盘')];
   if(ps().length>16)return [I18n.t('每批最多选择 16 个打印盘')];
   if(!printer()?.enabled||model(printerModel(printer())?.name||printer().model)!==model(slice.model))return [I18n.t('切片机型与目标设备不匹配')];
   const problems=[],sums={};
   for(const p of ps()){
    if((p.multiNozzle||p.filaments.length!==1)&&!dual(p)){problems.push(I18n.t('当前供料组合尚未适配，请核对喷嘴与切片设置'));continue}
    if(!Number.isFinite(p.seconds)||p.seconds<=0||Math.ceil(p.seconds/60)>100000)problems.push(I18n.t('切片未提供有效时长，请使用高级准备核对'));
    const mapped={};for(const f of p.filaments){
    const sp=candidates(p,f).find(s=>s.id===choices[key(p,f)]),g=f.grams;
    if(!sp){problems.push(I18n.t('请为每盘选择本机已装载的实际料卷'));continue}
    if(!Number.isInteger(g)||g<1||g>10000)problems.push(I18n.t('切片未提供有效用量，请使用高级准备核对'));
    sums[sp.id]=(sums[sp.id]||0)+g;
    mapped[f.index]={spoolId:sp.id,grams:g};
    }
    problems.push(...PrintPrepare.checks({id:0},printer(),slice,p,dual(p)||usesAMS(p),mapped).filter(r=>r.blocking).map(r=>r.title));
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
   $('#upMaterials').innerHTML=selected.map(p=>{
    const supported=p.filaments.length===1&&!p.multiNozzle||dual(p);
    return `<div class="up-material"><strong>${esc(I18n.msg`打印盘 ${p.id}`)} · ${p.filaments.reduce((n,f)=>n+(f.usedGrams||f.grams||0),0).toFixed(2)} g</strong>${!dual(p)?`<label><span class="field-label">${I18n.t('本盘供料来源')}</span><select data-up-source="${p.id}" ${lock?'disabled':''}><option value="external" ${!usesAMS(p)?'selected':''}>${I18n.t('外置料架')}</option>${printer()?.configuration==='ams'?`<option value="ams" ${usesAMS(p)?'selected':''}>AMS</option>`:''}</select></label>`:''}${!supported?`<p>${I18n.t('当前供料组合尚未适配，请核对喷嘴与切片设置')}</p>`:p.filaments.map(f=>{
     const k=key(p,f),list=candidates(p,f);if(!list.some(s=>s.id===choices[k]))choices[k]=list.find(s=>color(s.color).toLowerCase()===color(f.color).toLowerCase())?.id||'';
     return `${dual(p)?`<p><strong>${esc(I18n.t(PrintPrepare.dualSource(f)==='ams'?'主热端 · AMS':'辅助热端 · 外挂料'))}</strong> · ${f.grams} g</p>`:''}<p class="form-help">${I18n.t('文件原色')} <i class="up-color" style="background:${color(f.color)}"></i> ${esc(f.material)}</p>${list.map(sp=>`<label class="up-spool"><input type="radio" name="up-spool-${k}" data-up-spool="${k}" value="${esc(sp.id)}" ${choices[k]===sp.id?'checked':''} ${lock?'disabled':''}><i class="up-color" style="background:${color(sp.color)}"></i><span><strong>${esc(sp.colorName)} · ${esc(sp.material)}</strong><small>${esc(sp.label)} · ${esc(sp.mount.slot.startsWith('external')?I18n.t('外置料架'):'AMS '+sp.mount.slot)} · ${Workspace.available(data,sp)} g ${I18n.t('可分配')}</small></span></label>`).join('')||`<p>${I18n.t('没有匹配的已装载料卷，请到耗材库登记或确认装载。')}</p>`}`;
    }).join('')}</div>`;
   }).join('');
   if(!selected.some(p=>p.id===first))first=selected[0]?.id;
   $('#upFirstRow').hidden=selected.length<2;$('#upFirst').innerHTML=selected.map(p=>`<option value="${p.id}" ${p.id===first?'selected':''}>${esc(I18n.msg`打印盘 ${p.id}`)}</option>`).join('');$('#upFirst').disabled=lock;
   const problems=errors(),start=direct();startOffered=start;lastAvailability=availability();$('#upChecks').textContent=problems[0]||I18n.t('设备与供料检查通过，提交时会再次检查。');
   const queued=Workspace.queue(data,pid).rows.length;
   const reason=start?(selected.length>1?I18n.msg`确认后先打印第 ${first} 盘，其余盘进入队列`:I18n.t('确认后自动上传、保存准备并开始，无需返回队列')):printer()?.state!=='idle'?I18n.t('设备当前未空闲，本次仅入队；就绪后在队列开始'):queued?I18n.t('本机已有等待任务，本次排在其后，不会插队启动'):I18n.t('设备状态或控制权限暂不满足启动条件，本次仅入队');
   $('#upPrinterNote').textContent=printer()?printer().name+' · '+reason:I18n.t('没有匹配机型的设备');
   $('#upAll').disabled=lock;$('#upPrinter').disabled=lock;$('#upLevel').disabled=lock;$('#upClear').disabled=lock;
   $('#upClearRow').hidden=!start||!!pending;$('#upQueue').hidden=!start;$('#upQueue').disabled=lock||!!problems.length;
   $('#upAdvanced').hidden=!!file.inspectionId||selected.some(dual);$('#upAdvanced').disabled=lock;$('#upStart').disabled=lock||!!problems.length||start&&!$('#upClear').checked;
   $('#upStart').textContent=busy?I18n.t('正在提交…'):start?(selected.length>1?I18n.msg`加入 ${selected.length} 盘并开始首盘`:I18n.t('确认并开始打印')):I18n.t('确认加入队列');
   $('#upTotal').textContent=I18n.msg`本次 ${selected.length} 盘 · 预留 ${selected.reduce((n,p)=>n+p.filaments.reduce((n,f)=>n+(f.grams||0),0),0)} g`;
   $('#upCancel').textContent=pending?I18n.t('返回任务列表'):I18n.t('取消本次上传');$('#upRead').hidden=!pending;$('#upRead').disabled=busy;$('#upRetry').hidden=!mayRetry;$('#upRetry').disabled=busy;
   $('#upStatus').textContent=statusText||(pending&&!busy?I18n.t('上次提交结果待核对。读取结果不会再次启动打印；重新打开页面也不会继续执行。'):'');
  }
  function persist(){sessionStorage.setItem(storeKey(uid),JSON.stringify(pending))}
  function leave(){clearInterval(watcher);controller?.abort();controller=null;uploading=false;exit(file);if(pending)toast(I18n.t('已返回队列；本次提交结果仍需核对，不会自动重试。'))}
  async function read(){
   if(busy||!pending)return;busy=true;statusText=I18n.t('正在核对上次提交结果…');paint();try{const r=await Remote.request('/api/jobs/import-receipt?requestId='+encodeURIComponent(pending.requestId));if(!current())return;
    if(r.found){sessionStorage.removeItem(storeKey(uid));pending=null;exit(file);await Remote.refresh({renderPage:!$('#modal').open});toast(I18n.t('已找到上次保存的任务，请在队列查看；没有重新启动打印。'))}
    else{mayRetry=pending.sha256===review.sha256&&(!pending.stage||pending.stage==='upload');statusText=I18n.t('尚未找到导入记录；可沿用原编号重试，不会重复建任务。');$('#upStatus').textContent=statusText}
   }catch(e){if(current())statusText=I18n.system(e.message)}finally{busy=false;if(current())paint()}
  }
  async function submit(startRequested,retry=false){
   if(busy||!current()||pending&&!retry)return;
   if(!retry&&errors().length)return;
   if(startRequested&&(!direct()||!$('#upClear').checked)){statusText=I18n.t('设备状态或队列已变化，尚未提交，请重新确认安排');$('#upClear').checked=false;paint();return;}
   busy=true;mayRetry=false;statusText=I18n.t('正在重新检查设备与耗材…');paint();let deadline;
   try{
    await Remote.refresh({renderPage:false});if(!current())return;
    if(!retry){const problem=errors()[0];if(problem)throw Error(problem);if(startRequested&&!direct())throw Error(I18n.t('设备状态或队列已变化，请重新确认安排'));
     pending={requestId:identifier(),sha256:review.sha256,filename:file.name,items:ps().map(p=>({plate:p.id,printer:pid,estimate:Math.ceil(p.seconds/60),grams:p.filaments[0].grams,spoolId:choices[key(p,p.filaments[0])]})),options:ps().map(options),note:$('#note')?.value.trim()||'',stage:'upload'};persist();
    }
    if(pending.sha256!==review.sha256)throw Error(I18n.t('请先核对上一批导入结果'));
    const batch={requestId:pending.requestId,items:pending.items,...(ps().some(dual)?{options:pending.options}:{})};
    const body=new FormData();if(!file.inspectionId)body.append('file',file);body.append('batch',JSON.stringify(batch));body.append('note',pending.note||'');
    controller=new AbortController();deadline=setTimeout(()=>controller.abort(),600000);uploading=true;statusText=I18n.t('正在上传并创建独立任务…');$('#upStatus').textContent=statusText;
    const response=file.inspectionId?null:await fetch('/api/upload/batch',{method:'POST',credentials:'same-origin',headers:{'X-CSRF-Token':Remote.csrfToken(),'Accept-Language':I18n.language()},body,signal:controller.signal});
    const result=file.inspectionId?await Remote.request('/api/files/inspections/import',{inspectionId:file.inspectionId,batch,note:pending.note||''}):await response.json();if(!current())return;
    if(response&&!response.ok||result?.ok!==true||!Array.isArray(result.receipt?.jobIds)||result.receipt.jobIds.length!==pending.items.length)throw Error(result.error||I18n.t('上传结果未确认，请先核对结果；重试会沿用原编号。'));
    // A replay is an existing batch, never a fresh permission to start.
    pending.stage='prepare';pending.jobIds=result.receipt.jobIds;persist();statusText=I18n.t('上传完成，正在保存每盘耗材与打印选项…');$('#upStatus').textContent=statusText;await Remote.refresh({renderPage:false});if(!current())return;
    for(let n=0;n<pending.jobIds.length;n++){
     const j=data.jobs.find(j=>j.id===pending.jobIds[n]);if(!j||j.user!==data.user.name||j.state!=='queued'||j.printer!==pid||j.sha256&&j.sha256!==review.sha256)throw Error(I18n.t('已创建任务的状态已变化，请在队列中核对'));
     if(pending.options?.[n]&&!batch.options)await Remote.request('/api/jobs/options',{jobId:j.id,options:pending.options[n]});if(!current())return;
    }
    if(startRequested&&!retry&&!result.receipt.replayed){
     await Remote.refresh({renderPage:false});if(!current())return;const id=pending.jobIds[0],j=data.jobs.find(j=>j.id===id),p=printer(),q=Workspace.queue(data,pid);
     if(!j||j.state!=='queued'||p?.state!=='idle'||!Remote.printerFresh(p)||Remote.deviceBlocked(pid)||q.rows[0]?.job.id!==id||!$('#upClear').checked)throw Error(I18n.t('任务已入队；设备或队首变化，请在队列重新确认开始'));
     pending.stage='dispatch';persist();statusText=I18n.t('准备已保存，正在请求传输与启动；无需再次点击准备打印。');$('#upStatus').textContent=statusText;await Remote.request('/api/jobs/dispatch',{jobId:id});if(!current())return;
     sessionStorage.removeItem(storeKey(uid));pending=null;picked=null;BoardUI.resetUpload();uploading=false;$('#modal').close();await Remote.refresh();PrinterControl.open(pid);toast(I18n.t('已提交传输与启动，请在设备控制台查看真实进度。'));
    }else{sessionStorage.removeItem(storeKey(uid));pending=null;uploading=false;exit(file);await Remote.refresh();toast(I18n.t('任务已分别加入队列，没有自动开始打印。'))}
   }catch(e){if(current())statusText=(pending?I18n.t('提交结果需要核对；不会自动重试或再次启动。')+' ':'')+I18n.system(e.message)}
   finally{clearTimeout(deadline);controller=null;uploading=false;busy=false;if(current())paint()}
  }
  $('#upPrinter').onchange=e=>{pid=Number(e.target.value);choices={};sources={};$('#upClear').checked=false;paint()};
  $('#upPlates').onclick=e=>{const b=e.target.closest('[data-up-plate]');if(!b||busy||pending)return;const id=Number(b.dataset.upPlate);active=id;ids=ids.includes(id)?ids.filter(x=>x!==id):[...ids,id];FileIntake.select(file,ids);$('#upClear').checked=false;paint()};
  $('#upAll').onclick=()=>{if(busy||pending)return;ids=ids.length===all.length?[]:all.slice(0,16).map(p=>p.id);FileIntake.select(file,ids);$('#upClear').checked=false;paint()};
  $('#upFirst').onchange=e=>{if(busy||pending)return;first=Number(e.target.value);$('#upClear').checked=false;statusText='';paint()};
  $('#upMaterials').onchange=e=>{if(!e.target.dataset.upSpool||busy||pending)return;choices[e.target.dataset.upSpool]=e.target.value;$('#upClear').checked=false;paint()};$('#upClear').onchange=paint;
  root.addEventListener('change',event=>{const input=event.target;if(!input.matches('[data-up-source]'))return;if(busy||pending)return;sources[Number(input.dataset.upSource)]=input.value;delete choices[Number(input.dataset.upSource)];$('#upClear').checked=false;paint()});
  $('#upCancel').onclick=leave;$('#upRead').onclick=read;$('#upRetry').onclick=()=>submit(false,true);$('#upQueue').onclick=()=>submit(false);$('#upStart').onclick=()=>submit(startOffered);
  $('#upAdvanced').onclick=()=>{if(busy||pending)return;$('#modal').close();Remote.upload()};paint();
  // Workspace polling continues while this dialog is open. Refresh only when
  // availability changes, preserving scroll and draft choices on ordinary telemetry.
  watcher=setInterval(()=>{
   if(!current()){clearInterval(watcher);return}
   if(busy||pending||availability()===lastAvailability)return;
   $('#upClear').checked=false;
   statusText=direct()?I18n.t('设备已可启动，请重新确认打印板已清空后开始。'):I18n.t('设备或队列状态已变化，请查看当前安排；不会自动开始打印。');
   paint();
  },1000);
 }
 return {open,exit};
})();
