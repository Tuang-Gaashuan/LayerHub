'use strict';
// A fixed local protocol opens only the program explicitly registered by the user.
// Paths never leave the browser. No local HTTP service or remote launch API exists.
const StudioShortcut=(()=>{
 const storageKey='layerhub.local.bambuStudio.path',scheme='layerhub-studio://open';
 function normalize(value){
  let p=String(value||'').trim();if(p.startsWith('"')&&p.endsWith('"'))p=p.slice(1,-1);
  p=p.replaceAll('/','\\');
  if(p.length>2048||!/^[a-z]:\\/i.test(p)||/[<>"|?*%\x00-\x1f]/.test(p)||p.slice(2).includes(':')||p.split('\\').some(s=>s==='.'||s==='..')||!/[\\](?:bambu-studio|bambustudio)\.exe$/i.test(p))throw Error(I18n.t('请填写本机 Bambu Studio 主程序的完整 .exe 路径'));
  return p;
 }
 function read(){try{return normalize(localStorage.getItem(storageKey))}catch{return ''}}
 const regEscape=s=>s.replaceAll('\\','\\\\').replaceAll('"','\\"');
 function registration(path){
  const p=normalize(path);
  // Deliberately no %1: URLs cannot supply files, CLI options or a different EXE.
  return 'Windows Registry Editor Version 5.00\r\n\r\n[HKEY_CURRENT_USER\\Software\\Classes\\layerhub-studio]\r\n@="URL:LayerHub Bambu Studio"\r\n"URL Protocol"=""\r\n\r\n[HKEY_CURRENT_USER\\Software\\Classes\\layerhub-studio\\shell\\open\\command]\r\n@="'+regEscape('"'+p+'"')+'"\r\n';
 }
 function removal(){return 'Windows Registry Editor Version 5.00\r\n\r\n[-HKEY_CURRENT_USER\\Software\\Classes\\layerhub-studio]\r\n'}
 function utf16(text){const raw=new Uint8Array(2+text.length*2);raw[0]=255;raw[1]=254;for(let i=0;i<text.length;i++){const n=text.charCodeAt(i);raw[2+i*2]=n&255;raw[3+i*2]=n>>8}return raw}
 function download(text,name){const url=URL.createObjectURL(new Blob([utf16(text)],{type:'application/octet-stream'}));const a=document.createElement('a');a.href=url;a.download=name;document.body.append(a);a.click();a.remove();setTimeout(()=>URL.revokeObjectURL(url),60000)}
 function launch(){
  if(!read()){open();return}
  const a=document.createElement('a');a.href=scheme;document.body.append(a);a.click();a.remove();
  toast(I18n.t('已请求打开 Bambu Studio，请确认浏览器提示；未打开时请检查本机配置。'));
 }
 function bind(){const b=$('#studioShortcut');if(b){b.hidden=!data.user;b.onclick=launch;b.title=read()?I18n.t('打开本机 Bambu Studio；路径可在个人账户中修改'):I18n.t('先设置本机 Bambu Studio 路径')}}
 function open(){
  modal(I18n.t('本机 Bambu Studio'),I18n.html`<p>在 Bambu Studio 中完成切片，再将切片好的 3MF 上传到 LayerHub。</p><form id="studioSetup"><label><span class="field-label">Bambu Studio 程序路径（Windows）</span><input class="input" name="path" required maxlength="2048" autocomplete="off" spellcheck="false" value="${esc(read())}" placeholder="C:\\Program Files\\Bambu Studio\\bambu-studio.exe"></label><p class="form-help">在资源管理器中找到主程序，右键复制文件路径。路径只保存在此浏览器，不上传到服务器。</p><ol class="studio-steps"><li>填写路径，下载本机配置。</li><li>双击下载的 .reg 文件，确认导入到当前 Windows 用户。</li><li>点击侧栏 Bambu Studio，按浏览器提示打开。</li></ol><p class="form-help">首次使用需要导入一次；更换程序路径后需要重新导入。网页无法检查文件是否存在，也无法确认程序是否已打开。内置浏览器若拦截，请使用 Edge 或 Chrome 访问本网站。</p><p class="form-error" id="studioError" role="alert"></p><div class="studio-tools"><button class="button small" id="studioRemove" type="button">下载移除配置</button><button class="button small" id="studioForget" type="button">清除浏览器路径</button></div></form>`,I18n.html('<button class="button" data-cancel>返回</button><button class="button" id="studioTry" type="button">打开 Studio</button><button class="button primary" form="studioSetup" type="submit">保存并下载本机配置</button>'));
  const f=$('#studioSetup');$('#studioTry').disabled=!read();
  $('#studioTry').onclick=launch;
  f.onsubmit=e=>{e.preventDefault();try{const path=normalize(f.elements.path.value),reg=registration(path);localStorage.setItem(storageKey,path);download(reg,'LayerHub-Bambu-Studio.reg');f.elements.path.value=path;$('#studioTry').disabled=false;$('#studioError').textContent='';bind();toast(I18n.t('已下载配置。请双击导入后，再点击打开 Studio。'))}catch(e){$('#studioError').textContent=I18n.system(e.message)}};
  $('#studioRemove').onclick=()=>{try{download(removal(),'LayerHub-Remove-Bambu-Studio.reg');toast(I18n.t('已下载移除配置，双击导入后生效。'))}catch(e){$('#studioError').textContent=I18n.system(e.message)}};
  $('#studioForget').onclick=()=>{try{localStorage.removeItem(storageKey);f.elements.path.value='';$('#studioTry').disabled=true;bind();toast(I18n.t('已清除浏览器路径；Windows 关联需通过移除配置删除。'))}catch(e){$('#studioError').textContent=I18n.system(e.message)}};
 }
 return {bind,open};
})();
