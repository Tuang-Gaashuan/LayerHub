'use strict';
const Diagnostics=(()=>{
 let view=null;
 const labels=()=>({ok:I18n.t('已确认'),warning:I18n.t('待排查'),error:I18n.t('需修复'),unknown:I18n.t('尚未验证'),optional:I18n.t('可选组件'),disabled:I18n.t('已关闭')});
 const attention=c=>['warning','error','unknown'].includes(c.status);
 const groups=()=>({server:I18n.t('应用服务'),storage:I18n.t('数据与存储'),slicing:I18n.t('切片环境'),camera:I18n.t('视频组件'),mail:I18n.t('邮件通知'),safety:I18n.t('控制与保护'),gateway:I18n.t('网关配置'),printers:I18n.t('打印机连接')});
 const live=v=>view===v&&v.root.isConnected&&page==='diagnostics'&&data.user?.admin&&data.user.id===v.uid;
 const fresh=v=>!!v.report&&Date.now()-v.received<60000&&Date.now()>=v.received&&navigator.onLine!==false;
 function valid(r){return r?.ok===true&&r.schema===1&&Array.isArray(r.checks)&&r.checks.every(c=>typeof c.id==='string'&&typeof c.label==='string'&&typeof c.detail==='string'&&['ok','warning','error','unknown','optional','disabled'].includes(c.status))}
 function renderPage(){
  if(!data.user?.admin)return;
  $('#pageContent').innerHTML=heading(I18n.t('连接清楚，排障有据。'),I18n.t('分段查看浏览器、云端入口、NAS 与打印机，不把未知状态当作正常。'),I18n.html`<button class="button" id="diagnosticExport" disabled>${icon('download')} 下载脱敏报告</button><button class="button primary" id="diagnosticRefresh">${icon('refresh')} 运行检查</button>`,'SYSTEM DIAGNOSTICS')+I18n.html`<section id="diagnosticsRoot" class="diagnostics-root"><div class="diagnostic-note">${icon('shield')}<p>检查不启动打印、不发送设备指令或测试邮件，不修改业务数据。页面只读取已有设备遥测。</p></div><div id="diagnosticChain" class="diagnostic-chain" aria-label="连接链路"></div><div id="diagnosticSummary" class="diagnostic-summary" role="status" aria-live="polite">准备检查</div><p id="diagnosticError" class="form-error" role="alert"></p><div class="diagnostic-toolbar"><div class="segmented"><button data-diagnostic-filter="attention" class="active">需要关注</button><button data-diagnostic-filter="all">全部检查</button></div><a class="text-link" href="https://github.com/Tuang-Gaashuan/LayerHub/blob/main/DIAGNOSTICS.md" target="_blank" rel="noopener">诊断说明 ${icon('arrow-up-right')}</a></div><div id="diagnosticChecks"></div><details class="diagnostic-local"><summary>服务无法启动时如何检查？</summary><p>在安装目录，以服务实际使用的账户和环境运行以下命令。程序不会启动服务或连接设备，也不会自动读取 .env 文件。</p><pre>Windows: ./bin/layerhub.exe --doctor --language=zh-CN
Linux:   ./bin/layerhub --doctor --language=zh-CN</pre><p>加 --json 可输出脱敏 JSON；加 --probe-write 才会在已有数据目录创建、读回并删除独立临时文件。该检查不能证明引擎、SMTP 或实机控制已通过。</p></details></section>`;
  const v={root:$('#diagnosticsRoot'),uid:data.user.id,report:null,received:0,gateway:'unknown',busy:false,filter:'attention',failure:false};view=v;
  $('#diagnosticRefresh').onclick=()=>run(v);$('#diagnosticExport').onclick=()=>download(v);
  $$('[data-diagnostic-filter]').forEach(b=>b.onclick=()=>{v.filter=b.dataset.diagnosticFilter;paint(v)});
  paint(v);run(v);
 }
 async function gateway(){
  const controller=new AbortController();let timer;
  try{return await Promise.race([(async()=>{const r=await fetch('/gateway-healthz',{method:'GET',credentials:'omit',cache:'no-store',redirect:'error',signal:controller.signal});if(!r.ok)return 'unknown';const body=await r.json();return body?.ok===true&&body.role==='layerhub-gateway'&&body.schema===1?'ok':'unknown'})(),new Promise(resolve=>{timer=setTimeout(()=>{controller.abort();resolve('unknown')},5000)})])}catch{return 'unknown'}finally{clearTimeout(timer)}
 }
 async function run(v){
  if(!live(v)||v.busy)return;v.busy=true;v.report=null;v.received=0;v.attempted=0;v.gateway='unknown';v.failure=false;$('#diagnosticError').textContent='';paint(v);
  if(navigator.onLine===false){v.failure=true;v.attempted=Date.now();v.busy=false;paint(v);return}
  const results=await Promise.allSettled([Remote.request('/api/diagnostics'),gateway()]);
  if(!live(v))return;
  v.busy=false;v.attempted=Date.now();v.gateway=results[1].status==='fulfilled'?results[1].value:'unknown';
  if(results[0].status==='fulfilled'&&valid(results[0].value)){v.report=results[0].value;v.received=Date.now()}
  else{v.failure=true;$('#diagnosticError').textContent=results[0].status==='rejected'?I18n.system(results[0].reason.message):I18n.t('诊断响应格式不匹配，请刷新页面或核对部署版本。')}
  paint(v);
 }
 function chainCard(status,label,text,glyph){return `<article class="diagnostic-node ${status}"><span class="diagnostic-node-icon">${icon(glyph)}</span><strong>${esc(label)}</strong><span class="diagnostic-status ${status}">${esc(labels()[status])}</span><p>${esc(text)}</p></article>`}
 function paint(v){
  if(!live(v))return;
  const key=JSON.stringify([v.busy,fresh(v),v.gateway,v.failure,v.filter,v.received,v.attempted,navigator.onLine!==false,!!v.attempted&&Date.now()-v.attempted<60000]);if(key===v.paintKey)return;v.paintKey=key;
  const current=fresh(v),gatewayFresh=v.attempted>0&&Date.now()-v.attempted<60000&&Date.now()>=v.attempted&&navigator.onLine!==false,checks=current?v.report.checks:[],printers=checks.filter(c=>c.group==='printers'),real=printers.some(c=>c.id==='printer'&&c.status==='ok');
  const online=navigator.onLine!==false;
  $('#diagnosticChain').innerHTML=chainCard(online?'ok':'error',I18n.t('浏览器网络'),online?I18n.t('浏览器未报告离线；不代表公网或 NAS 必然可达。'):I18n.t('浏览器报告离线，请先恢复本机网络。'),'home')+chainCard(gatewayFresh&&v.gateway==='ok'?'ok':'unknown',I18n.t('云端入口'),gatewayFresh&&v.gateway==='ok'?I18n.t('同源网关独立健康标记已响应；不代表 NAS 正常。'):I18n.t('未确认网关标记；可能是本地直连、旧网关或入口不可达。'),'wifi')+chainCard(current?'ok':v.failure?'warning':'unknown',I18n.t('NAS 业务服务'),current?I18n.t('已收到经过登录校验的业务诊断响应。'):v.failure&&v.gateway==='ok'?I18n.t('云端可达，业务通道未确认；检查隧道与 NAS 服务。'):I18n.t('业务链路未确认，不能据此判断 NAS 关机或打印停止。'),'layers')+chainCard(real?'ok':printers.some(c=>c.status==='warning')?'warning':'unknown',I18n.t('打印机'),real?I18n.t('至少一台真实设备遥测新鲜；逐台结果见下方。'):I18n.t('没有已确认的新鲜实机遥测；不会尝试发送控制。'),'printer');
  const summary=$('#diagnosticSummary');
  summary.textContent=v.busy?I18n.t('正在读取检查结果…'):!v.report?I18n.t('未取得业务诊断；可先使用本地 --doctor 自检。'):!current?I18n.t('上次结果已过期或网络已断开，请重新运行检查；旧结果不再显示为正常。'):I18n.msg`检查时间：${new Date(v.report.checkedAt).toLocaleString(I18n.locale())} · ${checks.filter(attention).length} 项需要关注`;
  $('#diagnosticRefresh').disabled=v.busy;$('#diagnosticExport').disabled=v.busy||!v.attempted;
  $$('[data-diagnostic-filter]').forEach(b=>{const selected=b.dataset.diagnosticFilter===v.filter;b.classList.toggle('active',selected);b.setAttribute('aria-pressed',String(selected))});
  const filtered=checks.filter(c=>v.filter==='all'||attention(c)),groupNames=groups();
  $('#diagnosticChecks').innerHTML=Object.entries(groupNames).map(([key,title])=>{const items=filtered.filter(c=>c.group===key);if(!items.length)return '';return `<section class="diagnostic-group"><h2>${esc(title)}</h2>${items.map(c=>`<article class="diagnostic-check"><div><strong>${esc(I18n.system(c.label))}${c.printerId?' · #'+Number(c.printerId):''}</strong><span class="diagnostic-status ${c.status}">${esc(labels()[c.status])}</span></div><p>${esc(I18n.system(c.detail))}</p>${c.fix?`<p class="diagnostic-fix">${icon('info')}${esc(I18n.system(c.fix))}</p>`:''}${c.observedAt?`<small>${esc(I18n.t('观察时间'))} · ${esc(new Date(c.observedAt).toLocaleString(I18n.locale()))}</small>`:''}</article>`).join('')}</section>`}).join('')||`<div class="diagnostic-empty">${esc(current?I18n.t('当前筛选没有需要处理的项目，可切换到全部检查。'):I18n.t('等待新的检查结果。'))}</div>`;
 }
 function exportReport(){
  const v=view;if(!v||!live(v)||!v.attempted)return null;
  const r=v.report;
  return {schema:1,exportedAt:Date.now(),stale:!fresh(v),browserOnline:navigator.onLine!==false,gatewayMarker:v.gateway,failure:v.failure,report:r?{schema:r.schema,version:r.version,platform:r.platform,checkedAt:r.checkedAt,mode:r.mode,checks:r.checks.map(c=>({id:c.id,group:c.group,status:c.status,label:c.label,detail:c.detail,...(c.fix?{fix:c.fix}:{}),...(c.printerId?{printerId:c.printerId}:{}),...(c.observedAt?{observedAt:c.observedAt}:{})}))}:null};
 }
 function download(v){if(!live(v))return;const value=exportReport();if(!value)return;const url=URL.createObjectURL(new Blob([JSON.stringify(value,null,2)+'\n'],{type:'application/json'})),a=document.createElement('a');a.href=url;a.download='LayerHub-diagnostics.json';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000)}
 return {render:renderPage,paintConnection:()=>{if(view&&view.root.isConnected&&page==='diagnostics'&&(!data.user?.admin||data.user.id!==view.uid)){view.root.replaceChildren();view=null;page='board';location.hash='board';render();return}if(view&&live(view))paint(view)},exportReport};
})();
