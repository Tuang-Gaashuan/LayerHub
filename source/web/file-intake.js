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
 function plates(info,previews={},selected=[]){return info?.plates?.map(p=>{
  const preview=previews[p.id],image=typeof preview==='string'&&/^data:image\/png;base64,[A-Za-z0-9+/=]+$/.test(preview)&&preview.length<140000?`<img class="intake-thumbnail" src="${preview}" alt="${esc(I18n.msg`打印盘 ${p.id} 的切片预览`)}">`:`<span class="intake-thumbnail empty">${I18n.t('暂无预览')}</span>`;
  return `<article class="intake-plate"><label class="intake-plate-choice"><input type="checkbox" data-import-plate="${p.id}" ${selected.includes(p.id)?'checked':''}><span>${image}</span><span><strong>${esc(I18n.msg`打印盘 ${p.id}`)}</strong><small>${p.seconds?I18n.msg`约 ${Math.ceil(p.seconds/60)} 分钟`:I18n.t('用时未提供')}</small></span></label><div class="intake-colors">${p.filaments.map(f=>`<span>${color(f.color)?`<i style="background:${color(f.color)}"></i>`:''}<strong>${esc(f.material)}</strong> ${esc(f.color||I18n.t('颜色未提供'))} · ${Number(f.usedGrams||f.grams||0).toFixed(2)} g</span>`).join('')}</div>${p.multiNozzle?`<p class="intake-warning">${I18n.t('本盘包含多个喷嘴，当前启动流程暂不支持。')}</p>`:p.filaments.length>1?`<p class="intake-warning">${I18n.t('多耗材盘请单独创建并完成供料准备。')}</p>`:''}</article>`
 }).join('')||''}
 function selection(file){const r=reports.get(file);return r?.uid===data.user?.id?r.review.sliced.info.plates.filter(p=>r.selected?.has(p.id)):[]}
 function select(file,ids){const r=reports.get(file);if(!forFile(file)||!Array.isArray(ids)||ids.length>16||ids.some(id=>!r.review.sliced.info.plates.some(p=>p.id===id)))return false;r.selected=new Set(ids);return true}
 function plateBadge(j){const plate=j?.selectedPlate||j?.printOptions?.plate;return plate?`<small class="job-plate-label">${esc(I18n.msg`打印盘 ${plate}`)}</small>`:''}
 function decorateJobs(){
  $$('[data-job]').forEach(button=>{const j=data.jobs.find(j=>j.id===Number(button.dataset.job)),plate=j?.selectedPlate||j?.printOptions?.plate,row=button.closest('.job-row,.device-queue-job');if(!plate||!row)return;let tag=row.querySelector('[data-job-plate]');if(!tag){const title=row.querySelector('.job-main h3,.device-queue-copy strong');if(!title)return;tag=document.createElement('small');tag.dataset.jobPlate='';tag.className='job-plate-label';title.append(tag)}tag.textContent=I18n.msg`打印盘 ${plate}`});
 }
 function close(){epoch++;controller?.abort();controller=null;const panel=$('#fileIntakePanel');if(panel){panel.remove();$('#modal').close()}}
 function forFile(file){const r=reports.get(file);return r?.uid===data.user?.id?r.review:null}
 function paint(file){
  const host=$('#fileReview');if(!host)return;const r=forFile(file);if(!r){host.replaceChildren();return}
  host.innerHTML=I18n.html`<div class="intake-ready"><span>${icon('check')} 已校验切片 3MF · ${r.sliced.info.plates.length} 盘</span><details><summary>查看打印盘与颜色</summary>${plates(r.sliced.info,r.sliced.previews,selection(file).map(p=>p.id))}${reference(r.reference)}<p class="form-help">打印前仍需确认机型、喷嘴、打印板和实际料卷。LayerHub 不修改文件内的切片工艺。</p><button class="button small" id="fileReviewExport">导出检查报告</button></details></div>`;
  const details=host.querySelector('details');if(r.sliced.info.plates.length>1)details.open=true;
  details.insertAdjacentHTML('afterbegin',`<div class="intake-selection"><strong data-selection-summary></strong><button type="button" class="text-link" data-select-batch>${I18n.t('选择可批量入队的盘')}</button><button type="button" class="text-link" data-clear-plates>${I18n.t('清空选择')}</button><small>${I18n.t('最多选择 16 盘；每盘单独成为任务，不会自动连续打印。')}</small></div>`);
  const update=()=>{host.querySelector('[data-selection-summary]').textContent=I18n.msg`已选择 ${selection(file).length} 盘`;host.querySelectorAll('[data-import-plate]').forEach(input=>input.checked=selection(file).some(p=>p.id===Number(input.dataset.importPlate)))};
  host.querySelectorAll('[data-import-plate]').forEach(input=>input.onchange=()=>{const ids=[...host.querySelectorAll('[data-import-plate]:checked')].map(v=>Number(v.dataset.importPlate));if(!select(file,ids))toast(I18n.t('每批最多选择 16 个打印盘'));update()});
  host.querySelector('[data-select-batch]').onclick=()=>{const ids=r.sliced.info.plates.filter(p=>!p.multiNozzle&&p.filaments.length===1).map(p=>p.id);if(ids.length>16)return toast(I18n.t('超过 16 盘，请分批选择'));select(file,ids);update()};
  host.querySelector('[data-clear-plates]').onclick=()=>{select(file,[]);update()};update();
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
   reports.set(file,{uid,review:r,selected:new Set(r.sliced.info.plates.length===1?[r.sliced.info.plates[0].id]:[])});close();pickFile(file,{reviewed:true});toast(I18n.t('切片已校验，请确认设备与耗材。'));
  }catch(e){
   if(!current())return;
   modal(I18n.t('文件暂不能安排打印'),I18n.html`<div id="fileIntakePanel" class="file-intake"><strong class="intake-filename">${esc(file.name)}</strong><p role="alert">${esc(e.name==='AbortError'?I18n.t('检查已超时，请检查连接后重新选择文件。'):I18n.system(e.message))}</p><p>请在 Bambu Studio 选择正确机型与耗材，完成切片并导出切片盘 3MF。STL、未切片工程和原始 G-code 均不接受。</p><p class="form-help">现有任务和耗材保持原样。</p></div>`,I18n.html('<button class="button" data-cancel>返回</button>'),{onDismiss:close});
  }finally{clearTimeout(timeout);if(controller===active)controller=null}
 }
 function estimates(file){const r=forFile(file);if(!r)return {};const ps=selection(file);if(ps.length!==1)return {estimate:null,grams:null};const p=ps[0],total=p.filaments.reduce((n,f)=>n+Number(f.usedGrams||f.grams||0),0),bound=(n,max)=>Number.isFinite(n)&&n>0&&n<=max?Math.ceil(n):null;return {estimate:bound(p.seconds/60,100000),grams:bound(total,10000)}}
 return {open,close,forFile,paint,estimates,selection,select,decorateJobs,plateBadge};
})();
