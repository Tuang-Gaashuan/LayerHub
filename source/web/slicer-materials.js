'use strict';
const SlicerMaterials=(()=>{
 const profiles=[{key:'petg-basic',label:'PETG Basic',printReady:true},{key:'pla-basic',label:'PLA Basic',printReady:false},{key:'petg-hf',label:'PETG HF',printReady:false}];
 const get=key=>profiles.find(p=>p.key===(key||'petg-basic'))||profiles[0];
 function selector(key){return `<label class="sr-only" for="sliceMaterial">材料切片预设</label><select id="sliceMaterial" aria-label="材料切片预设">${profiles.map(p=>`<option value="${p.key}" ${get(key).key===p.key?'selected':''}>${p.label}</option>`).join('')}</select><small id="sliceMaterialHint">${hint(key)}</small>`}
 function hint(key){return get(key).printReady?'已完成实机验证 · 打印前选择实际料卷':'可切片、预览与下载 · 在线打印待实机验证'}
 function sync(key){if($('#sliceMaterial'))$('#sliceMaterial').value=get(key).key;if($('#sliceMaterialHint'))$('#sliceMaterialHint').textContent=hint(key)}
 return {get,selector,sync};
})();
