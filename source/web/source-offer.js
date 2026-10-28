'use strict';
(()=>{
 const $=s=>document.querySelector(s),viewer=$('#documentViewer'),status=$('#documentStatus'),content=$('#documentText');
 const docs={license:{path:'./source/LICENSE.txt',title:{'zh-CN':document.querySelector('.document-card h3 [data-language="zh-CN"]').textContent+' · AGPL-3.0',en:'GNU Affero General Public License v3.0'}},notices:{path:'./source/THIRD_PARTY_NOTICES.md',title:{'zh-CN':document.querySelectorAll('.document-card h3 [data-language="zh-CN"]')[1].textContent,en:'Third-party notices'}}};
 let language='zh-CN',controller=null,sequence=0,documentKey=null,documentState='';
 function setLanguage(value,persist=false){
  language=value==='en'?'en':'zh-CN';document.documentElement.lang=language;
  document.title=(language==='en'?'Licenses & source':document.querySelector('h1 [data-language="zh-CN"]').textContent)+' · LayerHub';
  document.querySelectorAll('[data-set-language]').forEach(b=>b.setAttribute('aria-pressed',String(b.dataset.setLanguage===language)));
  if(persist)try{localStorage.setItem('layerhub.language',language)}catch{}
  if(documentKey)$('#documentTitle').textContent=docs[documentKey].title[language];
  if(documentState)status.textContent=status.dataset[(documentState==='error'?'error':'loading')+(language==='en'?'En':'Zh')];
 }
 try{const saved=localStorage.getItem('layerhub.language');setLanguage(saved==='en'||saved!=='zh-CN'&&navigator.language?.toLowerCase().startsWith('en')?'en':'zh-CN')}catch{setLanguage(navigator.language?.toLowerCase().startsWith('en')?'en':'zh-CN')}
 document.querySelectorAll('[data-set-language]').forEach(b=>b.onclick=()=>setLanguage(b.dataset.setLanguage,true));
 function close(){sequence++;controller?.abort();controller=null;documentKey=null;documentState='';viewer.close()}
 $('#closeViewer').onclick=close;viewer.addEventListener('cancel',e=>{e.preventDefault();close()});viewer.addEventListener('click',e=>{if(e.target===viewer)close()});
 document.querySelectorAll('[data-document]').forEach(button=>button.onclick=async()=>{
  const key=button.dataset.document,doc=docs[key];if(!doc)return;
  controller?.abort();const active=new AbortController();controller=active;const generation=++sequence;documentKey=key;documentState='loading';content.textContent='';$('#documentOriginal').href=doc.path;setLanguage(language);viewer.showModal();
  const deadline=setTimeout(()=>active.abort(),15000);
  try{
   const response=await fetch(doc.path,{signal:active.signal,credentials:'omit'});if(!response.ok)throw Error('Document unavailable');
   const text=await response.text();if(generation!==sequence||!viewer.open)return;
   content.textContent=text;documentState='';status.textContent='';
  }catch{if(generation===sequence&&viewer.open){documentState='error';setLanguage(language)}}finally{clearTimeout(deadline);if(controller===active)controller=null}
 });
})();
