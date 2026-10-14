// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;
internal sealed partial class FloatingWindow {
    async Task ExportMarkdown(long taskId,string taskTitle,Form owner) {
        using(var dialog=DpiDialog(new Form{Text="导出任务 · "+taskTitle,ClientSize=new Size(470,170),MinimumSize=new Size(440,200),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent})) {
            var label=new Label{Dock=DockStyle.Fill,Padding=new Padding(14),Text="导出已保存的任务描述、每日进展、遗留事项和可读取的子任务。图片内嵌在 Markdown 文件中。"};
            var row=new FlowLayoutPanel{Dock=DockStyle.Bottom,Height=45,Padding=new Padding(8),FlowDirection=FlowDirection.RightToLeft};
            var save=new Button{Text="导出",AutoSize=true};var preview=new Button{Text="预览",AutoSize=true};var cancel=new Button{Text="取消",AutoSize=true};row.Controls.Add(save);row.Controls.Add(preview);row.Controls.Add(cancel);dialog.Controls.Add(label);dialog.Controls.Add(row);cancel.Click+=delegate{dialog.Close();};
            save.Click+=async delegate{
                using(var picker=new SaveFileDialog{Title="导出 Markdown",Filter="Markdown 文档|*.md",FileName=SafeMarkdownName(taskTitle),DefaultExt="md",AddExtension=true,OverwritePrompt=true}) {
                    if(picker.ShowDialog(dialog)!=DialogResult.OK)return;
                    save.Enabled=preview.Enabled=false;label.Text="正在导出…";
                    try {var result=await Api("GET","/tasks/"+taskId+"/markdown",null);File.WriteAllText(picker.FileName,Convert.ToString(result["markdown"]),new UTF8Encoding(false));label.Text="已保存："+picker.FileName;if(result.ContainsKey("warnings")){var warnings=result["warnings"] as System.Collections.IEnumerable;if(warnings!=null)foreach(var warning in warnings)label.Text+="\n"+Convert.ToString(warning);}}
                    catch(Exception error){label.Text="导出失败："+error.Message;}
                    finally{if(!dialog.IsDisposed)save.Enabled=preview.Enabled=true;}
                }
            };
            preview.Click+=async delegate{preview.Enabled=false;try{string session=await PrepareBrowserSession();Process.Start(new ProcessStartInfo(url+"/tasks/"+taskId+"/markdown#tasktrace-local="+Uri.EscapeDataString(session)){UseShellExecute=true});}catch(Exception error){label.Text="预览打开失败："+error.Message;}finally{if(!dialog.IsDisposed)preview.Enabled=true;}};
            await ShowEditorWindowAsync(dialog,owner);
        }
    }
    static string SafeMarkdownName(string title) {foreach(char invalid in Path.GetInvalidFileNameChars())title=title.Replace(invalid,'_');title=title.Trim().TrimEnd('.');if(title.Length>100)title=title.Substring(0,100);if(title.Length==0)title="任务";if(System.Text.RegularExpressions.Regex.IsMatch(title,@"^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(?:\.|$)",System.Text.RegularExpressions.RegexOptions.IgnoreCase))title="_"+title;return title+".md";}
}
