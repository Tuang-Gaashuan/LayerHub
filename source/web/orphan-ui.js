'use strict';
const OrphanUI=(()=>{
 function render(p,records,root){
  if(!data.user?.admin)return;
  if(p.completionReview){
   const notice=document.createElement('section');notice.className='motion-review completion-recovery';notice.innerHTML=I18n.html('<strong>补登记取件与用量</strong><p>设备已完成打印，原任务记录缺失。库存没有自动扣减；请核对实际料卷和用量，再确认取件。</p><button class="button primary" id="settleMissingTask">补登记取件与用量</button>');
   notice.querySelector('button').onclick=()=>settle(p);root.querySelector('.control-notice').after(notice);
  }
  const entries=records.filter(r=>r.state==='uncertain'&&r.action==='start'&&!data.jobs.some(j=>j.id===r.jobId));if(!entries.length)return;
  const box=document.createElement('div');box.className='motion-review';box.innerHTML=I18n.html('<strong>设备任务记录需要恢复核对</strong><p>原任务已缺失，保留了启动记录。核对只解除对应指令锁，不重建任务、不扣减耗材，也不会发送打印指令。</p>')+entries.map(r=>`<button class="button" data-orphan-id="${r.id}">${esc(I18n.msg`核对孤立任务 job-${r.jobId}`)}</button>`).join('');
  box.querySelectorAll('[data-orphan-id]').forEach(b=>b.onclick=()=>open(p,entries.find(r=>r.id===Number(b.dataset.orphanId))));root.querySelector('.control-history').append(box);
 }
 function open(p,r){
  const expected='job-'+r.jobId;
  modal(I18n.t('核对孤立启动记录'),I18n.html`<p>${esc(p.name)} · ${esc(expected)}</p><p>请现场核对设备任务和运行状态。此操作不会恢复已丢失的任务、文件归属或耗材账目。</p><form id="orphanReview"><label><span class="field-label">输入设备任务标识</span><input class="input" name="confirmTask" autocomplete="off" required placeholder="${esc(expected)}"></label><label class="check-row"><input type="checkbox" name="confirmed" required>我已核对本机确实正在执行或已完成上述任务</label><p class="form-error" id="orphanReviewError"></p></form>`,I18n.html('<button class="button" data-cancel>取消</button><button class="button primary" type="submit" form="orphanReview" disabled>记录核对结果</button>'));
  const form=$('#orphanReview'),button=$('[form="orphanReview"]');let busy=false,unknown=false;
  const check=()=>{button.disabled=busy||unknown||form.elements.confirmTask.value.trim()!==expected||!form.elements.confirmed.checked||!Remote.ready()};form.oninput=check;form.onchange=check;
  form.onsubmit=async e=>{e.preventDefault();if(button.disabled||busy||unknown)return;busy=true;check();try{await Remote.request('/api/printers/orphans/review',{printerId:p.id,commandId:r.id,confirmTask:form.elements.confirmTask.value.trim(),confirmed:form.elements.confirmed.checked});$('#modal').close();await Remote.refresh();PrinterControl.open(p.id)}catch(e){unknown=!!e.unconfirmed;$('#orphanReviewError').textContent=I18n.system(e.message)+(unknown?I18n.t('请重新读取控制记录，不要重复提交。'):'')}finally{busy=false;check()}};
 }
 function settle(p){
  const v=p.completionReview,expected='job-'+v.jobId;
  const spools=data.spools.filter(s=>Workspace.editable(s,data));
  modal(I18n.t('补登记取件与用量'),I18n.html`<p>${esc(p.name)} · ${esc(expected)}</p><p>此登记不会重建原任务或发送打印完成邮件。请按实际消耗填写，不会自动采用估算。</p><form id="orphanSettlement"><label><span class="field-label">输入设备任务标识</span><input class="input" name="task" required autocomplete="off" placeholder="${esc(expected)}"></label><div id="orphanUses">${spools.map(s=>`<label><span class="field-label">${esc(s.label)} · ${esc(s.material)} · ${esc(s.colorName)} / g</span><input class="input" type="number" min="0" max="${Workspace.available(data,s)}" step="0.001" data-use="${esc(s.id)}" placeholder="${esc(I18n.t('未使用请留空'))}"></label>`).join('')}</div><label><span class="field-label">核对说明</span><textarea class="input" name="note" required maxlength="500"></textarea></label><label class="check-row"><input type="checkbox" name="collected" required>我已取件并清空打印板，确认以上实际用量</label><p class="form-error" id="orphanSettleError"></p></form>`,I18n.html('<button class="button" data-cancel>返回</button><button class="button primary" form="orphanSettlement" type="submit">确认取件并结算</button>'));
  const f=$('#orphanSettlement'),button=$('[form="orphanSettlement"]');let busy=false,unknown=false;
  if(!spools.length){const empty=document.createElement('p');empty.className='form-help';empty.textContent=I18n.t('没有可登记用量的耗材。请先在耗材库登记团队公共或本人的料卷。');$('#orphanUses').append(empty)}
  const connected=Remote.printerFresh(p);
  if(!connected){const notice=document.createElement('p');notice.className='form-error';notice.textContent=I18n.t('打印机当前离线，恢复连接后才能确认取件与用量。');f.prepend(notice)}
  const update=()=>{button.disabled=busy||unknown||!spools.length||!connected};update();
  f.onsubmit=async e=>{e.preventDefault();if(busy||unknown||!spools.length||!connected)return;
   const uses=Object.fromEntries([...f.querySelectorAll('[data-use]')].filter(x=>x.value.trim()!=='').map(x=>[x.dataset.use,Number(x.value)]));
   if(f.elements.task.value.trim()!==expected||!Object.keys(uses).length){$('#orphanSettleError').textContent=I18n.t('请核对任务标识并填写实际使用的料卷');return}
   busy=true;button.disabled=true;
   try{await Remote.request('/api/printers/orphans/settle',{printerId:p.id,jobId:v.jobId,confirmTask:expected,collected:f.elements.collected.checked,uses,note:f.elements.note.value});$('#modal').close();await Remote.refresh();PrinterControl.open(p.id)}catch(error){unknown=!!error.unconfirmed;$('#orphanSettleError').textContent=I18n.system(error.message)+(unknown?I18n.t('请重新读取控制记录，不要重复提交。'):'')}finally{busy=false;update()}
  };
 }
 return {render};
})();
