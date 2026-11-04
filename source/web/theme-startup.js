// Runs before styles and body; no account data or service request is needed.
(()=>{
 let mode='system';try{const saved=localStorage.getItem('layerhub.ui.startupMode');if(['system','light','dark'].includes(saved))mode=saved}catch{}
 const dark=mode==='dark'||mode==='system'&&typeof matchMedia==='function'&&matchMedia('(prefers-color-scheme: dark)').matches;
 const root=document.documentElement;root.dataset.uiMode=mode;root.dataset.uiColorScheme=dark?'dark':'light';root.style.colorScheme=dark?'dark':'light';
 const meta=document.querySelector('meta[name="theme-color"]');if(meta)meta.content=dark?'#101722':'#3159ed';
})();
