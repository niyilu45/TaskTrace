// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Drawing;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    Label searchCountLabel;
    int searchMatchedCount,searchTotalCount;
    bool settingTaskSummary,filterDataReady;
    long filterProjectId;
    string filterSnapshotContext;
    List<long> filterTaskIds=new List<long>();
    TaskOrderState filterOrder=new TaskOrderState();
    Dictionary<long,SharedList> filterSharedLists=new Dictionary<long,SharedList>();
    readonly Timer searchFilterTimer=new Timer{Interval=150};
    TableLayoutPanel CreateFilterRow() {
        var row=new TableLayoutPanel{Dock=DockStyle.Fill,ColumnCount=3,RowCount=1,AutoSize=true,AutoSizeMode=AutoSizeMode.GrowAndShrink};
        row.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));row.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,100));row.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
        row.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        var label=new Label{Text="筛选：",AutoSize=true,Anchor=AnchorStyles.Left,Margin=new Padding(0,0,3,0)};
        searchCountLabel=new Label{AutoSize=true,Anchor=AnchorStyles.Left,Margin=new Padding(6,0,0,0),AccessibleName="筛选后的事项数"};
        var input=new FloatingFilterInput(search){Dock=DockStyle.Fill,BorderStyle=BorderStyle.FixedSingle,Padding=new Padding(4,3,2,3),Margin=Padding.Empty};
        var clear=new FloatingFilterClearButton{Dock=DockStyle.Right,Width=22,Visible=search.Text.Length>0};
        search.BorderStyle=BorderStyle.None;search.AccessibleName=search.AccessibleDescription="筛选事项";
        input.Controls.Add(search);input.Controls.Add(clear);
        search.TextChanged+=delegate {clear.Visible=search.Text.Length>0;searchFilterTimer.Stop();if(!rendering && !selfTest)searchFilterTimer.Start();};
        clear.Click+=async delegate {search.Clear();search.Focus();await ApplySearchFilter();};
        search.FontChanged+=delegate {row.PerformLayout();};
        searchFilterTimer.Tick+=async delegate {await ApplySearchFilter();};
        Disposed+=delegate {searchFilterTimer.Dispose();};
        row.Controls.Add(label,0,0);row.Controls.Add(input,1,0);row.Controls.Add(searchCountLabel,2,0);
        UpdateSearchCount(searchMatchedCount,searchTotalCount);return row;
    }
    async Task ApplySearchFilter() {
        searchFilterTimer.Stop();if(closing || IsDisposed)return;
        if(busy){searchFilterTimer.Start();return;}
        try {page=1;await LoadTasks(false,true);}
        catch(Exception error){Error(error);}
    }
    void UpdateSearchCount(int matched,int count) {
        searchMatchedCount=matched;searchTotalCount=count;
        if(searchCountLabel==null)return;
        string text=matched+"项";if(searchCountLabel.Text!=text)searchCountLabel.Text=text;
    }
    void InitializeStatusRow() {
        SetStatusRowVisible(false);
        status.TextChanged+=delegate {if(!settingTaskSummary)SetStatusRowVisible(!String.IsNullOrWhiteSpace(status.Text));};
    }
    void SetStatusRowVisible(bool visible) {
        status.Visible=visible;
        if(content.RowStyles.Count>5)content.RowStyles[5].Height=visible?Math.Max(28,Font.Height+10):0;
    }
    void SetTaskSummary(string text) {
        settingTaskSummary=true;
        try {status.Text=text;SetStatusRowVisible(false);}
        finally {settingTaskSummary=false;}
    }
}
internal sealed class FloatingFilterInput : Panel {
    readonly TextBox input;
    internal FloatingFilterInput(TextBox input){this.input=input;AutoSize=true;AutoSizeMode=AutoSizeMode.GrowAndShrink;Size=GetPreferredSize(Size.Empty);}
    public override Size GetPreferredSize(Size proposedSize){return new Size(0,input.PreferredHeight+Padding.Vertical+2);}
}
internal sealed class FloatingFilterClearButton : Button {
    internal FloatingFilterClearButton(){Text="";AccessibleName="清除筛选";FlatStyle=FlatStyle.Flat;FlatAppearance.BorderSize=0;TabStop=true;Margin=Padding.Empty;}
    protected override void OnPaint(PaintEventArgs e) {
        base.OnPaint(e);int radius=Math.Max(3,Math.Min(ClientSize.Width,ClientSize.Height)/5),x=ClientSize.Width/2,y=ClientSize.Height/2;
        using(var pen=new Pen(ForeColor,1.5f)){e.Graphics.DrawLine(pen,x-radius,y-radius,x+radius,y+radius);e.Graphics.DrawLine(pen,x-radius,y+radius,x+radius,y-radius);}
    }
}
