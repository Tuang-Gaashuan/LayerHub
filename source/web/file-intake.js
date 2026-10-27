'use strict';
const FileIntake=(()=>{
 let epoch=0,controller=null;
 const reports=new WeakMap();
 function intakeModal(...args){modal(...args);$('#modal').classList.add('file-intake-modal')}
 const color=value=>/^#[0-9a-f]{6}([0-9a-f]{2})?$/i.test(value||'')?value.slice(0,7):null;
 function reference(r,compare=false){
  if(!r)return I18n.html('<p class="form-help">文件没有可读取的原工程参数；不会猜测或继承设置。</p>');
  const rows=[[I18n.t('打印机'),r.printer,'Bambu Lab X2D'],[I18n.t('喷嘴直径'),(r.nozzles||[]).join(' / '),'0.4 mm'],[I18n.t('打印板'),r.bed,I18n.t('纹理 PEI 板')],[I18n.t('材料'),(r.materials||[]).join(' / '),'PETG Basic'],[I18n.t('层高'),r.layer,'0.20 mm'],[I18n.t('填充密度'),r.infill,'15%'],[I18n.t('填充图案'),r.pattern,I18n.t('网格')]].filter(([,v])=>v);
  return `<div class="intake-reference"><table><caption>${I18n.t(compare?'原参数仅供参考 · 导入后使用工作台工艺':'原工程参数 · 仅供参考')}</caption><thead><tr><th>${I18n.t('项目')}</th><th>${I18n.t('文件记录')}</th>${compare?`<th>${I18n.t('导入后')}</th>`:''}</tr></thead><tbody>${rows.map(([k,v,next])=>`<tr><th>${esc(k)}</th><td>${esc(v)}</td>${compare?`<td>${esc(next)}</td>`:''}</tr>`).join('')}</tbody></table>${(r.colors||[]).length?`<div class="intake-colors">${r.colors.map(c=>`<span>${color(c)?`<i style="background:${color(c)}"></i>`:''}${esc(c)}</span>`).join('')}</div>`:''}</div>`;
 }
 function plates(info){return info?.plates?.map(p=>I18n.html`<article class="intake-plate"><div><strong>打印盘 ${p.id}</strong><span>${p.seconds?I18n.msg`约 ${Math.ceil(p.seconds/60)} 分钟`:I18n.t('用时未提供')}</span></div><div class="intake-colors">${p.filaments.map(f=>`<span>${color(f.color)?`<i style="background:${color(f.color)}"></i>`:''}<strong>${esc(f.material)}</strong> ${esc(f.color||I18n.t('颜色未提供'))} · ${Number(f.usedGrams||f.grams||0).toFixed(2)} g</span>`).join('')}</div>${p.multiNozzle?I18n.html('<p class="intake-warning">本盘包含多个喷嘴，当前启动流程暂不支持。</p>'):''}</article>`).join('')||''}
 function close(){epoch++;controller?.abort();controller=null;const panel=$('#fileIntakePanel');if(panel){panel.remove();$('#modal').close()}}
 function forFile(file){const r=reports.get(file);return r?.uid===data.user?.id?r.review:null}
 async function open(file,{onGeometry,onSlice}={}){
  if(!data.user)return;if(!/\.3mf$/i.test(file.name)||!file.size||file.size>500*1024*1024)return toast(I18n.t('请选择 500 MB 以内的 3MF 文件。'));
  close();const serial=epoch,uid=data.user.id,origin=$('#pageContent').firstElementChild;
  const current=()=>epoch===serial&&data.user?.id===uid&&origin?.isConnected&&!!$('#fileIntakePanel');
  intakeModal(I18n.t('识别文件 · 选择下一步'),I18n.html`<div id="fileIntakePanel" class="file-intake"><strong class="intake-filename">${esc(file.name)}</strong><p role="status">正在检查文件内容，不会创建任务或启动打印…</p></div>`,I18n.html('<button class="button" data-cancel>取消检查</button>'),{onDismiss:close});
  const active=new AbortController();controller=active;const timeout=setTimeout(()=>active.abort(),90000);
  try{
   const body=new FormData();body.append('file',file);
   const response=await fetch('/api/files/inspect',{method:'POST',credentials:'same-origin',headers:{'X-CSRF-Token':Remote.csrfToken(),'Accept-Language':I18n.language()},body,signal:active.signal});
   const result=await response.json();if(!current())return;
   if(!response.ok||result?.ok!==true||result.review?.schema!==1)throw Error(result.error||I18n.t('检查结果无效，请重新选择文件。'));
   const review=result.review;reports.set(file,{uid,review});show(review);
  }catch(e){if(!current())return;intakeModal(I18n.t('文件检查未完成'),I18n.html`<div id="fileIntakePanel" class="file-intake"><strong class="intake-filename">${esc(file.name)}</strong><p role="alert">${esc(e.name==='AbortError'?I18n.t('检查已超时，请检查连接后重新选择文件。'):I18n.system(e.message))}</p><p class="form-help">当前模型、草稿、队列和耗材保持原样。</p></div>`,I18n.html('<button class="button" data-cancel>关闭</button>'),{onDismiss:close})}
  finally{clearTimeout(timeout);if(controller===active)controller=null}
  function show(r){
   const geometry=r.geometry,sliced=r.sliced;
   intakeModal(I18n.t('识别文件 · 选择下一步'),I18n.html`<div id="fileIntakePanel" class="file-intake"><strong class="intake-filename">${esc(file.name)}</strong><p class="form-help">按实际内容识别，与文件名是否含 gcode 无关。这里只检查，不会创建工程或任务。</p><div class="intake-routes"><section><span class="eyebrow">${I18n.t('保留原切片')}</span><h3>安排打印</h3><p>${sliced.available?I18n.t('保留原切片的路径、温度和工艺；下一步确认设备与料卷。'):esc(I18n.system(sliced.reason))}</p><button class="button primary" id="intakeSlice" ${sliced.available?'':'disabled'}>使用已有切片</button></section><section><span class="eyebrow">${I18n.t('重新编辑模型')}</span><h3>进入模型工作台</h3><p>${geometry.available?I18n.t('保留几何和分盘；重新排版，使用工作台工艺重新切片。'):esc(I18n.system(geometry.reason))}</p><button class="button" id="intakeGeometry" ${geometry.available?'':'disabled'}>导入可编辑模型</button></section></div>${!geometry.available?I18n.html('<p class="intake-warning">需要编辑：在原切片软件导出实体 STL，或另存不含涂色、修改器和特殊体积的普通网格 3MF。</p>'):''}${!sliced.available?I18n.html('<p class="form-help">需要保留原工艺：在 Bambu Studio 完成切片，再导出“切片盘文件”，不要只保存工程。</p>'):''}${sliced.available?`<details open><summary>${I18n.t('查看打印盘与颜色')}</summary>${plates(sliced.info)}<p class="form-help">${I18n.t('检测到切片不代表兼容当前设备。启动前仍检查机型、喷嘴、打印板、料卷和设备状态。')}</p></details>`:''}<details><summary>查看原工程参数</summary>${reference(r.reference)}</details><p class="form-help">导出报告包含本文件名和参数，请自行决定分享范围。</p></div>`,I18n.html('<button class="button" data-cancel>返回</button><button class="button" id="intakeExport">导出检查报告</button>'),{onDismiss:close});
   $('#intakeGeometry').onclick=()=>{if(!current()||!geometry.available)return;close();if(onGeometry)onGeometry(file,geometry.review);else {navigate('slicer');SlicerWorkbench.importFile(file,geometry.review)}};
   $('#intakeSlice').onclick=()=>{if(!current()||!sliced.available)return;close();if(onSlice)onSlice(file,r);else {focusUpload();pickFile(file,{reviewed:true})}};
   $('#intakeExport').onclick=()=>{if(!current())return;const blob=new Blob([JSON.stringify({schema:1,filename:file.name,review:r},null,2)],{type:'application/json'}),url=URL.createObjectURL(blob),a=document.createElement('a');a.href=url;a.download='layerhub-file-review.json';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000)};
  }
 }
 function estimates(file){const r=forFile(file);if(!r)return {};const ps=r.sliced?.info?.plates;if(ps?.length!==1)return {estimate:null,grams:null};const p=ps[0],total=p.filaments.reduce((n,f)=>n+Number(f.usedGrams||f.grams||0),0),bound=(n,max)=>Number.isFinite(n)&&n>0&&n<=max?Math.ceil(n):null;return {estimate:bound(p.seconds/60,100000),grams:bound(total,10000)}}
 return {open,close,forFile,reference,plates,estimates};
})();
