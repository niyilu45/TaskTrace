// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Drawing;
using System.Linq;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    enum EdgeDock { None, Left, Right, Top, Bottom }

    readonly Timer edgeHideTimer = new Timer { Interval = 120 };
    ToolStripMenuItem edgeHideMenu;
    bool edgeHideEnabled, edgeHidden, edgeSizing;
    EdgeDock edgeDock;
    Rectangle edgeRestoreBounds, edgeWorkingArea;
    DateTime edgePointerLeftUtc = DateTime.MinValue;
    const int EdgeSnapTolerance = 8;
    const int EdgeRevealStrip = 2;
    const int EdgeTriggerThickness = 5;

    void InitializeEdgeHide(ContextMenuStrip trayMenu) {
        trayMenu.Items.Add(new ToolStripSeparator());
        trayMenu.Items.Add("重置悬浮窗位置", null, delegate { ResetFloatingWindowPosition(); });
        edgeHideMenu = new ToolStripMenuItem("边缘隐藏") { CheckOnClick = true, Checked = edgeHideEnabled };
        edgeHideMenu.CheckedChanged += delegate { SetEdgeHideEnabled(edgeHideMenu.Checked); };
        trayMenu.Items.Add(edgeHideMenu);
        trayMenu.Items.Add(new ToolStripSeparator());
        edgeHideTimer.Tick += delegate { PollEdgeHide(); };
        if(edgeHideEnabled)edgeHideTimer.Start();
        Shown += delegate { if(edgeHideEnabled)BeginInvoke(new Action(delegate { TryHideAtTouchedEdge(); })); };
    }

    void SetEdgeHideEnabled(bool enabled) {
        edgeHideEnabled = enabled;
        if(enabled) {
            edgeHideTimer.Start();
            BeginInvoke(new Action(delegate { TryHideAtTouchedEdge(); }));
        } else {
            RestoreFromEdge(true);
            edgeHideTimer.Stop();
        }
        SaveBounds();
    }

    void HandleEdgeHideWindowMessage(int message) {
        const int EnterSizeMove = 0x0231, ExitSizeMove = 0x0232;
        if(message == EnterSizeMove) {
            edgeSizing = true;
            RestoreFromEdge(true);
        } else if(message == ExitSizeMove) {
            edgeSizing = false;
            if(edgeHideEnabled)BeginInvoke(new Action(delegate { TryHideAtTouchedEdge(); }));
        }
    }

    EdgeDock TouchedEdge(Rectangle bounds, Rectangle area) {
        int left = Math.Abs(bounds.Left - area.Left), right = Math.Abs(bounds.Right - area.Right);
        int top = Math.Abs(bounds.Top - area.Top), bottom = Math.Abs(bounds.Bottom - area.Bottom);
        int nearest = Math.Min(Math.Min(left, right), Math.Min(top, bottom));
        if(nearest > EdgeSnapTolerance)return EdgeDock.None;
        if(nearest == left)return EdgeDock.Left;
        if(nearest == right)return EdgeDock.Right;
        if(nearest == top)return EdgeDock.Top;
        return EdgeDock.Bottom;
    }

    void TryHideAtTouchedEdge() {
        if(!edgeHideEnabled || edgeSizing || edgeHidden || closing || IsDisposed || !Visible || WindowState != FormWindowState.Normal)return;
        var bounds = Bounds;
        var area = Screen.FromRectangle(bounds).WorkingArea;
        var edge = TouchedEdge(bounds, area);
        if(edge == EdgeDock.None) { edgeDock = EdgeDock.None; edgePointerLeftUtc = DateTime.MinValue; return; }
        if(simpleMode)fullBounds.Location = bounds.Location;
        edgeRestoreBounds = bounds;
        edgeWorkingArea = area;
        edgeDock = edge;
        HideDockedWindow();
    }

    void HideDockedWindow() {
        if(edgeDock == EdgeDock.None || edgeRestoreBounds.IsEmpty || closing || IsDisposed)return;
        var hidden = edgeRestoreBounds;
        if(edgeDock == EdgeDock.Left)hidden.X = edgeWorkingArea.Left - hidden.Width + EdgeRevealStrip;
        else if(edgeDock == EdgeDock.Right)hidden.X = edgeWorkingArea.Right - EdgeRevealStrip;
        else if(edgeDock == EdgeDock.Top)hidden.Y = edgeWorkingArea.Top - hidden.Height + EdgeRevealStrip;
        else hidden.Y = edgeWorkingArea.Bottom - EdgeRevealStrip;
        edgeHidden = true;
        edgePointerLeftUtc = DateTime.MinValue;
        Bounds = hidden;
    }

    Rectangle EdgeTriggerBounds() {
        int left = Math.Max(edgeWorkingArea.Left, Math.Min(edgeRestoreBounds.Left, edgeWorkingArea.Right - 1));
        int top = Math.Max(edgeWorkingArea.Top, Math.Min(edgeRestoreBounds.Top, edgeWorkingArea.Bottom - 1));
        int width = Math.Max(1, Math.Min(edgeRestoreBounds.Width, edgeWorkingArea.Right - left));
        int height = Math.Max(1, Math.Min(edgeRestoreBounds.Height, edgeWorkingArea.Bottom - top));
        if(edgeDock == EdgeDock.Left)return new Rectangle(edgeWorkingArea.Left, top, EdgeTriggerThickness, height);
        if(edgeDock == EdgeDock.Right)return new Rectangle(edgeWorkingArea.Right - EdgeTriggerThickness, top, EdgeTriggerThickness, height);
        if(edgeDock == EdgeDock.Top)return new Rectangle(left, edgeWorkingArea.Top, width, EdgeTriggerThickness);
        if(edgeDock == EdgeDock.Bottom)return new Rectangle(left, edgeWorkingArea.Bottom - EdgeTriggerThickness, width, EdgeTriggerThickness);
        return Rectangle.Empty;
    }

    void PollEdgeHide() {
        if(!edgeHideEnabled || edgeDock == EdgeDock.None || closing || IsDisposed || !Visible || WindowState != FormWindowState.Normal || edgeSizing)return;
        var pointer = Cursor.Position;
        if(edgeHidden) {
            if(EdgeTriggerBounds().Contains(pointer))RestoreFromEdge(false);
            return;
        }
        bool interacting = Bounds.Contains(pointer) || ContainsFocus || Control.MouseButtons != MouseButtons.None || AutoRefreshInteractionActive();
        if(interacting) { edgePointerLeftUtc = DateTime.MinValue; return; }
        if(edgePointerLeftUtc == DateTime.MinValue) { edgePointerLeftUtc = DateTime.UtcNow; return; }
        if((DateTime.UtcNow - edgePointerLeftUtc).TotalMilliseconds >= 500)HideDockedWindow();
    }

    void RestoreFromEdge(bool clearDock) {
        if(edgeHidden && !edgeRestoreBounds.IsEmpty) {
            Bounds = edgeRestoreBounds;
            edgeHidden = false;
        }
        edgePointerLeftUtc = DateTime.MinValue;
        if(clearDock)edgeDock = EdgeDock.None;
    }

    void ResetFloatingWindowPosition() {
        RestoreFromEdge(true);
        WindowState = FormWindowState.Normal;
        var screen = Screen.FromRectangle(Bounds);
        var area = screen.WorkingArea;
        int x = area.Left + Math.Max(0, (area.Width - Width) / 2);
        int y = area.Top + Math.Max(0, (area.Height - Height) / 2);
        Location = new Point(x, y);
        if(simpleMode) {
            int fullX = area.Left + Math.Max(0, (area.Width - fullBounds.Width) / 2);
            int fullY = area.Top + Math.Max(0, (area.Height - fullBounds.Height) / 2);
            fullBounds.Location = new Point(fullX, fullY);
        }
        if(!Visible)Show();
        Activate();
        SaveBounds();
    }

    void TestEdgeHideBehavior() {
        var originalBounds = Bounds;
        var originalPointer = Cursor.Position;
        bool originalEnabled = edgeHideEnabled;
        var area = Screen.FromRectangle(originalBounds).WorkingArea;
        try {
            if(edgeHideMenu == null || edgeHideMenu.Text != "边缘隐藏" || !edgeHideMenu.CheckOnClick || tray.ContextMenuStrip == null || !tray.ContextMenuStrip.Items.Cast<ToolStripItem>().Any(item => item.Text == "重置悬浮窗位置") || !tray.ContextMenuStrip.Items.Cast<ToolStripItem>().Any(item => item.Text == "打开程序路径"))throw new Exception("Tray edge-hide, reset-position, or open-program-path menu item is missing");
            edgeHideEnabled = true;
            Bounds = new Rectangle(area.Left, area.Top + Math.Min(80, Math.Max(0, area.Height - Height)), Width, Height);
            TryHideAtTouchedEdge();
            if(!edgeHidden || edgeDock != EdgeDock.Left || Bounds.Right != area.Left + EdgeRevealStrip)throw new Exception("Edge hide did not move a docked window outside the working area");
            var trigger = EdgeTriggerBounds();
            if(trigger.Width != EdgeTriggerThickness || !trigger.Contains(new Point(area.Left, edgeRestoreBounds.Top)))throw new Exception("Edge restore trigger does not match the hidden window segment");
            Cursor.Position = new Point(trigger.Left + trigger.Width / 2, trigger.Top + trigger.Height / 2);
            PollEdgeHide();
            if(edgeHidden || Bounds != edgeRestoreBounds)throw new Exception("Edge hover restore did not recover the original bounds");
            ResetFloatingWindowPosition();
            var centered = Bounds;
            if(Math.Abs((centered.Left + centered.Width / 2) - (area.Left + area.Width / 2)) > 1 || Math.Abs((centered.Top + centered.Height / 2) - (area.Top + area.Height / 2)) > 1)throw new Exception("Reset window position did not center the floating window");
            SaveBounds();
            var saved = ReadObject(System.IO.File.ReadAllText(System.IO.Path.Combine(data, "floating-window.json")));
            if(!saved.ContainsKey("edgeHide") || !Convert.ToBoolean(saved["edgeHide"]))throw new Exception("Edge-hide preference was not persisted");
        } finally {
            Cursor.Position = originalPointer;
            RestoreFromEdge(true);
            edgeHideEnabled = originalEnabled;
            if(edgeHideMenu != null)edgeHideMenu.Checked = originalEnabled;
            Bounds = originalBounds;
            if(originalEnabled)edgeHideTimer.Start();else edgeHideTimer.Stop();
        }
    }

    void DisposeEdgeHide() { edgeHideTimer.Stop(); edgeHideTimer.Dispose(); }
}
