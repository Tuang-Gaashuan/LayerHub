'use strict';
// Presentation only: all commands continue through the existing guarded handlers.
const ControlLayout=(()=>{
 let printerId=0,active='overview',inventoryOpen=false;
 // Update the existing console in place. Tabs, disclosure, focused controls and
 // the camera keep their DOM identity; only changed readings are patched.
 function patch(old,next){
  if(old.nodeType!==next.nodeType||old.nodeName!==next.nodeName||old.nodeType===1&&(old.id!==next.id||old.dataset.equipment!==next.dataset.equipment)){old.replaceWith(next);return}
  if(old.nodeType!==1){if(old.nodeValue!==next.nodeValue)old.nodeValue=next.nodeValue;return}
  for(const a of [...old.attributes])if(!next.hasAttribute(a.name))old.removeAttribute(a.name);
  for(const a of [...next.attributes])if(old.getAttribute(a.name)!==a.value)old.setAttribute(a.name,a.value);
  for(const event of ['onclick','onchange','onkeydown','oninput'])old[event]=next[event];
  const previous=[...old.childNodes],incoming=[...next.childNodes];
  incoming.forEach((node,i)=>previous[i]?patch(previous[i],node):old.append(node));
  previous.slice(incoming.length).forEach(node=>node.remove());
 }
 function update(root,next,p,j,records){
  // These notices belong outside the tab panels. Detach them before patching
  // the task, whose unarranged template originally contains both nodes.
  const alerts=document.createElement('div');alerts.className='control-alerts';
  const errors=next.querySelector('.equipment-errors'),notice=next.querySelector('#controlNotice');
  if(errors)alerts.append(errors);
  notice.classList.toggle('control-quiet-notice',!notice.classList.contains('warning')&&!records.some(r=>r.state==='pending'));
  alerts.append(notice,next.querySelector('#connectionReviewNotice'));
  const recovery=next.querySelector('.completion-recovery');if(recovery)alerts.append(recovery);
  const refresh=selector=>{const old=root.querySelector(selector),fresh=next.querySelector(selector);if(old&&fresh)patch(old,fresh)};
  refresh('.control-machine-head');refresh('.control-connection');
  refresh('.control-work');refresh('.equipment-panel');refresh('.equipment-live-supply');refresh('.control-supply');
  const oldMotion=root.querySelector('.motion-panel'),newMotion=next.querySelector('.motion-panel');
  if(oldMotion&&newMotion){const checked=newMotion.querySelector('[data-motion-clear]').checked;for(const a of ['id','role','aria-labelledby','tabindex'])newMotion.setAttribute(a,oldMotion.getAttribute(a)||'');newMotion.hidden=oldMotion.hidden;patch(oldMotion,newMotion);oldMotion.querySelector('[data-motion-clear]').checked=checked}
  const work=root.querySelector('.control-work');work.classList.toggle('control-task-empty',!j&&!p.completionReview&&!['printing','pause'].includes(p.state));
  if(!j&&!p.completionReview&&!['printing','pause'].includes(p.state)){
   work.querySelector('.control-filename').textContent=p.state==='idle'?I18n.t('打印机已就绪'):I18n.t('等待设备连接');
   work.querySelector('.control-task-meta').textContent=p.state==='idle'?I18n.t('选择队列中的任务，在确认准备后开始。'):I18n.t('连接恢复后显示任务与实时状态。');
  }
  const history=root.querySelector('.control-history'),freshHistory=next.querySelector('.control-history');
  freshHistory.id=history.id;freshHistory.setAttribute('role','tabpanel');freshHistory.setAttribute('aria-labelledby','controlTab-history');freshHistory.tabIndex=0;freshHistory.hidden=history.hidden;patch(history,freshHistory);
  root.querySelector('[data-control-tab="history"] span').textContent=String(records.length);
  patch(root.querySelector('.control-alerts'),alerts);
  const paths=root.querySelector('.equipment-paths'),freshPaths=next.querySelector('.equipment-paths');
  if(paths&&freshPaths)patch(paths,freshPaths);else if(paths)paths.remove();else if(freshPaths)root.querySelector('.control-materials-pane').append(freshPaths);
 }
 function selectPrinter(id){if(printerId!==id){printerId=id;active='overview';inventoryOpen=false}}
 function arrange(root,p,j,records,focusTab){
  const take=selector=>root.querySelector(selector),shell=document.createElement('div');
  shell.className='control-shell';root.classList.add('control-redesign');
  const head=document.createElement('header');head.className='control-identity';
  const art=take('.control-art');art.querySelector('.control-art-label')?.remove();art.className='control-identity-art';
  head.append(art,take('.control-machine-head'),take('.control-connection'));
  const topline=take('.control-topline');
  if(data.capabilities?.deviceMode==='simulated')head.querySelector('.control-connection').append(topline.querySelector('.control-mode'));
  const nav=document.createElement('div');nav.className='control-tabs';nav.setAttribute('role','tablist');nav.setAttribute('aria-label',I18n.t('打印机控制台分区'));
  const tabs=[['overview',I18n.t('监控与控制')],['materials',I18n.t('耗材与供料')],['motion',I18n.t('手动移动')],['history',I18n.t('操作记录')]];
  nav.innerHTML=tabs.map(([id,label])=>`<button type="button" role="tab" id="controlTab-${id}" data-control-tab="${id}" aria-controls="controlPane-${id}">${label}${id==='history'?`<span>${records.length}</span>`:''}</button>`).join('');
  const overview=document.createElement('div');overview.className='control-overview';overview.id='controlPane-overview';
  const materials=document.createElement('div');materials.className='control-materials-pane';materials.id='controlPane-materials';
  const history=take('.control-history');history.id='controlPane-history';
  const visual=take('.control-visual');visual.querySelector('.control-temperatures')?.remove();visual.querySelector('.control-readonly')?.remove();
  const paths=take('.equipment-paths'),notice=take('#controlNotice'),connectionReview=take('#connectionReviewNotice');
  const work=take('.control-work');work.classList.toggle('control-task-empty',!j&&!p.completionReview&&!['printing','pause'].includes(p.state));
  if(!j&&!p.completionReview&&!['printing','pause'].includes(p.state)){
   work.querySelector('.control-filename').textContent=p.state==='idle'?I18n.t('打印机已就绪'):I18n.t('等待设备连接');
   work.querySelector('.control-task-meta').textContent=p.state==='idle'?I18n.t('选择队列中的任务，在确认准备后开始。'):I18n.t('连接恢复后显示任务与实时状态。');
  }
  visual.append(work);
  const settings=take('.equipment-panel');overview.append(visual,settings);
  const alerts=document.createElement('div');alerts.className='control-alerts';
  const errors=take('.equipment-errors');if(errors)alerts.append(errors);
  notice.classList.toggle('control-quiet-notice',!notice.classList.contains('warning')&&!records.some(r=>r.state==='pending'));alerts.append(notice,connectionReview);
  const recovery=take('.completion-recovery');if(recovery)alerts.append(recovery);
  const live=take('.equipment-live-supply');materials.append(live);if(paths)materials.append(paths);
  const inventory=document.createElement('details');inventory.className='control-inventory';inventory.open=inventoryOpen;
  inventory.innerHTML=I18n.html('<summary><span>库存装载记录<small>查看已登记料卷、可分配量与归属</small></span><span class="control-disclosure-arrow" aria-hidden="true">⌄</span></summary>');
  inventory.append(take('.control-supply'));inventory.addEventListener('toggle',()=>{inventoryOpen=inventory.open});materials.append(inventory);
  const motion=take('.motion-panel');motion.id='controlPane-motion';
  const panes={overview,materials,motion,history};
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
  shell.append(head,alerts,nav,overview,materials,motion,history);root.replaceChildren(shell);activate(active);
  if(focusTab)nav.querySelector(`[data-control-tab="${focusTab}"]`)?.focus({preventScroll:true});
 }
 return {selectPrinter,arrange,update};
})();
