'use strict';
const SlicerSize=(()=>{
 const number=n=>Number(n).toLocaleString('zh-CN',{maximumFractionDigits:3});
 const signature=v=>JSON.stringify([v?.key,v?.placement]);
 function mount({host,get,canEdit,apply}){
  let alive=true;const uid=data.user?.id,button=host.querySelector('#sliceResize');
  const valid=()=>alive&&host.isConnected&&data.user?.id===uid;
  function sync(){if(!valid())return;const value=get();button.disabled=!value||!canEdit();const label=host.querySelector('#sliceScaleSummary');if(label)label.textContent=value?`当前比例 ${number(value.placement.scale*100)}%${value.placement.scale===1?' · 原始尺寸':''} · 可在“更多”中调整`:'默认按原始尺寸打印（100%）'}
  function open(){
   if(!valid()||!canEdit())return;const initial=get();if(!initial)return;
   const savedSignature=signature(initial),current=initial.placement.scale,size=initial.size;
   modal('调整模型尺寸',`<div class="model-size-root"><div class="model-size-heading"><span>${icon('box')}</span><div><strong>${esc(initial.filename)}</strong><p>仅调整选中对象 ${initial.index+1} · 等比修改实际打印尺寸</p></div></div><p class="model-size-original">原始尺寸（100%）：${size.map(number).join(' × ')} mm</p><form id="modelSizeForm"><label class="model-size-percent">尺寸比例 <span><input class="input" name="percent" id="modelSizePercent" type="number" min="1" max="1000" step="any" required value="${Number((current*100).toPrecision(12))}"><b>%</b></span></label><div class="model-size-reset"><small>100% 为模型原始尺寸 · 支持 1%–1000%</small><button type="button" class="text-link" id="modelSizeReset">恢复原尺寸</button></div><table class="model-size-table"><thead><tr><th>模型轴</th><th>当前尺寸</th><th>调整后</th></tr></thead><tbody>${size.map((v,i)=>`<tr><th>${'XYZ'[i]}</th><td>${number(v*current)} mm</td><td data-size-axis="${i}"></td></tr>`).join('')}</tbody></table><p class="form-help">尺寸按模型原始方向显示，旋转只改变摆放。等比调整会一起改变孔径、孔距和卡扣间隙；位置、角度及其他对象保持。点击“应用尺寸”后生效，可撤销。</p><p class="model-size-state" id="modelSizeState" role="status"></p><p class="form-error" id="modelSizeError" role="alert"></p></form></div>`,`<button class="button" data-cancel>取消</button><button class="button primary" form="modelSizeForm" type="submit" id="modelSizeApply">应用尺寸</button>`);
   $('#modal').classList.add('model-size-modal');const form=$('#modelSizeForm'),root=form.closest('.model-size-root'),input=form.elements.percent,submit=$('#modelSizeApply');
   function update(){const n=Number(input.value),ok=input.value.trim()!==''&&Number.isFinite(n)&&n>=1&&n<=1000;root.querySelectorAll('[data-size-axis]').forEach(el=>el.textContent=ok?number(size[Number(el.dataset.sizeAxis)]*n/100)+' mm':'—');submit.disabled=!ok||Math.abs(n/100-current)<1e-12;root.querySelector('#modelSizeState').textContent=ok?(Math.abs(n/100-current)<1e-12?'比例未变化，当前尺寸保持。':'尚未应用：确认后只修改此对象。'):'请输入 1%–1000% 的有效比例';}
   input.oninput=update;root.querySelector('#modelSizeReset').onclick=()=>{input.value='100';update()};update();
   form.onsubmit=e=>{e.preventDefault();if(!valid()||!root.isConnected||!$('#modal').open)return;if(!canEdit()){root.querySelector('#modelSizeError').textContent='当前对象尚未就绪，请结束拖动或等待提交完成后再应用。';return}const currentValue=get();if(!currentValue||signature(currentValue)!==savedSignature){root.querySelector('#modelSizeError').textContent='选中对象或摆放已变化，请关闭后重新检查。';return}const n=Number(input.value);if(input.value.trim()===''||!Number.isFinite(n)||n<1||n>1000){update();return}if(Math.abs(n/100-current)<1e-12)return;apply(n/100);$('#modal').close();toast('模型尺寸已调整，可使用撤销恢复')};
  }
  button.onclick=open;sync();return {sync,close(){alive=false;button.onclick=null}};
 }
 return {mount};
})();
