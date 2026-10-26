'use strict';
const SlicerFace=(()=>{
 function mount({host,get,canEdit,available,enter,exit,apply}){
  let alive=true,initial=null,candidate=null;const uid=data.user?.id,button=document.createElement('button'),panel=document.createElement('section');button.className='studio-tool';button.id='sliceFace';button.textContent=I18n.t('选面贴底…');host.querySelector('#sliceGround').before(button);panel.className='face-review';panel.hidden=true;panel.setAttribute('aria-label',I18n.t('选面贴底预览'));panel.innerHTML=I18n.html('<strong>选面贴底 · 先预览</strong><p id="faceReviewStatus" role="status"></p><small>蓝色为选中三角面，绿色为新朝向。尺寸和其他对象保持；接触面积不代表附着或免支撑。</small><div><button type="button" class="button" id="faceCancel">取消 · Esc</button><button type="button" class="button primary" id="faceApply">应用朝向</button></div>');host.querySelector('.slice-scene-panel').append(panel);const status=panel.querySelector('#faceReviewStatus'),submit=panel.querySelector('#faceApply'),valid=()=>alive&&host.isConnected&&data.user?.id===uid,signature=()=>JSON.stringify(get());let saved='';
  function cancel(){if(!initial)return;initial=null;candidate=null;panel.hidden=true;exit()}
  function sync(){if(!valid())return;const value=get();button.disabled=!available()||!canEdit()||!value;if(initial&&(!available()||!canEdit()||signature()!==saved))cancel()}
  button.onclick=()=>{if(!valid()||!available()||!canEdit())return;initial=get();if(!initial)return;saved=signature();candidate=null;submit.disabled=true;panel.hidden=false;status.textContent=I18n.t('点击选中模型的一个外侧平面；右键拖动可旋转视角。');enter()};
  function receive(value){if(!valid()||!initial)return;if(signature()!==saved){cancel();return}candidate=I18n.system(value?.error)?null:value;submit.disabled=!candidate;status.textContent=I18n.system(value?.error)||I18n.msg`尚未应用：对象 ${initial.index+1}，选中三角面 ${value.area.toFixed(2)} mm²；朝向尺寸 ${value.size.map(n=>n.toFixed(1)).join(' × ')} mm。`}
  panel.querySelector('#faceCancel').onclick=cancel;submit.onclick=()=>{if(!valid()||!initial||!candidate||!canEdit()||signature()!==saved){cancel();return}const rotation=[...candidate.rotation];cancel();apply(rotation)};
  sync();return {sync,receive,cancel,close(){cancel();alive=false;button.onclick=null;panel.remove()}};
 }
 return {mount};
})();
