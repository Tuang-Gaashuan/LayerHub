'use strict';
// Gettext-style catalogs translate application copy only. Interpolated user data is never translated.
const I18n=(()=>{
 const supported=['zh-CN','en'],storageKey='layerhub.language',catalog=globalThis.LayerHubLocaleEN||{messages:{},html:{}},cache=new WeakMap();
 const normalize=value=>typeof value==='string'&&(value==='zh-CN'||value.toLowerCase().startsWith('zh'))?'zh-CN':typeof value==='string'&&value.toLowerCase().startsWith('en')?'en':null;
 const detect=()=>{for(const value of navigator.languages||[navigator.language]){const lang=normalize(value);if(lang)return lang}return 'en'};
 let language=detect(),saved=false,accountSynced=false;
 try{const stored=normalize(localStorage.getItem(storageKey));if(stored){language=stored;saved=true}}catch{}
 const locale=()=>language==='en'?'en-US':'zh-CN';
 document.documentElement.lang=language;
 document.documentElement.dataset.language=language;
 const hash=s=>{let h=2166136261;for(let i=0;i<s.length;i++)h=Math.imul(h^s.charCodeAt(i),16777619);return (h>>>0).toString(16)};
 function t(source){source=String(source??'');if(language!=='en')return source;const key=source.trim(),translated=catalog.messages[key];return translated===undefined?source:source.match(/^\s*/u)[0]+translated+source.match(/\s*$/u)[0]}
 // Replace every slot once. A filename containing a slot-like string cannot expand another argument.
 const interpolate=(source,values)=>source.replace(/__lh_expr_(\d+)__/g,(_,i)=>String(values[Number(i)]??''));
 function sourceOf(strings){return strings.map((part,i)=>part+(i<strings.length-1?'__lh_expr_'+i+'__':'')).join('')}
 function msg(strings,...values){const source=sourceOf(strings);return interpolate(t(source),values)}
 const htmlStrings=new Map();
 function compileHTML(source){let result=source;if(language==='en')for(const [start,end,text]of catalog.html[hash(source)]||[])result=result.slice(0,start)+text+result.slice(end);return result}
 function html(strings,...values){
  if(typeof strings==='string'){if(!htmlStrings.has(strings))htmlStrings.set(strings,compileHTML(strings));return htmlStrings.get(strings)}
  let source=cache.get(strings);if(!source){source=compileHTML(sourceOf(strings));cache.set(strings,source)}
  return interpolate(source,values);
 }
 const regexEscape=s=>s.replace(/[.*+?^${}()|[\]\\]/g,'\\$&');
 const patternToken=/__lh_expr_\d+__|%(?:\[\d+\])?[+#0 -]*\d*(?:\.\d+)?[sdqvf]/g;
 let patterns;
 function system(source){
  if(typeof source!=='string')return source;if(language!=='en'||!/[\u3400-\u9fff]/u.test(source))return source;
  const direct=t(source);if(direct!==source)return direct;
  const minutes=source.match(/^(\d+)\s*分钟$/u);if(minutes)return minutes[1]+' min';
  if(!patterns)patterns=Object.entries(catalog.messages).filter(([key])=>/%(?:\[\d+\])?[+#0 -]*\d*(?:\.\d+)?[sdqvf]/.test(key)||key==='__lh_expr_0__ · 已由 __lh_expr_1__ 核对设备实际状态').map(([key,value])=>{
   let offset=0,expression='^',slots=[];
   for(const token of key.matchAll(new RegExp(patternToken.source,'g'))){const capture=token[0].endsWith('d')?'([+-]?\\d+)':token[0].endsWith('f')?'([+-]?(?:\\d+(?:\\.\\d*)?|\\.\\d+))':'([\\s\\S]*?)';expression+=regexEscape(key.slice(offset,token.index))+capture;slots.push(token[0]);offset=token.index+token[0].length}
   expression+=regexEscape(key.slice(offset))+'$';return {key,regex:new RegExp(expression),value,slots,weight:key.replace(new RegExp(patternToken.source,'g'),'').length};
  }).sort((a,b)=>b.weight-a.weight);
  for(const p of patterns){const match=source.match(p.regex);if(!match)continue;const counters=new Map();p.slots.forEach((slot,i)=>{if(!counters.has(slot))counters.set(slot,[]);const value=p.key==='__lh_expr_0__ · 已由 __lh_expr_1__ 核对设备实际状态'&&i===0?system(match[i+1]):match[i+1];counters.get(slot).push(value)});return p.value.replace(new RegExp(patternToken.source,'g'),token=>counters.get(token)?.shift()??token)}
  // System errors may append diagnostic details after a known prefix.
  for(const separator of [':','：','\n']){const split=source.indexOf(separator);if(split>0){const head=source.slice(0,split),translated=t(head);if(head!==translated)return translated+(separator==='\n'?'\n':': ')+system(source.slice(split+1).trimStart())}}
  return source;
 }
 function notificationTitle(source){if(language!=='en'||typeof source!=='string')return source;const parts=source.split(' · ');if(parts[0]==='LayerHub'&&parts.length>1){parts[1]=t(parts[1]);return parts.join(' · ')}return system(source)}
 function notificationBody(source){if(language!=='en'||typeof source!=='string')return source;const lines=system(source).split('\n');lines[0]=t(lines[0]);const blocks=lines.join('\n').split('\n\n');blocks[blocks.length-1]=t(blocks[blocks.length-1]);return blocks.join('\n\n')}
 function documentText(){
  if(typeof data==='undefined'||!data.user){const name=document.querySelector('#userName'),role=document.querySelector('#userRole');if(name)name.textContent=t('访客');if(role)role.textContent=t('尚未登录')}
  const walker=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);
  while(walker.nextNode()){
   const node=walker.currentNode,el=node.parentElement;
   if(!el||el.closest('script,style,#pageContent,#modal,#toast,#userName,#userRole,#profileBtn,.sidebar-user .avatar,[translate="no"]'))continue;
   if(/[\u3400-\u9fff]/u.test(node.data))node.data=t(node.data);
  }
  for(const el of document.querySelectorAll('[aria-label],[title],[placeholder]')){
   if(el.closest('#pageContent,#modal,#toast,[translate="no"]'))continue;
   for(const name of ['aria-label','title','placeholder'])if(el.hasAttribute(name))el.setAttribute(name,t(el.getAttribute(name)));
  }
  document.title=t('共印空间 · LayerHub');
  const description=document.querySelector('meta[name="description"]');if(description)description.content=t('连接每一位创作者的 3D 打印协作工作台。上传切片、共享设备、协同管理打印任务。');
 }
 function saveBrowser(next){localStorage.setItem(storageKey,next);saved=true}
 function loginSwitcher(){
  const form=document.querySelector('#realLogin');if(!form||form.querySelector('.language-picker'))return;
  const label=document.createElement('label');label.className='language-picker';const span=document.createElement('span');span.className='field-label';span.textContent='语言 / Language';
  const select=document.createElement('select');select.className='input';select.setAttribute('aria-label','Language');select.setAttribute('translate','no');
  select.innerHTML='<option value="zh-CN">简体中文</option><option value="en">English</option>';select.value=language;
  select.onchange=()=>{try{sessionStorage.setItem('layerhub.loginLanguage',select.value);saveBrowser(select.value);location.reload()}catch{select.value=language;const error=document.querySelector('#loginError');if(error)error.textContent=t('无法保存浏览器语言偏好，请检查浏览器存储权限。')}};
  label.append(span,select);form.prepend(label);
 }
 async function change(next,{reload=()=>location.reload()}={}){
  if(!supported.includes(next))throw Error(t('请选择简体中文或 English'));
  if(typeof uploading!=='undefined'&&uploading)throw Error(t('请先等待当前上传、切片提交或模型编辑完成。'));
  if(typeof SlicerWorkbench!=='undefined'&&typeof page!=='undefined'&&page==='slicer')await SlicerWorkbench.beforeLanguageChange();
  if(typeof data!=='undefined'&&data.user){await Remote.request('/api/account/language',{language:next});}
  // Do not reload until draft and account writes are acknowledged.
  saveBrowser(next);if(next!==language)reload();
 }
 function open(){
  modal(t('语言设置'),html`<form id="languageForm"><label><span class="field-label">界面语言</span><select class="input" name="language" translate="no"><option value="zh-CN">简体中文</option><option value="en">English</option></select></label><p class="form-help">切换语言会重新载入页面。工作台草稿会先保存；未提交的表单请先保存。</p><p class="form-help">语言偏好会随账户保存在服务器，并用于之后生成的邮件。登录页也会记住此浏览器的选择。</p><p class="form-error" id="languageError" role="alert"></p></form>`,html`<button class="button" data-cancel>取消</button><button class="button primary" form="languageForm" type="submit">保存并切换语言</button>`);
  const form=document.querySelector('#languageForm');form.elements.language.value=language;
  form.onsubmit=async event=>{event.preventDefault();const button=document.querySelector('[form="languageForm"]');button.disabled=true;document.querySelector('#languageError').textContent=t('正在保存草稿与语言…');try{await change(form.elements.language.value);if(form.elements.language.value===language)document.querySelector('#modal').close()}catch(e){document.querySelector('#languageError').textContent=system(e.message)||t('语言设置未能保存，请重试。');button.disabled=false}};
 }
 function accountSwitcher(){
  if(!document.querySelector('#realLogout')||document.querySelector('#accountLanguage'))return;
  let options=document.querySelector('.account-options');
  if(!options){options=document.createElement('div');options.className='account-options';document.querySelector('#modalContent .modal-body')?.append(options);for(const id of ['sourceOffer','appearanceSettings']){const item=document.getElementById(id);if(item)options.append(item)}}
  const button=document.createElement('button');button.className='button';button.id='accountLanguage';button.textContent=t('语言 / Language')+' · '+(language==='en'?'English':'简体中文');button.onclick=open;options.append(button);
 }
 let observed=false;
 function init(){
  documentText();
  const button=document.createElement('button');button.type='button';button.id='languageBtn';button.className='language-button';button.setAttribute('aria-label','Language');button.setAttribute('translate','no');button.textContent=language==='en'?'EN':'中';button.title=t('语言 / Language');button.onclick=()=>{if(typeof data!=='undefined'&&data.user){if(document.querySelector('#modal')?.open)return;open()}else loginSwitcher()};
  document.querySelector('.topbar-right')?.prepend(button);
  const modal=document.querySelector('#modalContent');if(modal&&!observed){observed=true;new MutationObserver(()=>{loginSwitcher();accountSwitcher()}).observe(modal,{childList:true,subtree:true})}
  loginSwitcher();accountSwitcher();
 }
 if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',init,{once:true});else queueMicrotask(init);
 function syncAccount(user){if(accountSynced||!user)return;accountSynced=true;const next=normalize(user.language);if(next&&next!==language){try{saveBrowser(next);location.reload()}catch{}}}
 function loginOverride(){try{return normalize(sessionStorage.getItem('layerhub.loginLanguage'))||''}catch{return ''}}
 function loginComplete(){try{sessionStorage.removeItem('layerhub.loginLanguage')}catch{}accountSynced=false}
 return {t,msg,html,system,notificationTitle,notificationBody,locale,language:()=>language,hasSaved:()=>saved,open,change,init,normalize,syncAccount,loginOverride,loginComplete};
})();
globalThis.I18n=I18n;
