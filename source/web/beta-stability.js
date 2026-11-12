'use strict';
const SpoolHistory=(()=>{
 const labels={baseline:'启用时库存',create:'登记新卷',mount:'挂载',unmount:'卸下',adjust:'调整余量',edit:'编辑资料',consume:'任务结算',orphan_consume:'补登记用量',delete:'删除记录'};
 const position=s=>!s?'—':s.mount?printerName(s.mount.printerId)+' · '+(s.mount.slot.startsWith('external')?I18n.t('外置料架')+' '+s.mount.slot:'AMS '+s.mount.slot):I18n.t('库存中');
 async function open(id){
  const back=PrinterControl.returnPoint(),uid=data.user?.id;modal(I18n.t('耗材操作流水'),`<div id="spoolHistoryRoot"><p>${I18n.t('正在读取记录…')}</p></div>`,'',{onDismiss:back});
  const root=$('#spoolHistoryRoot');let cursor=0,busy=false;
  root.innerHTML=`<p class="form-help">${I18n.t('流水从本功能启用时开始记录，早期操作不会补造。')}</p><ol class="spool-event-list"></ol><p role="alert" class="form-error"></p><button class="button" data-history-more>${I18n.t('读取记录')}</button>`;
  const load=async()=>{if(busy||!root.isConnected||data.user?.id!==uid)return;busy=true;const button=root.querySelector('[data-history-more]');button.disabled=true;
   try{const r=await Remote.request('/api/spools/history?id='+encodeURIComponent(id)+'&before='+cursor);if(!root.isConnected||data.user?.id!==uid)return;
    for(const e of r.events){const before=e.before,after=e.after,sp=after||before;root.querySelector('ol').insertAdjacentHTML('beforeend',`<li class="spool-event"><strong>${esc(I18n.t(labels[e.action]||e.action))} · ${esc(sp?.label||id)}</strong><small>${esc(e.actor||I18n.t('系统'))} · ${new Date(e.created).toLocaleString(I18n.locale())}${e.jobId?' · #'+e.jobId:''}</small><p>${before?before.remaining+' g':'—'} → ${after?after.remaining+' g':'—'}</p><p>${esc(position(before))} → ${esc(position(after))}</p>${before&&after&&['label','material','color','capacity','owner','shared','state','location'].some(k=>before[k]!==after[k])?`<details><summary>${I18n.t('资料变更')}</summary>${['label','material','color','capacity','owner','shared','state','location'].filter(k=>before[k]!==after[k]).map(k=>`<p>${esc(I18n.t({label:'耗材名称',material:'材质',color:'颜色预览',capacity:'标称净重（g）',owner:'所属范围',shared:'共享',state:'可用状态',location:'存放位置（未装载时）'}[k]))}: ${esc(String(before[k]))} → ${esc(String(after[k]))}</p>`).join('')}</details>`:''}</li>`)}
    cursor=r.next;button.hidden=!cursor;button.textContent=I18n.t('加载更早记录');root.querySelector('[role="alert"]').textContent='';
   }catch(e){if(root.isConnected)root.querySelector('[role="alert"]').textContent=I18n.system(e.message)}finally{busy=false;button.disabled=false}
  };root.querySelector('[data-history-more]').onclick=load;await load();
 }
 function button(s){return data.capabilities?.spoolHistory?`<button type="button" class="text-link" data-spool-history="${esc(s.id)}">${I18n.t('操作流水')}</button>`:''}
 function bind(root){root.querySelectorAll('[data-spool-history]').forEach(b=>b.onclick=()=>open(b.dataset.spoolHistory))}
 return {open,button,bind};
})();

