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
    void SaveLocalAppearance(){
        try{
            Directory.CreateDirectory(data);string path=AppearancePath,temp=path+".tmp";
            File.WriteAllText(temp,json.Serialize(new {tasks=localTaskColors.ToDictionary(pair=>pair.Key.ToString(),pair=>pair.Value),outstanding=localOutstandingColors}));
            if(File.Exists(path))File.Replace(temp,path,null);else File.Move(temp,path);
        }catch{}
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
        foreach(var node in SimpleTaskNodes(tasks.Nodes)){
            var task=node as TaskNode;if(task!=null){long id=(long)task.Tag;task.LocalColorKey=LocalTaskAppearance(id);task.EffectiveColorKey=EffectiveTaskAppearance(id,taskParents);}
            foreach(TreeNode child in node.Nodes){var leaf=child.Tag as OutstandingLeaf;if(leaf!=null){leaf.LocalColorKey=LocalOutstandingAppearance(leaf.TaskId,leaf.Id);var owner=node as TaskNode;leaf.EffectiveColorKey=leaf.LocalColorKey!=""?leaf.LocalColorKey:(owner==null?"":owner.EffectiveColorKey);}}
        }
        taskSurface.Rebuild(true);tasks.Invalidate();
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
