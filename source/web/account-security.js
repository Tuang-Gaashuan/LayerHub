'use strict';
const AccountSecurity=(()=>{
 const time=(n,milliseconds=false)=>n>0?new Date(milliseconds?n:n*1000).toLocaleString(I18n.locale(),{hour12:false}):I18n.t('未记录');
 const role=k=>({owner:I18n.t('空间所有者'),admin:I18n.t('管理员'),member:I18n.t('成员'),'':I18n.t('无')})[k]||I18n.t('未知角色');
 function shell(tab){
  modal(I18n.t('登录与安全'),I18n.html`<div id="accountSecurityRoot"><div class="security-tabs" role="group" aria-label="安全管理"><button class="${tab==='sessions'?'active':''}" data-security-tab="sessions" aria-pressed="${tab==='sessions'}">我的登录</button>${Roles.owner()?I18n.html`<button class="${tab==='audit'?'active':''}" data-security-tab="audit" aria-pressed="${tab==='audit'}">权限记录</button>`:''}</div><div id="securityContent"></div></div>`,I18n.html('<button class="button" data-cancel>关闭</button>'));
  $('#modal').classList.add('account-security-modal');
  const root=$('#accountSecurityRoot'),uid=data.user.id,live=()=>root.isConnected&&$('#modal').open&&data.user?.id===uid;
  $('[data-security-tab="sessions"]').onclick=sessions;const auditButton=$('[data-security-tab="audit"]');if(auditButton)auditButton.onclick=audit;
  return {root,live,content:$('#securityContent')};
 }
 function sessions(){
  if(!data.user||!data.capabilities?.accountSecurity)return;
  const {root,live,content}=shell('sessions');let snapshot=null,busy=false,unknown=false;
  content.innerHTML=I18n.html('<p class="form-help">这里只显示你自己的有效登录。客户端名称由浏览器信息识别，仅供参考；退出网页登录不会停止 NAS 后台或正在进行的打印。</p><div class="security-toolbar"><strong id="sessionSummary" role="status">正在读取…</strong><button class="button small" id="sessionRefresh">刷新列表</button></div><div class="security-list" id="sessionList"></div><div id="sessionConfirm"></div><p class="form-error" id="sessionError" role="alert"></p><button class="button secondary" id="sessionOthers" disabled>退出全部其他登录</button>');
  const list=$('#sessionList'),error=$('#sessionError'),refresh=$('#sessionRefresh'),all=$('#sessionOthers'),confirm=$('#sessionConfirm'),summary=$('#sessionSummary');
  function controls(){refresh.disabled=busy;all.disabled=busy||unknown||!snapshot?.other;root.querySelectorAll('[data-revoke-session]').forEach(b=>b.disabled=busy||unknown)}
  async function load(){
   if(busy||!live())return;busy=true;controls();error.textContent='';confirm.replaceChildren();
   try{const value=await Remote.request('/api/account/sessions');if(!live())return;snapshot=value;unknown=false;summary.textContent=I18n.msg`${value.total} 个有效登录${value.total>value.sessions.length?I18n.t(' · 显示最近 ')+value.sessions.length+I18n.t(' 个'):''}`;
    list.innerHTML=value.sessions.map(v=>I18n.html`<article class="security-card"><div class="security-card-title"><strong>${esc(v.client)}</strong>${v.current?I18n.html('<span class="status idle">当前登录</span>'):I18n.html`<button class="text-link" data-revoke-session="${esc(v.id)}">退出此登录</button>`}</div><p>登录时间：${esc(time(v.created))}<br>到期时间：${esc(time(v.expires))}</p></article>`).join('');
    root.querySelectorAll('[data-revoke-session]').forEach(b=>b.onclick=()=>choose(value.sessions.find(v=>v.id===b.dataset.revokeSession)));
   }catch(e){if(live())error.textContent=I18n.system(e.message)}finally{busy=false;if(live())controls()}
  }
  function choose(target){
   if(busy||unknown||!live()||target?.current||!target&&!snapshot?.other)return;
   confirm.innerHTML=I18n.html`<form id="sessionRevokeForm" class="security-confirm"><h3>${target?I18n.t('退出此登录'):I18n.t('退出全部其他登录')}</h3><p>${target?esc(target.client)+' · '+esc(time(target.created)):I18n.t('其他浏览器将需要重新登录，当前登录保留。')}</p><label>你的登录密码<input class="input" name="currentPassword" type="password" required autocomplete="current-password" maxlength="256"></label><div class="security-confirm-actions"><button class="button" type="button" id="sessionCancel">取消</button><button class="button danger" type="submit">确认退出</button></div><p class="form-error" id="sessionRevokeError" role="alert"></p></form>`;
   const form=$('#sessionRevokeForm'),submit=form.querySelector('[type="submit"]'),cancel=$('#sessionCancel'),err=$('#sessionRevokeError');cancel.onclick=()=>{if(!busy)confirm.replaceChildren()};
   form.onsubmit=async e=>{
    e.preventDefault();if(!live()||!form.isConnected||busy||unknown||submit.disabled||!form.elements.currentPassword.value)return;
    busy=true;controls();submit.disabled=cancel.disabled=true;err.textContent='';
    const payload={currentPassword:form.elements.currentPassword.value,...(target?{sessionId:target.id}:{others:true})};
    try{const result=await Remote.request('/api/account/sessions/revoke',payload);if(!live())return;form.elements.currentPassword.value='';confirm.replaceChildren();toast(I18n.msg`已退出 ${result.revoked} 个其他登录`);busy=false;await load()}
    catch(e){if(live()&&form.isConnected){form.elements.currentPassword.value='';unknown=!!e.unconfirmed;err.textContent=I18n.system(e.message)+(unknown?I18n.t(' 请刷新列表核对结果，不要重复提交。'):'');submit.disabled=unknown;cancel.disabled=false}}
    finally{busy=false;if(live()){controls();if(form.isConnected&&!unknown)submit.disabled=cancel.disabled=false}}
   };
  }
  refresh.onclick=load;all.onclick=()=>choose(null);load();
 }
 function audit(){
  if(!data.user||!data.capabilities?.accountSecurity||!Roles.owner())return;
  const {live,content}=shell('audit');let cursor=0,busy=false;
  content.innerHTML=I18n.html('<p class="form-help">记录账户创建、角色调整与所有权转让，仅空间所有者可查看。姓名为账户当前显示名称，登录用户名和编号用于辨认原账户。</p><div class="security-toolbar"><strong>权限变更历史</strong><button class="button small" id="auditRefresh">刷新</button></div><div id="roleEventList" class="security-list" aria-live="polite"></div><p class="form-error" id="auditError" role="alert"></p><button class="button secondary" id="auditMore" hidden>加载更早记录</button>');
  const list=$('#roleEventList'),more=$('#auditMore'),refresh=$('#auditRefresh'),error=$('#auditError');
  const person=p=>`${esc(p.name)} <small>@${esc(p.username)} · #${p.id}</small>`;
  async function load(reset){
   if(busy||!live()||!Roles.owner())return;busy=true;more.disabled=refresh.disabled=true;error.textContent='';
   try{const value=await Remote.request('/api/users/role-events'+(!reset&&cursor?'?before='+cursor:''));if(!live()||!Roles.owner())return;if(reset)list.replaceChildren();
    for(const v of value.events){const reason=({initial_migration:I18n.t('初始所有者迁移'),account_created:I18n.t('创建账户'),role_change:I18n.t('调整角色'),ownership_transfer:I18n.t('转让所有权')})[v.reason]||I18n.t('权限操作');list.insertAdjacentHTML('beforeend',I18n.html`<article class="security-card"><div class="security-card-title"><strong>${reason}</strong><time>${esc(time(v.created,true))}</time></div><p>执行者：${person(v.actor)}<br>目标账户：${person(v.target)}</p><div class="security-role-change">${role(v.oldRole)} ${icon('arrow')} <strong>${role(v.newRole)}</strong></div></article>`)}
    if(!list.children.length)list.innerHTML=I18n.html('<p class="form-help">暂无权限变更记录。</p>');cursor=value.nextCursor;more.hidden=!cursor;
   }catch(e){if(live())error.textContent=I18n.system(e.message)}finally{busy=false;if(live())more.disabled=refresh.disabled=false}
  }
  refresh.onclick=()=>load(true);more.onclick=()=>load(false);load(true);
 }
 return {sessions,audit};
})();
