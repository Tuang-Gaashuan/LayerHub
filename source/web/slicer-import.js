'use strict';
const SlicerImport=(()=>{
 function mount({host,onBusy,onImported}){
  const uid=data.user?.id;let alive=true,epoch=0,controller=null,timer=null;
  const owned=()=>alive&&host.isConnected&&data.user?.id===uid;
  function stop(){epoch++;controller?.abort();controller=null;clearTimeout(timer);timer=null;onBusy(false)}
  function dismiss(){stop();const panel=$('#import3mfPanel');if(panel){panel.remove();$('#modal').close()}}
  async function send(file,mode,extra={}){
   const form=new FormData();form.append('file',file);form.append('mode',mode);for(const [key,value]of Object.entries(extra))form.append(key,value);
   const activeController=new AbortController(),activeTimer=setTimeout(()=>activeController.abort(),90000);controller=activeController;timer=activeTimer;
   try{let response,result;try{response=await fetch('/api/slicing/import-3mf',{method:'POST',credentials:'same-origin',headers:{'X-CSRF-Token':Remote.csrfToken()},body:form,signal:activeController.signal});result=await response.json()}catch{throw Object.assign(Error(mode==='review'?I18n.t('工程检查中断，请重新选择文件。'):I18n.t('没有收到导入确认。可先到“我的工程”核对；点击重试会沿用同一导入标识，避免重复创建。')),{unconfirmed:mode==='commit'})}
    if(!response.ok||result?.ok!==true){const rejected=response.status>=400&&response.status<500&&result?.ok===false;throw Object.assign(Error(rejected?I18n.system(result.error):I18n.t('服务尚未确认导入结果，请先检查我的工程。')),{unconfirmed:mode==='commit'&&!rejected})}return result;
   }finally{clearTimeout(activeTimer);if(controller===activeController){timer=null;controller=null}}
  }
  async function open(file){
   if(!owned())return;if(!/\.3mf$/i.test(file.name)||!file.size||file.size>32*1024*1024){toast(I18n.t('请选择 32 MB 以内的工程 3MF。'));return}
   stop();const serial=epoch,key=crypto.randomUUID();onBusy(true);
   modal(I18n.t('检查工程 3MF'),I18n.html`<div id="import3mfPanel" class="import3mf-panel"><div class="import3mf-file">${icon('files')}<div><strong>${esc(file.name)}</strong><small>正在读取模型、组件变换与打印板…</small></div></div><p class="form-help">检查阶段不会保存模型或替换当前工程。</p></div>`,I18n.html`<button class="button" data-cancel>取消检查</button>`,{onDismiss:dismiss});
   try{const response=await send(file,'review');if(!owned()||epoch!==serial||!$('#import3mfPanel'))return;onBusy(false);review(file,response.review,key,serial)}
   catch(e){if(!owned()||epoch!==serial||!$('#import3mfPanel'))return;onBusy(false);modal(I18n.t('工程暂不能导入'),I18n.html`<div id="import3mfPanel" class="import3mf-panel"><div class="import3mf-file">${icon('files')}<strong>${esc(file.name)}</strong></div><p class="import3mf-error" role="alert">${esc(I18n.system(e.message))}</p><p class="form-help">请在 Bambu Studio 另存包含模型的工程 3MF。仅有切片路径的文件，可到协作看板上传安排打印。</p></div>`,I18n.html`<button class="button" data-cancel>关闭</button>`,{onDismiss:dismiss})}
   finally{if(owned()&&epoch===serial)onBusy(false)}
  }
  function review(file,info,key,serial){
   modal(I18n.t('导入工程 · 确认内容'),I18n.html`<form id="import3mfPanel" class="import3mf-panel"><div class="import3mf-file">${icon('files')}<div><strong>${esc(file.name)}</strong><small>${info.plates.length} 块打印板 · ${info.objects} 个对象 · ${info.sources.length} 种模型</small></div></div><div class="import3mf-plates">${info.plates.map((p,i)=>I18n.html`<div><span>${String(i+1).padStart(2,'0')}</span><strong>${esc(p.name)}</strong><small>${p.objects} 个${p.objects?' · '+Number(p.triangles).toLocaleString(I18n.locale())+I18n.t(' 面'):I18n.t(' · 空盘')}</small></div>`).join('')}</div><div class="import3mf-notice"><strong>导入模型，使用工作台工艺</strong><p>保留模型尺寸、朝向与分盘关系；组件合并为网格，变换固化到几何。每盘重新排版并贴底，原 X/Y 坐标不保留。</p><p>原机型、颜色、耗材、速度、温度和自定义工艺不继承。导入后使用 X2D · 0.4 mm · 单色 PETG Basic，0.20 mm 层高、15% 网格填充；请逐盘检查后重新切片。</p>${info.hasGcode?I18n.html('<p>文件中的已有 G-code 不会导入或执行。</p>'):''}${info.skipped?I18n.html`<p>跳过 ${info.skipped} 个未启用打印的对象。</p>`:''}</div><details class="import3mf-models"><summary>查看模型尺寸</summary>${info.sources.map(s=>`<p><strong>${esc(s.name)}</strong><span>${s.size.map(n=>Number(n).toFixed(1)).join(' × ')} mm</span></p>`).join('')}</details><label class="import3mf-agree"><input type="checkbox" id="import3mfAgree">我已了解工艺替换与重新排版，确认导入为新工程</label><p class="form-help">成功后保存为独立私有工程。打开它之前，可以保留或保存当前摆放。导入不会创建切片或打印任务。</p><p id="import3mfError" class="form-error" role="alert"></p></form>`,I18n.html`<button class="button" data-cancel>取消</button><button class="button primary" id="import3mfCommit" type="submit" form="import3mfPanel" disabled>导入为新工程</button>`,{onDismiss:dismiss});
   const form=$('#import3mfPanel'),button=$('#import3mfCommit'),agree=$('#import3mfAgree');let pending=false;
   agree.onchange=()=>button.disabled=pending||!agree.checked;
   form.onsubmit=async e=>{e.preventDefault();if(pending||!agree.checked||!owned()||epoch!==serial)return;pending=true;button.disabled=true;button.textContent=I18n.t('正在导入…');onBusy(true);$('#import3mfError').textContent='';
    try{const result=await send(file,'commit',{sha256:info.sha256,requestKey:key,acknowledge:'geometry-only'});if(!owned()||epoch!==serial||!form.isConnected)return;stop();form.remove();$('#modal').close();toast(I18n.t('工程已保存；请逐盘确认工艺，再安排切片。'));onImported(result.project)}
    catch(error){if(!owned()||epoch!==serial||!form.isConnected)return;$('#import3mfError').textContent=I18n.system(error.message);button.textContent=error.unconfirmed?I18n.t('核对并重试导入'):I18n.t('重试导入')}
    finally{if(owned()&&epoch===serial){pending=false;onBusy(false);if(button.isConnected)button.disabled=!agree.checked}}
   };
  }
  return {open,close(){dismiss();alive=false}};
 }
 return {mount};
})();
