'use strict';
// Read the display preference before the first paint. No account or model data is stored.
(()=>{
 const key='layerhub.ui.sidebarCollapsed',html=document.documentElement;
 let collapsed=false;
 try{collapsed=localStorage.getItem(key)==='1'}catch{}
 html.classList.toggle('nav-collapsed',collapsed);
 function mount(){
  const sidebar=document.getElementById('sidebar'),button=document.getElementById('menuBtn');
  if(!sidebar||!button)return;
  const desktop=()=>window.innerWidth>760;
  const glyph=closed=>`<svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16"/><path d="${closed?'m13 9 3 3-3 3':'m16 9-3 3 3 3'}"/></svg>`;
  sidebar.querySelector('.brand')?.setAttribute('aria-label','共印空间 · 返回看板');
  for(const item of sidebar.querySelectorAll('.nav-item')){
   const label=[...item.childNodes].filter(n=>n.nodeType===3).map(n=>n.textContent).join('').trim();
   if(label){item.setAttribute('aria-label',label);item.title=label}
  }
  function paint(){
   const wide=desktop();if(wide)sidebar.classList.remove('open');
   const expanded=wide?!collapsed:sidebar.classList.contains('open');
   button.setAttribute('aria-expanded',String(expanded));
   button.setAttribute('aria-controls','sidebar');
   const label=wide?(collapsed?'展开侧栏':'收起侧栏'):(expanded?'关闭导航':'打开导航');
   button.setAttribute('aria-label',label);button.title=label;button.innerHTML=glyph(!expanded);
  }
  button.onclick=()=>{
   if(desktop()){
    collapsed=!collapsed;html.classList.add('nav-motion');html.classList.toggle('nav-collapsed',collapsed);
    try{localStorage.setItem(key,collapsed?'1':'0')}catch{}
   }else sidebar.classList.toggle('open');
   paint();
  };
  document.addEventListener('click',e=>{const path=e.composedPath();if(!desktop()&&!path.includes(sidebar)&&!path.includes(button)){sidebar.classList.remove('open');paint()}});
  document.addEventListener('keydown',e=>{if(e.key==='Escape'&&!desktop()&&sidebar.classList.contains('open')){sidebar.classList.remove('open');paint();button.focus()}});
  window.addEventListener('resize',paint);window.addEventListener('hashchange',paint);
  window.addEventListener('storage',e=>{if(e.key===key){collapsed=e.newValue==='1';html.classList.toggle('nav-collapsed',collapsed);paint()}});
  paint();
 }
 if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',mount,{once:true});else mount();
})();
