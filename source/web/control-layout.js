'use strict';
// Presentation only: all commands continue through the existing guarded handlers.
const ControlLayout=(()=>{
 let printerId=0,active='overview',inventoryOpen=false;
 function selectPrinter(id){if(printerId!==id){printerId=id;active='overview';inventoryOpen=false}}
 function arrange(root,p,j,records,focusTab){
  const take=selector=>root.querySelector(selector),shell=document.createElement('div');
  shell.className='control-shell';root.classList.add('control-redesign');
  const head=document.createElement('header');head.className='control-identity';
  const art=take('.control-art');art.querySelector('.control-art-label')?.remove();art.className='control-identity-art';
  head.append(art,take('.control-machine-head'),take('.control-connection'));
  const topline=take('.control-topline');
  if(data.capabilities?.deviceMode==='simulated')head.querySelector('.control-connection').append(topline.querySelector('.control-mode'));
  const nav=document.createElement('div');nav.className='control-tabs';nav.setAttribute('role','tablist');nav.setAttribute('aria-label','打印机控制台分区');
  const tabs=[['overview','监控与控制'],['materials','耗材与供料'],['history','操作记录']];
  nav.innerHTML=tabs.map(([id,label])=>`<button type="button" role="tab" id="controlTab-${id}" data-control-tab="${id}" aria-controls="controlPane-${id}">${label}${id==='history'?`<span>${records.length}</span>`:''}</button>`).join('');
  const overview=document.createElement('div');overview.className='control-overview';overview.id='controlPane-overview';
  const materials=document.createElement('div');materials.className='control-materials-pane';materials.id='controlPane-materials';
  const history=take('.control-history');history.id='controlPane-history';
  const visual=take('.control-visual');visual.querySelector('.control-temperatures')?.remove();visual.querySelector('.control-readonly')?.remove();
  const paths=take('.equipment-paths'),notice=take('#controlNotice'),connectionReview=take('#connectionReviewNotice');
  const work=take('.control-work');work.classList.toggle('control-task-empty',!j&&!['printing','pause'].includes(p.state));
  if(!j&&!['printing','pause'].includes(p.state)){
   work.querySelector('.control-filename').textContent=p.state==='idle'?'打印机已就绪':'等待设备连接';
   work.querySelector('.control-task-meta').textContent=p.state==='idle'?'选择队列中的任务，在确认准备后开始。':'连接恢复后显示任务与实时状态。';
  }
  visual.append(work);
  const settings=take('.equipment-panel');overview.append(visual,settings);
  const alerts=document.createElement('div');alerts.className='control-alerts';
  const errors=take('.equipment-errors');if(errors)alerts.append(errors);
  notice.classList.toggle('control-quiet-notice',!notice.classList.contains('warning')&&!records.some(r=>r.state==='pending'));alerts.append(notice,connectionReview);
  const live=take('.equipment-live-supply');materials.append(live);if(paths)materials.append(paths);
  const inventory=document.createElement('details');inventory.className='control-inventory';inventory.open=inventoryOpen;
  inventory.innerHTML='<summary><span>库存装载记录<small>查看已登记料卷、可分配量与归属</small></span><span class="control-disclosure-arrow" aria-hidden="true">⌄</span></summary>';
  inventory.append(take('.control-supply'));inventory.addEventListener('toggle',()=>{inventoryOpen=inventory.open});materials.append(inventory);
  const panes={overview,materials,history};
  for(const [id,pane] of Object.entries(panes)){pane.setAttribute('role','tabpanel');pane.setAttribute('aria-labelledby','controlTab-'+id);pane.tabIndex=0}
  function activate(id,focus=false){
   active=id;
   for(const [key,pane] of Object.entries(panes))pane.hidden=key!==id;
   nav.querySelectorAll('[role=tab]').forEach(button=>{const selected=button.dataset.controlTab===id;button.setAttribute('aria-selected',String(selected));button.tabIndex=selected?0:-1});
   PrinterCamera.setVisible?.(id==='overview');
   if(focus)nav.querySelector(`[data-control-tab="${id}"]`).focus({preventScroll:true});
  }
  nav.querySelectorAll('button').forEach(button=>button.onclick=()=>activate(button.dataset.controlTab));
  nav.onkeydown=event=>{
   if(!['ArrowLeft','ArrowRight','Home','End'].includes(event.key))return;event.preventDefault();
   const index=tabs.findIndex(([id])=>id===active),next=event.key==='Home'?0:event.key==='End'?tabs.length-1:(index+(event.key==='ArrowRight'?1:-1)+tabs.length)%tabs.length;
   activate(tabs[next][0],true);
  };
  shell.append(head,alerts,nav,overview,materials,history);root.replaceChildren(shell);activate(active);
  if(focusTab)nav.querySelector(`[data-control-tab="${focusTab}"]`)?.focus({preventScroll:true});
 }
 return {selectPrinter,arrange};
})();
