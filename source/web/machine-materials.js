'use strict';
const MachineMaterials=(()=>{
 const hex=value=>/^#?(?:[0-9a-f]{6}|[0-9a-f]{8})$/i.test(value||'')?'#'+value.replace('#','').slice(0,6).toLowerCase():null;
 const existing=(p,slot)=>data.spools.find(s=>s.mount?.printerId===p.id&&s.mount.slot===slot);
 const privateSlot=(p,slot)=>data.occupiedSlots?.some(m=>m.printerId===p.id&&m.slot===slot);
 const fresh=(p,f)=>Remote.ready()&&Remote.printerFresh(p)&&p.enabled&&Number.isFinite(Date.parse(f.updated))&&Date.now()-Date.parse(f.updated)<90000;
 const usable=f=>f&&f.present!==false&&!!f.material&&f.material!==I18n.t('个人耗材');
 function action(p,f){
  if(!usable(f))return '';
  const s=existing(p,f.slot);
  if(s){const mismatch=s.material.toLowerCase()!==f.material.toLowerCase()||hex(f.color)&&hex(s.color)!==hex(f.color);return Workspace.editable(s,data)?`<button class="text-link machine-register" data-machine-spool="${esc(s.id)}">${mismatch?I18n.t('核对已登记料卷'):I18n.t('已登记 · 查看')}</button>`:I18n.html('<span class="machine-register-note">已登记耗材</span>')}
  if(privateSlot(p,f.slot))return I18n.html('<span class="machine-register-note">已登记 · 私有耗材</span>');
  const ready=fresh(p,f)&&!data.capabilities?.restorePending;
  return `<button class="button small machine-register" data-register-feed="${p.id}" data-feed-slot="${esc(f.slot)}" ${ready?'':'disabled'} title="${ready?I18n.t('确认归属与净余量后保存到耗材库'):I18n.t('等待设备与耗材状态更新')}">${ready?I18n.t('登记入库'):I18n.t('等待状态更新')}</button>`;
 }
 function discovery(){
  if(materialMountFilter==='stock')return '';
  const feeds=data.printers.filter(p=>materialMountFilter==='all'||String(p.id)===materialMountFilter).flatMap(p=>(p.equipment?.feeds||[]).filter(f=>usable(f)&&!existing(p,f.slot)&&!privateSlot(p,f.slot)).map(f=>({p,f})));
  if(!feeds.length)return '';
  return I18n.html`<section class="machine-discovery"><div class="control-section-heading"><div><span class="control-section-label">机器发现的耗材</span><small>${feeds.length} 个装载位置待登记 · 确认归属和净余量后入库</small></div></div><div class="machine-discovery-grid">${feeds.map(({p,f})=>`<article><span class="machine-feed-swatch" style="background:${hex(f.color)||'#a5afbe'}"></span><div><strong>${esc(f.material)}</strong><small>${esc(p.name)} · ${esc(f.label)}</small><small>${f.remaining!==null&&f.remaining!==undefined?I18n.t('设备估算 ')+esc(I18n.system(f.remaining))+esc(f.remainingUnit||''):I18n.t('设备未提供余量')}</small></div>${action(p,f)}</article>`).join('')}</div><p class="form-help">机器读数不会自动覆盖库存。每条库存记录对应一卷耗材，登记后保留实际装载位置。</p></section>`;
 }
 async function register(printerId,slot){
  try{await Remote.refresh({renderPage:false})}catch(error){toast(I18n.system(error.message));return}
  const p=data.printers.find(p=>p.id===printerId),f=p?.equipment?.feeds?.find(f=>f.slot===slot);
  if(!p||!usable(f)||!fresh(p,f)||data.capabilities?.restorePending)return toast(I18n.t('耗材状态已变化，请等待设备更新后重新登记'));
  const current=existing(p,slot);if(current){if(Workspace.editable(current,data))return spoolForm(current);return toast(I18n.t('此位置已经登记耗材，请勿重复入库'))}
  if(privateSlot(p,slot))return toast(I18n.t('此位置已登记个人耗材，请由所属成员维护'));
  const color=hex(f.color),grams=f.remainingUnit==='g'&&Number.isInteger(f.remaining)&&f.remaining>=0?f.remaining:'',percent=f.remainingUnit==='%'&&Number.isFinite(f.remaining)&&f.remaining>=0&&f.remaining<=100?f.remaining:null;
  const draft={label:(f.material+' · '+f.label).slice(0,60),brand:'',material:f.material,colorName:color||'',color:color||'#808080',capacity:'',remaining:grams,owner:Workspace.username(data),shared:false,state:grams===0?'empty':'available',location:'',mount:{printerId:p.id,slot}};
  spoolForm(undefined,draft);
  const form=$('#spoolForm');if(!form)return;
  const material=form.elements.namedItem('material');if(![...material.options].some(o=>o.value===f.material)){const option=document.createElement('option');option.value=f.material;option.textContent=f.material;material.append(option)}material.value=f.material;
  form.insertAdjacentHTML('afterbegin',I18n.html`<div class="machine-import-note"><strong>来自 ${esc(p.name)} · ${esc(f.label)}</strong><p>已带入材质、颜色与装载位置。请填写品牌、标称净重及当前净余量，并确认公共或个人归属。${color?'':I18n.t('设备未提供颜色，请选择实际颜色。')}</p>${percent!==null?I18n.html`<p>设备估算剩余 ${esc(percent)}%，该百分比不等于库存克数。</p><button type="button" class="button small" data-estimate-remaining disabled>按标称重量估算净余量</button><p data-estimate-note role="status"></p>`:grams!==''?I18n.html('<p>设备提供了克数，请核对后保存。</p>'):I18n.html('<p>设备未提供有效余量，请确认或称重后填写。</p>')}</div>`);
  if(percent!==null){const capacity=form.elements.namedItem('capacity'),remaining=form.elements.namedItem('remaining'),button=form.querySelector('[data-estimate-remaining]'),note=form.querySelector('[data-estimate-note]');capacity.addEventListener('input',()=>{const n=Number(capacity.value);button.disabled=!Number.isInteger(n)||n<1||n>10000;if(note.textContent)note.textContent=I18n.t('标称重量已变化，请重新估算或手动确认净余量')});button.onclick=()=>{const n=Number(capacity.value);if(!Number.isInteger(n)||n<1||n>10000)return;remaining.value=String(Math.round(n*percent/100));note.textContent=I18n.t('已按设备百分比估算，请确认后保存；后续遥测不会覆盖库存余量。')}}
 }
 function bind(root){
  root.querySelectorAll('[data-register-feed]').forEach(b=>b.onclick=()=>register(Number(b.dataset.registerFeed),b.dataset.feedSlot));
  root.querySelectorAll('[data-machine-spool]').forEach(b=>b.onclick=()=>spoolForm(spoolById(b.dataset.machineSpool)));
 }
 return {action,discovery,bind,register};
})();
