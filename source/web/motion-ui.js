'use strict';
const MotionUI=(()=>{
 let selected=0,clearedUntil=0,busy=false,confirmHome=false,unknown=false;
 const fresh=v=>Number.isFinite(Date.parse(v))&&Date.now()-Date.parse(v)<5000&&Date.now()-Date.parse(v)>=-1000;
 function reason(p,records){
  if(!data.user?.admin)return I18n.t('手动移动仅管理员可操作');
  if(p.modelId!=='x2d')return I18n.t('此机型尚未完成移动适配');
  if(!Remote.printerFresh(p)||!p.enabled||!['IDLE','FINISH'].includes(p.rawState))return I18n.t('仅空闲设备允许手动移动');
  if(data.capabilities?.restorePending||Remote.deviceBlocked(p.id))return I18n.t('请先核对连接状态');
  if(data.jobs.some(j=>j.printer===p.id&&['assigned','uploading','starting','printing','paused','stopped','checking'].includes(j.state)))return I18n.t('请先完成本机任务的取件和用量登记');
  if(p.equipment?.hms?.length||p.printError||p.equipment?.switching||p.equipment?.controlIssue)return I18n.t('请先处理设备告警或供料切换');
  if(records.some(r=>['pending','uncertain'].includes(r.state)))return I18n.t('请等待或核对上一条设备指令');
  if(!fresh(p.equipment?.motion?.capabilitiesAt))return I18n.t('移动状态未知或过期');
  return '';
 }
 function render(p,records,root){
  if(selected!==p.id){selected=p.id;clearedUntil=0;confirmHome=false;unknown=false}
  const m=p.equipment?.motion||{},why=reason(p,records),clear=clearedUntil>Date.now(),blocked=busy||unknown||!!why||!clear;
  const homed=fresh(m.homeAt)&&m.homeMask===7,jog=blocked||!homed||!m.nativeJog;
  const section=document.createElement('section');section.className='motion-panel';
  section.innerHTML=I18n.html`<div class="motion-heading"><div><span class="control-section-label">手动移动</span><h3>维护时的小步调整</h3><p>喷头 X / Y · 热床 Z。单击只发送一步，不长按连动。</p></div><strong>1 mm</strong></div><div class="motion-homing">${['X','Y','Z'].map((axis,i)=>`<span>${axis} · ${fresh(m.homeAt)&&m.homeMask!=null?(m.homeMask&(1<<i)?I18n.t('已回零'):I18n.t('未确认回零')):I18n.t('状态未知')}</span>`).join('')}</div><p class="motion-status" role="status">${esc(why||I18n.t('坐标未提供，以机器实际位置为准'))}</p><label class="motion-clear"><input type="checkbox" data-motion-clear ${clear?'checked':''} ${busy?'disabled':''}><span>我在现场，已清空打印板并确认运动路径无人、无障碍物</span></label><div class="motion-directions"><div><span>喷头</span><div class="motion-cross"><button class="button" data-motion-axis="Y" data-dir="1" ${jog?'disabled':''}>Y + ↑</button><button class="button" data-motion-axis="X" data-dir="-1" ${jog?'disabled':''}>← X −</button><span>1 mm</span><button class="button" data-motion-axis="X" data-dir="1" ${jog?'disabled':''}>X + →</button><button class="button" data-motion-axis="Y" data-dir="-1" ${jog?'disabled':''}>Y − ↓</button></div></div><div class="motion-z"><span>热床升降</span><button class="button" data-motion-axis="Z" data-dir="-1" ${jog?'disabled':''}>抬升 · Z −</button><button class="button" data-motion-axis="Z" data-dir="1" ${jog?'disabled':''}>下降 · Z +</button></div></div><div class="motion-home"><button class="button secondary" data-motion-home ${blocked||!m.nativeHome?'disabled':''}>整机归位</button><small>归位可能需要约一分钟，等待状态确认后再移动。</small></div>${confirmHome?I18n.html('<div class="motion-confirm" role="group" aria-label="确认归位"><p>归位将移动喷头和热床。确认现场安全后再执行。</p><button class="button" data-motion-cancel>取消</button><button class="button primary" data-motion-confirm>确认归位一次</button></div>'):''}${records.filter(r=>r.state==='uncertain'&&r.action.startsWith('motion:')).map(r=>`<div class="motion-review"><p>${esc(message(r.message))}</p><button class="button" data-motion-review="${r.id}" ${busy||!clear?'disabled':''}>${I18n.t('已现场确认停稳、无异常')}</button></div>`).join('')}${unknown?I18n.html('<div class="motion-review"><p>提交结果不明，不要重复点击。重新读取记录后核对机器。</p><button class="button" data-motion-reread>重新读取控制记录</button></div>'):''}<p class="form-help">10 mm、连续移动和挤出暂不开放。点动回执只说明设备接收；每次移动后需现场确认停稳。</p>`;
  section.querySelector('[data-motion-clear]').onchange=e=>{clearedUntil=e.target.checked?Date.now()+90000:0;PrinterControl.redraw()};
  section.querySelectorAll('[data-motion-axis]').forEach(b=>b.onclick=()=>send(p,{action:'jog',axis:b.dataset.motionAxis,direction:Number(b.dataset.dir),step:1}));
  section.querySelector('[data-motion-home]').onclick=()=>{confirmHome=true;PrinterControl.redraw()};
  const cancel=section.querySelector('[data-motion-cancel]');if(cancel)cancel.onclick=()=>{confirmHome=false;PrinterControl.redraw()};
  const confirm=section.querySelector('[data-motion-confirm]');if(confirm){confirm.disabled=blocked||!m.nativeHome;confirm.onclick=()=>send(p,{action:'home'})}
  section.querySelectorAll('[data-motion-review]').forEach(b=>b.onclick=()=>send(p,{action:'review',commandId:Number(b.dataset.motionReview)}));
  const reread=section.querySelector('[data-motion-reread]');if(reread)reread.onclick=async()=>{try{await PrinterControl.refresh();unknown=false;clearedUntil=0;PrinterControl.redraw()}catch(e){toast(I18n.system(e.message))}};
  root.append(section);
 }
 async function send(p,action){
  if(busy||unknown||selected!==p.id||clearedUntil<=Date.now())return;
  busy=true;confirmHome=false;clearedUntil=0;PrinterControl.redraw();
  try{await Remote.request('/api/printers/motion',{printerId:p.id,...action,requestId:crypto.randomUUID(),clear:true});await PrinterControl.refresh({urgent:true})}
  catch(e){unknown=true;toast(I18n.system(e.message))}
  finally{busy=false;PrinterControl.redraw()}
 }
 function label(action){return ({'motion:home':I18n.t('整机归位'),'motion:jog':I18n.t('手动点动'),'motion-test:home':I18n.t('归位实测'),'motion-test:X':I18n.t('X 轴实测'),'motion-test:Y':I18n.t('Y 轴实测'),'motion-test:Z':I18n.t('Z 轴实测')})[action]}
 function message(value){return I18n.system(String(value||'').replace(/^requestID=[A-Za-z0-9_-]+;action=[a-z]+;axis=[XYZ]?;dir=-?\d+;step=\d+;/,''))}
 return {render,label,message};
})();
