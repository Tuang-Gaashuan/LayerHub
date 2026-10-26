'use strict';
// A deadline covers response headers and body. Cancelling HTTP does not undo a command.
const Transport=(()=>{
 class RequestError extends Error{
  constructor(message,{unconfirmed=false,unavailable=false,status=0}={}){super(I18n.system(message));this.name='RequestError';this.unconfirmed=unconfirmed;this.unavailable=unavailable;this.status=status}
 }
 async function request(path,payload,csrf,{timeoutMs=payload===undefined?10000:20000}={}){
  const write=payload!==undefined,controller=new AbortController();let timer;
  const operation=(async()=>{
   let response;
   try{response=await fetch(path,{signal:controller.signal,credentials:'same-origin',headers:{'Accept-Language':I18n.language(),...(write?{'Content-Type':'application/json','X-CSRF-Token':csrf}:{})},method:write?'POST':'GET',body:write?JSON.stringify(payload):undefined})}
   catch{throw new RequestError(write?I18n.t('连接中断，请求结果待核对；请先刷新状态，不要重复提交。'):I18n.t('服务链路不可用，请检查云端入口或 NAS 通道。'),{unconfirmed:write,unavailable:true})}
   let result;
   try{result=await response.json()}catch{throw new RequestError(write?I18n.t('没有收到有效确认，请求结果待核对；请先刷新状态，不要重复提交。'):I18n.t('服务暂时无法返回数据，请检查云端入口或 NAS 通道。'),{unconfirmed:write,unavailable:true,status:response.status})}
   if(!response.ok||result?.ok!==true){
    // Gateways can return JSON too. Only explicit application rejections are definitive.
    const rejected=result?.ok===false&&typeof I18n.system(result.error)==='string'&&response.status>=400&&response.status<500;
    throw new RequestError(rejected?I18n.system(result.error):write?I18n.t('服务未能确认请求结果，请先刷新状态与记录。'):I18n.system(result?.error)||I18n.t('服务暂时无法返回数据。'),{unconfirmed:write&&!rejected,unavailable:!rejected,status:response.status})
   }
   return result;
  })();
  const deadline=new Promise((_,reject)=>{timer=setTimeout(()=>{reject(new RequestError(write?I18n.t('请求已超时，结果待核对；超时不会撤销操作，请先刷新状态与记录。'):I18n.t('读取状态超时，当前显示的可能是旧数据。'),{unconfirmed:write,unavailable:true}));controller.abort()},timeoutMs)});
  try{return await Promise.race([operation,deadline])}finally{clearTimeout(timer)}
 }
 return {request,RequestError};
})();
