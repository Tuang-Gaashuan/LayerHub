'use strict';
// Camera requests stay behind the authenticated gateway. Neither the browser nor
// the video decoder command line receives printer access credentials.
const PrinterCamera=(()=>{
 let id=0,name='',panel=null,viewing=false,controller=null,timer=null,objectURL=null,generation=0,observer=null,captureController=null,available=false,sectionVisible=true;
 const visible=()=>sectionVisible&&panel?.isConnected&&$('#modal')?.open&&!document.hidden;
 const message=text=>{if(panel)panel.querySelector('[data-camera-message]').textContent=text};
 const status=text=>{if(panel)panel.querySelector('[data-camera-status]').textContent=text};
 const clearFrame=()=>{if(objectURL){URL.revokeObjectURL(objectURL);objectURL=null}const img=panel?.querySelector('img');if(img){img.hidden=true;img.removeAttribute('src')}};
 function stop(text=I18n.t('预览已停止')){
  viewing=false;generation++;clearTimeout(timer);controller?.abort();controller=null;captureController?.abort();captureController=null;clearFrame();
  if(panel){panel.querySelector('[data-camera-toggle]').textContent=I18n.t('开始观看');message(text);panel.querySelector('[data-camera-time]').textContent='';panel.classList.remove('watching');panel.querySelector('[data-camera-capture]').disabled=!available}
 }
 async function checkedResponse(url,signal){
  const response=await fetch(url,{credentials:'same-origin',cache:'no-store',signal});
  if(!response.ok){let text=response.status===401?I18n.t('登录已过期，请刷新页面后登录'):I18n.t('摄像头暂时不可用，请稍后重新连接');try{const body=await response.json();if(typeof I18n.system(body.error)==='string')text=I18n.system(body.error)}catch{}throw Error(text)}
  return response;
 }
 function show(blob,timestamp,token){
  if(!viewing||token!==generation||!visible())return;
  const next=URL.createObjectURL(blob),previous=objectURL;objectURL=next;
  const image=panel.querySelector('img');image.src=next;image.hidden=false;if(previous)URL.revokeObjectURL(previous);
  panel.classList.add('watching');message(I18n.t('正在观看'));
  panel.querySelector('[data-camera-time]').textContent=I18n.t('画面更新 ')+new Date(timestamp).toLocaleTimeString(I18n.locale(),{hour12:false});
 }
 function timestamp(value){const n=Number(value);if(!Number.isFinite(n)||n<=0)throw Error(I18n.t('摄像头画面已过期，请重新连接'));return n}
 async function imageBlob(response){
  if(!(response.headers.get('Content-Type')||'').startsWith('image/jpeg'))throw Error(I18n.t('没有收到有效的摄像头画面'));
  const blob=await response.blob();if(blob.size===0||blob.size>2*1024*1024)throw Error(I18n.t('摄像头画面无效'));return blob;
 }
 async function frame(token){
  if(!viewing||token!==generation)return;if(!visible()){stop();return}
  const requestController=new AbortController();controller=requestController;let deadline;
  const resetDeadline=()=>{clearTimeout(deadline);deadline=setTimeout(()=>requestController.abort(),21000)};resetDeadline();
  try{
   const smooth=panel.querySelector('[data-camera-mode]').value==='smooth';
   const response=await checkedResponse('/api/printers/camera?printerId='+id+(smooth?'&stream=1':''),requestController.signal);
   if(smooth){
    if(!(response.headers.get('Content-Type')||'').startsWith('multipart/x-mixed-replace')||!response.body?.getReader)throw Error(I18n.t('视频预览不可用，请切换省流模式'));
    const reader=response.body.getReader();let buffer=new Uint8Array(0),length=null,updated=0;
    try{
     while(viewing&&token===generation&&visible()){
      const chunk=await reader.read();if(chunk.done)throw Error(I18n.t('视频连接已结束，请重新连接'));
      if(buffer.length+chunk.value.length>2*1024*1024+16384)throw Error(I18n.t('视频帧过大'));
      const next=new Uint8Array(buffer.length+chunk.value.length);next.set(buffer);next.set(chunk.value,buffer.length);buffer=next;
      while(true){
       if(length===null){
        let end=-1;for(let i=0;i+3<buffer.length;i++)if(buffer[i]===13&&buffer[i+1]===10&&buffer[i+2]===13&&buffer[i+3]===10){end=i;break}
        if(end<0){if(buffer.length>8192)throw Error(I18n.t('视频帧头无效'));break}
        const header=new TextDecoder().decode(buffer.slice(0,end));
        if(!/^\r?\n?--layerhub-frame\r\n/i.test(header)||!/^Content-Type: image\/jpeg\r?$/im.test(header))throw Error(I18n.t('视频格式无效'));
        const size=header.match(/^Content-Length: (\d+)\r?$/im),time=header.match(/^X-Frame-Updated: (\d+)\r?$/im);
        length=Number(size?.[1]);updated=timestamp(time?.[1]);if(!Number.isInteger(length)||length<4||length>2*1024*1024)throw Error(I18n.t('视频帧无效'));buffer=buffer.slice(end+4);
       }
       if(buffer.length<length)break;
       const bytes=buffer.slice(0,length);if(bytes[0]!==255||bytes[1]!==216||bytes[length-2]!==255||bytes[length-1]!==217)throw Error(I18n.t('视频画面无效'));
       show(new Blob([bytes],{type:'image/jpeg'}),updated,token);resetDeadline();buffer=buffer.slice(length);length=null;
      }
     }
    }finally{await reader.cancel().catch(()=>{});reader.releaseLock()}
   }else{
    const blob=await imageBlob(response);show(blob,timestamp(response.headers.get('X-Frame-Updated')),token);timer=setTimeout(()=>frame(token),500);
   }
  }catch(error){if(viewing&&token===generation)stop(error.name==='AbortError'?I18n.t('读取画面超时，请重新连接'):I18n.system(error.message))}
  finally{clearTimeout(deadline);if(controller===requestController)controller=null}
 }
 function start(){viewing=true;generation++;panel.querySelector('[data-camera-toggle]').textContent=I18n.t('停止观看');message(I18n.t('正在连接摄像头…'));status('');frame(generation)}
 async function capture(){
  if(!available||captureController)return;
  const requestController=new AbortController(),target=panel,printerId=id,printerName=name;captureController=requestController;
  const button=target.querySelector('[data-camera-capture]');button.disabled=true;button.textContent=I18n.t('正在拍照…');status(I18n.t('正在获取新画面…'));
  const deadline=setTimeout(()=>requestController.abort(),21000);
  try{
   const response=await checkedResponse('/api/printers/camera?printerId='+printerId+'&capture=1',requestController.signal);
   const blob=await imageBlob(response),updated=timestamp(response.headers.get('X-Frame-Updated'));
   if(requestController.signal.aborted||panel!==target||!visible())return;
   const url=URL.createObjectURL(blob),link=document.createElement('a');link.href=url;
   const date=new Date(updated),pad=n=>String(n).padStart(2,'0');
   const time=date.getFullYear()+pad(date.getMonth()+1)+pad(date.getDate())+'_'+pad(date.getHours())+pad(date.getMinutes())+pad(date.getSeconds());
   link.download='LayerHub_'+printerName.replace(/[<>:"/\\|?*\x00-\x1f]/g,'_').slice(0,60)+'_'+time+'.jpg';document.body.append(link);link.click();link.remove();setTimeout(()=>URL.revokeObjectURL(url),30000);
   status(I18n.t('照片已发送至浏览器下载 · ')+date.toLocaleTimeString(I18n.locale(),{hour12:false}));
  }catch(error){if(panel===target&&!requestController.signal.aborted)status(I18n.t('拍照失败：')+I18n.system(error.message));else if(panel===target&&visible())status(I18n.t('拍照已取消或超时，请重试'))}
  finally{clearTimeout(deadline);if(captureController===requestController)captureController=null;button.textContent=I18n.t('拍照并下载');if(panel===target)button.disabled=!available||!!captureController}
 }
 function mount(p){
  const root=$('#controlRoot .control-visual');if(!root)return;
  if(id!==p.id||!panel){stop();id=p.id;panel=document.createElement('section');panel.className='printer-camera';panel.innerHTML=I18n.html('<div class="camera-heading"><strong>实时画面</strong><div class="camera-actions"><select aria-label="画面模式" data-camera-mode><option value="smooth">实时传输 · 按设备帧率</option><option value="economy">定时取图 · 按设备帧率</option></select><button class="button small" data-camera-toggle>开始观看</button><button class="button small" data-camera-capture>拍照并下载</button></div></div><div class="camera-viewport"><img alt="打印机内部实时画面" hidden><div class="camera-placeholder"><span class="camera-lens" aria-hidden="true">◉</span><p data-camera-message>点击开始观看，也可直接拍照</p></div></div><div class="camera-footer"><span data-camera-hint>按需开启 · 照片保存到本机</span><span data-camera-time></span></div><p class="camera-status" role="status" data-camera-status></p>');panel.querySelector('[data-camera-toggle]').onclick=()=>viewing?stop():start();panel.querySelector('[data-camera-capture]').onclick=capture;panel.querySelector('[data-camera-mode]').onchange=()=>{if(viewing){stop();start()}}}
  panel.querySelector('[data-camera-hint]').textContent=['a1','a1-mini'].includes(p.modelId)?I18n.t('A1 原厂相机为低帧率，画面可能数秒更新一次'):I18n.t('实际画面帧率取决于设备相机与网络');name=p.name||p.model||I18n.t('打印机');if(panel.parentNode!==root)root.append(panel);panel.querySelector('img').onerror=()=>{if(viewing)stop(I18n.t('摄像头画面无法解码，请重新连接'))};
  const supported=p.equipment?.camera?.supported&&data.capabilities?.deviceMode==='live';available=supported&&Remote.printerFresh(p)&&Remote.ready();
  panel.querySelector('[data-camera-toggle]').disabled=!available;panel.querySelector('[data-camera-capture]').disabled=!available||!!captureController;
  if(!available){if(viewing||captureController)stop(I18n.t('设备或服务连接不可用'));else message(!supported?I18n.t('设备尚未上报可用视频接口'):I18n.t('等待设备连接恢复'))}
  if(!observer){observer=new MutationObserver(()=>{if((viewing||captureController)&&!visible())stop()});observer.observe($('#modal'),{childList:true,subtree:true,attributes:true,attributeFilter:['open']});document.addEventListener('visibilitychange',()=>{if(document.hidden)stop()})}
 }
 function setVisible(visible){sectionVisible=visible;if(!visible&&(viewing||captureController))stop()}
 return {mount,stop,setVisible};
})();
