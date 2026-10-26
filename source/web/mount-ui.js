'use strict';
let materialMountFilter='all';
function mountFilter(){
  return I18n.html`<label class="mount-filter"><span>${icon('printer')} 装载设备</span><select id="materialMountFilter" aria-label="按装载设备筛选"><option value="all" ${materialMountFilter==='all'?'selected':''}>全部位置</option><option value="stock" ${materialMountFilter==='stock'?'selected':''}>未装载 · 库存中</option>${data.printers.map(p=>`<option value="${p.id}" ${String(p.id)===materialMountFilter?'selected':''}>${esc(p.name)}</option>`).join('')}</select></label>`;
}
function spoolMountFields(spool){
  return I18n.html`<fieldset class="mount-fields"><legend>实际装载位置</legend><label><span class="field-label">装载打印机</span><select id="mountPrinter" name="mountPrinter"><option value="0">未装载 · 库存中</option>${data.printers.map(p=>`<option value="${p.id}" ${spool.mount?.printerId===p.id?'selected':''}>${esc(p.name)}${p.enabled?'':I18n.t(' · 已停用')}</option>`).join('')}</select></label><label id="mountSlotField"><span class="field-label">供料位置 / 槽位</span><select id="mountSlot" name="mountSlot"></select></label><p class="form-help">手动记录实际放置位置。登记或调整任务不会自动改变装载记录。</p></fieldset>`;
}
function bindSpoolMountFields(spool){
  $('#mountSlotField').insertAdjacentHTML('afterend',I18n.html('<div class="mount-custom-fields" id="customMountFields" hidden><label><span class="field-label">AMS 单元编号（设备上报 ID）</span><input class="input" id="mountUnit" type="number" min="1" max="254" step="1" value="1"></label><label><span class="field-label">单元内槽位（1–4）</span><input class="input" id="mountUnitSlot" type="number" min="1" max="4" step="1" value="1"></label></div>'));
  const custom=()=>{$('#customMountFields').hidden=$('#mountSlot').value!=='custom'};
  const update=()=>{
    const p=data.printers.find(p=>p.id===Number($('#mountPrinter').value));
    $('#mountSlotField').hidden=!p;
    const selected=p&&spool.mount&&p.id===spool.mount.printerId?spool.mount.slot:'external';
    $('#mountSlot').innerHTML=p?Workspace.slots(p).map(s=>`<option value="${s.id}" ${s.id===selected?'selected':''}>${esc(s.label)}</option>`).join('')+(p.configuration==='ams'?I18n.html('<option value="custom">登记其他 AMS 单元…</option>'):''):'';
    custom();
  };
  $('#mountPrinter').onchange=update;$('#mountSlot').onchange=custom;update();
}
function readSpoolMount(form){
  const f=new FormData(form),printerId=Number(f.get('mountPrinter'));
  const slot=f.get('mountSlot')==='custom'?`ams:${Number($('#mountUnit').value)}:${Number($('#mountUnitSlot').value)-1}`:f.get('mountSlot');
  return printerId?{printerId,slot}:null;
}
function printerMountedSpools(p){
  const slots=Workspace.slots(p);
  return I18n.html`<section class="mounted-spools"><div class="mounted-heading"><h3>已登记的装载耗材</h3><span>手动记录</span></div><div class="mounted-slot-list">${slots.map(slot=>{
    const s=data.spools.find(s=>s.mount?.printerId===p.id&&s.mount.slot===slot.id),visible=s&&Workspace.visible(s,data),privateMount=data.occupiedSlots?.some(m=>m.printerId===p.id&&m.slot===slot.id);
    return `<div class="mounted-slot">${visible?spoolRing(s,true):`<span class="empty-slot-icon">${icon('box')}</span>`}<div><small>${esc(slot.label)}</small><strong>${visible?esc(s.label):s||privateMount?I18n.t('个人耗材 · 未共享'):I18n.t('未登记耗材')}</strong>${visible?`<span>${I18n.system(s.remaining)} g · ${esc(SPOOL_STATES[s.state]||I18n.t('状态待确认'))}</span>`:''}</div></div>`;
  }).join('')}</div><button class="text-link" id="viewMountedLibrary">查看本机耗材 ${icon('arrow')}</button></section>`;
}
