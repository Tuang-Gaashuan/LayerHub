'use strict';
let materialFilter='all',materialSearch='',queuePrinter=null;
const SPOOL_STATES={available:'可使用',drying:'干燥中',reserved:'暂不可用',empty:'已用尽'};
const duration=n=>n<=0?'现在':n<60?`${n} 分钟`:`${Math.floor(n/60)} 小时${n%60?' '+n%60+' 分钟':''}`;
const canManageJob=j=>j.user===data.user?.name||data.user?.admin;
const spoolById=id=>data.spools.find(s=>s.id===id);
const spoolOwner=s=>s.owner==='team'?'团队公共':data.members.find(m=>m.username===s.owner)?.name||'个人';
function jobMaterial(j){const s=spoolById(j.spoolId);return s&&Workspace.visible(s,data)?`${s.material} · ${s.colorName} / ${j.grams||0} g`:s?'个人耗材':'待确认耗材'}
function spoolRing(s,small=false){return `<span class="spool-ring ${small?'small':''}" style="--filament:${esc(/^#[0-9a-f]{6}$/i.test(s.color)?s.color:'#9daebb')}"><i></i></span>`}
function renderMaterials(){
  const visible=data.spools.filter(s=>Workspace.visible(s,data));
  const mine=Workspace.username(data);
  const rows=visible.filter(s=>(materialMountFilter==='all'||(materialMountFilter==='stock'?!s.mount:String(s.mount?.printerId)===materialMountFilter))&&(materialFilter==='all'||(materialFilter==='team'?s.owner==='team':materialFilter==='mine'?s.owner===mine:materialFilter==='low'?TeamTools.isLow(s):s.owner!=='team'&&s.shared))&&`${s.label} ${s.brand} ${s.material} ${s.colorName}`.toLowerCase().includes(materialSearch.toLowerCase()));
  const usable=visible.filter(s=>!Workspace.spoolIssue(data,s,1));
  $('#pageContent').innerHTML=heading('每一卷，都有用武之地。','登记公共与个人耗材，确认余量，再把合适的一卷交给下一个创意。',`<button class="button primary" id="registerSpool">${icon('plus')} 登记一卷耗材</button>`,'FILAMENT LIBRARY')+PrintPrepare.materialBanner()+`<section class="material-summary"><article><small>可见耗材</small><strong>${visible.length}<em>卷</em></strong></article><article><small>当前可使用</small><strong>${usable.length}<em>卷</em></strong></article><article><small>可分配余量</small><strong>${(usable.reduce((n,s)=>n+Workspace.available(data,s),0)/1000).toFixed(2)}<em>kg</em></strong></article><article><small>我的私有耗材</small><strong>${visible.filter(s=>s.owner===mine&&!s.shared).length}<em>卷</em></strong></article></section>${visible.some(s=>TeamTools.isLow(s))?`<div class="stock-alert-banner" role="status">${visible.filter(s=>TeamTools.isLow(s)).length} 卷耗材的可分配余量已达到提醒阈值，请核对库存与任务预留。</div>`:''}<div class="toolbar"><div class="segmented" aria-label="耗材范围">${[['all','全部可见'],['team','团队公共'],['mine','我的耗材'],['shared','个人共享'],['low','余量提醒']].map(([v,t])=>`<button data-material-filter="${v}" class="${materialFilter===v?'active':''}" aria-pressed="${materialFilter===v}">${t}</button>`).join('')}</div><div class="search-wrap">${icon('search')}<input class="input" id="materialSearch" aria-label="搜索耗材" placeholder="搜索材质、颜色或品牌…" value="${esc(materialSearch)}"></div></div>${mountFilter()}${MachineMaterials.discovery()}<div class="filament-grid">${rows.map(s=>{const reserved=Workspace.reserved(data,s.id),available=Workspace.available(data,s),pct=Math.round(s.remaining/s.capacity*100);return `<article class="filament-card"><div class="filament-card-top">${spoolRing(s)}<span class="filament-ownership ${s.owner==='team'?'team':s.shared?'shared':'private'}">${s.owner==='team'?'团队公共':s.shared?'个人共享':'仅自己可见'}</span></div><h2>${esc(s.label)}</h2><p>${esc(s.brand)} · ${esc(s.material)} · ${esc(s.colorName)}</p><div class="filament-weight"><strong>${s.remaining}<small> g</small></strong><span>/ ${s.capacity} g · ${pct}%</span></div><div class="progress filament-progress"><i style="width:${Math.min(100,pct)}%;background:${esc(s.color)}"></i></div><div class="filament-reservation"><span>队列预留 ${reserved} g</span><strong>可分配 ${available} g</strong></div>${TeamTools.stockMarkup(s)}<div class="filament-location">${icon(s.mount?'printer':'box')} ${esc(s.mount?Workspace.mountLabel(data,s):s.location||'未装载 · 库存中')}</div><div class="filament-card-bottom"><span>${esc(spoolOwner(s))}</span><span class="filament-state ${s.state}"><i></i>${s.state==='available'&&available===0?'已全部预留':SPOOL_STATES[s.state]||'暂不可用'}</span>${Workspace.editable(s,data)?`<button class="icon-button" data-edit-spool="${s.id}" aria-label="编辑 ${esc(s.label)}">${icon('settings')}</button>`:''}</div></article>`}).join('')||'<div class="empty">没有符合条件的耗材。登记新的一卷，或调整筛选条件。</div>'}</div><div class="workspace-note">${icon('shield')} 个人耗材默认私有；开启共享后，其他成员才可查看和选择。余量由登记人维护，任务完成时确认实际用量。</div>`;
  PrintPrepare.bindMaterialBanner();
  $('#registerSpool').onclick=()=>spoolForm();
  MachineMaterials.bind($('#pageContent'));TeamTools.bindStock();
  $('#materialMountFilter').onchange=e=>{materialMountFilter=e.target.value;render()};
  $$('[data-material-filter]').forEach(b=>b.onclick=()=>{materialFilter=b.dataset.materialFilter;render()});
  $$('[data-edit-spool]').forEach(b=>b.onclick=()=>spoolForm(spoolById(b.dataset.editSpool)));
  $('#materialSearch').oninput=e=>{materialSearch=e.target.value;const pos=e.target.selectionStart;render();$('#materialSearch').focus();$('#materialSearch').setSelectionRange(pos,pos)};
}
function localSpoolForm(existing,draft){
  if(!data.user)return showLogin();
  if(existing&&!Workspace.editable(existing,data))return;
  const own=Workspace.username(data),s=existing||draft||{label:'',brand:'',material:'PLA',colorName:'',color:'#88a9c0',capacity:1000,remaining:1000,owner:own,shared:false,state:'available',location:''};
  modal(existing?'编辑耗材记录':'登记一卷耗材',`<form id="spoolForm"><div class="form-grid"><label><span class="field-label">耗材名称</span><input class="input" name="label" required maxlength="60" value="${esc(s.label)}" placeholder="例如：雾白 PLA · 01"></label><label><span class="field-label">品牌</span><input class="input" name="brand" required maxlength="40" value="${esc(s.brand)}" placeholder="例如：Bambu Lab"></label><label><span class="field-label">材质</span><select name="material">${['PLA','PLA+','PETG','ABS','ASA','TPU','PA','PC','PVA','其他'].map(m=>`<option ${s.material===m?'selected':''}>${m}</option>`).join('')}</select></label><label><span class="field-label">颜色名称</span><input class="input" name="colorName" required maxlength="30" value="${esc(s.colorName)}" placeholder="例如：雾白"></label><label><span class="field-label">颜色预览</span><input class="input color-input" type="color" name="color" value="${esc(s.color)}"></label><label><span class="field-label">标称净重（g）</span><input class="input" type="number" name="capacity" required min="1" max="10000" step="1" value="${s.capacity}"></label><label><span class="field-label">当前净余量（g）</span><input class="input" type="number" name="remaining" required min="0" max="10000" step="1" value="${s.remaining}"></label><label><span class="field-label">可用状态</span><select name="state">${Object.entries(SPOOL_STATES).map(([v,t])=>`<option value="${v}" ${s.state===v?'selected':''}>${t}</option>`).join('')}</select></label></div><label><span class="field-label">所属范围</span><select name="owner" id="spoolOwner"><option value="${esc(own)}" ${s.owner!=='team'?'selected':''}>我的个人耗材</option><option value="team" ${s.owner==='team'?'selected':''}>团队公共耗材</option></select></label><label class="check-label" id="sharingField"><input type="checkbox" name="shared" ${s.shared?'checked':''}>共享给团队使用（默认关闭）</label>${spoolMountFields(s)}<label><span class="field-label">存放位置（未装载时）</span><input class="input" name="location" maxlength="80" value="${esc(s.location)}" placeholder="例如：耗材柜 A2 / 个人储物格"></label><p class="form-help">每条记录对应一卷。请填写扣除线盘重量后的耗材净余量。</p><p class="form-error" id="spoolError"></p></form>`,`<button class="button" data-cancel>取消</button><button class="button primary" type="submit" form="spoolForm">保存耗材</button>`);
  bindSpoolMountFields(s);
  const sharing=()=>$('#sharingField').hidden=$('#spoolOwner').value==='team';$('#spoolOwner').onchange=sharing;sharing();
}
function queueAvailability(p,q){
  if(q.reason)return q.reason+' · 时间待定';
  if(q.total===null)return '前序任务时长未知 · 时间待定';
  if(!p.enabled)return '设备已停用';
  if(!p.connected||p.state==='offline')return '设备离线 · 时间待定';
  if(p.state==='confirming')return '等待确认实际用量';
  if(q.paused)return '已暂停 · 时间待定';if(p.state==='failed')return '设备状态待核对';
  if(q.running&&!q.total)return '设备忙碌 · 结束时间待定';
  return q.total?'约 '+duration(q.total)+'后空闲':'现在可用';
}
function queueCardSummary(p){const q=Workspace.queue(data,p.id);return `<button class="printer-queue-summary" data-queue-printer="${p.id}"><span>${icon('clock')} 等待 <strong>${q.rows.length}</strong> 个任务</span><span><span data-queue-summary="${p.id}">${queueAvailability(p,q)}</span> ${icon('chevron')}</span></button>`}
function queuePanel(){
  if(queuePrinter===null||queuePrinter!==0&&!data.printers.some(p=>p.id===queuePrinter))queuePrinter=data.printers[0]?.id||0;
  const p=data.printers.find(p=>p.id===queuePrinter),q=Workspace.queue(data,queuePrinter);
  const pending=data.jobs.filter(j=>!j.printer&&['queued','assigned'].includes(j.state)).sort((a,b)=>a.order-b.order);
  const rows=p?q.rows:pending.map((job,i)=>({job,position:i+1,start:0}));
  return `<section class="panel device-queue-panel"><div class="panel-header"><h2>单机打印队列</h2><span class="queue-estimate-label">${p?esc(p.name.replace('Bambu Lab ','')):'待分配任务'}</span></div><div class="device-queue-tabs" role="group" aria-label="选择设备队列">${[...data.printers.map(p=>({id:p.id,name:p.name.replace('Bambu Lab ',''),count:Workspace.queue(data,p.id).rows.length})),{id:0,name:'待分配',count:pending.length}].map(t=>`<button data-queue-tab="${t.id}" class="${queuePrinter===t.id?'active':''}" aria-pressed="${queuePrinter===t.id}">${esc(t.name)}<span>${t.count}</span></button>`).join('')}</div>${p?`<div class="queue-running"><span class="queue-running-icon">${icon(q.running?'activity':'printer')}</span><div><strong>${q.running?esc(q.current?.filename||p.file||'设备正在打印'):!p.enabled?'设备已停用':!p.connected?'设备离线':p.state==='failed'?'设备状态待核对':q.rows[0]?.job.state==='assigned'?'文件已就绪，等待启动':'设备空闲'}</strong><small>${q.reason?esc(q.reason)+' · 开始与结束时间待定':q.running?`${p.state==='confirming'?'打印完成 · 待登记取件与用量':q.paused?'已暂停 · 预计时间待定':'当前剩余约 '+duration(Workspace.minutes(p.remaining,q.current?.estimate||45))} · ${esc(p.user||'')}`:!p.enabled?'重新启用后才能开始打印':!p.connected?'恢复连接后才能传输任务':p.state==='failed'?'请先核对设备告警与实际任务':q.rows.length?`前方 ${q.rows.length} 个等待任务`:'下一个创意可以从这里开始'}</small></div>${q.current&&canManageJob(q.current)?`<button class="button small" data-job="${q.current.id}" data-action="done">确认取件与用量</button>`:''}</div>`:'<p class="pending-description">任务尚未确认目标设备，不会自动进入某台打印机的队列。</p>'}<div class="device-queue-list">${rows.map(({job:j,position,start,startAt,endAt,reason})=>`<article class="device-queue-job"><span class="queue-position">${String(position).padStart(2,'0')}</span><div class="device-queue-copy"><strong>${esc(j.filename)}</strong><small>${esc(j.user)} · 预计打印 ${duration(Number(j.estimate)||45)} · ${esc(jobMaterial(j))}</small><span class="queue-start" data-queue-job-time="${j.id}" data-time-printer="${p?.id||0}">${TeamTools.queueWindow({job:j,start,endAt,startAt,reason},p)}${j.state==='assigned'?' · 已就绪':''}</span>${j.error?`<small class="form-error">${esc(j.error)}</small>`:''}</div>${canManageJob(j)?`<div class="queue-job-actions">${j.state==='queued'?`<button class="text-link" data-schedule-job="${j.id}">调整安排</button><button class="button small" data-job="${j.id}" data-action="assign">准备打印</button>`:j.state==='assigned'?`<button class="button primary small" data-job="${j.id}" data-action="start">开始打印</button>`:'<span class="queue-start">'+(STATE[Workspace.jobState(j)]||j.state)+'</span>'}</div>`:''}</article>`).join('')||'<div class="empty">这台设备没有等待任务，队列很轻松。</div>'}</div><div class="queue-footer"><span>${icon('info')} 按本机入队顺序安排 · 按连续衔接估算 · 每项任务需人工确认启动${q.paused?' · 暂停期间不计算开始时间':''}</span><button class="text-link" data-go="mine">我的任务 ${icon('chevron')}</button></div></section>`;
}
function renderQueues(){
  if(queuePrinter===null||queuePrinter!==0&&!data.printers.some(p=>p.id===queuePrinter))queuePrinter=data.printers[0]?.id||0;
  $('#pageContent').innerHTML=heading('每台设备，都有自己的节奏。','分别查看正在打印和等待中的任务，再确认适合你的设备。',`<button class="button primary" data-new-task>${icon('plus')} 新建打印任务</button>`,'PRINTER QUEUES')+`<div class="queue-overview-grid">${data.printers.map(p=>{const q=Workspace.queue(data,p.id);return `<button class="queue-overview ${p.id===queuePrinter?'active':''}" data-overview-printer="${p.id}">${printerArt(p)}<span><strong>${esc(p.name.replace('Bambu Lab ',''))}</strong><small>${badge(p.enabled?p.state:'offline')} 等待 ${q.rows.length} 个任务</small><em data-queue-summary="${p.id}">${queueAvailability(p,q)}</em></span></button>`}).join('')}</div>${queuePanel()}`;
  $$('[data-overview-printer]').forEach(b=>b.onclick=()=>{queuePrinter=Number(b.dataset.overviewPrinter);render()});
}
function bindWorkspaceCommon(){
  $$('[data-queue-printer]').forEach(b=>b.onclick=()=>{queuePrinter=Number(b.dataset.queuePrinter);navigate('queues')});
  $$('[data-queue-tab]').forEach(b=>b.onclick=()=>{if(uploading)return toast('请等待当前文件上传完成');queuePrinter=Number(b.dataset.queueTab);render()});
  $$('[data-job]').forEach(b=>b.onclick=()=>jobAction(Number(b.dataset.job),b.dataset.action));
  $$('[data-schedule-job]').forEach(b=>b.onclick=()=>scheduleJob(data.jobs.find(j=>j.id===Number(b.dataset.scheduleJob))));
}
function schedulingFields(j={}){
  const planningData={...data,jobs:data.jobs.filter(x=>x.id!==j.id)};
  const rec=Workspace.recommend(planningData),selected=j.printer===undefined?rec?.printer.id||0:j.printer;
  return `<div class="recommendation">${icon('sparkles')}<div><strong>${rec?'建议选择 '+esc(rec.printer.name.replace('Bambu Lab ','')):'暂无可推荐的设备'}</strong><small>${rec?(rec.queue.total?'当前预计等待 '+duration(rec.queue.total)+'，是可用设备中队列最短的一台。':'当前空闲，且没有等待任务。'):'可以先保留在待分配列表。'} 最终由你确认，推荐不校验切片机型与材料兼容性。</small></div></div><label><span class="field-label">确认目标设备</span><select name="printer" id="schedulePrinter"><option value="0" ${selected===0?'selected':''}>暂不分配 · 稍后由我确认</option>${data.printers.filter(p=>p.enabled||p.id===selected).map(p=>{const q=Workspace.queue(planningData,p.id);return `<option value="${p.id}" ${p.id===selected?'selected':''}>${esc(p.name.replace('Bambu Lab ',''))} · ${queueAvailability(p,q)}</option>`}).join('')}</select></label><div class="form-grid"><label><span class="field-label">预计打印时长（分钟）</span><input class="input" name="estimate" type="number" min="1" max="100000" step="1" required value="${Number(j.estimate)||45}"></label><label><span class="field-label">预计耗材净重（g）</span><input class="input" name="grams" id="jobGrams" type="number" min="1" max="10000" step="1" required value="${Number(j.grams)||20}"></label></div>${SpoolPicker.markup()}<p class="form-help">按切片结果填写时长与用量。创建任务会预留耗材，完成时确认实际扣减；余量不会提前扣除。</p><p class="form-error" id="scheduleError"></p>`;
}
function validateSchedule(form,job={}){
  const f=new FormData(form),printer=Number(f.get('printer')),estimate=Number(f.get('estimate')),grams=Number(f.get('grams')),spool=spoolById(f.get('spoolId'));
  if(!Number.isInteger(estimate)||estimate<1||estimate>100000||!Number.isInteger(grams)||grams<1||grams>10000)return {error:'请填写有效的预计时长与耗材净重'};
  if(printer&&!data.printers.some(p=>p.id===printer&&p.enabled))return {error:'该打印机已停用，请选择其他设备'};
  const error=Workspace.spoolIssue(data,spool,grams,job.id);if(error)return {error};
  return {printer,estimate,grams,spoolId:spool.id};
}
function localScheduleJob(j){
  if(!j||!canManageJob(j)||j.state!=='queued')return;
  modal('确认任务安排',`<p class="scheduling-filename">${esc(j.filename)}</p><form id="scheduleForm">${schedulingFields(j)}</form>`,`<button class="button" data-cancel>取消</button><button class="button primary" form="scheduleForm" type="submit">确认安排</button>`);
  SpoolPicker.bind($('#scheduleForm'),j);
}
function localUploadForm(){
  if(uploading)return;if(!data.user)return showLogin();if(!picked)return toast('请先选择需要打印的切片文件');
  const file=picked,note=$('#note').value.trim(),selected=Number($('#pickPrinter').value);
  modal('确认设备与耗材',`<p class="scheduling-filename">${esc(file.name)}</p><form id="uploadScheduleForm">${schedulingFields(selected?{printer:selected}:{})}</form>`,`<button class="button" data-cancel>返回</button><button class="button primary" type="submit" form="uploadScheduleForm">确认加入队列</button>`);
  SpoolPicker.bind($('#uploadScheduleForm'));
}
function completionForm(j){
  const s=spoolById(j.spoolId),allowed=s&&Workspace.visible(s,data);
  modal('确认取件与用量',`<p>确认「${esc(j.filename)}」已完成并可以取件。</p><form id="completeForm"><label><span class="field-label">实际使用耗材净重（g）</span><input class="input" name="used" type="number" min="0" max="${allowed?Workspace.available(data,s,j.id):10000}" step="1" required value="${j.grams||0}"></label><p class="form-help">扣减实际用量，释放本任务预留的 ${j.grams||0} g。设备空闲状态以实时遥测为准。</p><p class="form-error" id="completeError"></p></form>`, '<button class="button" data-cancel>取消</button><button class="button primary" form="completeForm" type="submit">确认完成与实际用量</button>');
}