const CompletionCenter=(()=>{
 const pending=()=>data.jobs.filter(j=>canManageJob(j)&&['printing','paused','stopped'].includes(j.state)&&!j.settledAt&&(j.finishedAt||j.state==='stopped'));
 const missing=()=>data.user?.admin?data.printers.filter(p=>p.completionReview):[];
 const count=()=>pending().length+missing().length;
 function open(){
  const uid=data.user?.id;modal(I18n.t('完工处理'),'<div id="completionCenterRoot"></div>');const root=$('#completionCenterRoot');
  const draw=()=>{if(!root.isConnected||uid!==data.user?.id)return;const jobs=pending(),orphans=missing();root.innerHTML=`<p class="form-help">${I18n.t('设备结束、取件清板和耗材结算分别核对；只在确认用量后扣库存。')}</p>${jobs.map(j=>`<article class="completion-entry"><strong>${esc(j.filename)}</strong><small>${esc(printerName(j.printer))} · #${j.id}</small><p>${I18n.t(j.state==='stopped'?'打印已停止 · 待登记消耗':j.collectedAt?'已取件 · 待结算':'设备已结束 · 待取件及结算')}</p><button class="button primary" data-finish-job="${j.id}">${I18n.t('登记取件与用量')}</button></article>`).join('')}${orphans.map(p=>`<article class="completion-entry"><strong>${esc(p.name)}</strong><p>${I18n.t('设备完工记录需补登记')}</p><button class="button" data-finish-printer="${p.id}">${I18n.t('打开控制台')}</button></article>`).join('')}${!jobs.length&&!orphans.length?`<p>${I18n.t('没有待处理的完工任务')}</p>`:''}<button class="button" data-finish-refresh>${I18n.t('重新读取')}</button><p class="form-error" role="alert"></p>`;root.querySelectorAll('[data-finish-job]').forEach(b=>b.onclick=()=>{const j=data.jobs.find(x=>x.id===Number(b.dataset.finishJob));if(j&&canManageJob(j))SettlementUI.open(j)});root.querySelectorAll('[data-finish-printer]').forEach(b=>b.onclick=()=>PrinterControl.open(Number(b.dataset.finishPrinter)));root.querySelector('[data-finish-refresh]').onclick=async()=>{try{await Remote.refresh({renderPage:false});draw()}catch(e){if(root.isConnected)root.querySelector('[role="alert"]').textContent=I18n.system(e.message)}};
  };draw();
 }
 function decorate(){
  let nav=$('#completionCenterButton');if(!nav){const mine=$('[data-page="mine"]');if(!mine)return;nav=document.createElement('button');nav.id='completionCenterButton';nav.className='nav-item';mine.after(nav);nav.onclick=open}
  nav.hidden=!data.capabilities?.settlement;nav.textContent=I18n.t('完工处理')+(count()?' · '+count():'');
  if(!['board','queues','mine'].includes(page)||!count())return;
  if(!$('#completionSummary')){$('#pageContent').insertAdjacentHTML('afterbegin',`<div id="completionSummary" class="beta-summary"><span>${I18n.t('有打印已结束，等待取件与用量确认')} · ${count()}</span><button class="button small" data-open-completions>${I18n.t('去处理')}</button></div>`);$('[data-open-completions]').onclick=open}
 }
 return {open,decorate};
})();

