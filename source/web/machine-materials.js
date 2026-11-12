'use strict';
const MachineMaterials=(()=>{
 const hex=value=>/^#?(?:[0-9a-f]{6}|[0-9a-f]{8})$/i.test(value||'')?'#'+value.replace('#','').slice(0,6).toLowerCase():null;
 const syncing=new Set();
 const existing=(p,slot)=>data.spools.find(s=>s.mount?.printerId===p.id&&s.mount.slot===slot);
 const privateSlot=(p,slot)=>data.occupiedSlots?.some(m=>m.printerId===p.id&&m.slot===slot);
 const fresh=(p,f)=>Remote.ready()&&Remote.printerFresh(p)&&p.enabled&&Number.isFinite(Date.parse(f.updated))&&Date.now()-Date.parse(f.updated)<90000;
 const usable=f=>f&&f.present!==false&&!!f.material&&f.material!==I18n.t('个人耗材');
 function comparison(p,f,s=existing(p,f?.slot)){
  if(!s||!f||!fresh(p,f))return null;
  const normal=v=>String(v||'').trim().toUpperCase();
  const reasons=[];
  if(f.present===false)reasons.push(I18n.t('机器上报该位置未装载耗材'));
  else if(usable(f)){
   if(normal(s.material)!==normal(f.material))reasons.push(I18n.t('材质不同'));
  }
  return reasons.length?{spool:s,reasons}:null;
 }
 function notice(p,f,s){
  const result=comparison(p,f,s);if(!result)return '';
  const machine=f.present===false?I18n.t('未装载'):String(f.material||I18n.t('材质未知'))+' · '+(hex(f.color)||I18n.t('颜色未提供'));
  return `<div class="detail-notice warning machine-mount-check" data-mount-mismatch role="status"><strong>${I18n.t('耗材不一致')}</strong><p>${esc(result.reasons.join(' / '))} · ${I18n.t('请检查实际挂载是否与打印机反馈一致。')}</p><small>${I18n.t('打印机反馈：')}${esc(machine)}</small><small style="display:block">${I18n.t('库中登记：')}${esc(result.spool.material+' · '+result.spool.colorName+' · '+(hex(result.spool.color)||''))}</small></div>`;
 }
 function noticeForSpool(s){
  if(!s.mount)return '';const p=data.printers.find(p=>p.id===s.mount.printerId);if(!p)return '';
  return notice(p,(p.equipment?.feeds||[]).find(f=>f.slot===s.mount.slot),s);
 }
 function savedNotice(mount){
  const p=data.printers.find(p=>p.id===mount?.printerId),f=p?.equipment?.feeds?.find(f=>f.slot===mount.slot);
  return p&&comparison(p,f)?I18n.t('挂载登记已保存，但与打印机反馈不一致，请检查实际挂载。'):I18n.t('耗材登记已保存到服务器');
 }
 function syncAction(s){
  const history=typeof SpoolHistory!=='undefined'?SpoolHistory.button(s):'';
  if(!Workspace.editable(s,data))return history;
  if(!s.mount)return history+`<button type="button" class="text-link machine-register" data-stock-mount="${esc(s.id)}">${I18n.t('挂载到打印机')}</button>`;
  const p=data.printers.find(p=>p.id===s.mount.printerId),f=p?.equipment?.feeds?.find(f=>f.slot===s.mount.slot);
  const ready=p&&f&&fresh(p,f)&&Date.now()>=Date.parse(f.updated)&&f.present===true&&usable(f)&&!data.capabilities?.restorePending&&!syncing.has(s.id);
  if(data.capabilities?.spoolReplace)return history+`<button type="button" class="text-link machine-register" data-replace-spool="${esc(s.id)}" ${ready?'':'disabled'}>${I18n.t('更换料卷')}</button>`;
  return history+`<button type="button" class="text-link machine-register" data-sync-spool="${esc(s.id)}" ${ready?'':'disabled'} title="${I18n.t('同步材质与颜色，保留库存克数、归属和挂载位置')}">${I18n.t('按打印机更新')}</button>`;
 }
 async function syncSpool(id){
  if(syncing.has(id))return;const before=spoolById(id),uid=data.user?.id;if(!before?.mount||!Workspace.editable(before,data))return;
  const mount={...before.mount},revision=before.revision;syncing.add(id);
  try{
   await Remote.refresh({renderPage:false});const s=spoolById(id),p=data.printers.find(p=>p.id===mount.printerId),f=p?.equipment?.feeds?.find(f=>f.slot===mount.slot);
   if(data.user?.id!==uid||!s||!Workspace.editable(s,data)||!Number.isInteger(revision)||revision<1||s.revision!==revision||s.mount?.printerId!==mount.printerId||s.mount?.slot!==mount.slot)throw Error(I18n.t('耗材记录已变化，请重新读取后再更新'));
   if(!p||!f||!fresh(p,f)||Date.now()<Date.parse(f.updated)||f.present!==true||!usable(f)||data.capabilities?.restorePending)throw Error(I18n.t('打印机未提供新鲜的已装料信息，未更新耗材'));
   const nextColor=hex(f.color)||s.color,nextName=hex(f.color)&&hex(f.color)!==hex(s.color)?hex(f.color):s.colorName;
   if(s.material===f.material&&hex(s.color)===hex(nextColor))return toast(I18n.t('耗材已与打印机上报一致'));
   const body=Object.fromEntries(['id','revision','label','brand','material','colorName','color','capacity','remaining','owner','shared','state','location','mount'].map(k=>[k,s[k]]));
   Object.assign(body,{material:f.material,color:nextColor,colorName:nextName});
   await Remote.request('/api/spools',body);
   await Remote.refresh({renderPage:!$('#modal').open});if($('#controlRoot'))await PrinterControl.refresh();
   toast(I18n.t('已按打印机更新材质与颜色，库存克数保持不变'));
  }catch(error){toast(I18n.system(error.message))}finally{syncing.delete(id);document.querySelectorAll('[data-sync-spool]').forEach(b=>{if(b.dataset.syncSpool===id)b.disabled=false})}
 }
 function action(p,f){
  if(!usable(f))return '';
  const s=existing(p,f.slot);
  if(s){const mismatch=!!comparison(p,f,s);return Workspace.editable(s,data)?`<button class="text-link machine-register" data-machine-spool="${esc(s.id)}">${mismatch?I18n.t('核对已登记料卷'):I18n.t('已登记 · 查看')}</button>${syncAction(s)}`:I18n.html('<span class="machine-register-note">已登记耗材</span>')}
  if(privateSlot(p,f.slot))return I18n.html('<span class="machine-register-note">已登记 · 私有耗材</span>');
  const ready=fresh(p,f)&&!data.capabilities?.restorePending;
  return `<button class="button small machine-register" data-register-feed="${p.id}" data-feed-slot="${esc(f.slot)}" ${ready?'':'disabled'} title="${ready?I18n.t('选择库存卷或登记全新料卷'):I18n.t('等待设备与耗材状态更新')}">${ready?I18n.t('选择料卷'):I18n.t('等待状态更新')}</button>`;
 }
 function discovery(){
  if(materialMountFilter==='stock')return '';
  const feeds=data.printers.filter(p=>materialMountFilter==='all'||String(p.id)===materialMountFilter).flatMap(p=>(p.equipment?.feeds||[]).filter(f=>usable(f)&&!existing(p,f.slot)&&!privateSlot(p,f.slot)).map(f=>({p,f})));
  if(!feeds.length)return '';
  return I18n.html`<section class="machine-discovery"><div class="control-section-heading"><div><span class="control-section-label">机器发现的耗材</span><small>${feeds.length} 个装载位置待登记 · 确认归属和净余量后入库</small></div></div><div class="machine-discovery-grid">${feeds.map(({p,f})=>`<article><span class="machine-feed-swatch" style="background:${hex(f.color)||'#a5afbe'}"></span><div><strong>${esc(f.material)}</strong><small>${esc(p.name)} · ${esc(Workspace.feedLabel(f))}</small><small>${f.remaining!==null&&f.remaining!==undefined?I18n.t('设备估算 ')+esc(I18n.system(f.remaining))+esc(f.remainingUnit||''):I18n.t('设备未提供余量')}</small></div>${action(p,f)}</article>`).join('')}</div><p class="form-help">机器读数不会自动覆盖库存。每条库存记录对应一卷耗材，登记后保留实际装载位置。</p></section>`;
 }
 async function finish(form,back){
  await Remote.refresh({renderPage:false});if(!form.isConnected||!$('#modal').open)return;if(back)back();else{$('#modal').close();render()}
 }
 async function register(printerId,slot,newRecord=false,onReturn=null){
  const back=onReturn||PrinterControl.returnPoint();
  try{await Remote.refresh({renderPage:false})}catch(error){toast(I18n.system(error.message));return}
  const p=data.printers.find(p=>p.id===printerId),f=p?.equipment?.feeds?.find(f=>f.slot===slot);
  if(!p||!usable(f)||!fresh(p,f)||data.capabilities?.restorePending)return toast(I18n.t('耗材状态已变化，请等待设备更新后重新登记'));
  const current=existing(p,slot);if(current){if(Workspace.editable(current,data))return replaceSpool(current.id,false,null,back);return toast(I18n.t('此位置已经登记耗材，请勿重复入库'))}
  if(privateSlot(p,slot))return toast(I18n.t('此位置已登记个人耗材，请由所属成员维护'));
  if(!newRecord)return replaceSpool('',false,{printerId,slot},back);
  const color=hex(f.color),grams=f.remainingUnit==='g'&&Number.isInteger(f.remaining)&&f.remaining>=0?f.remaining:'',percent=f.remainingUnit==='%'&&Number.isFinite(f.remaining)&&f.remaining>=0&&f.remaining<=100?f.remaining:null;
  const draft={label:(f.material+' · '+Workspace.feedLabel(f)).slice(0,60),brand:'',material:f.material,colorName:color||'',color:color||'#808080',capacity:'',remaining:grams,owner:Workspace.username(data),shared:false,state:grams===0?'empty':'available',location:'',mount:{printerId:p.id,slot}};
  spoolForm(undefined,draft,back);
  const form=$('#spoolForm');if(!form)return;
  const material=form.elements.namedItem('material');if(![...material.options].some(o=>o.value===f.material)){const option=document.createElement('option');option.value=f.material;option.textContent=f.material;material.append(option)}material.value=f.material;
  form.insertAdjacentHTML('afterbegin',I18n.html`<div class="machine-import-note"><strong>来自 ${esc(p.name)} · ${esc(Workspace.feedLabel(f))}</strong><p>已带入材质、颜色与装载位置。请填写品牌、标称净重及当前净余量，并确认公共或个人归属。${color?'':I18n.t('设备未提供颜色，请选择实际颜色。')}</p>${percent!==null?I18n.html`<p>设备估算剩余 ${esc(percent)}%，该百分比不等于库存克数。</p><button type="button" class="button small" data-estimate-remaining disabled>按标称重量估算净余量</button><p data-estimate-note role="status"></p>`:grams!==''?I18n.html('<p>设备提供了克数，请核对后保存。</p>'):I18n.html('<p>设备未提供有效余量，请确认或称重后填写。</p>')}</div>`);
  form.querySelectorAll('select[name="mountPrinter"],select[name="mountSlot"],select[name="material"]').forEach(e=>e.disabled=true);if(hex(f.color))form.elements.color.disabled=true;
  let saving=false,uncertain=false;form.onsubmit=async event=>{event.preventDefault();if(saving||uncertain||!form.reportValidity())return;const v=new FormData(form),spool={label:v.get('label').trim(),brand:v.get('brand').trim(),material:f.material,colorName:v.get('colorName').trim(),color:hex(f.color)||v.get('color'),capacity:Number(v.get('capacity')),remaining:Number(v.get('remaining')),owner:v.get('owner'),shared:v.get('owner')!=='team'&&v.has('shared'),state:v.get('state'),location:v.get('location').trim(),mount:{printerId,slot}};saving=true;const button=document.querySelector('[form="spoolForm"][type="submit"]');button.disabled=true;try{await Remote.request('/api/spools/replace',{mount:{printerId,slot},profileId:f.profileId||'',spool});await finish(form,back);toast(I18n.t('新料卷已登记并挂载'))}catch(error){uncertain=!!error.unconfirmed;if(form.isConnected)$('#spoolError').textContent=uncertain?I18n.t('换卷结果尚未确认，请刷新耗材库核对，不要重复提交。'):I18n.system(error.message)}finally{saving=false;button.disabled=uncertain}};
  if(percent!==null){const capacity=form.elements.namedItem('capacity'),remaining=form.elements.namedItem('remaining'),button=form.querySelector('[data-estimate-remaining]'),note=form.querySelector('[data-estimate-note]');capacity.addEventListener('input',()=>{const n=Number(capacity.value);button.disabled=!Number.isInteger(n)||n<1||n>10000;if(note.textContent)note.textContent=I18n.t('标称重量已变化，请重新估算或手动确认净余量')});button.onclick=()=>{const n=Number(capacity.value);if(!Number.isInteger(n)||n<1||n>10000)return;remaining.value=String(Math.round(n*percent/100));note.textContent=I18n.t('已按设备百分比估算，请确认后保存；后续遥测不会覆盖库存余量。')}}
 }
 async function replaceSpool(id,newRecord=false,destination=null,onReturn=null){
  const back=onReturn||PrinterControl.returnPoint();
  try{
   await Remote.refresh({renderPage:false});const old=id?spoolById(id):{mount:destination,revision:0},uid=data.user?.id;
   if(!old?.mount||(id&&!Workspace.editable(old,data))||!data.capabilities?.spoolReplace)return;
   const mount={...old.mount},revision=old.revision,p=data.printers.find(p=>p.id===mount.printerId),f=p?.equipment?.feeds?.find(f=>f.slot===mount.slot);
   if(!p||!f||!fresh(p,f)||f.present!==true)throw Error(I18n.t('打印机未提供新鲜的已装料信息，未更新耗材'));
   const material=f.material,color=hex(f.color)||'#808080',profileId=f.profileId||'';
   if(!newRecord){
    const reason=s=>!Workspace.editable(s,data)?I18n.t('无权调整此卷挂载，请联系管理员或登记人'):s.mount?I18n.t('仍登记在其他装载位置，请先卸下'):Workspace.reserved(data,s.id)>0?I18n.t('仍有任务预留或待结算'):s.state!=='available'||s.remaining<=0?I18n.t('当前不可用或余量不足'):'';
    const candidates=data.spools.filter(s=>s.id!==id&&Workspace.visible(s,data)&&s.material.toLowerCase()===material.toLowerCase());
    const available=candidates.filter(s=>!reason(s));
    modal(I18n.t(id?'更换料卷':'选择料卷'),`<form id="replaceExistingForm"><p>${I18n.t('优先选择耗材库已有料卷；只有首次使用的新卷才需要登记。')}</p><p>${esc(p.name)} · ${esc(Workspace.feedLabel(f))} · ${esc(material)}</p><div class="project-color-choices">${candidates.map(s=>`<label class="project-color-choice"><input type="radio" name="existingSpool" value="${esc(s.id)}" ${reason(s)?'disabled':'required'}><i class="project-color-dot" style="background:${hex(s.color)}"></i><span><strong>${esc(s.label)}</strong><small>${esc(s.colorName)} · ${s.remaining} g${reason(s)?' · '+esc(reason(s)):''}</small></span></label>`).join('')||`<p>${I18n.t('没有同材质且未挂载的可用库存卷。可先核对库存记录，或登记新卷。')}</p>`}</div><p>${I18n.t('换回已有料卷会保留它原有的编号、余量和历史，不新增库存记录。')}</p><p class="form-error" id="replaceExistingError"></p></form>`,`<button class="button" id="replacementNew">${I18n.t('登记新卷')}</button><button class="button primary" type="submit" form="replaceExistingForm" ${available.length?'':'disabled'}>${I18n.t('使用所选库存卷')}</button>`,{onDismiss:back});
    const form=$('#replaceExistingForm'),button=document.querySelector('[form="replaceExistingForm"]');let busy=false,unknown=false;
    $('#replacementNew').onclick=()=>{if(!busy&&!unknown)id?replaceSpool(id,true,null,back):register(mount.printerId,mount.slot,true,back)};
    form.onsubmit=async e=>{e.preventDefault();if(busy||unknown||!form.reportValidity()||data.user?.id!==uid)return;const target=candidates.find(s=>s.id===new FormData(form).get('existingSpool'));if(!target||reason(target))return;busy=true;button.disabled=true;$('#replacementNew').disabled=true;
     try{await Remote.request('/api/spools/replace',{...(id?{oldId:id,revision}:{mount}),profileId,existingId:target.id,existingRevision:target.revision});await finish(form,back);toast(I18n.t(id?'库存料卷已挂载，原料卷已改为未挂载':'库存料卷已挂载，编号与余量保持不变'))}
     catch(error){unknown=!!error.unconfirmed;if(form.isConnected)$('#replaceExistingError').textContent=unknown?I18n.t('换卷结果尚未确认，请刷新耗材库核对，不要重复提交。'):I18n.system(error.message)}finally{busy=false;button.disabled=unknown;if($('#replacementNew'))$('#replacementNew').disabled=unknown}
    };return;
   }
   spoolForm(undefined,{label:material+' · '+color,brand:'',material,colorName:color,color,capacity:'',remaining:'',owner:old.owner==='team'?'team':Workspace.username(data),shared:false,state:'available',location:'',mount},back);
   const form=$('#spoolForm');if(!form)return;
   form.insertAdjacentHTML('afterbegin',`<div class="machine-import-note"><strong>${I18n.t('登记新卷并卸下旧卷')}</strong><p>${esc(old.label)} · ${old.remaining} g → ${esc(material+' · '+color)}</p><p>${I18n.t('旧卷保留原资料和余量，改为未挂载；请填写新卷的净重和余量，两步一起保存。')}</p></div>`);
   // The replacement destination and reported material are fixed for this form.
   form.querySelectorAll('select[name="mountPrinter"],select[name="mountSlot"],select[name="material"],input[name="color"]').forEach(e=>e.disabled=true);
   let busy=false,unknown=false;
   form.onsubmit=async e=>{
    e.preventDefault();if(busy||unknown||!form.isConnected||data.user?.id!==uid||!form.reportValidity())return;
    const v=new FormData(form),spool={label:v.get('label').trim(),brand:v.get('brand').trim(),material,colorName:v.get('colorName').trim(),color,capacity:Number(v.get('capacity')),remaining:Number(v.get('remaining')),owner:v.get('owner'),shared:v.get('owner')!=='team'&&v.has('shared'),state:v.get('state'),location:v.get('location').trim(),mount};
    busy=true;const button=document.querySelector('[form="spoolForm"][type="submit"]');button.disabled=true;
    try{await Remote.request('/api/spools/replace',{oldId:id,revision,profileId,spool});await finish(form,back);toast(I18n.t('新料卷已登记，旧料卷已改为未挂载'))}
    catch(error){unknown=!!error.unconfirmed;if(form.isConnected)$('#spoolError').textContent=unknown?I18n.t('换卷结果尚未确认，请刷新耗材库核对，不要重复提交。'):I18n.system(error.message)}
    finally{busy=false;button.disabled=unknown}
   };
  }catch(error){toast(I18n.system(error.message))}
 }
 async function mountStock(id){
  try{await Remote.refresh({renderPage:false});const s=spoolById(id);if(!s||s.mount||!Workspace.editable(s,data))return;
   const targets=data.printers.flatMap(p=>(p.equipment?.feeds||[]).filter(f=>f.present===true&&fresh(p,f)&&f.material.toLowerCase()===s.material.toLowerCase()&&!privateSlot(p,f.slot)&&(!existing(p,f.slot)||Workspace.editable(existing(p,f.slot),data))).map(f=>({p,f})));
   modal(I18n.t('挂载到打印机'),`<form id="stockMountForm"><p>${esc(s.label)} · ${esc(s.colorName)} · ${s.remaining} g</p><p>${I18n.t('先在机器上装好这卷，再选择对应装载位置；此操作只更新库存关联，不会驱动进料。')}</p><label>${I18n.t('实际装载位置')}<select name="target" required><option value="">${I18n.t('请选择')}</option>${targets.map(({p,f},i)=>`<option value="${i}">${esc(p.name)} · ${esc(Workspace.feedLabel(f))}</option>`).join('')}</select></label>${targets.length?'':`<p>${I18n.t('没有新鲜的同材质装载位置，请检查设备上报与实际装料。')}</p>`}</form>`,`<button class="button primary" type="submit" form="stockMountForm" ${targets.length?'':'disabled'}>${I18n.t('选择装载位置')}</button>`);
   $('#stockMountForm').onsubmit=async e=>{e.preventDefault();const form=e.currentTarget;if(!form.reportValidity())return;const v=form.elements.target.value,t=targets[Number(v)];if(v===''||!t)return;await register(t.p.id,t.f.slot);const radio=document.querySelector('[name="existingSpool"][value="'+id+'"]');if(radio&&!radio.disabled){radio.checked=true;radio.dispatchEvent(new Event('change',{bubbles:true}))}};
  }catch(error){toast(I18n.system(error.message))}
 }
 function bind(root){
  if(typeof SpoolHistory!=='undefined')SpoolHistory.bind(root);
  root.querySelectorAll('[data-stock-mount]').forEach(b=>b.onclick=()=>mountStock(b.dataset.stockMount));
  root.querySelectorAll('[data-replace-spool]').forEach(b=>b.onclick=()=>replaceSpool(b.dataset.replaceSpool));
  root.querySelectorAll('[data-sync-spool]').forEach(b=>b.onclick=()=>{b.disabled=true;syncSpool(b.dataset.syncSpool)});
  root.querySelectorAll('[data-register-feed]').forEach(b=>b.onclick=()=>register(Number(b.dataset.registerFeed),b.dataset.feedSlot));
  root.querySelectorAll('[data-machine-spool]').forEach(b=>b.onclick=()=>spoolForm(spoolById(b.dataset.machineSpool)));
 }
 return {syncAction,syncSpool,action,discovery,bind,register,notice,noticeForSpool,savedNotice};
})();
