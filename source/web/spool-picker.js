'use strict';
const SpoolPicker=(()=>{
 const hex=value=>/^#?(?:[0-9a-f]{6}|[0-9a-f]{8})$/i.test(value||'')?'#'+value.replace('#','').slice(0,6).toUpperCase():null;
 function markup(){return I18n.html`<fieldset class="schedule-spools"><legend>选择一卷耗材 · 按颜色确认</legend><label class="schedule-spool-search"><span class="sr-only">搜索耗材颜色、材质或名称</span><input class="input" type="search" data-spool-search placeholder="搜索颜色、材质或名称…"></label><div data-schedule-spools></div><p class="schedule-spool-summary" role="status" data-spool-summary>尚未选择料卷</p><p class="form-help">装载位置是登记记录；实际材质与供料仍需在打印前核对。</p></fieldset>`}
 function bind(form,job={},options={}){
  const root=form.querySelector('[data-schedule-spools]');if(!root)return;
  const search=form.querySelector('[data-spool-search]'),summary=form.querySelector('[data-spool-summary]');
  let selected=job.spoolId||'';
  const available=s=>Math.max(0,Workspace.available(data,s,job.id)-Number(options.reserved?.[s.id]||0));
  const issue=s=>Workspace.spoolIssue(data,s,grams(),job.id)||(available(s)<grams()?I18n.t('本批次其他盘已预留此卷余量'):null);
  const printer=()=>Number(form.elements.namedItem('printer').value),grams=()=>Math.max(1,Number(form.elements.namedItem('grams').value)||1);
  function updateSummary(){const s=data.spools.find(s=>s.id===selected&&Workspace.visible(s,data));summary.textContent=s?I18n.msg`已选择：${s.colorName} · ${s.material} · ${s.label} · ${available(s)} g 可分配${issue(s)?' · '+issue(s):''}`:I18n.t('尚未选择料卷')}
  function render(){
   const query=search.value.trim().toLowerCase(),target=printer();
   const visible=data.spools.filter(s=>Workspace.visible(s,data)&&(!options.material||s.material.toLowerCase()===options.material.toLowerCase()));
   const rows=visible.filter(s=>`${s.colorName} ${s.color} ${s.material} ${s.label} ${s.brand}`.toLowerCase().includes(query));
   const groups=target?[[I18n.msg`已装载到所选打印机`,rows.filter(s=>s.mount?.printerId===target)],[I18n.msg`库存与其他设备`,rows.filter(s=>s.mount?.printerId!==target)]]:[[I18n.t('可见料卷'),rows]];
   root.innerHTML=groups.filter(([,spools])=>spools.length).map(([name,spools])=>`<div class="schedule-spool-group"><h4>${esc(name)}</h4><div class="schedule-spool-grid">${spools.map(s=>{const problem=issue(s),color=hex(s.color);return I18n.html`<label class="schedule-spool-card ${problem?'unavailable':''}"><input type="radio" name="spoolId" value="${esc(s.id)}" ${selected===s.id?'checked':''} ${problem?'disabled':''} aria-label="${esc(s.colorName+' · '+s.material+' · '+s.label)}"><i class="schedule-color-chip ${color?'':'unknown'}" ${color?'style="background:'+color+'"':''} aria-hidden="true"></i><span class="schedule-spool-detail"><strong>${esc(s.colorName||I18n.t('颜色未命名'))} <em>${esc(s.material)}</em></strong><span>${esc(color||I18n.t('颜色未提供'))} · ${esc(s.label)}</span><small>${esc(Workspace.mountLabel(data,s))}</small><b>${available(s)} g 可分配${problem?' · '+esc(problem):''}</b></span><span class="schedule-spool-check" aria-hidden="true">✓</span></label>`}).join('')}</div></div>`).join('')||I18n.html('<p class="schedule-spool-empty">没有符合条件的可见料卷。可调整搜索，或先在耗材库登记。</p>');
   // Keep a selected roll in FormData when searching hides it. The search is
   // a view filter, not an implicit change to the user's stock reservation.
   if(selected&&!rows.some(s=>s.id===selected)&&visible.some(s=>s.id===selected))root.insertAdjacentHTML('beforeend',`<input type="hidden" name="spoolId" value="${esc(selected)}">`);
   root.querySelectorAll('input[type="radio"]').forEach(input=>input.onchange=()=>{selected=input.value;updateSummary()});updateSummary();
  }
  search.oninput=render;form.elements.namedItem('printer').addEventListener('change',render);form.elements.namedItem('grams').addEventListener('input',render);render();
 }
 return {markup,bind};
})();
