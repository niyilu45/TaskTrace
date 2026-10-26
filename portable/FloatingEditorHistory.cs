// SPDX-License-Identifier: AGPL-3.0-or-later
internal static class FloatingEditorHistory {
    internal const string Script=@"
var taskTraceHistoryObserver=null;
function invalidateTaskTraceHistoryMedia(records){for(var i=0;i<records.length;i++){var record=records[i],keys=record.target._taskTraceHistoryMedia;if(keys)delete keys[record.attributeName.toLowerCase()];}}
function observeTaskTraceHistoryMedia(){if(taskTraceHistoryObserver)taskTraceHistoryObserver.disconnect();if(window.MutationObserver){taskTraceHistoryObserver=new MutationObserver(invalidateTaskTraceHistoryMedia);taskTraceHistoryObserver.observe(document.getElementById('progress-editor'),{subtree:true,attributes:true,attributeFilter:['src','href']});}}
var taskTraceHistory={undo:[],redo:[],known:null,nativeBefore:null,applying:false,media:{},mediaSequence:0};
function taskTraceHistoryNode(node){
 if(node.nodeType===3)return {text:node.nodeValue};
 if(node.nodeType===8)return {comment:node.nodeValue};
 if(node.nodeType!==1)return null;
 var attrs=[],media=node._taskTraceHistoryMedia||(node._taskTraceHistoryMedia={});
 for(var i=0;i<node.attributes.length;i++){
  var attr=node.attributes[i],name=attr.name.toLowerCase();
  if((name==='src'&&node.tagName==='IMG')||(name==='href'&&node.tagName==='A')){
   var key=media[name];if(!key||!taskTraceHistory.media[key]){key=++taskTraceHistory.mediaSequence;taskTraceHistory.media[key]=attr.value;media[name]=key;}attrs.push([name,{media:key}]);
  }else attrs.push([name,attr.value]);
 }
 var children=[];for(var child=node.firstChild;child;child=child.nextSibling){var saved=taskTraceHistoryNode(child);if(saved)children.push(saved);}
 return {tag:node.tagName,attrs:attrs,children:children};
}
function taskTraceHistoryPath(node){var editor=document.getElementById('progress-editor'),path=[];while(node&&node!==editor){var index=0;for(var sibling=node.previousSibling;sibling;sibling=sibling.previousSibling)index++;path.unshift(index);node=node.parentNode;}return node===editor?path:null;}
function taskTraceHistoryPoint(path){var node=document.getElementById('progress-editor');if(!path)return node;for(var i=0;i<path.length;i++){if(!node.childNodes[path[i]])return node;node=node.childNodes[path[i]];}return node;}
function captureTaskTraceHistory(range){
 if(taskTraceHistoryObserver)invalidateTaskTraceHistoryMedia(taskTraceHistoryObserver.takeRecords());
 var editor=document.getElementById('progress-editor'),nodes=[];for(var node=editor.firstChild;node;node=node.nextSibling){var saved=taskTraceHistoryNode(node);if(saved)nodes.push(saved);}
 range=range||taskTraceRange()||taskTraceSavedRange;var selection=null;
 if(range&&taskTraceInside(range.commonAncestorContainer))selection={start:taskTraceHistoryPath(range.startContainer),startOffset:range.startOffset,end:taskTraceHistoryPath(range.endContainer),endOffset:range.endOffset};
 return {nodes:nodes,key:JSON.stringify(nodes),selection:selection,viewport:taskTraceViewport(null)};
}
function taskTraceHistorySources(nodes,used){for(var i=0;i<nodes.length;i++){var node=nodes[i];if(node.attrs)for(var j=0;j<node.attrs.length;j++){var value=node.attrs[j][1];if(typeof value==='object')used[value.media]=true;}if(node.children)taskTraceHistorySources(node.children,used);}}
function trimTaskTraceHistory(){
 var history=taskTraceHistory,total=0;for(var i=0;i<history.undo.length;i++)total+=history.undo[i].before.key.length+history.undo[i].after.key.length;
 while(history.undo.length>1&&(history.undo.length>50||total>4*1024*1024)){var old=history.undo.shift();total-=old.before.key.length+old.after.key.length;}
 var used={};if(history.known)taskTraceHistorySources(history.known.nodes,used);
 for(var list=0;list<2;list++){var entries=list?history.redo:history.undo;for(var k=0;k<entries.length;k++){taskTraceHistorySources(entries[k].before.nodes,used);taskTraceHistorySources(entries[k].after.nodes,used);}}
 for(var key in history.media)if(!used[key])delete history.media[key];
}
function commitTaskTraceHistory(before,kind){
 if(taskTraceHistory.applying)return;
 var after=captureTaskTraceHistory(),history=taskTraceHistory;history.nativeBefore=null;
 if(!before)before=history.known;
 if(before&&before.key!==after.key){
  var previous=history.undo.length?history.undo[history.undo.length-1]:null,now=new Date().getTime();
  if(kind==='typing'&&previous&&previous.kind===kind&&now-previous.time<700&&previous.after.key===before.key&&JSON.stringify(previous.after.selection)===JSON.stringify(before.selection)){previous.after=after;previous.time=now;}
  else history.undo.push({before:before,after:after,kind:kind,time:now});
  history.redo=[];
 }
 history.known=after;trimTaskTraceHistory();
}
function resetTaskTraceHistory(){observeTaskTraceHistoryMedia();taskTraceHistory={undo:[],redo:[],known:null,nativeBefore:null,applying:false,media:{},mediaSequence:0};taskTraceHistory.known=captureTaskTraceHistory();}
function restoreTaskTraceHistoryNode(saved){
 if(saved.text!==undefined)return document.createTextNode(saved.text);
 if(saved.comment!==undefined)return document.createComment(saved.comment);
 var node=document.createElement(saved.tag),media={};
 for(var i=0;i<saved.attrs.length;i++){var attr=saved.attrs[i],value=attr[1];if(typeof value==='object'){media[attr[0]]=value.media;value=taskTraceHistory.media[value.media]||'';}node.setAttribute(attr[0],value);}
 node._taskTraceHistoryMedia=media;
 for(var j=0;j<saved.children.length;j++)node.appendChild(restoreTaskTraceHistoryNode(saved.children[j]));return node;
}
function restoreTaskTraceHistory(state){
 var history=taskTraceHistory,editor=document.getElementById('progress-editor');history.applying=true;resetTaskTracePastes();resetTaskTraceImageViews();taskTraceSavedRange=null;
 try{
  var fragment=document.createDocumentFragment();for(var i=0;i<state.nodes.length;i++)fragment.appendChild(restoreTaskTraceHistoryNode(state.nodes[i]));
  while(editor.firstChild)editor.removeChild(editor.firstChild);editor.appendChild(fragment);
  var range=document.createRange(),selected=state.selection;
  if(selected){var start=taskTraceHistoryPoint(selected.start),end=taskTraceHistoryPoint(selected.end);range.setStart(start,Math.min(selected.startOffset,start.nodeType===3?start.length:start.childNodes.length));range.setEnd(end,Math.min(selected.endOffset,end.nodeType===3?end.length:end.childNodes.length));}
  else {range.selectNodeContents(editor);range.collapse(false);}
  restoreTaskTraceRange(range);taskTraceSavedRange=range.cloneRange();restoreTaskTraceViewport(state.viewport);history.known=state;history.nativeBefore=null;
 }finally{history.applying=false;}
 window.external.NotifyPaste();
}
function undoTaskTrace(){
 if(document.getElementById('progress-editor').getAttribute('contenteditable')==='false')return false;
 if(taskTracePasteOrder.length){var id=taskTracePasteOrder.pop();delete taskTracePendingPastes[id];window.external.NotifyPaste();return true;}
 var entry=taskTraceHistory.undo.pop();if(!entry)return false;taskTraceHistory.redo.push(entry);restoreTaskTraceHistory(entry.before);return true;
}
function redoTaskTrace(){
 if(document.getElementById('progress-editor').getAttribute('contenteditable')==='false')return false;
 var entry=taskTraceHistory.redo.pop();if(!entry)return false;taskTraceHistory.undo.push(entry);restoreTaskTraceHistory(entry.after);return true;
}
var taskTraceHistoryInsert=insertTaskTraceHtml;
insertTaskTraceHtml=function(html,range){var before=captureTaskTraceHistory(range),changed=taskTraceHistoryInsert(html,range);if(changed)commitTaskTraceHistory(before,'paste');return changed;};
var taskTraceHistoryCopy=copyTaskTrace;
copyTaskTrace=function(e,cut){var before=cut?captureTaskTraceHistory():null,result=taskTraceHistoryCopy(e,cut);if(cut)commitTaskTraceHistory(before,'cut');return result;};
document.addEventListener('keydown',function(e){
 var code=e.keyCode||e.which;if(!taskTraceInside(e.target||e.srcElement))return;
 if(e.ctrlKey&&!e.altKey&&(code===90||code===89)){if(code===89||e.shiftKey)redoTaskTrace();else undoTaskTrace();if(e.preventDefault)e.preventDefault();e.returnValue=false;return;}
 if(document.getElementById('progress-editor').getAttribute('contenteditable')==='false'||e.altKey||e.metaKey)return;
 if(e.ctrlKey&&(code===65||code===67||code===86||code===88))return;
 if(code===8||code===13||code===46||code===229||(code>=32&&!e.ctrlKey))taskTraceHistory.nativeBefore=captureTaskTraceHistory();
},true);
document.addEventListener('input',function(e){if(!taskTraceHistory.applying&&taskTraceInside(e.target||e.srcElement))commitTaskTraceHistory(taskTraceHistory.nativeBefore,'typing');});
";
}
