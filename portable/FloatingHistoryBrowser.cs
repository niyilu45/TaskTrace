// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Runtime.InteropServices;

[ComVisible(true)]
public sealed class ProgressHistoryBrowserBridge {
    internal Action<int> LoadMore;
    internal Action<string> ImageRequested;
    public void OpenImage(string source) {var callback=ImageRequested;if(callback!=null)callback(source);}
    public void RequestMore(int direction) {var callback=LoadMore;if(callback!=null)callback(direction<0?-1:1);}
    internal const string Script=@"
function historyScroll(){return document.documentElement.scrollTop||document.body.scrollTop||0;}
function historyWheel(e){e=e||window.event;var top=historyScroll(),height=Math.max(document.documentElement.scrollHeight,document.body.scrollHeight),viewport=document.documentElement.clientHeight||window.innerHeight;
 if(e.wheelDelta>0 && top<=1)window.external.RequestMore(-1);
 else if(e.wheelDelta<0 && top+viewport>=height-2)window.external.RequestMore(1);}
document.onmousewheel=historyWheel;
function openHistoryImage(e){e=e||window.event;var image=e.target||e.srcElement;if(image && image.tagName==='IMG')window.external.OpenImage(image.src);}
document.ondblclick=openHistoryImage;
function appendHistoryDay(date,html,prepend){var previous=historyScroll(),oldHeight=document.body.scrollHeight;var section=document.createElement('section');section.id='history-'+date;var title=document.createElement('h2');title.appendChild(document.createTextNode(date));section.appendChild(title);var content=document.createElement('div');content.innerHTML=html;section.appendChild(content);
 if(prepend)document.body.insertBefore(section,document.body.firstChild);else document.body.appendChild(section);
 if(prepend)window.scrollTo(0,previous+document.body.scrollHeight-oldHeight);
}
";
}
