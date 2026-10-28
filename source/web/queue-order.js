'use strict';
const QueueOrder=(()=>{
 const live=new Set(['queued','assigned','uploading','starting','printing','paused','stopped','checking']);
 const busy=new Set(),uncertain=new Set();
 const sequence=pid=>data.jobs.filter(j=>j.printer===pid&&live.has(j.state)).sort((a,b)=>a.order-b.order||a.id-b.id);
 const expected=pid=>sequence(pid).map(j=>({id:j.id,order:j.order,state:j.state}));
 const own=j=>j.userId===data.user?.id;
 function allowed(j,move){
  if(!j||j.state!=='queued'||!j.printer||!data.capabilities?.queueOrder||data.capabilities?.restorePending||!data.user||(!data.user.admin&&!own(j)))return false;
  const waiting=sequence(j.printer).filter(x=>x.state==='queued'),index=waiting.findIndex(x=>x.id===j.id),dest=move==='first'?0:move==='up'?index-1:index+1;
  return index>=0&&dest>=0&&dest<waiting.length&&dest!==index&&(data.user.admin||waiting.slice(Math.min(index,dest),Math.max(index,dest)+1).every(own));
 }
 function paint(){document.querySelectorAll('[data-queue-move]').forEach(b=>{const j=data.jobs.find(j=>j.id===Number(b.dataset.queueJob));b.disabled=!Remote.ready()||busy.has(j?.printer)||uncertain.has(j?.printer)||!allowed(j,b.dataset.queueMove)})}
 function decorate(){
  if(!data.capabilities?.queueOrder)return;
  document.querySelectorAll('[data-queue-entry]').forEach(row=>{
   row.querySelectorAll('.queue-order-tools,.queue-next-label').forEach(n=>n.remove());
   const j=data.jobs.find(j=>j.id===Number(row.dataset.queueEntry));if(!j?.printer)return;
   const copy=row.querySelector('.device-queue-copy'),next=Workspace.queue(data,j.printer).rows[0]?.job;
   if(next?.id===j.id)copy.insertAdjacentHTML('afterbegin',`<span class="queue-next-label">${I18n.t('下一项 · 需确认启动')}</span>`);
   const prepare=row.querySelector('[data-action="assign"]');if(prepare&&j.state==='queued'&&next?.id!==j.id){prepare.disabled=true;prepare.title=I18n.t('请先调整为下一项，或等待前面的任务完成。')}
   if(j.state!=='queued'||(!data.user?.admin&&!own(j)))return;
   copy.insertAdjacentHTML('beforeend',`<details class="queue-order-tools"><summary>${I18n.t('调整顺序')}</summary><div>${[['up',I18n.t('上移')],['down',I18n.t('下移')],['first',I18n.t('设为下一项')]].map(([move,label])=>`<button type="button" class="button small" data-queue-move="${move}" data-queue-job="${j.id}">${label}</button>`).join('')}</div></details>`);
  });
  const panel=document.querySelector('.device-queue-panel');if(panel&&!panel.querySelector('.queue-order-note'))panel.querySelector('.device-queue-tabs').insertAdjacentHTML('afterend',`<div class="queue-order-note"><span>${I18n.t('按顺序逐项执行，每次启动都需确认；不会自动连续打印。')}</span><small>${data.user?.admin?I18n.t('管理员可调整等待任务；已传输和执行中的任务保持原位。'):I18n.t('成员可调整自己连续等待的任务；跨越其他成员请联系管理员。')}</small><p class="form-error" data-queue-order-error role="status"></p><button type="button" class="text-link" data-queue-reread>${I18n.t('重新读取队列')}</button></div>`);
  panel?.querySelectorAll('[data-queue-move]').forEach(b=>b.onclick=()=>move(Number(b.dataset.queueJob),b.dataset.queueMove));
  if(panel?.querySelector('[data-queue-reread]'))panel.querySelector('[data-queue-reread]').onclick=reread;
  paint();
 }
 async function reread(){try{await Remote.refresh();uncertain.clear();paint();toast(I18n.t('已重新读取队列，请核对顺序后再操作'))}catch(e){toast(I18n.system(e.message))}}
 function move(id,direction){
  const j=data.jobs.find(j=>j.id===id);if(!allowed(j,direction)||!Remote.ready()||busy.has(j.printer)||uncertain.has(j.printer))return;
  const payload={printerId:j.printer,jobId:j.id,move:direction,expected:expected(j.printer)},uid=data.user.id;
  if(direction==='first'){
   modal(I18n.t('设为下一项'),`<p>${esc(j.filename)+FileIntake.plateBadge(j)}</p><p>${I18n.t('将此任务移到等待队列最前，不会立即开始打印，也不会打断当前任务。')}</p><p id="queueMoveError" class="form-error" role="status"></p>`,`<button class="button" data-cancel>${I18n.t('取消')}</button><button class="button primary" id="queueMoveConfirm">${I18n.t('确认调整顺序')}</button>`);
   const button=$('#queueMoveConfirm');button.onclick=()=>send(payload,uid,button);return;
  }
  send(payload,uid);
 }
 async function send(payload,uid,button){
  if(data.user?.id!==uid||busy.has(payload.printerId)||uncertain.has(payload.printerId))return;
  busy.add(payload.printerId);if(button)button.disabled=true;paint();
  try{
   await Remote.request('/api/jobs/reorder',payload);if(data.user?.id!==uid)return;
   if(button?.isConnected)$('#modal').close();await Remote.refresh();toast(I18n.t('等待顺序已更新，打印仍需单独确认'));
  }catch(e){
   if(data.user?.id!==uid)return;
   uncertain.add(payload.printerId);const target=button?.isConnected?$('#queueMoveError'):document.querySelector('[data-queue-order-error]');if(target)target.textContent=I18n.system(e.message)+' '+I18n.t('请重新读取队列后核对，不要重复提交。');else toast(I18n.system(e.message));
  }finally{busy.delete(payload.printerId);paint()}
 }
 return {decorate,paint,allowed};
})();
