'use strict';
const SpoolPicker=(()=>{
 const hex=value=>/^#?(?:[0-9a-f]{6}|[0-9a-f]{8})$/i.test(value||'')?'#'+value.replace('#','').slice(0,6).toUpperCase():null;
 function markup(){return `<fieldset class="schedule-spools"><legend>选择一卷耗材 · 按颜色确认</legend><label class="schedule-spool-search"><span class="sr-only">搜索耗材颜色、材质或名称</span><input class="input" type="search" data-spool-search placeholder="搜索颜色、材质或名称…"></label><div data-schedule-spools></div><p class="schedule-spool-summary" role="status" data-spool-summary>尚未选择料卷</p><p class="form-help">装载位置是登记记录；实际材质与供料仍需在打印前核对。</p></fieldset>`}
 function bind(form,job={}){
  const root=form.querySelector('[data-schedule-spools]');if(!root)return;
  const search=form.querySelector('[data-spool-search]'),summary=form.querySelector('[data-spool-summary]');
  let selected=job.spoolId||'';
  const printer=()=>Number(form.elements.namedItem('printer').value),grams=()=>Math.max(1,Number(form.elements.namedItem('grams').value)||1);
  function updateSummary(){const s=data.spools.find(s=>s.id===selected&&Workspace.visible(s,data));summary.textContent=s?`已选择：${s.colorName} · ${s.material} · ${s.label} · ${Workspace.available(data,s,job.id)} g 可分配${Workspace.spoolIssue(data,s,grams(),job.id)?' · '+Workspace.spoolIssue(data,s,grams(),job.id):''}`:'尚未选择料卷'}
  function render(){
   const query=search.value.trim().toLowerCase(),target=printer();
   const visible=data.spools.filter(s=>Workspace.visible(s,data));
   const rows=visible.filter(s=>`${s.colorName} ${s.color} ${s.material} ${s.label} ${s.brand}`.toLowerCase().includes(query));
   const groups=target?[[`已装载到所选打印机`,rows.filter(s=>s.mount?.printerId===target)],[`库存与其他设备`,rows.filter(s=>s.mount?.printerId!==target)]]:[['可见料卷',rows]];
   root.innerHTML=groups.filter(([,spools])=>spools.length).map(([name,spools])=>`<div class="schedule-spool-group"><h4>${esc(name)}</h4><div class="schedule-spool-grid">${spools.map(s=>{const issue=Workspace.spoolIssue(data,s,grams(),job.id),color=hex(s.color);return `<label class="schedule-spool-card ${issue?'unavailable':''}"><input type="radio" name="spoolId" value="${esc(s.id)}" ${selected===s.id?'checked':''} ${issue?'disabled':''} aria-label="${esc(s.colorName+' · '+s.material+' · '+s.label)}"><i class="schedule-color-chip ${color?'':'unknown'}" ${color?'style="background:'+color+'"':''} aria-hidden="true"></i><span class="schedule-spool-detail"><strong>${esc(s.colorName||'颜色未命名')} <em>${esc(s.material)}</em></strong><span>${esc(color||'颜色未提供')} · ${esc(s.label)}</span><small>${esc(Workspace.mountLabel(data,s))}</small><b>${Workspace.available(data,s,job.id)} g 可分配${issue?' · '+esc(issue):''}</b></span><span class="schedule-spool-check" aria-hidden="true">✓</span></label>`}).join('')}</div></div>`).join('')||'<p class="schedule-spool-empty">没有符合条件的可见料卷。可调整搜索，或先在耗材库登记。</p>';
   // Keep a selected roll in FormData when searching hides it. The search is
   // a view filter, not an implicit change to the user's stock reservation.
   if(selected&&!rows.some(s=>s.id===selected)&&visible.some(s=>s.id===selected))root.insertAdjacentHTML('beforeend',`<input type="hidden" name="spoolId" value="${esc(selected)}">`);
   root.querySelectorAll('input[type="radio"]').forEach(input=>input.onchange=()=>{selected=input.value;updateSummary()});updateSummary();
  }
  search.oninput=render;form.elements.namedItem('printer').addEventListener('change',render);form.elements.namedItem('grams').addEventListener('input',render);render();
 }
 return {markup,bind};
})();
