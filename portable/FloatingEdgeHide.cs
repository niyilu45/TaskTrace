// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Drawing;
using System.Linq;
using System.Runtime.InteropServices;
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

    [StructLayout(LayoutKind.Sequential)]
    struct EdgeFrameRect { internal int Left, Top, Right, Bottom; }
    [DllImport("dwmapi.dll")]
    static extern int DwmGetWindowAttribute(IntPtr window, int attribute, out EdgeFrameRect value, int size);

    Padding EdgeFrameInsets() {
        if(FormBorderStyle==FormBorderStyle.None || !IsHandleCreated)return Padding.Empty;
        try {
            EdgeFrameRect frame;
            // Win11 resize borders extend beyond the visible frame, especially at high DPI.
            if(DwmGetWindowAttribute(Handle,9,out frame,Marshal.SizeOf(typeof(EdgeFrameRect)))==0 && frame.Right>frame.Left && frame.Bottom>frame.Top) {
                var outer=Bounds;
                return new Padding(Math.Max(0,frame.Left-outer.Left),Math.Max(0,frame.Top-outer.Top),Math.Max(0,outer.Right-frame.Right),Math.Max(0,outer.Bottom-frame.Bottom));
            }
        } catch(DllNotFoundException) { } catch(EntryPointNotFoundException) { }
        return Padding.Empty;
    }

    static Rectangle VisibleEdgeBounds(Rectangle bounds, Padding frame) {
        return new Rectangle(bounds.Left+frame.Left,bounds.Top+frame.Top,Math.Max(1,bounds.Width-frame.Horizontal),Math.Max(1,bounds.Height-frame.Vertical));
    }

    EdgeDock TouchedEdge(Rectangle bounds, Rectangle area) {
        return TouchedFrameEdge(bounds,area,EdgeFrameInsets());
    }

    static EdgeDock TouchedFrameEdge(Rectangle bounds, Rectangle area, Padding frame) {
        var edge=TouchedVisibleEdge(VisibleEdgeBounds(bounds,frame),area);
        // Saved outer bounds can be clamped to the work area when the app restarts.
        return edge==EdgeDock.None?TouchedVisibleEdge(bounds,area):edge;
    }

    static EdgeDock TouchedVisibleEdge(Rectangle bounds, Rectangle area) {
        int left = Math.Abs(bounds.Left - area.Left), right = Math.Abs(bounds.Right - area.Right);
        int top = Math.Abs(bounds.Top - area.Top), bottom = Math.Abs(bounds.Bottom - area.Bottom);
        int nearest = Math.Min(Math.Min(left, right), Math.Min(top, bottom));
        if(nearest > EdgeSnapTolerance)return EdgeDock.None;
        if(nearest == left)return EdgeDock.Left;
        if(nearest == right)return EdgeDock.Right;
        if(nearest == top)return EdgeDock.Top;
        return EdgeDock.Bottom;
    }

    Rectangle AlignBoundsToEdge(Rectangle bounds, Rectangle area, EdgeDock edge) {
        return AlignBoundsToEdge(bounds,area,edge,EdgeFrameInsets());
    }

    static Rectangle AlignBoundsToEdge(Rectangle bounds, Rectangle area, EdgeDock edge, Padding frame) {
        if(edge == EdgeDock.Left)bounds.X = area.Left-frame.Left;
        else if(edge == EdgeDock.Right)bounds.X = area.Right - bounds.Width+frame.Right;
        else if(edge == EdgeDock.Top)bounds.Y = area.Top-frame.Top;
        else if(edge == EdgeDock.Bottom)bounds.Y = area.Bottom - bounds.Height+frame.Bottom;
        if(edge == EdgeDock.Left || edge == EdgeDock.Right)
            bounds.Y = Math.Max(area.Top, Math.Min(bounds.Y, area.Bottom - bounds.Height));
        else if(edge == EdgeDock.Top || edge == EdgeDock.Bottom)
            bounds.X = Math.Max(area.Left, Math.Min(bounds.X, area.Right - bounds.Width));
        return bounds;
    }

    void SyncFullBoundsToSimpleEdge(Rectangle simpleBounds, Rectangle area, EdgeDock edge) {
        if(!simpleMode || fullBounds.IsEmpty)return;
        var bounds = fullBounds;
        if(edge == EdgeDock.Left || edge == EdgeDock.Right)bounds.Y = simpleBounds.Y;
        else if(edge == EdgeDock.Top || edge == EdgeDock.Bottom)bounds.X = simpleBounds.X;
        else bounds.Location = simpleBounds.Location;
        fullBounds = edge == EdgeDock.None ? bounds : AlignBoundsToEdge(bounds, area, edge);
    }

    void ArmEdgeHideAfterLayoutChange(EdgeDock preferredEdge, Rectangle preferredArea) {
        if(!edgeHideEnabled || edgeSizing || closing || IsDisposed || !Visible || WindowState != FormWindowState.Normal)return;
        var area = preferredArea.IsEmpty ? Screen.FromRectangle(Bounds).WorkingArea : preferredArea;
        var bounds = Bounds;
        if(preferredEdge != EdgeDock.None) {
            bounds = AlignBoundsToEdge(bounds, area, preferredEdge);
            if(Bounds != bounds)Bounds = bounds;
        }
        var edge = preferredEdge == EdgeDock.None ? TouchedEdge(bounds, area) : preferredEdge;
        if(edge == EdgeDock.None) {
            edgeDock = EdgeDock.None;
            edgePointerLeftUtc = DateTime.MinValue;
            return;
        }
        SyncFullBoundsToSimpleEdge(bounds, area, edge);
        edgeRestoreBounds = bounds;
        edgeWorkingArea = area;
        edgeDock = edge;
        edgeHidden = false;
        edgePointerLeftUtc = Bounds.Contains(Cursor.Position) ? DateTime.MinValue : DateTime.UtcNow;
        edgeHideTimer.Start();
    }

    void RearmEdgeHideAfterLayoutChange(EdgeDock preferredEdge, Rectangle preferredArea) {
        if(!edgeHideEnabled || closing || IsDisposed)return;
        BeginInvoke(new Action(delegate { ArmEdgeHideAfterLayoutChange(preferredEdge, preferredArea); }));
    }

    void TryHideAtTouchedEdge() {
        if(!edgeHideEnabled || edgeSizing || edgeHidden || closing || IsDisposed || !Visible || WindowState != FormWindowState.Normal)return;
        var bounds = Bounds;
        var area = Screen.FromRectangle(bounds).WorkingArea;
        var edge = TouchedEdge(bounds, area);
        if(edge == EdgeDock.None) { edgeDock = EdgeDock.None; edgePointerLeftUtc = DateTime.MinValue; return; }
        ArmEdgeHideAfterLayoutChange(edge,area);
        edgePointerLeftUtc = DateTime.MinValue;
    }

    void HideDockedWindow() {
        if(edgeDock == EdgeDock.None || edgeRestoreBounds.IsEmpty || closing || IsDisposed)return;
        var hidden = HiddenEdgeBounds(edgeRestoreBounds,edgeWorkingArea,edgeDock,EdgeFrameInsets());
        edgeHidden = true;
        edgePointerLeftUtc = DateTime.MinValue;
        Bounds = hidden;
    }

    static Rectangle HiddenEdgeBounds(Rectangle bounds, Rectangle area, EdgeDock edge, Padding frame) {
        if(edge==EdgeDock.Left)bounds.X=area.Left-bounds.Width+EdgeRevealStrip+frame.Right;
        else if(edge==EdgeDock.Right)bounds.X=area.Right-EdgeRevealStrip-frame.Left;
        else if(edge==EdgeDock.Top)bounds.Y=area.Top-bounds.Height+EdgeRevealStrip+frame.Bottom;
        else if(edge==EdgeDock.Bottom)bounds.Y=area.Bottom-EdgeRevealStrip-frame.Top;
        return bounds;
    }

    bool EdgeHideInteractionActive() {
        if(!Enabled || dragging || AutoRefreshInteractionActive() || OwnedForms.Any(form=>form.Visible))return true;
        if(projects.DroppedDown || entryPriority.DroppedDown)return true;
        // Clicking a task or a toolbar button leaves focus in the full window. That alone
        // must not pin it open after the pointer leaves; focused text entry still stays open.
        return entry.ContainsFocus || search.ContainsFocus;
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

    bool CanPollEdgeHide() {
        return edgeHideEnabled && edgeDock!=EdgeDock.None && !closing && !IsDisposed && Visible && WindowState==FormWindowState.Normal && !edgeSizing;
    }

    void PollEdgeHide() {
        if(!CanPollEdgeHide())return;
        PollEdgeHide(Cursor.Position,DateTime.UtcNow,!edgeHidden && EdgeHideInteractionActive());
    }

    void PollEdgeHide(Point pointer, DateTime now, bool interactionActive) {
        if(!CanPollEdgeHide())return;
        if(edgeHidden) {
            if(EdgeTriggerBounds().Contains(pointer))RestoreFromEdge(false);
            return;
        }
        bool interacting = Bounds.Contains(pointer) || interactionActive;
        if(interacting) { edgePointerLeftUtc = DateTime.MinValue; return; }
        if(edgePointerLeftUtc == DateTime.MinValue) { edgePointerLeftUtc = now; return; }
        if((now - edgePointerLeftUtc).TotalMilliseconds >= 500)HideDockedWindow();
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
            var outside=new Point(Bounds.Right+50,Bounds.Bottom+50);var now=DateTime.UtcNow;
            PollEdgeHide(outside,now,false);PollEdgeHide(outside,now.AddSeconds(1),false);
            if(!edgeHidden || edgeDock != EdgeDock.Left || VisibleEdgeBounds(Bounds,EdgeFrameInsets()).Right != area.Left + EdgeRevealStrip)throw new Exception("Edge hide did not move a docked window outside the working area");
            var trigger = EdgeTriggerBounds();
            if(trigger.Width != EdgeTriggerThickness || !trigger.Contains(new Point(area.Left, edgeRestoreBounds.Top)))throw new Exception("Edge restore trigger does not match the hidden window segment");
            Cursor.Position = new Point(trigger.Left + trigger.Width / 2, trigger.Top + trigger.Height / 2);
            PollEdgeHide();
            if(edgeHidden || Bounds != edgeRestoreBounds)throw new Exception("Edge hover restore did not recover the original bounds");
            Bounds = new Rectangle(area.Right - Width, Bounds.Top, Width, Height);
            var preferredEdge = TouchedEdge(Bounds, area);
            Bounds = new Rectangle(Bounds.Left, Bounds.Top, Math.Max(160, Width / 2), Height);
            ArmEdgeHideAfterLayoutChange(preferredEdge, area);
            if(edgeHidden || edgeDock != EdgeDock.Right || VisibleEdgeBounds(Bounds,EdgeFrameInsets()).Right != area.Right)throw new Exception("Edge hide was not re-armed after a layout size change");
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
