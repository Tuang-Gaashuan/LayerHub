'use strict';
const SlicerHistory=(()=>{
 const copy=value=>JSON.parse(JSON.stringify(value));
 function create(limit=50){let past=[],future=[],current=null,lastGroup='';return {
  reset(value){past=[];future=[];current=copy(value);lastGroup=''},
  record(value,group=''){const next=copy(value);if(JSON.stringify(next)===JSON.stringify(current))return;if(current!==null&&(!group||group!==lastGroup)){past.push(current);if(past.length>limit)past.shift()}current=next;future=[];lastGroup=group},
  end(){lastGroup=''},
  select(value){current=copy(value);lastGroup=''},
  undo(){if(!past.length)return null;future.push(current);current=past.pop();lastGroup='';return copy(current)},
  redo(){if(!future.length)return null;past.push(current);current=future.pop();lastGroup='';return copy(current)},
  get canUndo(){return past.length>0},get canRedo(){return future.length>0}
 }}
 return {create};
})();
