'use strict';
const PrintPrepare=(()=>{
 let generation=0,resume=null;
 const modelKey=v=>String(v||'').toLowerCase().replace(/^bambu\s*(lab)?/,'').replace(/[\s_-]/g,'');
 const duration=n=>{const m=Math.floor(Number(n)/60);return m>=60?Math.floor(m/60)+' 小时 '+m%60+' 分':m?m+' 分钟':'时间未提供'};
 const weight=n=>Number(n).toFixed(2).replace(/\.?0+$/,'');
 function checks(j,p,slice,plate,ams,choices){
  const rows=[],add=(state,title,detail,blocking=false,action='',actionLabel='')=>rows.push({state,title,detail,blocking,action,actionLabel});
  const target=p?(printerModel(p)?.name||p.model||''):'';
  if(!p)add('blocked','尚未选择目标设备','返回任务安排选择打印机。',true,'plate','查看目标设备');
  else if(!slice.model||modelKey(slice.model)!==modelKey(target))add('blocked','切片机型与目标设备不匹配',(slice.model||'机型未识别')+' → '+target+'，请在切片软件选择目标机型重新切片，然后上传新文件。',true,'plate','查看切片与目标机型');
  else add('ok','切片机型一致',target);
  if(p&&(p.modelId==='x2d'&&!data.capabilities?.x2dSingleNozzlePrint||p.modelId.startsWith('h2')&&p.modelId!=='h2s'))add('pending','打印启动映射尚未开放','这是软件待适配项，无需反复调整设备设置。供料与库存检查通过后可保存准备，暂不能传输或启动。',false,'support','查看适配说明');
  if(p?.modelId==='x2d'&&data.capabilities?.x2dSingleNozzlePrint)add('note','X2D 单喷嘴打印验证',(plate.nozzle?'文件逻辑喷嘴 '+plate.nozzle.logical+' → 物理喷嘴 '+plate.nozzle.physical+'。':'')+'当前仅开放明确物理喷嘴映射的单喷嘴、单耗材盘，使用第一组 AMS；启动时核对实际喷嘴规格、AMS 绑定与耗材预设。');
  if(p?.modelId==='x2d'&&data.capabilities?.x2dSingleNozzlePrint&&(!plate.nozzle||plate.filaments.length!==1))add('blocked','当前文件缺少可用的单喷嘴启动映射','请在 X2D 预设下选择一个喷嘴、一份耗材重新切片，并保留物理喷嘴映射。',true,'plate','查看切片信息');
  if(plate.multiNozzle)add('blocked','该盘包含多喷嘴或混料','当前只支持保存单喷嘴盘的准备；请在切片软件选择单喷嘴重新切片，或等待多喷嘴适配。',true,'support','查看适配说明');
  if(ams&&p?.configuration!=='ams')add('blocked','目标设备没有配置 AMS','在配色与供料中关闭 AMS，或核对设备配置。',true,'mapping','调整供料方式');
  const sums={};let missing=false,invalid=false,changed=false;
  for(const f of plate.filaments){
   const choice=choices[f.index],sp=choice&&spoolById(choice.spoolId),grams=Number(choice?.grams);
   if(!sp){missing=true;continue}
   if(!Number.isInteger(grams)||grams<f.grams||grams>10000||sp.material.toLowerCase()!==f.material.toLowerCase()||sp.mount?.printerId!==p?.id||sp.state!=='available'||(ams?!['1','2','3','4'].includes(sp.mount.slot):sp.mount.slot!=='external'))invalid=true;
   sums[sp.id]=(sums[sp.id]||0)+grams;
   changed ||= !!f.color&&String(f.color).replace('#','').slice(0,6).toLowerCase()!==String(sp.color).replace('#','').slice(0,6).toLowerCase();
  }
  if(missing)add('blocked','还有配色未选择料卷','先到配色与供料选择实际料卷；没有候选时，可进入本机耗材库登记或确认装载。',true,'mapping','去选择料卷');
  else if(invalid)add('blocked','供料位置或预留用量需要调整','在配色与供料核对材质、装载位置和最低预留量；实际换料后同步更新耗材库。',true,'mapping','调整料卷与用量');
  else if(Object.entries(sums).some(([id,g])=>g>Workspace.available(data,spoolById(id),j.id)))add('blocked','可分配余量不足','同一卷对应多个配色时按合计用量检查。请选择余量充足的料卷；记录错误时到耗材库核对。',true,'mapping','重新选择料卷');
  else add('ok','供料与库存预留已匹配',Object.values(sums).reduce((a,b)=>a+b,0)+' g 将作为本任务预留量。');
  if(!ams&&plate.filaments.length!==1)add('blocked','外挂料架仅支持单耗材盘','请选择单耗材打印盘或在配色与供料中启用 AMS。',true,'mapping','调整供料方式');
  if(changed)add('note','所选颜色与文件原色不同','将按你选择的实际料卷配色打印，缩略图保留文件原色。');
  add('note','核对实际喷嘴和打印板',(slice.nozzleDiameters?.length?slice.nozzleDiameters.join(' / ')+' mm 喷嘴':'喷嘴规格未提供')+' · '+(slice.bedType==='Textured PEI Plate'?'纹理 PEI 板':slice.bedType||'板类型未提供')+'，请在机器上确认。');
  add(p&&Remote.printerFresh(p)&&p.state==='idle'?'ok':'note','设备实时状态',p&&Remote.printerFresh(p)?(STATE[p.state]||p.state)+' · 启动前将重新检查设备与队列。':'暂未取得新鲜设备状态，实际启动前需重新检查。');
  return rows;
 }
 async function open(j){
  const token=++generation,owner=$('#modalContent').firstElementChild,wasOpen=$('#modal').open;
  try{
   const {slice}=await Remote.request('/api/jobs/slice?jobId='+j.id);
   if(token!==generation||owner!==$('#modalContent').firstElementChild||wasOpen&&!$('#modal').open)return;
   const p=data.printers.find(p=>p.id===j.printer),saved=j.printOptions,back=()=>p?PrinterControl.open(p.id):$('#modal').close();
   if(!slice.plates?.length)throw Error('文件中没有可准备的打印盘');
   const restored=resume?.jobId===j.id&&resume.user===Workspace.username(data)?resume:null;if(restored)resume=null;
   let stage=0;const drafts=new Map(restored?.drafts||[]);
   modal('打印前准备',`<div class="prepare-intro"><div><span>PRINT PREPARATION</span><h2>${esc(j.filename)}</h2><p>选定打印盘与实际耗材，再核对准备结果。</p></div><span class="prepare-mode">只保存准备 · 不启动打印</span></div><nav class="prepare-steps" aria-label="打印准备步骤">${['打印盘','配色与供料','检查与保存'].map((name,i)=>`<button type="button" data-prepare-step="${i}"><b>${i+1}</b><span>${name}</span></button>`).join('')}</nav><div class="prepare-layout"><aside class="prepare-preview"><div id="projectPreview"></div><div class="prepare-file-facts"><div><span>切片机型</span><strong>${esc(slice.model||'未识别')}</strong></div><div><span>喷嘴规格</span><strong>${esc(slice.nozzleDiameters?.length?slice.nozzleDiameters.join(' / ')+' mm':'未提供')}</strong></div><div><span>打印板</span><strong>${esc(slice.bedType==='Textured PEI Plate'?'纹理 PEI 板':slice.bedType||'未提供')}</strong></div></div><p>预览与估算来自切片文件；颜色映射不修改预览图。</p></aside><form id="projectOptions" novalidate><section data-prepare-pane="0"><div class="prepare-section-head"><span>01 / PRINT PLATE</span><h3>先确认要打印的这一盘</h3></div><label><span class="field-label">打印盘</span><select name="plate" id="projectPlate">${slice.plates.map(x=>`<option value="${x.id}" ${x.id===saved?.plate?'selected':''}>第 ${x.id} 盘 · ${x.filaments.length} 份耗材</option>`).join('')}</select></label><div class="prepare-target"><div><span>目标打印机</span><strong>${esc(p?.name||'尚未选择')}</strong><small>${esc(p?.model||'请先在任务安排中确认设备')}</small></div>${p?badge(p.state):''}</div><div id="prepareCompatibility"></div><div class="prepare-info"><strong>切片不是通用文件</strong><p>机型或材质不同，需要回到切片软件重新切片；选择颜色不会转换文件里的温度、运动或启动流程。</p></div></section><section data-prepare-pane="1" hidden><div class="prepare-section-head"><span>02 / MATERIAL MAPPING</span><h3>把文件配色对应到实际料卷</h3><p>只显示本机已登记、已装载且材质一致的可用料卷。</p></div><label class="prepare-ams-toggle"><input type="checkbox" name="useAMS" id="projectAMS" ${saved?saved.useAMS?'checked':'':p?.configuration==='ams'?'checked':''}><span>使用 AMS</span><small>关闭时使用外置料架，仅单耗材</small></label><div id="prepareReportedFeeds"></div><div id="projectMappings"></div><div class="prepare-material-help"><strong>没有合适的料卷？</strong><p>进入本机耗材库登记料卷，或编辑已有记录确认实际装载位置。实际换料仍需在机器上完成；PETG 不能直接替代 PLA 切片。</p><button type="button" class="button small" id="prepareMaterials">打开本机耗材库</button><small>返回时保留当前选择，不自动修改库存或机器。</small></div></section><section data-prepare-pane="2" hidden><div class="prepare-section-head"><span>03 / FINAL CHECK</span><h3>最后核对一次</h3><button type="button" class="text-link prepare-recheck" id="prepareRecheck">重新读取设备与库存</button><p>保存后更新本任务的盘号、供料与库存预留；不会传输或启动打印。</p></div><div class="prepare-print-options"><label><span><strong>打印前自动调平</strong><small>实际启动时执行</small></span><input type="checkbox" name="bedLeveling" ${saved?saved.bedLeveling?'checked':'':'checked'}></label><label><span><strong>流量校准</strong><small>需对应机型与固件支持</small></span><input type="checkbox" name="flowCalibration" ${saved?.flowCalibration?'checked':''}></label><label><span><strong>延时摄影</strong><small id="prepareTimelapseNote">需设备支持与可用存储</small></span><input type="checkbox" name="timelapse" ${saved?.timelapse?'checked':''}></label></div><div id="prepareChecklist" aria-live="polite"></div><details class="prepare-support" id="prepareSupport"><summary>关于打印启动适配</summary><p>X2D 当前仅接入单喷嘴、单耗材盘与第一组 AMS 的打印验证。喷嘴规格、AMS 绑定和耗材预设需与文件一致。其他多头机型仍待适配。</p><p>多喷嘴、混料、外挂、多 AMS、流量校准和延时摄影暂未开放。符合条件的准备可保存；传输与启动由服务端重新检查，开启启动开关后仍需人工确认。</p></details><div id="prepareFileWarnings"></div></section><p class="form-error" id="projectError" role="status"></p></form></div>`,`<button class="button" id="backToControl">返回控制台</button><div class="prepare-footer-actions"><button class="button secondary" type="button" id="preparePrevious" hidden>上一步</button><button class="button primary" type="button" id="prepareNext">下一步 · 配色与供料</button><button class="button primary" type="submit" form="projectOptions" id="prepareSave" hidden>保存打印准备</button></div>`,{onDismiss:back});
   $('#modal').classList.add('prepare-modal');
   const form=$('#projectOptions'),plateSelect=$('#projectPlate'),amsToggle=$('#projectAMS');
   const currentPlate=()=>slice.plates.find(x=>x.id===Number(plateSelect.value));
   const choices=()=>Object.fromEntries(currentPlate().filaments.map(f=>[f.index,{spoolId:$(`input[name="spool-${f.index}"]:checked`,form)?.value,grams:new FormData(form).get('grams-'+f.index)}]));
   const rows=()=>checks(j,data.printers.find(x=>x.id===j.printer),slice,currentPlate(),amsToggle.checked,choices());
   function review(){
    const list=rows(),blocked=list.some(x=>x.blocking);
    $('#prepareChecklist').innerHTML=list.map(x=>`<div class="prepare-check ${x.state}"><span aria-hidden="true">${x.state==='ok'?'✓':x.state==='blocked'?'!':'i'}</span><div><strong>${esc(x.title)}</strong><p>${esc(x.detail)}</p>${x.action?`<button type="button" class="text-link prepare-fix" data-prepare-fix="${x.action}">${esc(x.actionLabel)} →</button>`:''}</div></div>`).join('');
    $('#prepareCompatibility').innerHTML=list.filter(x=>x.title.includes('机型')).map(x=>`<div class="prepare-compatibility ${x.state}"><strong>${x.state==='ok'?'✓':'!'} ${esc(x.title)}</strong><p>${esc(x.detail)}</p></div>`).join('');
    $$('[data-prepare-fix]').forEach(button=>button.onclick=()=>{if(button.dataset.prepareFix==='support'){const detail=$('#prepareSupport');detail.open=true;detail.scrollIntoView({block:'nearest'})}else{setStage(button.dataset.prepareFix==='mapping'?1:0);$('#projectMappings')?.scrollIntoView({block:'nearest'})}});
    $('#prepareSave').disabled=blocked;
    $('#prepareSave').textContent=blocked?'请先处理检查项':'保存打印准备';
    return list;
   }
   function setStage(next){
    stage=next;$('#modal').dataset.prepareStage=String(stage);
    $$('[data-prepare-pane]').forEach(el=>el.hidden=Number(el.dataset.preparePane)!==stage);
    $$('[data-prepare-step]').forEach(el=>{const selected=Number(el.dataset.prepareStep)===stage;el.classList.toggle('selected',selected);el.setAttribute('aria-current',selected?'step':'false')});
    $('#preparePrevious').hidden=stage===0;$('#prepareNext').hidden=stage===2;$('#prepareSave').hidden=stage!==2;
    $('#prepareNext').textContent=stage===0?'下一步 · 配色与供料':'下一步 · 检查与保存';review();
   }
   function mappings(){
    const p=data.printers.find(x=>x.id===j.printer),plate=currentPlate(),ams=amsToggle.checked,key=plate.id+':'+ams,draft=drafts.get(key);
    const used=plate.filaments.reduce((n,f)=>n+(f.usedGrams||f.grams),0),reserved=plate.filaments.reduce((n,f)=>n+f.grams,0);
    $('#projectPreview').innerHTML=`<div class="prepare-plate-art">${plate.thumbnail?`<img src="/api/jobs/thumbnail?jobId=${j.id}&plate=${plate.id}" alt="第 ${plate.id} 盘切片预览" onerror="this.hidden=true">`:'<div class="prepare-no-preview">此盘没有缩略图</div>'}<span>PLATE ${String(plate.id).padStart(2,'0')}</span></div><div class="prepare-preview-stats"><div><span>预计打印</span><strong>${esc(duration(plate.seconds))}</strong></div><div><span>切片用量</span><strong>${weight(used)} <small>g</small></strong></div></div><p class="prepare-reservation">${plate.filaments.length} 份耗材 · 至少预留 ${reserved} g</p>`;
    $('#projectMappings').innerHTML=plate.filaments.map(f=>{const choice=draft?.[f.index]||(saved?.plate===plate.id&&saved.useAMS===ams?saved.mapping?.find(m=>m.filament===f.index):null);return EquipmentUI.filamentChoices(j,p,f,ams,choice?.spoolId,choice?.grams)}).join('');
    EquipmentUI.bindFilamentChoices(plate);
    const feeds=(p?.equipment?.feeds||[]).filter(f=>f.present!==false&&f.material);
    $('#prepareReportedFeeds').innerHTML=feeds.length?`<div class="prepare-feed-summary"><span>机器当前上报</span><div>${feeds.map(f=>`<span>${esc(f.label||f.slot)} · ${esc(f.material)}</span>`).join('')}</div><small>机器读数用于核对，库存净重仍以登记为准。</small></div>`:'';
    const x2d=p?.modelId==='x2d'&&data.capabilities?.x2dSingleNozzlePrint;
    form.elements.flowCalibration.disabled=x2d;if(x2d)form.elements.flowCalibration.checked=false;
    const warnings=plate.warnings||[],unsupported=x2d||warnings.includes('文件不支持传统延时摄影');
    form.elements.timelapse.disabled=unsupported;if(unsupported)form.elements.timelapse.checked=false;
    $('#prepareTimelapseNote').textContent=x2d?'X2D 首轮打印验证暂未开放':unsupported?'文件提示不支持传统延时摄影':'需设备支持与可用存储';
    $('#prepareFileWarnings').innerHTML=warnings.length?`<div class="prepare-file-warning"><strong>切片提示</strong>${warnings.map(w=>`<p>${esc(w)}</p>`).join('')}</div>`:'';
    const capture=()=>{drafts.set(key,choices());$('#projectError').textContent='';review()};
    $$('input',$('#projectMappings')).forEach(input=>{input.addEventListener('change',capture);input.addEventListener('input',capture)});
    review();
   }
   plateSelect.onchange=mappings;amsToggle.onchange=mappings;
   $$('[data-prepare-step]').forEach(b=>b.onclick=()=>setStage(Number(b.dataset.prepareStep)));
   $('#prepareNext').onclick=()=>setStage(Math.min(2,stage+1));$('#preparePrevious').onclick=()=>setStage(Math.max(0,stage-1));$('#backToControl').onclick=back;
   $('#prepareMaterials').onclick=()=>{drafts.set(currentPlate().id+':'+amsToggle.checked,choices());resume={jobId:j.id,filename:j.filename,user:Workspace.username(data),plate:currentPlate().id,ams:amsToggle.checked,drafts:[...drafts],settings:{bedLeveling:form.elements.bedLeveling.checked,flowCalibration:form.elements.flowCalibration.checked,timelapse:form.elements.timelapse.checked}};materialMountFilter=String(j.printer);$('#modal').close();navigate('materials')};
   $('#prepareRecheck').onclick=async()=>{const button=$('#prepareRecheck');button.disabled=true;try{await Remote.refresh({renderPage:false});if($('#projectOptions')===form){drafts.set(currentPlate().id+':'+amsToggle.checked,choices());mappings()}}catch(error){toast(error.message)}finally{if(button.isConnected)button.disabled=false}};
   form.onsubmit=async event=>{
    event.preventDefault();if(stage!==2){setStage(2);return}
    const blocked=review().find(x=>x.blocking);if(blocked){$('#projectError').textContent=blocked.title+'：'+blocked.detail;return}
    const f=new FormData(form),plate=currentPlate();
    const mapping=plate.filaments.map(v=>{const sp=spoolById(f.get('spool-'+v.index)),slot=Number(sp?.mount?.slot)-1;return {filament:v.index,amsId:Number.isFinite(slot)?Math.floor(slot/4):0,slotId:Number.isFinite(slot)?slot%4:0,spoolId:sp?.id||'',grams:Number(f.get('grams-'+v.index))}});
    await Remote.mutate('/api/jobs/options',{jobId:j.id,options:{plate:plate.id,useAMS:f.has('useAMS'),mapping,bedLeveling:f.has('bedLeveling'),flowCalibration:f.has('flowCalibration'),timelapse:!form.elements.timelapse.disabled&&f.has('timelapse')}},form,'#projectError',()=>{toast('打印准备已保存');const prepared=data.jobs.find(x=>x.id===j.id);if(prepared&&form.isConnected)PrintFlow.open(prepared)});
   };
   if(restored){if(slice.plates.some(x=>x.id===restored.plate))plateSelect.value=String(restored.plate);amsToggle.checked=restored.ams;for(const [key,value] of Object.entries(restored.settings))form.elements[key].checked=value}
   mappings();setStage(restored?1:0);
  }catch(error){toast(error.message)}
 }
 function materialBanner(){if(!resume||resume.user!==Workspace.username(data))return '';return `<div class="preparation-return-banner"><div><strong>正在处理：${esc(resume.filename)}</strong><small>登记或核对装载后，返回打印准备继续选料；本页操作不会替你在机器上换料。</small></div><button type="button" class="button primary small" id="resumePreparation">返回打印准备 →</button></div>`}
 function bindMaterialBanner(){const button=$('#resumePreparation');if(button)button.onclick=()=>{const job=data.jobs.find(j=>j.id===resume?.jobId);if(!job||!canManageJob(job)||!['queued','assigned'].includes(job.state)){resume=null;render();return toast('任务状态已变化，请重新打开任务')}open(job)}}
 return {open,checks,materialBanner,bindMaterialBanner};
})();
