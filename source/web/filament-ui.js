'use strict';
const FilamentUI=(()=>{
 let printer=0,chosen='',confirm=null,clearUntil=0,busy=false,unknown=false;
 const fresh=v=>Number.isFinite(Date.parse(v))&&Date.now()-Date.parse(v)>=-1000&&Date.now()-Date.parse(v)<45000;
 const color=v=>/^[0-9a-f]{6}([0-9a-f]{2})?$/i.test(v||'')?'#'+v.slice(0,6):'transparent';
 const steps={0:I18n.t('供料待机'),1:I18n.t('流程暂停'),2:I18n.t('加热喷嘴'),3:I18n.t('切断耗材'),4:I18n.t('退回原耗材'),5:I18n.t('送入新耗材'),6:I18n.t('送入喷嘴'),7:I18n.t('排出旧色'),8:I18n.t('核对供料位置'),11:I18n.t('核对供料位置')};
 function reason(p,records,review=false){
  const f=p.equipment?.filament;
  if(!data.user?.admin)return I18n.t('进换退料仅管理员可操作');
  if(!Remote.ready()||!Remote.printerFresh(p)||!p.enabled)return I18n.t('请先核对连接状态');
  if(!['IDLE','FINISH'].includes(p.rawState)||p.state!=='idle'||p.completionReview||data.jobs.some(j=>j.printer===p.id&&['assigned','uploading','starting','printing','paused','stopped','checking'].includes(j.state)))return I18n.t('仅空闲设备允许进换退料');
  if(p.printError||p.equipment?.hms?.length||p.equipment?.controlIssue)return I18n.t('请先等待供料结束并处理设备告警');
  if(!f?.supported)return I18n.t('当前仅开放 X2D 主喷嘴与 AMS 2 Pro 的 PETG 供料控制');
  if(!fresh(f.updated))return I18n.t('供料状态未知或过期，请重新读取设备状态');
  if(f.busy||p.equipment?.switching||f.step!==0||![0,3].includes(f.mainState))return I18n.t('请先等待供料结束并处理设备告警');
  if(!review&&(unknown||Remote.deviceBlocked(p.id)||records.some(r=>['pending','uncertain'].includes(r.state))))return I18n.t('请等待或核对上一条设备指令');
  return '';
 }
 const valid=f=>f?.present===true&&f.material==='PETG'&&fresh(f.controlUpdated)&&Number.isInteger(f.nozzleMin)&&Number.isInteger(f.nozzleMax)&&f.nozzleMin>=200&&f.nozzleMax<=300&&f.nozzleMin<=f.nozzleMax&&color(f.color)!=='transparent';
 const identity=(f,feed)=>JSON.stringify([f.hasFilament,f.source,feed?.slot,feed?.material,feed?.color,feed?.nozzleMin,feed?.nozzleMax]);
 function render(p,records,root){
  if(p.modelId!=='x2d')return;
  if(printer!==p.id){printer=p.id;chosen='';confirm=null;clearUntil=0;unknown=false}
  const f=p.equipment?.filament||{},feeds=(p.equipment?.feeds||[]).filter(v=>['1','2','3','4'].includes(v.slot)),feed=feeds.find(v=>v.slot===chosen),current=feeds.find(v=>v.slot===f.source),why=reason(p,records),clear=clearUntil>Date.now();
  if(confirm&&confirm.identity!==identity(f,confirm.action==='unload'?current:feed)){confirm=null;clearUntil=0}
  const disabled=busy||!!why||!clear,canLoad=valid(feed)&&(!f.hasFilament||f.source!==chosen);
  const el=document.createElement('section');el.className='filament-panel';
  el.innerHTML=I18n.html`<div class="filament-heading"><div><span class="control-section-label">AMS 进换退料</span><h3>选择颜色，切换主喷嘴耗材</h3></div><strong>${esc(steps[f.step]||I18n.t('等待供料状态'))}</strong></div><p class="form-help">选择料槽不会操作机器。流程会自动加热、切料及送丝；请在现场观察。</p><div class="filament-choices">${['1','2','3','4'].map(slot=>{const v=feeds.find(x=>x.slot===slot);return `<button type="button" class="filament-choice" data-filament-slot="${slot}" aria-pressed="${slot===chosen}" ${busy||records.some(r=>r.state==='pending')?'disabled':''}><i style="background:${color(v?.color)}"></i><span>${I18n.t('料槽 ')+slot}<strong>${esc(v?.material||I18n.t('未装载'))}</strong><small>${v?.color?esc('#'+v.color.slice(0,6)):I18n.t('颜色未上报')}</small></span>${f.hasFilament&&f.source===slot?`<b>${I18n.t('当前料源')}</b>`:''}</button>`}).join('')}</div><p class="filament-current">${f.hasFilament?I18n.t('主喷嘴当前料槽：')+esc(f.source||I18n.t('未知')):I18n.t('主喷嘴未检测到已装入的耗材')}</p><p class="motion-status" role="status">${esc(why||(!clear?I18n.t('请先确认现场安全，再选择进料、换料或退料。'):!canLoad&&chosen&&f.source!==chosen?I18n.t('料槽需提供新鲜的 PETG、颜色与有效温度范围'):I18n.t('可以准备供料操作。')))}</p><label class="motion-clear"><input type="checkbox" data-filament-clear ${clear?'checked':''} ${busy?'disabled':''}><span>我在现场，已确认打印板和供料路径安全，可以自动加热和运动</span></label><div class="filament-actions"><button class="button primary" data-filament-action="load" ${disabled||!canLoad?'disabled':''}>${f.hasFilament?I18n.t('换入所选耗材'):I18n.t('进料')}</button><button class="button" data-filament-action="unload" ${disabled||!f.hasFilament||!valid(current)?'disabled':''}>退回当前耗材</button></div>${confirm?I18n.html`<div class="filament-confirm" role="group" aria-label="确认供料操作"><strong>${confirm.action==='unload'?I18n.t('确认退回主喷嘴耗材'):I18n.t('确认切换到料槽 ')+esc(chosen)}</strong><p>${confirm.action==='unload'?I18n.t('耗材将退回原料槽。'):I18n.t('目标颜色：')+esc('#'+feed.color.slice(0,6))} ${I18n.t('设备配置温度：')}${Math.floor(((confirm.action==='unload'?current:feed).nozzleMin+(confirm.action==='unload'?current:feed).nozzleMax)/2)}°C</p><button class="button" data-filament-cancel>取消</button><button class="button primary" data-filament-send ${disabled||(confirm.action==='load'?!canLoad:!f.hasFilament||!valid(current))?'disabled':''}>确认执行一次</button></div>`:''}${records.filter(r=>r.state==='uncertain'&&r.action.startsWith('filament:')).map(r=>`<div class="filament-confirm"><p>${esc(I18n.system(r.message.replace(/^requestID=[\w-]+;action=[a-z]+;slot=[1-4]?;/,'')))}</p><button class="button" data-filament-review="${r.id}" ${busy||!clear||reason(p,records,true)?'disabled':''}>${I18n.t('已现场确认供料结束、目标正确且停稳')}</button></div>`).join('')}${unknown?I18n.html('<p class="form-help">提交结果不明，请重新读取控制记录并核对机器；不要重复执行。</p><button class="button" data-filament-reread>重新读取控制记录</button>'):''}<p class="form-help">设备完成排料后才算流程结束；如面板要求确认出料，请在现场处理。本操作不会登记取件或扣减库存克数。</p>`;
  el.querySelector('[data-filament-clear]').onchange=e=>{clearUntil=e.target.checked?Date.now()+90000:0;PrinterControl.redraw()};
  el.querySelectorAll('[data-filament-slot]').forEach(b=>b.onclick=()=>{chosen=b.dataset.filamentSlot;confirm=null;PrinterControl.redraw()});
  el.querySelectorAll('[data-filament-action]').forEach(b=>b.onclick=()=>{confirm={action:b.dataset.filamentAction,identity:identity(f,b.dataset.filamentAction==='unload'?current:feed)};PrinterControl.redraw()});
  const cancel=el.querySelector('[data-filament-cancel]');if(cancel)cancel.onclick=()=>{confirm=null;PrinterControl.redraw()};
  const send=el.querySelector('[data-filament-send]');if(send)send.onclick=()=>submit(p,{action:confirm.action,...(confirm.action==='load'?{slot:chosen}:{})});
  const reread=el.querySelector('[data-filament-reread]');if(reread)reread.onclick=async()=>{try{await PrinterControl.refresh();unknown=false;PrinterControl.redraw()}catch(e){toast(I18n.system(e.message))}};
  el.querySelectorAll('[data-filament-review]').forEach(b=>b.onclick=()=>submit(p,{action:'review',commandId:Number(b.dataset.filamentReview)}));
  root.querySelector('.equipment-live-supply')?.prepend(el);
 }
 async function submit(p,action){if(busy||clearUntil<=Date.now()||printer!==p.id)return;busy=true;confirm=null;clearUntil=0;PrinterControl.redraw();try{await Remote.request('/api/printers/filament',{printerId:p.id,...action,clear:true,requestId:crypto.randomUUID()});await PrinterControl.refresh({urgent:true})}catch(e){unknown=true;toast(I18n.system(e.message))}finally{busy=false;PrinterControl.redraw()}}
 const label=action=>action==='filament:load'?I18n.t('进料 / 换料'):action==='filament:unload'?I18n.t('退料'):action.startsWith('ams-cycle-')?I18n.t('供料实测'):undefined;
 return {render,label};
})();
