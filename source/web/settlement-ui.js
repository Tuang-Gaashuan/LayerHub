'use strict';
const SettlementUI=(()=>{
 const grams=n=>Number(n).toLocaleString(I18n.locale(),{maximumFractionDigits:3})+' g';
 async function open(j){
  modal(I18n.t('取件与耗材结算'),I18n.html('<div id="settlementRoot" class="settlement-root"><p role="status">正在读取任务与耗材记录…</p></div>'));
  $('#modal').classList.add('settlement-modal');
  const root=$('#settlementRoot');
  try{
   const result=await Remote.request('/api/jobs/settlement?jobId='+j.id);
   if(!root.isConnected)return;
   draw(root,result);
  }catch(e){if(root.isConnected)root.innerHTML=I18n.html`<p class="form-error" role="alert">${esc(I18n.system(e.message))}</p><button class="button" id="settlementRead">重新读取</button>`;if($('#settlementRead'))$('#settlementRead').onclick=()=>open(j)}
 }
 function draw(root,{job:j,rolls}){
  const archived=['done','cancelled'].includes(j.state),finished=!!j.finishedAt,allowed=finished||j.state==='stopped';
  root.innerHTML=I18n.html`<div class="settlement-heading"><span class="settlement-mark">${icon(archived?'circleCheck':'box')}</span><div><strong>${esc(j.filename)}</strong><p>${archived?I18n.t('已归档 · 不会再次扣减'):finished?I18n.t('打印完成 · 待登记'):j.state==='stopped'?I18n.t('打印已停止 · 登记已消耗用量'):I18n.t('等待设备确认打印结束')}</p></div></div><div class="settlement-steps"><span class="${finished?'complete':''}">1 设备结束</span><span class="${j.collectedAt?'complete':''}">2 取件清板</span><span class="${archived?'complete':''}">3 用量结算</span></div><form id="settlementForm">${rolls.map((r,i)=>I18n.html`<section class="settlement-roll" data-roll="${i}"><div class="settlement-roll-title"><i style="background:${esc(r.color)}"></i><strong>${esc(r.label)}</strong>${archived?`<span>${r.source==='estimated'?I18n.t('按切片估算'):r.source==='measured'?I18n.t('按实测用量'):I18n.t('历史登记')}</span>`:''}</div><dl class="settlement-stats"><div><dt>切片估算</dt><dd>${r.estimate==null?I18n.t('暂无'):grams(r.estimate)}</dd></div><div><dt>${archived?I18n.t('原预留（已释放）'):I18n.t('本任务预留')}</dt><dd>${grams(r.reserved)}</dd></div><div><dt>${archived?I18n.t('库存已扣减'):I18n.t('可用于结算')}</dt><dd>${grams(archived?r.booked??0:r.available)}</dd></div></dl>${archived?I18n.html`<p>登记用量 ${r.reported==null?I18n.t('未记录精度'):grams(r.reported)}</p>`:I18n.html`<div class="settlement-source" role="group" aria-label="${esc(r.label)}用量来源"><label><input type="radio" name="source${i}" value="estimated" ${r.estimate==null?'disabled':'checked'}> 按切片估算</label><label><input type="radio" name="source${i}" value="measured" ${r.estimate==null?'checked':''}> 填写实测用量</label></div><label class="settlement-measured" ${r.estimate==null?'':'hidden'}>实际净用量（g）<input class="input" type="number" name="grams${i}" min="0" step="0.001" max="${r.available}" placeholder="称重后填写" ${r.estimate==null?'required':'disabled'}></label><p class="settlement-booked" aria-live="polite"></p>`}</section>`).join('')}${archived?'':I18n.html`<label class="settlement-collected"><input type="checkbox" name="collected" required> 我已取走成品或残料，并清空打印板</label><p class="form-help">库存按整克记账，小数用量向上取整；保留原始用量和来源，释放本任务全部预留。完成邮件在设备结束时自动发送。</p>${!allowed?I18n.html('<p class="form-error">当前还没有设备完成记录，请先核对打印机状态；停止任务可登记已消耗用量。</p>'):''}<p class="form-error" id="settlementError" role="alert"></p><div class="settlement-actions"><button type="button" class="button" data-cancel>稍后登记</button><button type="submit" class="button primary" ${allowed?'':'disabled'}>确认取件并结算</button></div>`}</form>`;
  if(archived)return;
  const form=root.querySelector('form');
  form.querySelector('[data-cancel]').onclick=()=>$('#modal').close();
  function update(){rolls.forEach((r,i)=>{const measured=form.elements['source'+i].value==='measured',input=form.elements['grams'+i],section=root.querySelector(`[data-roll="${i}"]`);input.disabled=!measured;input.required=measured;input.parentElement.hidden=!measured;const value=measured?(input.value===''?NaN:Number(input.value)):r.estimate;section.querySelector('.settlement-booked').textContent=Number.isFinite(value)?I18n.msg`将扣减 ${grams(Math.ceil(value))}，释放 ${grams(r.reserved)} 预留${measured?'':I18n.t(' · 估算结算，不代表称重实测')}`:I18n.t('填写实测后显示扣减量')})}
  form.onchange=update;form.oninput=update;update();
  form.onsubmit=async e=>{
   e.preventDefault();if(form.dataset.submitting)return;
   form.dataset.submitting='true';const submit=form.querySelector('[type=submit]');submit.disabled=true;
   const uses=Object.fromEntries(rolls.map((r,i)=>[r.spoolId,{source:form.elements['source'+i].value,grams:Number(form.elements['grams'+i].value)||0}]));
   try{await Remote.request('/api/jobs/settle',{jobId:j.id,collected:form.elements.collected.checked,uses});$('#modal').close();await Remote.refresh();toast(I18n.t('已登记取件与用量，任务已归档'))}
   catch(error){if(!form.isConnected)return;$('#settlementError').textContent=I18n.system(error.message);if(error.unconfirmed){$('#settlementError').textContent+=I18n.t(' 请重新读取记录确认结果，不要重复提交。');const read=document.createElement('button');read.type='button';read.className='button';read.textContent=I18n.t('重新读取结算记录');read.onclick=()=>open(j);form.append(read)}else{delete form.dataset.submitting;submit.disabled=!allowed}}
  };
 }
 return {open};
})();