const OperationRecovery=(()=>{
 const key='layerhub.operations.v1',pending=new Map();let checking=false,lastCheck=0;
 try{for(const v of JSON.parse(sessionStorage.getItem(key)||'[]'))if(/^[a-f0-9]{32}$/.test(v.id)&&Number.isInteger(v.user))pending.set(v.id,v)}catch{}
 const save=()=>{try{sessionStorage.setItem(key,JSON.stringify([...pending.values()].map(({id,user,path,at})=>({id,user,path,at}))))}catch{}};
 function begin(path,payload){if(!data.capabilities?.operationRecovery||payload===undefined||!['/api/spools','/api/spools/replace','/api/spools/delete','/api/jobs/settle'].includes(path))return null;const id=Array.from(crypto.getRandomValues(new Uint8Array(16)),v=>v.toString(16).padStart(2,'0')).join(''),op={id,user:data.user.id,path,at:Date.now(),owner:$('#modalContent')?.firstElementChild};pending.set(id,op);save();return op}
 function done(op){if(op){pending.delete(op.id);save()}}
 async function check(force=false){if(checking||!Remote.ready()||!data.capabilities?.operationRecovery||!force&&Date.now()-lastCheck<5000)return;checking=true;lastCheck=Date.now();
  try{for(const op of [...pending.values()].filter(v=>v.user===data.user?.id&&Date.now()-v.at>1000).sort((a,b)=>(a.checkedAt||0)-(b.checkedAt||0)).slice(0,8)){
   op.checkedAt=Date.now();const r=await Remote.request('/api/operations/result?id='+op.id);if(data.user?.id!==op.user)return;if(r.state!=='committed')continue;op.committed=true;await Remote.refresh({renderPage:!$('#modal').open});if(data.user?.id!==op.user)return;done(op);toast(I18n.t('已核对：上次操作已成功保存'));
   if(op.owner?.isConnected&&$('#modal').open){const body=$('#modal .modal-body');body.querySelectorAll('.form-error').forEach(e=>e.textContent='');body.querySelectorAll('form').forEach(f=>{f.dataset.uncertain='true';body.querySelectorAll('[type=submit]').forEach(b=>b.disabled=true);document.querySelectorAll('[form="'+f.id+'"][type=submit]').forEach(b=>b.disabled=true)});body.querySelector('[data-recovered]')?.remove();body.insertAdjacentHTML('beforeend',`<p role="status" data-recovered>${I18n.t('上次操作已保存，请关闭表单并查看最新记录。')} <button type="button" class="button small" data-recovered-close>${I18n.t('查看最新状态')}</button></p>`);body.querySelector('[data-recovered-close]').onclick=()=>{dismissModal();Remote.refresh({renderPage:!$('#modal').open}).catch(()=>{})}}
  }}catch{}finally{checking=false;paint()}
 }
 function open(){
  const user=data.user?.id;modal(I18n.t('待核对操作'),`<div id="operationReviewRoot"><p>${I18n.t('没有回执不能证明失败。请先核对耗材流水或任务结算，不要重复提交。')}</p></div>`);const root=$('#operationReviewRoot');
  for(const op of [...pending.values()].filter(v=>v.user===user)){const row=document.createElement('article');row.className='completion-entry';row.innerHTML=`<strong>${I18n.t(op.path==='/api/jobs/settle'?'任务结算':'耗材操作')}</strong><small>${new Date(op.at).toLocaleString(I18n.locale())}</small><label><input type="checkbox"> ${I18n.t('我已核对最新记录，仅清除此提示')}</label><button class="button" disabled>${I18n.t('清除提示')}</button>`;root.append(row);row.querySelector('input').onchange=e=>row.querySelector('button').disabled=!e.target.checked;row.querySelector('button').onclick=async()=>{if(!row.querySelector('input').checked||data.user?.id!==user)return;try{await Remote.refresh({renderPage:false});if(!root.isConnected||data.user?.id!==user)return;done(op);row.remove();paint()}catch(e){toast(I18n.system(e.message))}}}
 }
 function paint(){let el=$('#operationRecoveryBanner');const count=[...pending.values()].filter(o=>o.user===data.user?.id&&Date.now()-o.at>1000).length;if(!count){el?.remove();return}if(!el){$('#pageContent')?.insertAdjacentHTML('afterbegin',`<div id="operationRecoveryBanner" class="beta-summary"><span></span><button class="button small">${I18n.t('核对上次操作')}</button><button class="text-link" data-review-operations>${I18n.t('查看待核对操作')}</button></div>`);el=$('#operationRecoveryBanner');if(!el)return;el.querySelector('button').onclick=()=>check(true);el.querySelector('[data-review-operations]').onclick=open}el.querySelector('span').textContent=I18n.t('有操作结果尚待核对，正在只读查询服务端记录。')+' '+count}
 return {begin,done,check,paint,open};
})();
