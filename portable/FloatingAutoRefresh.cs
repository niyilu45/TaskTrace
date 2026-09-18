// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    readonly object autoRefreshGate = new object();
    FileSystemWatcher autoRefreshWatcher;
    bool autoRefreshInitialized, autoRefreshDisposed, autoRefreshDirty, autoRefreshRunning;
    bool autoRefreshRestartWatcher, autoRefreshWatcherFailed;
    long autoRefreshRevision, autoRefreshAcknowledgedRevision, autoRefreshReadCount;
    DateTime autoRefreshLastSignalUtc, autoRefreshRetryAfterUtc, autoRefreshWatchRetryAfterUtc;
    string autoRefreshFileSignature;
    IntPtr autoRefreshDatabase;
    long autoRefreshDatabaseVersion,autoRefreshDatabaseReadCount;
    bool autoRefreshDatabaseVersionKnown,autoRefreshDatabaseFailed;
    DateTime autoRefreshDatabaseRetryAfterUtc;
    string autoRefreshDatabaseError="";
    DateTime teamSyncAfterUtc=DateTime.MinValue;
    bool teamSyncRunning;

    [DllImport("winsqlite3.dll",EntryPoint="sqlite3_open_v2",ExactSpelling=true,CallingConvention=CallingConvention.Cdecl)]
    static extern int AutoRefreshSqliteOpen(byte[] path,out IntPtr database,int flags,IntPtr vfs);
    [DllImport("winsqlite3.dll",EntryPoint="sqlite3_prepare_v2",ExactSpelling=true,CallingConvention=CallingConvention.Cdecl)]
    static extern int AutoRefreshSqlitePrepare(IntPtr database,byte[] sql,int length,out IntPtr statement,IntPtr tail);
    [DllImport("winsqlite3.dll",EntryPoint="sqlite3_step",ExactSpelling=true,CallingConvention=CallingConvention.Cdecl)]
    static extern int AutoRefreshSqliteStep(IntPtr statement);
    [DllImport("winsqlite3.dll",EntryPoint="sqlite3_column_int64",ExactSpelling=true,CallingConvention=CallingConvention.Cdecl)]
    static extern long AutoRefreshSqliteColumn(IntPtr statement,int column);
    [DllImport("winsqlite3.dll",EntryPoint="sqlite3_finalize",ExactSpelling=true,CallingConvention=CallingConvention.Cdecl)]
    static extern int AutoRefreshSqliteFinalize(IntPtr statement);
    [DllImport("winsqlite3.dll",EntryPoint="sqlite3_close_v2",ExactSpelling=true,CallingConvention=CallingConvention.Cdecl)]
    static extern int AutoRefreshSqliteClose(IntPtr database);

    // These diagnostics also let isolated self-tests exercise the production queue.
    long AutoRefreshRevision { get { lock(autoRefreshGate)return autoRefreshRevision; } }
    long AutoRefreshAcknowledgedRevision { get { lock(autoRefreshGate)return autoRefreshAcknowledgedRevision; } }
    long AutoRefreshDatabaseReadCount { get { lock(autoRefreshGate)return autoRefreshDatabaseReadCount; } }
    long AutoRefreshReadCount { get { lock(autoRefreshGate)return autoRefreshReadCount; } }
    bool AutoRefreshPending { get { lock(autoRefreshGate)return autoRefreshDirty; } }
    bool AutoRefreshInProgress { get { lock(autoRefreshGate)return autoRefreshRunning; } }

    void InitializeAutoRefresh() {
        lock(autoRefreshGate) {
            if(autoRefreshInitialized || autoRefreshDisposed)return;
            autoRefreshInitialized=true;autoRefreshRestartWatcher=true;
        }
        autoRefreshFileSignature=ReadAutoRefreshFileSignature();
        EnsureAutoRefreshWatcher();
        PollAutoRefreshFiles();
        timer.Interval=1000;
        timer.Tick+=AutoRefreshTimerTick;
        Disposed+=delegate {DisposeAutoRefresh();};
    }

    async void AutoRefreshTimerTick(object sender,EventArgs e) {
        if(selfTest)return;
        try {
            if(DateTime.UtcNow>=completedHideRefreshAfterUtc && Visible && !collapsed && !busy && !rendering && !dragging && !AutoRefreshInteractionActive())
                await LoadTasks(true);
            if(!teamSyncRunning && DateTime.UtcNow>=teamSyncAfterUtc) {
                teamSyncRunning=true;teamSyncAfterUtc=DateTime.UtcNow.AddSeconds(15);
                try {await Api("POST","/tasktrace/team/sync",null);}
                catch {teamSyncAfterUtc=DateTime.UtcNow.AddSeconds(15);}
                finally {teamSyncRunning=false;}
            }
            await ProcessAutoRefresh();
        }
        catch {DeferAutoRefreshFailure();}
    }

    static bool IsAutoRefreshDataFile(string name) {
        if(String.IsNullOrEmpty(name))return false;
        name=Path.GetFileName(name);
        return String.Equals(name,"tasktrace.db",StringComparison.OrdinalIgnoreCase)
            || String.Equals(name,"tasktrace.db-wal",StringComparison.OrdinalIgnoreCase)
            || String.Equals(name,"tasktrace.db-journal",StringComparison.OrdinalIgnoreCase);
    }

    void SignalAutoRefreshChange() {
        lock(autoRefreshGate) {
            if(autoRefreshDisposed)return;
            autoRefreshDirty=true;autoRefreshRevision++;autoRefreshLastSignalUtc=DateTime.UtcNow;
        }
    }

    void AutoRefreshFileChanged(object sender,FileSystemEventArgs e) {
        if(IsAutoRefreshDataFile(e.Name))SignalAutoRefreshChange();
    }

    void AutoRefreshFileRenamed(object sender,RenamedEventArgs e) {
        if(IsAutoRefreshDataFile(e.Name) || IsAutoRefreshDataFile(e.OldName))SignalAutoRefreshChange();
    }

    void AutoRefreshWatcherError(object sender,ErrorEventArgs e) {
        lock(autoRefreshGate) {
            if(autoRefreshDisposed)return;
            autoRefreshRestartWatcher=true;autoRefreshWatcherFailed=true;
            autoRefreshWatchRetryAfterUtc=DateTime.UtcNow.AddSeconds(5);
        }
        SignalAutoRefreshChange();
    }

    void EnsureAutoRefreshWatcher() {
        FileSystemWatcher previous;
        lock(autoRefreshGate) {
            if(autoRefreshDisposed || !autoRefreshRestartWatcher || DateTime.UtcNow<autoRefreshWatchRetryAfterUtc)return;
            previous=autoRefreshWatcher;autoRefreshWatcher=null;autoRefreshRestartWatcher=false;
        }
        if(previous!=null)try {previous.Dispose();}catch { }
        FileSystemWatcher created=null;
        try {
            created=new FileSystemWatcher(data,"tasktrace.db*") {
                IncludeSubdirectories=false,
                NotifyFilter=NotifyFilters.FileName|NotifyFilters.LastWrite|NotifyFilters.Size|NotifyFilters.CreationTime
            };
            created.Changed+=AutoRefreshFileChanged;created.Created+=AutoRefreshFileChanged;created.Deleted+=AutoRefreshFileChanged;
            created.Renamed+=AutoRefreshFileRenamed;created.Error+=AutoRefreshWatcherError;
            created.EnableRaisingEvents=true;
            bool rescan, disposed;
            lock(autoRefreshGate) {
                disposed=autoRefreshDisposed;
                if(!disposed)autoRefreshWatcher=created;
                rescan=autoRefreshWatcherFailed;autoRefreshWatcherFailed=false;
            }
            if(disposed){created.Dispose();return;}
            if(rescan)SignalAutoRefreshChange();
        } catch {
            if(created!=null)try {created.Dispose();}catch { }
            bool firstFailure;
            lock(autoRefreshGate) {
                if(autoRefreshDisposed)return;
                firstFailure=!autoRefreshWatcherFailed;autoRefreshWatcherFailed=true;autoRefreshRestartWatcher=true;
                autoRefreshWatchRetryAfterUtc=DateTime.UtcNow.AddSeconds(5);
            }
            // An unavailable watcher falls back to file metadata; do not request on every retry.
            if(firstFailure)SignalAutoRefreshChange();
        }
    }

    void CloseAutoRefreshDatabase() {
        var database=autoRefreshDatabase;autoRefreshDatabase=IntPtr.Zero;autoRefreshDatabaseVersionKnown=false;
        if(database!=IntPtr.Zero)try {AutoRefreshSqliteClose(database);}catch { }
    }

    void PollAutoRefreshDataVersion() {
        bool changed=false;
        lock(autoRefreshGate) {
            if(autoRefreshDisposed || DateTime.UtcNow<autoRefreshDatabaseRetryAfterUtc)return;
            IntPtr statement=IntPtr.Zero;
            try {
                if(autoRefreshDatabase==IntPtr.Zero) {
                    const int ReadOnly=0x1,FullMutex=0x10000;
                    int opened=AutoRefreshSqliteOpen(Encoding.UTF8.GetBytes(Path.Combine(data,"tasktrace.db")+"\0"),out autoRefreshDatabase,ReadOnly|FullMutex,IntPtr.Zero);
                    if(opened!=0)throw new InvalidOperationException("SQLite read-only open: "+opened);
                }
                // Only a persistent connection makes data_version comparable between reads.
                // Finalizing each statement releases its read transaction before returning to the UI.
                int prepared=AutoRefreshSqlitePrepare(autoRefreshDatabase,Encoding.UTF8.GetBytes("PRAGMA data_version;\0"),-1,out statement,IntPtr.Zero);
                if(prepared!=0)throw new InvalidOperationException("SQLite data_version prepare: "+prepared);
                int row=AutoRefreshSqliteStep(statement);
                if(row!=100)throw new InvalidOperationException("SQLite data_version step: "+row);
                long version=AutoRefreshSqliteColumn(statement,0);autoRefreshDatabaseReadCount++;
                changed=autoRefreshDatabaseFailed || (autoRefreshDatabaseVersionKnown && version!=autoRefreshDatabaseVersion);
                autoRefreshDatabaseVersion=version;autoRefreshDatabaseVersionKnown=true;
                autoRefreshDatabaseFailed=false;autoRefreshDatabaseRetryAfterUtc=DateTime.MinValue;autoRefreshDatabaseError="";
            }catch(Exception error) {
                changed=!autoRefreshDatabaseFailed;autoRefreshDatabaseFailed=true;
                autoRefreshDatabaseRetryAfterUtc=DateTime.UtcNow.AddSeconds(5);autoRefreshDatabaseError=error.Message;
                autoRefreshDirty=true;
            }finally {
                if(statement!=IntPtr.Zero)try {AutoRefreshSqliteFinalize(statement);}catch { }
                if(autoRefreshDatabaseFailed)CloseAutoRefreshDatabase();
            }
        }
        if(changed)SignalAutoRefreshChange();
    }
    string ReadAutoRefreshFileSignature() {
        return String.Join("|",new[]{"tasktrace.db","tasktrace.db-wal","tasktrace.db-journal"}.Select(name=> {
            try {
                var file=new FileInfo(Path.Combine(data,name));file.Refresh();
                return file.Exists?name+":"+file.Length+":"+file.LastWriteTimeUtc.Ticks+":"+file.CreationTimeUtc.Ticks:name+":missing";
            } catch(Exception error) {return name+":unavailable:"+error.GetType().Name;}
        }));
    }

    void PollAutoRefreshFiles() {
        string signature=ReadAutoRefreshFileSignature();
        bool changed=false;
        lock(autoRefreshGate) {
            if(autoRefreshDisposed)return;
            if(autoRefreshFileSignature!=null && autoRefreshFileSignature!=signature)changed=true;
            autoRefreshFileSignature=signature;
        }
        if(changed)SignalAutoRefreshChange();
        PollAutoRefreshDataVersion();
    }

    bool AutoRefreshInteractionActive() {
        if(Control.MouseButtons!=MouseButtons.None)return true;
        if(priorityFilterMenus.Any(menu=>!menu.IsDisposed && menu.Visible))return true;
        if(tasks.ContextMenuStrip!=null && tasks.ContextMenuStrip.Visible)return true;
        if(ContextMenuStrip!=null && ContextMenuStrip.Visible)return true;
        return tray.ContextMenuStrip!=null && tray.ContextMenuStrip.Visible;
    }

    async Task ProcessAutoRefresh() {
        lock(autoRefreshGate)if(autoRefreshDisposed || !autoRefreshInitialized)return;
        if(closing || IsDisposed)return;
        EnsureAutoRefreshWatcher();PollAutoRefreshFiles();
        if(busy || rendering || dragging || !Visible || collapsed || AutoRefreshInteractionActive())return;
        long revision;
        lock(autoRefreshGate) {
            var now=DateTime.UtcNow;
            if(autoRefreshDisposed || autoRefreshDatabaseFailed || autoRefreshRunning || !autoRefreshDirty || now<autoRefreshRetryAfterUtc || (now-autoRefreshLastSignalUtc).TotalMilliseconds<600)return;
            autoRefreshRunning=true;revision=autoRefreshRevision;autoRefreshReadCount++;
        }
        try {
            bool loaded=await LoadTasks(true);
            if(!loaded || closing || IsDisposed)return;
            await RefreshUndo();
            lock(autoRefreshGate) {
                if(autoRefreshDisposed)return;
                autoRefreshAcknowledgedRevision=revision;
                autoRefreshDirty=autoRefreshRevision!=revision;
                autoRefreshRetryAfterUtc=DateTime.MinValue;
            }
        } catch {DeferAutoRefreshFailure();}
        finally {lock(autoRefreshGate)autoRefreshRunning=false;}
    }

    void DeferAutoRefreshFailure() {
        lock(autoRefreshGate) {
            if(autoRefreshDisposed)return;
            autoRefreshDirty=true;autoRefreshRetryAfterUtc=DateTime.UtcNow.AddSeconds(5);
        }
    }

    void DisposeAutoRefresh() {
        FileSystemWatcher watcher;
        lock(autoRefreshGate) {
            if(autoRefreshDisposed)return;
            autoRefreshDisposed=true;watcher=autoRefreshWatcher;autoRefreshWatcher=null;CloseAutoRefreshDatabase();
        }
        timer.Tick-=AutoRefreshTimerTick;
        if(watcher!=null)try {watcher.Dispose();}catch { }
    }

    async Task WaitForAutoRefreshTest(Func<bool> condition,string message) {
        DateTime until=DateTime.UtcNow.AddSeconds(5);
        do {
            await ProcessAutoRefresh();
            if(condition() && !AutoRefreshPending && !AutoRefreshInProgress)return;
            await Task.Delay(100);
        }while(DateTime.UtcNow<until);
        throw new Exception(message+" (revision="+AutoRefreshRevision+", acknowledged="+AutoRefreshAcknowledgedRevision+", pending="+AutoRefreshPending+", reads="+AutoRefreshReadCount+")");
    }

    async Task WaitForAutoRefreshSignalTest(long previousRevision) {
        DateTime until=DateTime.UtcNow.AddSeconds(5);
        while(AutoRefreshRevision==previousRevision && DateTime.UtcNow<until){PollAutoRefreshFiles();if(AutoRefreshRevision==previousRevision)await Task.Delay(50);}
        if(AutoRefreshRevision==previousRevision)throw new Exception("Committed database change was not detected: "+autoRefreshDatabaseError);
    }

    TreeNode AutoRefreshTestTask(long id) {
        return tasks.Nodes.Find(id.ToString(),true).FirstOrDefault(node=>node.Tag is long && (long)node.Tag==id);
    }

    async Task TestAutoRefresh() {
        if(!selfTest)throw new InvalidOperationException("Auto refresh tests require the isolated self-test instance.");
        var originalProject=projects.SelectedItem as Project;
        long originalProjectId=originalProject==null?0:originalProject.Id;
        bool originalSimple=simpleMode,originalBusy=busy,originalVisible=Visible,originalCompleted=showCompleted.Checked;
        bool originalDetailsTesting=simpleDetailsTesting,originalSort=prioritySort.Checked;
        string originalSearch=search.Text;int originalPage=page;
        var originalPriorities=visiblePriorities.ToArray();var originalCollapsed=collapsedTasks.ToArray();
        var originalBounds=Bounds;var originalSimpleSize=simpleSize;
        var settings=new Dictionary<string,string>();
        foreach(string name in new[]{"floating-tree.json","simple-window.json","floating-window.json","floating-order.json"}) {
            string path=Path.Combine(data,name);settings[path]=File.Exists(path)?File.ReadAllText(path):null;
        }
        long projectId=0;Exception failure=null;
        try {
            SetBusy(true);simpleDetailsTesting=true;InvalidateSimpleOutstanding();
            rendering=true;
            try {
                if(collapsed)ToggleFold();SetSimpleMode(false);search.Clear();showCompleted.Checked=true;page=1;
                visiblePriorities.Clear();visiblePriorities.UnionWith(Enumerable.Range(0,10));prioritySort.Checked=false;collapsedTasks.Clear();
            }finally{rendering=false;}
            long parentId,childId;var shared=new SharedList();
            using(BeginUndoGroup()) {
                projectId=Convert.ToInt64((await Api("POST","/projects",new{title="自动同步专项验收 "+Guid.NewGuid().ToString("N")}))["id"]);
                parentId=Convert.ToInt64((await Api("POST","/projects/"+projectId+"/tasks",new{title="自动同步父任务"}))["id"]);
                childId=await CreateSubtask(parentId,projectId,"自动同步子任务");
                shared.Items.Add(new PendingItem{Id="auto-refresh-parent",Html="父任务遗留事项"});await WriteShared(parentId,shared);
                var childShared=new SharedList();childShared.Items.Add(new PendingItem{Id="auto-refresh-child",Html="子任务遗留事项"});await WriteShared(childId,childShared);
            }
            rendering=true;
            try {
                var selected=new Project{Id=projectId,Title="自动同步专项验收"};projects.Items.Add(selected);projects.SelectedItem=selected;
                SetSimpleMode(true);RestoreWindow();
            }finally{rendering=false;}
            projectsDirty=true;SetBusy(false);await LoadTasks();await Task.Delay(100);
            await WaitForAutoRefreshTest(delegate{return true;},"Initial fixture changes did not settle");
            if(AutoRefreshDatabaseReadCount==0 || autoRefreshDatabaseFailed)throw new Exception("Native SQLite commit detection was not exercised: "+autoRefreshDatabaseError);
            var parent=AutoRefreshTestTask(parentId);var child=AutoRefreshTestTask(childId);
            if(parent==null || child==null || child.Parent!=parent)throw new Exception("Auto refresh fixture hierarchy is missing");
            var leaf=parent.Nodes.Cast<TreeNode>().Single(node=>node.Tag is OutstandingLeaf);
            var childLeaf=child.Nodes.Cast<TreeNode>().Single(node=>node.Tag is OutstandingLeaf);
            parent.Expand();child.Collapse();tasks.SelectedNode=leaf;tasks.TopNode=parent;tasks.Update();await Task.Delay(100);
            var stableTop=tasks.TopNode;var stableBounds=tasks.Bounds;var stableWindowBounds=Bounds;
            int invalidations=0,enabledChanges=0;
            InvalidateEventHandler invalidated=delegate{invalidations++;};EventHandler enabledChanged=delegate{enabledChanges++;};
            tasks.Invalidated+=invalidated;tasks.EnabledChanged+=enabledChanged;
            try {
                if(!await LoadTasks(true))throw new Exception("Unchanged background load was unexpectedly cancelled");
                if(invalidations!=0 || enabledChanges!=0 || AutoRefreshTestTask(parentId)!=parent || AutoRefreshTestTask(childId)!=child || leaf.Parent!=parent || childLeaf.Parent!=child ||
                    !parent.IsExpanded || child.IsExpanded || tasks.SelectedNode!=leaf || tasks.TopNode!=stableTop || tasks.Bounds!=stableBounds || Bounds!=stableWindowBounds)
                    throw new Exception("Unchanged background load invalidated or disabled the tree, recreated nodes, or changed selection, expansion, scroll, or bounds");
                long reads=AutoRefreshReadCount;
                SignalAutoRefreshChange();SignalAutoRefreshChange();SignalAutoRefreshChange();
                await ProcessAutoRefresh();
                if(AutoRefreshReadCount!=reads || !AutoRefreshPending)throw new Exception("A file event skipped its debounce window");
                await WaitForAutoRefreshTest(delegate{return true;},"Coalesced file events were not applied");
                if(AutoRefreshReadCount!=reads+1 || invalidations!=0 || enabledChanges!=0)throw new Exception("A file event burst was not one silent background load");
                reads=AutoRefreshReadCount;
                for(int i=0;i<4;i++){await Task.Delay(100);await ProcessAutoRefresh();}
                if(AutoRefreshReadCount!=reads || invalidations!=0 || enabledChanges!=0)throw new Exception("Idle metadata polling requested data or changed the tree");
            }finally{tasks.Invalidated-=invalidated;tasks.EnabledChanged-=enabledChanged;}

            long previousRevision=AutoRefreshRevision;
            using(BeginUndoGroup())await Api("PATCH","/tasks/"+parentId,new{title="自动同步父任务已更新"});
            await WaitForAutoRefreshSignalTest(previousRevision);
            await WaitForAutoRefreshTest(delegate{var current=AutoRefreshTestTask(parentId);return current!=null && current.Text.Contains("已更新");},"Task edit did not propagate from commit detection");
            parent=AutoRefreshTestTask(parentId);child=AutoRefreshTestTask(childId);
            leaf=parent.Nodes.Cast<TreeNode>().Single(node=>node.Tag is OutstandingLeaf);
            if(!parent.IsExpanded || child.IsExpanded || tasks.SelectedNode!=leaf || tasks.TopNode!=parent || tasks.Bounds!=stableBounds)throw new Exception("Task edit lost simple tree selection, scroll, expansion, or viewport");
            previousRevision=AutoRefreshRevision;shared.Items[0].Html="父任务遗留事项已更新";
            using(BeginUndoGroup())await WriteShared(parentId,shared);
            await WaitForAutoRefreshSignalTest(previousRevision);
            await WaitForAutoRefreshTest(delegate {
                var current=AutoRefreshTestTask(parentId);
                return current!=null && current.Nodes.Cast<TreeNode>().Any(node=>node.Tag is OutstandingLeaf && ((OutstandingLeaf)node.Tag).Html=="父任务遗留事项已更新");
            },"Shared outstanding edit did not propagate from commit detection");
            var changedLeaf=parent.Nodes.Cast<TreeNode>().Single(node=>node.Tag is OutstandingLeaf);
            if(AutoRefreshTestTask(parentId)!=parent || AutoRefreshTestTask(childId)!=child || changedLeaf==leaf || tasks.SelectedNode!=changedLeaf || tasks.TopNode!=parent || child.IsExpanded)
                throw new Exception("Outstanding update recreated task nodes or lost item selection, scroll, or collapsed children");

            int modeLoads=taskLoadVersion;long modeReads=AutoRefreshReadCount;
            SetSimpleMode(false);
            if(taskLoadVersion!=modeLoads || AutoRefreshReadCount!=modeReads || AutoRefreshTestTask(parentId)!=parent || changedLeaf.Parent!=parent || tasks.SelectedNode!=changedLeaf || child.IsExpanded)throw new Exception("Full layout switch reloaded data or lost shared tree state");
            invalidations=0;enabledChanges=0;tasks.Invalidated+=invalidated;tasks.EnabledChanged+=enabledChanged;
            try {
                if(!await LoadTasks(true) || invalidations!=0 || enabledChanges!=0 || AutoRefreshTestTask(parentId)!=parent || changedLeaf.Parent!=parent || tasks.SelectedNode!=changedLeaf || child.IsExpanded)throw new Exception("Full mode unchanged refresh changed the shared tree");
            }finally{tasks.Invalidated-=invalidated;tasks.EnabledChanged-=enabledChanged;}
            var fullShared=ReadShared(await ReadHistory(childId));fullShared.Items[0].Html="完整悬浮窗更新遗留事项";
            previousRevision=AutoRefreshRevision;
            using(BeginUndoGroup())await WriteShared(childId,fullShared);
            await WaitForAutoRefreshSignalTest(previousRevision);
            await WaitForAutoRefreshTest(delegate{return child.Nodes.Cast<TreeNode>().Any(node=>node.Tag is OutstandingLeaf && ((OutstandingLeaf)node.Tag).Html=="完整悬浮窗更新遗留事项");},"Full mode did not apply direct outstanding changes");
            if(AutoRefreshTestTask(childId)!=child || child.IsExpanded || tasks.SelectedNode!=changedLeaf)throw new Exception("Full mode update lost shared tree state");
            var inFlight=LoadTasks(true);SetSimpleMode(true);
            if(!await inFlight || AutoRefreshTestTask(parentId)!=parent || tasks.SelectedNode!=changedLeaf)throw new Exception("Layout switch discarded an in-flight task refresh");

            long readsBefore=AutoRefreshReadCount;
            SetBusy(true);SignalAutoRefreshChange();await Task.Delay(700);await ProcessAutoRefresh();
            if(!AutoRefreshPending || AutoRefreshReadCount!=readsBefore)throw new Exception("Busy state dropped or consumed a pending change");
            SetBusy(false);await WaitForAutoRefreshTest(delegate{return true;},"Busy state did not resume its pending change");
            readsBefore=AutoRefreshReadCount;
            HideToTray();SignalAutoRefreshChange();await Task.Delay(700);await ProcessAutoRefresh();
            if(!AutoRefreshPending || AutoRefreshReadCount!=readsBefore)throw new Exception("Hidden state dropped or consumed a pending change");
            RestoreWindow();await WaitForAutoRefreshTest(delegate{return true;},"Restored window did not resume its pending change");

            SignalAutoRefreshChange();await Task.Delay(700);
            Task pending=ProcessAutoRefresh();
            if(!AutoRefreshInProgress)throw new Exception("Expected an asynchronous background read for revision acknowledgement test");
            SignalAutoRefreshChange();await pending;
            if(!AutoRefreshPending || AutoRefreshAcknowledgedRevision>=AutoRefreshRevision)throw new Exception("A change during background reads was acknowledged before it was read");
            await WaitForAutoRefreshTest(delegate{return true;},"A change during background reads was never retried");
        }catch(Exception error){failure=error;}
        // C# 5 cannot await in a finally block; cleanup still runs after every test failure.
        try {
            SetBusy(true);simpleDetailsTesting=true;InvalidateSimpleOutstanding();
            if(projectId>0)using(BeginUndoGroup())await Api("DELETE","/projects/"+projectId,null);
            rendering=true;
            try {
                SetSimpleMode(false);search.Text=originalSearch;showCompleted.Checked=originalCompleted;page=originalPage;
                visiblePriorities.Clear();visiblePriorities.UnionWith(originalPriorities);prioritySort.Checked=originalSort;
                collapsedTasks.Clear();collapsedTasks.UnionWith(originalCollapsed);simpleCollapsedDuringRead.Clear();
                projects.SelectedItem=projects.Items.Cast<Project>().FirstOrDefault(item=>item.Id==originalProjectId);
                preferredProjectId=originalProjectId;simpleSize=originalSimpleSize;Bounds=originalBounds;
                if(originalSimple)SetSimpleMode(true);
            }finally{rendering=false;}
            projectsDirty=true;SetBusy(false);await LoadTasks();await RefreshUndo();UpdatePriorityFilterControls();
            if(!originalVisible)HideToTray();SetBusy(originalBusy);
        }catch(Exception error){if(failure==null)failure=error;}
        finally {
            simpleDetailsTesting=originalDetailsTesting;
            foreach(var setting in settings)try {
                if(setting.Value==null){if(File.Exists(setting.Key))File.Delete(setting.Key);}else File.WriteAllText(setting.Key,setting.Value);
            }catch { }
        }
        File.WriteAllText(Path.Combine(data,"floating-auto-refresh-test.txt"),failure==null?
            "PASS: real isolated parent/child tasks and shared outstanding; both layouts share direct leaves and accept in-flight reads across switches; unchanged background load in both layouts retains task and leaf instances, selection, expansion, scroll, bounds, enabled state and zero invalidation; file bursts debounce to one silent read; idle polling makes no data request; committed task and outstanding edits are detected through persistent read-only SQLite data_version plus filesystem hints and apply; busy/hidden events persist until resumed; changes during reads keep a pending revision; fixtures removed.":"FAIL: "+failure);
        if(failure!=null)throw new Exception("Auto refresh self-test failed",failure);
    }
}
