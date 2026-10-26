'use strict';
const PrintFlow=(()=>{
 function summary(j){
  const p=data.printers.find(p=>p.id===j.printer),ids=j.printOptions?.mapping?.map(x=>x.spoolId)||[j.spoolId];
  return `<div class="print-summary"><strong>${esc(j.filename)}</strong><span>${esc(p?.name||'未指定设备')} · 预计 ${Number(j.estimate)||0} 分钟</span>${[...new Set(ids)].map(id=>{const s=data.spools.find(s=>s.id===id);return s?`<span class="print-roll"><i style="background:${esc(s.color)}"></i>${esc(s.label)} · ${esc(s.colorName)} · ${esc(s.mount?.slot==='external'?'外挂':`AMS ${s.mount?.slot||'未装载'}`)}</span>`:''}).join('')}</div>`;
 }
 function open(j){
  if(!['queued','assigned'].includes(j.state))return toast('任务状态已变化，请查看我的任务');
  modal('准备好后，一步开始打印',`${summary(j)}<p class="print-flow-note">确认打印板已清空、耗材已装好。${j.state==='queued'?'确认后自动传输文件，再启动打印。':'文件已传输，确认后直接启动打印。'}</p><p class="form-help">服务器会重新检查设备、队首和供料；检查失败时停止本次操作。打印完成后自动通知任务提交者。</p><p id="dispatchError" class="form-error" role="alert"></p>`,`<button class="button" data-cancel>稍后再打</button><button class="button primary" id="realSend" ${data.capabilities?.printStart?'':'disabled'}>确认并打印</button>`);
  const button=$('#realSend');button.dataset.printerId=String(j.printer);Remote.paintConnection();
  button.onclick=async()=>{if(button.disabled||button.dataset.busy)return;button.dataset.busy='1';button.disabled=true;try{await Remote.request('/api/jobs/dispatch',{jobId:j.id});$('#modal').close();await Remote.refresh();PrinterControl.open(j.printer);toast('打印请求已提交，正在传输并等待设备确认。')}catch(e){if($('#dispatchError'))$('#dispatchError').textContent=e.message;else toast(e.message)}finally{delete button.dataset.busy;button.disabled=!data.capabilities?.printStart;Remote.paintConnection()}};
 }
 return {open};
})();
