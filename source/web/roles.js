'use strict';
const Roles=(()=>{
 const owner=(u=data.user)=>!!u?.admin&&!!u?.owner;
 const label=u=>owner(u)?'空间所有者':u?.admin?'管理员':'成员';
 const canManage=m=>!!data.user?.admin&&!!m&&(m.id===data.user.id||!m.owner&&(owner()||!m.admin));
 function transfer(){
  if(!owner())return;const uid=data.user.id,candidates=data.members.filter(m=>m.id!==uid&&!m.disabled);
  modal('转让空间所有权',`<form id="ownerTransferForm"><p class="form-help">空间只能有一位所有者。接任者将成为所有者，你将保留管理员身份；双方需要重新登录。任务、耗材和模型归属保持。</p><label>接任账户<select class="input" name="userId" required><option value="">请选择接任者</option>${candidates.map(m=>`<option value="${m.id}">${esc(m.name)} · @${esc(m.username)} · ${label(m)}</option>`).join('')}</select></label><p id="ownerTransferTarget" class="role-transfer-preview" role="status">尚未选择，不会修改任何权限。</p><label>输入接任者的完整登录用户名<input class="input" name="confirmUsername" required autocomplete="off" placeholder="逐字输入登录用户名"></label><label>你当前的登录密码<input class="input" name="currentPassword" type="password" required autocomplete="current-password"></label><label class="check-label"><input type="checkbox" name="confirmed" required>我确认将最高管理权转交给此账户</label><p id="ownerTransferError" class="form-error" role="alert"></p></form>`,`<button class="button" data-cancel>取消</button><button class="button danger" type="submit" form="ownerTransferForm" id="ownerTransferSubmit" disabled>确认转让所有权</button>`);
  const form=$('#ownerTransferForm'),submit=$('#ownerTransferSubmit');let unknown=false;
  function update(){const target=candidates.find(m=>m.id===Number(form.elements.userId.value));$('#ownerTransferTarget').textContent=target?`将交给 ${target.name}（@${target.username}）。你将成为普通管理员。`:'尚未选择，不会修改任何权限。';submit.disabled=unknown||!target||form.elements.confirmUsername.value.trim()!==target.username||!form.elements.currentPassword.value||!form.elements.confirmed.checked}
  form.addEventListener('input',update);form.addEventListener('change',update);
  form.onsubmit=async e=>{e.preventDefault();if(!form.isConnected||!$('#modal').open||submit.disabled||unknown||form.dataset.busy||!owner()||data.user.id!==uid)return;form.dataset.busy='1';submit.disabled=true;try{await Remote.request('/api/users/transfer-owner',{userId:Number(form.elements.userId.value),currentPassword:form.elements.currentPassword.value,confirmUsername:form.elements.confirmUsername.value.trim()});if(data.user?.id!==uid)return;$('#modal').close();data.user=null;Remote.login();toast('所有权已转让，双方请重新登录')}catch(error){if(form.isConnected&&data.user?.id===uid){$('#ownerTransferError').textContent=error.message+(error.unconfirmed?' 请重新登录核对角色，不要重复转让。':'');unknown=!!error.unconfirmed}}finally{delete form.dataset.busy;if(form.isConnected)update()}};
 }
 return {owner,label,canManage,transfer};
})();
