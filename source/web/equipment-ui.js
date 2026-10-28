'use strict';
const EquipmentUI=(()=>{
 const pendingSettings=new Map();
 let editor=null;
 const names={'nozzle:0':I18n.t('喷嘴 0 温度'),'nozzle:1':I18n.t('喷嘴 1 温度'),'fan:1':I18n.t('模型风扇'),'fan:2':I18n.t('辅助风扇 1'),'fan:3':I18n.t('排风风扇'),'fan:10':I18n.t('辅助风扇 2'),nozzle:I18n.t('喷嘴温度'),bed:I18n.t('热床温度'),speed:I18n.t('速度档位'),chamber_light:I18n.t('机舱灯'),chamber_light2:I18n.t('机舱灯 2'),part_fan:I18n.t('模型风扇'),aux_fan:I18n.t('辅助风扇'),chamber_fan:I18n.t('机舱风扇')};
 const speedNames={1:I18n.t('静音 · 50%'),2:I18n.t('标准 · 100%'),3:I18n.t('运动 · 124%'),4:I18n.t('狂暴 · 166%')};
 const label=a=>a.startsWith('set:')?I18n.t('设置')+(names[a.slice(4)]||a.slice(4)):a;
 const value=s=>s.key==='speed'?(speedNames[s.value]||s.value):s.key.startsWith('chamber_light')?(s.value?I18n.t('开启'):I18n.t('关闭')):s.value+' '+s.unit;
 function canSet(p,j){return (!['printing','pause'].includes(p.state)||['printing','paused'].includes(j?.state))&&Remote.printerFresh(p)&&!Remote.deviceBlocked(p.id)&&p.enabled&&!data.capabilities?.restorePending&&(data.user?.admin||j&&canManageJob(j)&&['printing','paused'].includes(j.state))}
 function render(p,j,records,busy,root=$('#controlRoot')){
  if(!root)return;const eq=p.equipment||{},slots=Workspace.slots(p),source=id=>slots.find(s=>s.id===id)?.label||id||I18n.t('未上报');
  const temperatures=$('.control-temperatures',root),readings=eq.extruders||[];
  temperatures.innerHTML=(readings.length?readings:[{name:I18n.t('喷嘴状态待上报'),temperature:null,target:null}]).map(x=>`<div>${icon('temperature')}<span>${esc(I18n.system(x.name))}<strong>${p.connected?(x.temperatureExact??x.temperature??'—'):'—'}<small> / ${p.connected?(x.target??'—'):'—'} °C</small></strong></span></div>`).join('')+I18n.html`<div>${icon('layers')}<span>热床<strong>${p.connected?(eq.bedExact??eq.bed??'—'):'—'}<small> / ${p.connected?(eq.bedTarget??'—'):'—'} °C</small></strong></span><i>HEATBED</i></div>`;
  $('.control-readonly',root).textContent=readings.length>1?I18n.t('各喷嘴独立读数 · 编号沿用设备上报'):I18n.t('当前温度 / 目标温度 · 缺失数据显示 —');
  if(readings.length){const paths=document.createElement('div');paths.className='equipment-paths';paths.innerHTML=readings.map(x=>`<div><span>${esc(I18n.system(x.name))}</span><strong>${esc(source(x.currentSource))}</strong>${x.targetSource&&x.targetSource!==x.currentSource?I18n.html`<small>目标：${esc(source(x.targetSource))}</small>`:''}</div>`).join('')+(eq.switching?I18n.html('<p>机器正在切换喷嘴或供料</p>'):'');$('.control-visual',root).append(paths)}
  const pending=records.some(r=>r.state==='pending'),uncertain=records.some(r=>r.state==='uncertain'),disabled=!canSet(p,j)||pending||uncertain||busy||pendingSettings.has(p.id);
  const available=eq.settings||[],localPending=pendingSettings.get(p.id),serverPending=records.find(r=>r.state==='pending'&&r.action.startsWith('set:'));
  const pendingKey=localPending?.key||serverPending?.action.slice(4),pendingText=localPending?.phase==='sending'?(pendingKey?.startsWith('chamber_light')?(localPending.value?I18n.t('正在开灯…'):I18n.t('正在关灯…')):I18n.t('正在提交…')):I18n.t('等待设备确认');
  const button=(s,kind)=>{
   const stamp=Date.parse(s.updated),blocked=s.readonly||disabled||eq.controlIssue==='authorization_required'&&!s.key.startsWith('chamber_light')||!Number.isFinite(stamp)||Date.now()-stamp>90000||s.key==='speed'&&!['printing','pause'].includes(p.state);
   const label=s.feedback==='command_receipt'?I18n.t('照明灯'):names[s.key]||I18n.system(s.label);
   if(s.key.startsWith('chamber_light'))return `<button type="button" role="switch" aria-checked="${s.value<0?'mixed':s.value===1}" aria-label="${esc(label)}" class="equipment-setting setting-common setting-light ${pendingKey===s.key?'setting-pending':''}" aria-busy="${pendingKey===s.key}" data-equipment="${esc(s.key)}" ${blocked?'disabled':''}><span class="setting-light-label"><svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><path d="M9 17h6m-5 3h4M8 13a6 6 0 1 1 8 0c-1 1-1 2-1 4H9c0-2 0-3-1-4Z"/></svg><span>${esc(label)}<small>${pendingKey===s.key?pendingText:s.feedback==='command_receipt'?(s.value<0?I18n.t('状态未上报，点击开灯'):s.value?I18n.t('上次请求：开灯'):I18n.t('上次请求：关灯')):s.value?I18n.t('已开启'):I18n.t('已关闭')}</small></span></span><span class="device-switch" aria-hidden="true"><i></i></span></button>`;
   let body;
   if(kind==='thermal'){
    const extruder=s.key.startsWith('nozzle:')?readings.find(x=>x.id===Number(s.key.split(':')[1])):readings[0];
    const actual=Remote.printerFresh(p)?(s.key==='bed'?(eq.bedExact??eq.bed):s.readonly?s.observed:(extruder?.temperatureExact??extruder?.temperature)):null;
    body=I18n.html`<span>${esc(label.replace(I18n.t('温度'),''))}</span><strong class="setting-temperature">${actual??'—'}<small>°C</small></strong><span class="setting-target">目标 <b>${s.value??'—'}</b> °C <i aria-hidden="true">↗</i></span>`;
   }else if(kind==='fans'){
    body=`<span>${esc(label)}</span><strong>${s.value}<small>%</small></strong><span class="setting-meter" aria-hidden="true"><i style="width:${Math.max(0,Math.min(100,s.value))}%"></i></span><small class="setting-action">${blocked?I18n.t('当前读数'):I18n.t('调整 →')}</small>`;
   }else{
    body=`<span>${esc(label)}</span><strong>${esc(value(s))}</strong><small class="setting-action">${s.key==='speed'&&!['printing','pause'].includes(p.state)?I18n.t('打印中可调整'):blocked?I18n.t('当前读数'):I18n.t('调整 →')}</small>`;
   }
   if(pendingKey===s.key)body+=`<small class="setting-pending-label" role="status">${esc(pendingText)}</small>`;
   return I18n.html`<button type="button" aria-busy="${pendingKey===s.key}" class="equipment-setting setting-${kind} ${pendingKey===s.key?'setting-pending':''}" data-equipment="${esc(s.key)}" aria-label="${esc(I18n.msg`调整${label}，当前${value(s)}`)}" ${blocked?'disabled':''}>${body}</button>`;
  };
  const thermal=available.filter(s=>s.key==='bed'||s.key.startsWith('nozzle'));
  for(const [index,x] of (readings.length?readings:[{name:I18n.t('喷嘴'),target:null}]).entries()){
   const key=x.id===undefined?(readings.length===1?'nozzle':'reading:'+index):'nozzle:'+x.id;
   if(!thermal.some(s=>s.key===key))thermal.push({key,label:I18n.system(x.name)||I18n.t('喷嘴'),observed:x.temperatureExact??x.temperature,value:x.target,unit:'°C',readonly:true});
  }
  if(!thermal.some(s=>s.key==='bed'))thermal.push({key:'bed',label:I18n.t('热床温度'),value:eq.bedTarget,unit:'°C',readonly:true});
  const fans=available.filter(s=>s.key.startsWith('fan:')||s.key.endsWith('_fan'));
  const other=available.filter(s=>!thermal.includes(s)&&!fans.includes(s));
  const group=(title,note,items,kind)=>items.length?`<section class="setting-group"><div class="setting-group-heading"><h3>${title}</h3><small>${note}</small></div><div class="equipment-settings settings-${kind}">${items.map(s=>button(s,kind)).join('')}</div></section>`:'';
  const section=document.createElement('section');section.className='equipment-panel';section.innerHTML=I18n.html`<div class="equipment-panel-heading"><div><span class="control-section-label">设备控制</span><small>点击项目调整 · 以设备反馈为准</small></div><span class="equipment-tag">${data.capabilities?.deviceMode==='simulated'?I18n.t('模拟设备'):I18n.t('实时控制')}</span></div>${eq.controlIssue==='authorization_required'?I18n.html('<div class="equipment-authorization" role="status"><strong>打印机限制了第三方控制</strong><p>请在打印机屏幕核对局域网与开发者模式，并处理授权告警。</p><a class="text-link" href="https://wiki.bambulab.com/en/knowledge-sharing/enable-developer-mode" target="_blank" rel="noopener noreferrer">查看官方说明 ↗</a></div>'):''}${group(I18n.t('温度'),I18n.t('实际 / 目标 · °C'),thermal,'thermal')}${group(I18n.t('风扇'),I18n.t('风速 · %'),fans,'fans')}${group(I18n.t('常用设置'),'',other,'common')}${!available.length?I18n.html('<p class="equipment-empty">等待设备上报可调项目。连接恢复后将自动更新。</p>'):''}${records.some(r=>r.state==='uncertain'&&r.action.startsWith('set:'))?I18n.html('<button class="button small" id="resolveSettings">核对设备设置结果</button>'):''}`;
  $('.control-supply',root).before(section);
  $$('[data-equipment]',section).forEach(b=>b.onclick=()=>{const s=eq.settings.find(s=>s.key===b.dataset.equipment);if(s?.key.startsWith('chamber_light'))toggleLight(p,s);else if(s)edit(p,s)});
  if(root.querySelector('#resolveSettings'))root.querySelector('#resolveSettings').onclick=()=>confirmAction(I18n.t('核对设备设置'),I18n.t('请先检查机器的实际目标温度、灯光或风扇状态，确认后解除未确认设置的限制。'),async()=>{try{await Remote.request('/api/printers/settings',{printerId:p.id,key:'resolve_settings',value:0});PrinterControl.open(p.id);toast(I18n.t('设备设置核对已记录'))}catch(e){toast(I18n.system(e.message))}},{printerId:p.id,review:true});
  $$('.control-spool',root).forEach(card=>{const index=[...card.parentElement.children].indexOf(card),slot=slots[index];if(!slot)return;const feed=(eq.feeds||[]).find(f=>f.slot===slot.id);if(feed){const el=document.createElement('p');el.className='equipment-feed';el.textContent=I18n.t('机器上报：')+(feed.material||I18n.t('材质未知'))+(feed.remaining!==null&&feed.remaining!==undefined?' · '+feed.remaining+feed.remainingUnit:I18n.t(' · 余量未知'));if(/^#?[0-9a-f]{6}([0-9a-f]{2})?$/i.test(feed.color||'')){const dot=document.createElement('i');dot.className='equipment-feed-color';dot.style.backgroundColor='#'+feed.color.replace('#','').slice(0,6);dot.title=I18n.t('机器上报颜色');el.prepend(dot)}card.append(el)}});
  const live=document.createElement('section');live.className='equipment-live-supply';
  const units=(eq.amsUnits||[]),feeds=(eq.feeds||[]),currentSources=readings.map(x=>x.currentSource).filter(Boolean);
  const feedCard=slot=>{const f=feeds.find(x=>x.slot===slot.id),empty=f?.present===false,color=f&&!empty?colorHex(f.color):null,material=empty?I18n.t('未装载'):f?.material||I18n.t('材质未上报'),remaining=f&&!empty&&f.remaining!==null&&f.remaining!==undefined?I18n.t('设备估算 ')+f.remaining+(f.remainingUnit||''):I18n.t('余量未上报');return `<article class="machine-feed ${empty?'empty':''} ${currentSources.includes(slot.id)?'active':''}" data-machine-slot="${esc(slot.id)}"><div class="machine-feed-head"><span>${esc(I18n.system(slot.label))}</span>${currentSources.includes(slot.id)?I18n.html('<b>当前料源</b>'):''}</div><div class="machine-feed-material"><i class="machine-feed-swatch ${color?'':'unknown'}" ${color?'style="background:'+color+'"':''}></i><strong>${esc(f?material:I18n.t('未上报'))}</strong></div><small>${esc(empty?I18n.t('机器上报为空位置'):f?remaining:I18n.t('等待机器提供装载信息'))}</small>${f&&!empty?MachineMaterials.action(p,f):''}</article>`};
  live.innerHTML=I18n.html`<div class="control-section-heading"><div><span class="control-section-label">机器供料状态</span><small>${p.connected?I18n.t('设备实时上报 · 与库存登记分别记录'):I18n.t('设备离线 · 以下保留最后读数')}</small></div><span class="equipment-tag">${Remote.printerFresh(p)?I18n.t('遥测已连接'):I18n.t('遥测待更新')}</span></div>${units.length?`<div class="ams-unit-strip">${units.map(u=>I18n.html`<div><strong>${esc(u.kind)} · 单元 ${esc(u.id)}</strong><span>${u.temperature!==null&&u.temperature!==undefined?esc(u.temperature)+'°C':I18n.t('温度未上报')}${u.humidityIndex!==null&&u.humidityIndex!==undefined?I18n.t(' · 湿度等级 ')+esc(u.humidityIndex):I18n.t(' · 湿度未上报')}</span></div>`).join('')}</div>`:''}<div class="machine-feed-grid">${slots.filter(s=>!s.id.startsWith('external')).map(feedCard).join('')}</div><div class="machine-external-heading">外挂供料位置</div><div class="machine-feed-grid external">${slots.filter(s=>s.id.startsWith('external')).map(feedCard).join('')}</div><p class="machine-feed-note">百分比是机器估算值；库存克数和料卷归属仍以耗材库登记为准。</p>`;
  $('.control-supply',root).before(live);
  MachineMaterials.bind(live);
  if(editor?.printerId===p.id)editor.sync(p,disabled);
  const errors=[...(p.printError?[I18n.t('打印错误 · ')+p.printError.toString(16).toUpperCase().padStart(8,'0')]:[]),...(eq.hms||[]).map(c=>'HMS · '+c)];
  if(errors.length){const panel=document.createElement('section');panel.className='equipment-errors';panel.innerHTML=I18n.html`<div><strong>设备错误与排查</strong><a class="text-link" href="https://wiki.bambulab.com/" target="_blank" rel="noopener noreferrer">官方知识库 ↗</a></div>${errors.map(c=>`<code>${esc(c)}</code>`).join('')}<p>按编码查询官方说明；故障处理完成后再核对设备状态。</p>`;section.after(panel)}
 }
 async function toggleLight(p,s){
  if(pendingSettings.has(p.id))return;
  pendingSettings.set(p.id,{key:s.key,value:s.value===1?0:1,phase:'sending'});PrinterControl.redraw();
  try{await Remote.request('/api/printers/settings',{printerId:p.id,key:s.key,value:s.value===1?0:1});pendingSettings.get(p.id).phase='waiting';PrinterControl.redraw();toast(I18n.t('灯光指令已提交，等待设备确认'));await PrinterControl.refresh({urgent:true})}
  catch(error){toast(I18n.system(error.message))}
  finally{pendingSettings.delete(p.id);PrinterControl.redraw();Remote.paintConnection()}
 }
 function edit(p,s){
  if(editor)editor.close();
  const isFan=s.key.startsWith('fan:')||s.key.endsWith('_fan'),isSpeed=s.key==='speed',unit=isFan?'%':s.unit||'°C';
  const min=Number.isFinite(s.min)?s.min:0,max=Number.isFinite(s.max)?s.max:100;
  const returnScroll=$('#modal').scrollTop,d=document.createElement('dialog');
  d.id='equipmentDialog';d.className='device-setting-dialog '+(isFan?'fan-editor':isSpeed?'speed-editor':'temperature-editor');
  const title=names[s.key]||I18n.system(s.label);
  const presets=(isFan?[0,25,50,75,100]:s.key==='bed'?[0,40,50,60]:[0,150,200,220]).filter(v=>v>=min&&v<=max);
  const numeric=`<input id="settingValue" name="value" type="number" inputmode="numeric" min="${min}" max="${max}" step="1" required value="${s.value}" aria-label="${isFan?I18n.t('风扇速度'):I18n.t('目标温度')}"><span>${esc(unit)}</span>`;
  d.innerHTML=I18n.html`<div class="setting-editor-heading"><div><span class="setting-editor-kicker">${isFan?'FAN CONTROL':isSpeed?'PRINT SPEED':'TEMPERATURE'}</span><h2>${esc(title)}</h2><p>${esc(p.name)}</p></div><button type="button" class="icon-button" data-setting-close aria-label="返回打印机控制台">${icon('close')}</button></div><form id="equipmentForm" data-printer-id="${p.id}"><div class="setting-live-summary"><span>${isFan||isSpeed?I18n.t('当前设备上报'):I18n.t('当前温度')} <strong data-live-actual>—</strong></span><span>设备目标 <b data-current-target>${esc(value(s))}</b></span></div>${isSpeed?`<div class="speed-options">${Object.entries(speedNames).map(([k,n])=>`<label><input type="radio" name="value" value="${k}" ${+k===s.value?'checked':''}><span>${esc(n)}</span></label>`).join('')}</div>`:isFan?I18n.html`<div class="fan-dial" style="--fan-fill:${s.value}%"><div class="setting-number">${numeric}</div><span>风扇速度</span></div><label class="setting-range-label" for="settingRange">拖动调整</label><input id="settingRange" class="setting-editor-range" type="range" min="${min}" max="${max}" value="${s.value}" step="1" aria-label="风扇速度滑杆"><div class="setting-range-bounds"><span>${min}%</span><span>${max}%</span></div>`:I18n.html`<label class="setting-target-label" for="settingValue">目标温度</label><div class="temperature-stepper"><button type="button" data-temperature-step="-5" aria-label="降低 5°C">−</button><div class="setting-number">${numeric}</div><button type="button" data-temperature-step="5" aria-label="升高 5°C">+</button></div><p class="setting-temperature-hint" data-temperature-hint></p>`}${!isSpeed?`<div class="setting-presets" aria-label="${isFan?I18n.t('常用风速'):I18n.t('快捷温度')}">${presets.map(n=>`<button type="button" data-setting-preset="${n}">${n===0?(isFan?I18n.t('关闭'):I18n.t('关闭加热')):n+unit}</button>`).join('')}</div>`:''}<p class="setting-editor-help">${isSpeed?I18n.t('选择档位后确认应用。'):I18n.t('选值不会操作机器，确认后发送指令。')}</p><p class="form-error" id="equipmentError" role="status"></p><div class="setting-editor-actions"><button type="button" class="button" id="equipmentBack">取消</button><button class="button primary" type="submit" form="equipmentForm" id="applyEquipment">确认应用</button></div></form>`;
  document.body.append(d);
  const form=d.querySelector('form'),input=d.querySelector('#settingValue'),range=d.querySelector('#settingRange'),apply=d.querySelector('#applyEquipment');
  let sending=false,contextBlocked=false,observer;
  const current=()=>editor?.dialog===d&&d.isConnected&&d.open;
  const ready=()=>Remote.ready()&&Remote.printerFresh(data.printers.find(x=>x.id===p.id))&&!Remote.deviceBlocked(p.id);
  const chosen=()=>isSpeed?Number(new FormData(form).get('value')):input.value===''?NaN:Number(input.value);
  function update(){
   const n=chosen(),valid=Number.isInteger(n)&&n>=min&&n<=max;
   apply.disabled=sending||contextBlocked||!ready()||!valid;
   apply.textContent=sending?I18n.t('正在提交…'):!valid?I18n.t('确认应用'):isSpeed?I18n.t('应用 ')+(speedNames[n]||n):n===0?(isFan?I18n.t('关闭风扇'):I18n.t('关闭加热')):I18n.t('应用 ')+n+unit;
   if(range&&valid)range.value=String(n);
   if(isFan)d.querySelector('.fan-dial').style.setProperty('--fan-fill',(valid?n:0)+'%');
   d.querySelectorAll('[data-setting-preset]').forEach(b=>{const selected=Number(b.dataset.settingPreset)===n;b.classList.toggle('selected',selected);b.setAttribute('aria-pressed',String(selected))});
   const hint=d.querySelector('[data-temperature-hint]');if(hint)hint.textContent=valid&&n===0?I18n.t('加热关闭'):I18n.t('可调范围 ')+min+'–'+max+I18n.t('°C · 每次 ±5°C');
  }
  function close(){
   observer?.disconnect();if(d.open)d.close();d.remove();if(editor?.dialog===d)editor=null;
   if($('#controlRoot')?.dataset.printerId===String(p.id))$('#modal').scrollTop=returnScroll;
  }
  function sync(printer,blocked){
   const eq=printer.equipment||{},reading=s.key==='bed'?(eq.bedExact??eq.bed):s.key.startsWith('nozzle:')?(eq.extruders||[]).find(x=>x.id===Number(s.key.split(':')[1]))?.temperature:(eq.extruders?.[0]?.temperatureExact??eq.extruders?.[0]?.temperature);
   const live=(eq.settings||[]).find(x=>x.key===s.key);
   const stamp=Date.parse(live?.updated);
   contextBlocked=blocked||!live||!Number.isFinite(stamp)||Date.now()-stamp>90000||eq.controlIssue==='authorization_required'||isSpeed&&!['printing','pause'].includes(printer.state);
   d.querySelector('[data-live-actual]').textContent=Remote.printerFresh(printer)?isFan||isSpeed?(live?value(live):'—'):(reading??'—')+'°C':'—';
   d.querySelector('[data-current-target]').textContent=live?value(live):'—';update();
  }
  editor={dialog:d,printerId:p.id,close,sync};
  d.querySelector('[data-setting-close]').onclick=close;d.querySelector('#equipmentBack').onclick=close;
  d.onclick=event=>{if(event.target===d)close()};d.oncancel=event=>{event.preventDefault();close()};d.addEventListener('close',close,{once:true});
  d.querySelectorAll('[data-setting-preset]').forEach(b=>b.onclick=()=>{input.value=b.dataset.settingPreset;update()});
  d.querySelectorAll('[data-temperature-step]').forEach(b=>b.onclick=()=>{input.value=String(Math.max(min,Math.min(max,(Number.isFinite(chosen())?chosen():s.value)+Number(b.dataset.temperatureStep))));update()});
  if(input)input.oninput=update;if(range)range.oninput=()=>{input.value=range.value;update()};form.onchange=update;
  form.onsubmit=async event=>{
   event.preventDefault();if(sending||contextBlocked||!ready()||!form.reportValidity())return;
   const n=chosen();if(!Number.isInteger(n)||n<min||n>max)return;
   sending=true;form.dataset.submitting='true';pendingSettings.set(p.id,{key:s.key,value:n,phase:'sending'});update();PrinterControl.redraw();
   try{await Remote.request('/api/printers/settings',{printerId:p.id,key:s.key,value:n});pendingSettings.get(p.id).phase='waiting';if(current())close();toast(I18n.t('已提交设置，等待设备确认'));await PrinterControl.refresh({urgent:true})}
   catch(error){if(error.unconfirmed){if(current())close();PrinterControl.redraw();toast(I18n.system(error.message))}else if(current())d.querySelector('#equipmentError').textContent=I18n.system(error.message)}
   finally{sending=false;delete form.dataset.submitting;pendingSettings.delete(p.id);if(current())update();PrinterControl.redraw();Remote.paintConnection()}
  };
  observer=new MutationObserver(()=>{if(!$('#modal').open||$('#controlRoot')?.dataset.printerId!==String(p.id))close()});
  observer.observe($('#modal'),{childList:true,subtree:true,attributes:true,attributeFilter:['open']});
  d.showModal();sync(p,false);Remote.paintConnection();
 }
 function platePreview(j,plate){const grams=plate.filaments.reduce((n,f)=>n+f.grams,0);return I18n.html`<div class="project-preview">${plate.thumbnail?I18n.html`<img src="/api/jobs/thumbnail?jobId=${j.id}&plate=${plate.id}" alt="第 ${plate.id} 盘切片预览" onerror="this.hidden=true">`:I18n.html('<span class="project-no-preview">此文件没有缩略图</span>')}<div><small>第 ${plate.id} 盘 · 切片估算</small><strong>${plate.seconds?Math.ceil(plate.seconds/60)+I18n.t(' 分钟'):I18n.t('时间未提供')}</strong><span>耗材约 ${grams} g${plate.multiNozzle?I18n.t(' · 多喷嘴待适配'):''}</span></div></div>`}
 function temperatureSummary(p){if(!p.connected)return '—';const eq=p.equipment,values=(eq?.extruders||[]).map(x=>x.temperature===null?'—':(x.temperatureExact??x.temperature)+'°');return (values.length?values.join(' · '):'—')+' / '+(eq?.bed??'—')+'°C'}
 const colorHex=v=>/^#?(?:[0-9a-f]{6}|[0-9a-f]{8})$/i.test(v||'')?'#'+v.replace('#','').slice(0,6).toLowerCase():null;
 function filamentChoices(j,p,f,ams,selected,grams){
  const original=colorHex(f.color),candidates=data.spools.filter(s=>Workspace.visible(s,data)&&s.mount?.printerId===p?.id&&s.state==='available'&&(ams?['1','2','3','4'].includes(s.mount.slot):s.mount.slot==='external')&&s.material.toLowerCase()===f.material.toLowerCase());
  candidates.sort((a,b)=>Number(colorHex(b.color)===original)-Number(colorHex(a.color)===original));
  const sameStock=data.spools.filter(s=>Workspace.visible(s,data)&&s.material.toLowerCase()===f.material.toLowerCase()&&s.state==='available'&&Workspace.available(data,s,j.id)>=f.grams);
  const actual=[...new Set((p?.equipment?.feeds||[]).filter(x=>x.present!==false&&x.material&&x.material!==I18n.t('个人耗材')).map(x=>x.material))];
  const explanation=(actual.length?I18n.t('机器当前上报：')+actual.join(' / ')+'。':'')+(sameStock.length?I18n.t('库存中有 ')+sameStock.length+I18n.t(' 卷同材质且余量充足的耗材，请确认实物装载，再在耗材库更新位置。'):I18n.t('请在机器上准备 ')+f.material+I18n.t(' 并登记净余量与装载位置；若使用其他材质，需要按实际材质重新切片。'));
  return I18n.html`<fieldset class="project-filament project-color-mapping" data-filament="${f.index}"><legend>${esc(f.material)} · 选择打印颜色</legend><div class="project-source-color"><i class="project-color-dot ${original?'':'unknown'}" ${original?'style="background:'+original+'"':''}></i><span>文件原色 ${esc(original||I18n.t('未提供'))}<small>配色 ${f.index+1} · 至少 ${f.grams} g</small></span></div><div class="project-color-choices">${candidates.map(s=>{const available=Workspace.available(data,s,j.id),enough=available>=f.grams,match=!!original&&colorHex(s.color)===original;return I18n.html`<label class="project-color-choice ${enough?'':'unavailable'}"><input type="radio" name="spool-${f.index}" value="${esc(s.id)}" aria-label="配色 ${f.index+1} 使用 ${esc(s.colorName)} · ${esc(s.label)}" ${selected===s.id?'checked':''} ${enough?'required':'disabled'}><i class="project-color-dot" style="background:${colorHex(s.color)||'#b7c0ce'}"></i><span class="project-color-detail"><strong>${esc(s.colorName)} <small>${esc(s.material)}</small></strong><span>${esc(s.label)}</span><small>${esc(Workspace.mountLabel(data,s))}</small><span class="project-color-stock">${available} g 可用${enough?'':I18n.t(' · 余量不足')}${match?I18n.t(' · 与原色一致'):''}</span></span><span class="project-color-check" aria-hidden="true">✓</span></label>`}).join('')||I18n.html`<p class="project-color-empty">本机${ams?I18n.t('第一组 AMS'):I18n.t('外挂位置')}还没有可用的 ${esc(f.material)}。${esc(explanation)}</p>`}</div><p class="project-color-selection" data-color-summary="${f.index}" role="status"></p><label class="project-grams"><span class="field-label">预留用量（g）</span><input class="input" type="number" name="grams-${f.index}" min="${f.grams}" max="10000" required value="${esc(String(grams??f.grams))}" aria-label="配色 ${f.index+1} 预留克数"></label></fieldset>`;
 }
 function bindFilamentChoices(plate){
  for(const f of plate.filaments){const radios=$$(`input[name="spool-${f.index}"]`),summary=$(`[data-color-summary="${f.index}"]`);const update=()=>{const selected=radios.find(r=>r.checked&&!r.disabled),sp=selected&&spoolById(selected.value);if(!sp){summary.textContent=I18n.t('请选择一卷耗材的颜色');summary.classList.remove('changed');return}const different=colorHex(f.color)&&colorHex(sp.color)!==colorHex(f.color);summary.textContent=I18n.msg`将使用：${sp.colorName} · ${sp.material}${different?I18n.t('（与文件原色不同）'):''}`;summary.classList.toggle('changed',!!different)};radios.forEach(r=>r.onchange=update);update()}
 }
 return {render,label,pending:id=>pendingSettings.has(id),platePreview,temperatureSummary,filamentChoices,bindFilamentChoices};
})();

