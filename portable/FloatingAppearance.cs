// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    sealed class AppearanceChoice {
        public string Key,Label;
        public Color Color;
        public override string ToString(){return Label;}
    }
    static readonly AppearanceChoice[] AppearanceChoices={
        new AppearanceChoice{Key="",Label="继承 / 无背景",Color=Color.Empty},
        new AppearanceChoice{Key="mist-blue",Label="雾蓝",Color=Color.FromArgb(226,238,255)},
        new AppearanceChoice{Key="mint",Label="薄荷",Color=Color.FromArgb(226,246,237)},
        new AppearanceChoice{Key="warm-sand",Label="暖沙",Color=Color.FromArgb(255,242,207)},
        new AppearanceChoice{Key="soft-rose",Label="柔粉",Color=Color.FromArgb(255,231,236)},
        new AppearanceChoice{Key="lavender",Label="浅紫",Color=Color.FromArgb(240,234,252)},
        new AppearanceChoice{Key="cloud",Label="云灰",Color=Color.FromArgb(234,240,245)}
    };
    readonly Dictionary<long,string> localTaskColors=new Dictionary<long,string>();
    readonly Dictionary<string,string> localOutstandingColors=new Dictionary<string,string>(StringComparer.Ordinal);
    string AppearancePath {get{return Path.Combine(data,"floating-appearance.json");}}
    static string OutstandingAppearanceKey(long taskId,string itemId){return taskId+":"+(itemId??"");}
    static string NormalizeAppearance(string key){return AppearanceChoices.Any(choice=>choice.Key==key)?key:"";}
    internal static Color AppearanceColor(string key){var choice=AppearanceChoices.FirstOrDefault(item=>item.Key==key);return choice==null?Color.Empty:choice.Color;}
    void LoadLocalAppearance(){
        localTaskColors.Clear();localOutstandingColors.Clear();
        try{
            if(!File.Exists(AppearancePath))return;var root=ReadObject(File.ReadAllText(AppearancePath));object raw;
            if(root.TryGetValue("tasks",out raw)){var values=raw as Dictionary<string,object>;if(values!=null)foreach(var pair in values){long id;string key=NormalizeAppearance(Convert.ToString(pair.Value));if(Int64.TryParse(pair.Key,out id)&&key!="")localTaskColors[id]=key;}}
            if(root.TryGetValue("outstanding",out raw)){var values=raw as Dictionary<string,object>;if(values!=null)foreach(var pair in values){string key=NormalizeAppearance(Convert.ToString(pair.Value));if(key!="")localOutstandingColors[pair.Key]=key;}}
        }catch{}
    }
    void SaveLocalAppearance(bool reportFailure=false){
        try{
            Directory.CreateDirectory(data);string path=AppearancePath,temp=path+".tmp";
            File.WriteAllText(temp,json.Serialize(new {tasks=localTaskColors.ToDictionary(pair=>pair.Key.ToString(),pair=>pair.Value),outstanding=localOutstandingColors}));
            if(File.Exists(path))File.Replace(temp,path,null);else File.Move(temp,path);
        }catch{if(reportFailure)throw;}
    }
    string LocalTaskAppearance(long id){string key;return localTaskColors.TryGetValue(id,out key)?NormalizeAppearance(key):"";}
    string LocalOutstandingAppearance(long taskId,string itemId){string key;return localOutstandingColors.TryGetValue(OutstandingAppearanceKey(taskId,itemId),out key)?NormalizeAppearance(key):"";}
    string EffectiveTaskAppearance(long id,Dictionary<long,long> parents){
        var seen=new HashSet<long>();while(id>0&&seen.Add(id)){string key=LocalTaskAppearance(id);if(key!="")return key;if(parents==null||!parents.TryGetValue(id,out id))break;}return "";
    }
    void SetLocalTaskAppearance(long id,string key){key=NormalizeAppearance(key);if(key=="")localTaskColors.Remove(id);else localTaskColors[id]=key;SaveLocalAppearance();ApplyLocalAppearanceToTree();}
    void SetLocalOutstandingAppearance(long taskId,string itemId,string key){string mapKey=OutstandingAppearanceKey(taskId,itemId);key=NormalizeAppearance(key);if(key=="")localOutstandingColors.Remove(mapKey);else localOutstandingColors[mapKey]=key;SaveLocalAppearance();ApplyLocalAppearanceToTree();}
    void RemoveLocalOutstandingAppearance(long taskId,string itemId){if(localOutstandingColors.Remove(OutstandingAppearanceKey(taskId,itemId))){SaveLocalAppearance();ApplyLocalAppearanceToTree();}}
    void ApplyLocalAppearance(Dictionary<long,TreeNode> nodes,Dictionary<long,long> parents){
        foreach(var pair in nodes){var node=pair.Value as TaskNode;if(node==null)continue;node.LocalColorKey=LocalTaskAppearance(pair.Key);node.EffectiveColorKey=EffectiveTaskAppearance(pair.Key,parents);}
    }
    void ApplyLocalAppearanceToTree(){
        foreach(var node in AppearanceNodes(tasks.Nodes)){
            var task=node as TaskNode;if(task!=null){long id=(long)task.Tag;task.LocalColorKey=LocalTaskAppearance(id);task.EffectiveColorKey=EffectiveTaskAppearance(id,taskParents);}
            var leaf=node.Tag as OutstandingLeaf;if(leaf!=null){leaf.LocalColorKey=LocalOutstandingAppearance(leaf.TaskId,leaf.Id);leaf.EffectiveColorKey=leaf.LocalColorKey!=""?leaf.LocalColorKey:EffectiveTaskAppearance(leaf.TaskId,taskParents);}
        }
        taskSurface.Rebuild(true);tasks.Invalidate();
    }
    static IEnumerable<TreeNode> AppearanceNodes(TreeNodeCollection nodes){
        foreach(TreeNode node in nodes){yield return node;foreach(var child in AppearanceNodes(node.Nodes))yield return child;}
    }
    ToolStripMenuItem CreateAppearanceMenuItem(){
        var menu=new ToolStripMenuItem("背景色（仅本机）");
        foreach(var source in AppearanceChoices){
            var choice=source;
            var item=new ToolStripMenuItem(choice.Label){Tag=choice.Key,Image=AppearanceSwatch(choice.Color),ImageScaling=ToolStripItemImageScaling.None};
            item.Click+=delegate{ApplySelectedAppearance(choice.Key);};
            menu.DropDownItems.Add(item);
        }
        menu.DropDownOpening+=delegate{RefreshAppearanceMenu(menu);};
        return menu;
    }
    void RefreshAppearanceMenu(ToolStripMenuItem menu){
        var selection=SelectedActionNodes();menu.Enabled=selection.Count>0;menu.Text=selection.Count>1?"批量设置背景色（仅本机）":"背景色（仅本机）";if(!menu.Enabled)return;
        var node=selection[0];var leaf=node.Tag as OutstandingLeaf;bool task=node.Tag is long;
        var colors=selection.Select(item=>item.Tag is long?LocalTaskAppearance((long)item.Tag):LocalOutstandingAppearance(((OutstandingLeaf)item.Tag).TaskId,((OutstandingLeaf)item.Tag).Id)).Distinct().ToList();
        string selected=colors.Count==1?colors[0]:null;
        bool inherits=leaf!=null||(task&&taskParents.ContainsKey((long)node.Tag));
        for(int index=0;index<menu.DropDownItems.Count;index++){
            var item=menu.DropDownItems[index] as ToolStripMenuItem;if(item==null)continue;
            string key=Convert.ToString(item.Tag);item.Checked=key==selected;
            if(index==0)item.Text=selection.Count>1?"继承 / 无背景":leaf!=null?"继承所属任务":inherits?"继承父任务":"无背景";
        }
    }
    void ApplySelectedAppearance(string key){
        var selection=SelectedActionNodes();if(selection.Count==0)return;key=NormalizeAppearance(key);
        var beforeTasks=new Dictionary<long,string>(localTaskColors);var beforeOutstanding=new Dictionary<string,string>(localOutstandingColors);
        try {
            foreach(var node in selection){
                var leaf=node.Tag as OutstandingLeaf;
                if(node.Tag is long){long id=(long)node.Tag;if(key=="")localTaskColors.Remove(id);else localTaskColors[id]=key;}
                else if(leaf!=null){string id=OutstandingAppearanceKey(leaf.TaskId,leaf.Id);if(key=="")localOutstandingColors.Remove(id);else localOutstandingColors[id]=key;}
            }
            SaveLocalAppearance(true);ApplyLocalAppearanceToTree();
            status.Text="已设置 "+selection.Count+" 项背景色，仅保存在本机，不会同步给协作成员。";
        }catch(Exception error){
            localTaskColors.Clear();foreach(var pair in beforeTasks)localTaskColors[pair.Key]=pair.Value;
            localOutstandingColors.Clear();foreach(var pair in beforeOutstanding)localOutstandingColors[pair.Key]=pair.Value;
            Error(error);if(!selfTest)MessageBox.Show(this,"背景色未保存："+error.Message,"背景色",MessageBoxButtons.OK,MessageBoxIcon.Warning);
        }
    }
    static Bitmap AppearanceSwatch(Color color){
        var image=new Bitmap(18,18);using(var graphics=Graphics.FromImage(image)){
            graphics.Clear(Color.Transparent);var bounds=new Rectangle(2,2,13,13);
            using(var brush=new SolidBrush(color.IsEmpty?Color.White:color))graphics.FillRectangle(brush,bounds);
            using(var pen=new Pen(Color.FromArgb(145,156,172)))graphics.DrawRectangle(pen,bounds);
            if(color.IsEmpty)using(var pen=new Pen(Color.FromArgb(185,91,91),1.5f))graphics.DrawLine(pen,bounds.Left+2,bounds.Bottom-2,bounds.Right-2,bounds.Top+2);
        }return image;
    }
    ComboBox AppearancePicker(bool canInherit){
        var picker=new ComboBox{Dock=DockStyle.Fill,DropDownStyle=ComboBoxStyle.DropDownList,DrawMode=DrawMode.OwnerDrawFixed,ItemHeight=24,AccessibleName="背景色，仅保存在本机"};
        foreach(var source in AppearanceChoices){picker.Items.Add(new AppearanceChoice{Key=source.Key,Label=source.Key==""?(canInherit?"继承父任务":"无背景"):source.Label,Color=source.Color});}
        picker.DrawItem+=delegate(object sender,DrawItemEventArgs e){
            if(e.Index<0)return;var choice=(AppearanceChoice)picker.Items[e.Index];e.DrawBackground();var swatch=new Rectangle(e.Bounds.Left+7,e.Bounds.Top+5,14,14);
            using(var brush=new SolidBrush(choice.Color.IsEmpty?Color.White:choice.Color))e.Graphics.FillRectangle(brush,swatch);
            using(var pen=new Pen(choice.Color.IsEmpty?Color.FromArgb(170,178,190):Color.FromArgb(150,160,174)))e.Graphics.DrawRectangle(pen,swatch);
            TextRenderer.DrawText(e.Graphics,choice.Label,picker.Font,new Rectangle(swatch.Right+8,e.Bounds.Top,e.Bounds.Width-swatch.Right-12,e.Bounds.Height),e.ForeColor,TextFormatFlags.VerticalCenter|TextFormatFlags.EndEllipsis|TextFormatFlags.NoPrefix);e.DrawFocusRectangle();
        };
        picker.SelectedIndex=0;return picker;
    }
    static string SelectedAppearance(ComboBox picker){var choice=picker.SelectedItem as AppearanceChoice;return choice==null?"":choice.Key;}
    static void SelectAppearance(ComboBox picker,string key){key=NormalizeAppearance(key);for(int i=0;i<picker.Items.Count;i++){var choice=picker.Items[i] as AppearanceChoice;if(choice!=null&&choice.Key==key){picker.SelectedIndex=i;return;}}picker.SelectedIndex=0;}
}
