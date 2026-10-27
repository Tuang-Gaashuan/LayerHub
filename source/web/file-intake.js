'use strict';
const FileIntake=(()=>{
 let epoch=0,controller=null;
 const reports=new WeakMap();
 const color=value=>/^#[0-9a-f]{6}([0-9a-f]{2})?$/i.test(value||'')?value.slice(0,7):null;
 function reference(r){
  if(!r)return '';
  const rows=[[I18n.t('打印机'),r.printer],[I18n.t('喷嘴直径'),(r.nozzles||[]).join(' / ')],[I18n.t('打印板'),r.bed],[I18n.t('材料'),(r.materials||[]).join(' / ')],[I18n.t('层高'),r.layer],[I18n.t('填充密度'),r.infill],[I18n.t('填充图案'),r.pattern]].filter(([,v])=>v);
  return `<div class="intake-reference"><table><caption>${I18n.t('切片文件记录 · 仅供核对')}</caption><tbody>${rows.map(([k,v])=>`<tr><th>${esc(k)}</th><td>${esc(v)}</td></tr>`).join('')}</tbody></table></div>`;
 }
 function plates(info){return info?.plates?.map(p=>I18n.html`<article class="intake-plate"><div><strong>打印盘 ${p.id}</strong><span>${p.seconds?I18n.msg`约 ${Math.ceil(p.seconds/60)} 分钟`:I18n.t('用时未提供')}</span></div><div class="intake-colors">${p.filaments.map(f=>`<span>${color(f.color)?`<i style="background:${color(f.color)}"></i>`:''}<strong>${esc(f.material)}</strong> ${esc(f.color||I18n.t('颜色未提供'))} · ${Number(f.usedGrams||f.grams||0).toFixed(2)} g</span>`).join('')}</div>${p.multiNozzle?I18n.html('<p class="intake-warning">本盘包含多个喷嘴，当前启动流程暂不支持。</p>'):''}</article>`).join('')||''}
 function close(){epoch++;controller?.abort();controller=null;const panel=$('#fileIntakePanel');if(panel){panel.remove();$('#modal').close()}}
 function forFile(file){const r=reports.get(file);return r?.uid===data.user?.id?r.review:null}
 function paint(file){
  const host=$('#fileReview');if(!host)return;const r=forFile(file);if(!r){host.replaceChildren();return}
  host.innerHTML=I18n.html`<div class="intake-ready"><span>${icon('check')} 已校验切片 3MF · ${r.sliced.info.plates.length} 盘</span><details><summary>查看打印盘与颜色</summary>${plates(r.sliced.info)}${reference(r.reference)}<p class="form-help">打印前仍需确认机型、喷嘴、打印板和实际料卷。LayerHub 不修改文件内的切片工艺。</p><button class="button small" id="fileReviewExport">导出检查报告</button></details></div>`;
  $('#fileReviewExport').onclick=()=>{if(!host.isConnected||file!==picked||!forFile(file))return;const blob=new Blob([JSON.stringify({schema:2,filename:file.name,review:r},null,2)],{type:'application/json'}),url=URL.createObjectURL(blob),a=document.createElement('a');a.href=url;a.download='layerhub-sliced-3mf-review.json';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000)};
 }
 async function open(file){
  if(!data.user)return;
  if(!/\.3mf$/i.test(file.name)||!file.size||file.size>500*1024*1024)return toast(I18n.t('请选择 500 MB 以内的已切片 3MF 文件。'));
  close();const serial=epoch,uid=data.user.id,origin=$('#pageContent').firstElementChild;
  const current=()=>epoch===serial&&data.user?.id===uid&&origin?.isConnected&&!!$('#fileIntakePanel');
  modal(I18n.t('检查切片文件'),I18n.html`<div id="fileIntakePanel" class="file-intake"><strong class="intake-filename">${esc(file.name)}</strong><p role="status">正在校验打印盘、耗材与切片信息…</p><p class="form-help">检查不会创建任务或启动打印。</p></div>`,I18n.html('<button class="button" data-cancel>取消检查</button>'),{onDismiss:close});
  const active=new AbortController();controller=active;const timeout=setTimeout(()=>active.abort(),600000);
  try{
   const body=new FormData();body.append('file',file);
   const response=await fetch('/api/files/inspect',{method:'POST',credentials:'same-origin',headers:{'X-CSRF-Token':Remote.csrfToken(),'Accept-Language':I18n.language()},body,signal:active.signal});
   const result=await response.json();if(!current())return;
   if(!response.ok||result?.ok!==true)throw Error(result.error||I18n.t('检查结果无效，请重新选择文件。'));
   const r=result.review;if(r?.schema!==2||r.kind!=='sliced-3mf'||!r.sliced?.available||!r.sliced.info?.plates?.length)throw Error(I18n.t('检查结果无效，请重新选择文件。'));
   reports.set(file,{uid,review:r});close();pickFile(file,{reviewed:true});toast(I18n.t('切片已校验，请确认设备与耗材。'));
  }catch(e){
   if(!current())return;
   modal(I18n.t('文件暂不能安排打印'),I18n.html`<div id="fileIntakePanel" class="file-intake"><strong class="intake-filename">${esc(file.name)}</strong><p role="alert">${esc(e.name==='AbortError'?I18n.t('检查已超时，请检查连接后重新选择文件。'):I18n.system(e.message))}</p><p>请在 Bambu Studio 选择正确机型与耗材，完成切片并导出切片盘 3MF。STL、未切片工程和原始 G-code 均不接受。</p><p class="form-help">现有任务和耗材保持原样。</p></div>`,I18n.html('<button class="button" data-cancel>返回</button>'),{onDismiss:close});
  }finally{clearTimeout(timeout);if(controller===active)controller=null}
 }
 function estimates(file){const r=forFile(file);if(!r)return {};const ps=r.sliced.info.plates;if(ps.length!==1)return {estimate:null,grams:null};const p=ps[0],total=p.filaments.reduce((n,f)=>n+Number(f.usedGrams||f.grams||0),0),bound=(n,max)=>Number.isFinite(n)&&n>0&&n<=max?Math.ceil(n):null;return {estimate:bound(p.seconds/60,100000),grams:bound(total,10000)}}
 return {open,close,forFile,paint,estimates};
})();
