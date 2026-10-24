// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Windows.Forms;

// Reserve identities before asynchronous reads so rapid double-clicks cannot open duplicates.
internal sealed class FloatingEditorWindows {
    readonly Dictionary<string, Entry> entries = new Dictionary<string, Entry>();
    internal sealed class Entry : IDisposable {
        readonly FloatingEditorWindows owner;
        internal string Key;
        internal Form Window;
        internal Entry(FloatingEditorWindows owner,string key){this.owner=owner;Key=key;}
        internal void Attach(Form window){Window=window;}
        internal void Activate(){if(Window==null || Window.IsDisposed)return;if(Window.WindowState==FormWindowState.Minimized)Window.WindowState=FormWindowState.Normal;Window.Show();Window.BringToFront();Window.Activate();}
        internal bool ChangeKey(string key){Entry other;if(owner.entries.TryGetValue(key,out other) && other!=this){other.Activate();return false;}owner.entries.Remove(Key);Key=key;owner.entries[key]=this;return true;}
        public void Dispose(){Entry current;if(owner.entries.TryGetValue(Key,out current)&&current==this)owner.entries.Remove(Key);Window=null;}
    }
    internal Entry Open(string key){Entry existing;if(entries.TryGetValue(key,out existing)){existing.Activate();return null;}var entry=new Entry(this,key);entries.Add(key,entry);return entry;}
    internal static string OutstandingKey(long taskId,string itemId){return "outstanding:"+taskId+":"+(itemId??"new");}
}

internal sealed partial class FloatingWindow {
    readonly FloatingEditorWindows editorWindows = new FloatingEditorWindows();
}
