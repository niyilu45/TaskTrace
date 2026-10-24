// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Drawing;
using System.Globalization;
using System.IO;
using System.Linq;
using System.Net;
using System.Text;
using System.Text.RegularExpressions;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    sealed class ProgressReference {
        public string Id, Date, Html;
        public long TaskId;
        public List<long> CommentIds = new List<long>();
        public ProgressReference Copy() { return new ProgressReference {Id=Id,Date=Date,Html=Html,TaskId=TaskId,CommentIds=new List<long>(CommentIds)}; }
        public override string ToString() { string text=Plain(Html).Replace("\r\n"," "); return Date+" · "+(text.Length>65?text.Substring(0,65)+"…":text)+(Regex.IsMatch(Html??"",@"<img\b",RegexOptions.IgnoreCase)?" [图片]":""); }
    }
    sealed class ProgressParts { public string Body; public List<ProgressReference> References = new List<ProgressReference>(); }
    sealed class HtmlRange { public int Start, InnerStart, InnerEnd, End; public string Tag; public bool InsideReference; }
    static readonly Regex ProgressTags = new Regex(@"<!--.*?-->|<(?:(?:""[^""]*"")|(?:'[^']*')|[^'"">])*>",RegexOptions.Singleline);
    static string ReferenceAttribute(string tag,string name) {
        var matches=Regex.Matches(tag,@"(?:\s)"+Regex.Escape(name)+@"\s*=\s*(?:""([^""]*)""|'([^']*)'|([^\s>]+))",RegexOptions.IgnoreCase);
        if(matches.Count!=1)return null;
        var match=matches[0];return WebUtility.HtmlDecode(match.Groups[1].Success?match.Groups[1].Value:match.Groups[2].Success?match.Groups[2].Value:match.Groups[3].Value);
    }
    static List<HtmlRange> ProgressElementRanges(string html,string name) {
        var ranges=new List<HtmlRange>();var stack=new Stack<HtmlRange>();
        foreach(Match token in ProgressTags.Matches(html??"")) {
            var tag=Regex.Match(token.Value,@"^<\s*(/?)\s*"+name+@"\b",RegexOptions.IgnoreCase);if(!tag.Success)continue;
            if(tag.Groups[1].Value=="/") { if(stack.Count==0)continue;var open=stack.Pop();open.InnerEnd=token.Index;open.End=token.Index+token.Length;ranges.Add(open); }
            else if(!token.Value.EndsWith("/>"))stack.Push(new HtmlRange {Start=token.Index,InnerStart=token.Index+token.Length,Tag=token.Value,InsideReference=stack.Any(parent=>ReferenceAttribute(parent.Tag,"data-tasktrace-reference")!=null)});
        }
        return ranges.OrderBy(range=>range.Start).ToList();
    }
    static ProgressParts SplitProgressReferences(string html,long taskId=0) {
        html=html??"";var result=new ProgressParts();var body=new StringBuilder();int cursor=0,outerReferenceEnd=0;var seen=new HashSet<string>();
        foreach(var range in ProgressElementRanges(html,"blockquote")) {
            if(range.InsideReference || range.Start<cursor || range.Start<outerReferenceEnd)continue;
            string marker=ReferenceAttribute(range.Tag,"data-tasktrace-reference");if(marker==null)continue;
            outerReferenceEnd=range.End;if(marker!="1")continue;
            string id=ReferenceAttribute(range.Tag,"data-reference-id"),date=ReferenceAttribute(range.Tag,"data-date"),rawIds=ReferenceAttribute(range.Tag,"data-comment-ids");
            long sourceTask;DateTime parsedDate;
            if(String.IsNullOrWhiteSpace(id) || !Regex.IsMatch(id,@"^r[a-f0-9]{32}$") || !Regex.IsMatch(ReferenceAttribute(range.Tag,"data-task-id")??"",@"^[1-9][0-9]*$") || !Int64.TryParse(ReferenceAttribute(range.Tag,"data-task-id"),out sourceTask) || sourceTask<=0 || sourceTask>9007199254740991L || (taskId>0 && sourceTask!=taskId) || !DateTime.TryParseExact(date,"yyyy-MM-dd",CultureInfo.InvariantCulture,DateTimeStyles.None,out parsedDate) || String.IsNullOrEmpty(rawIds) || !Regex.IsMatch(rawIds,@"^[1-9][0-9]*(?:,[1-9][0-9]*)*$"))continue;
            var ids=new List<long>();bool valid=true;foreach(string value in rawIds.Split(',')){long noteId;if(!Int64.TryParse(value,out noteId)||noteId<=0||noteId>9007199254740991L){valid=false;break;}ids.Add(noteId);}if(!valid || ids.Count==0)continue;
            string inner=html.Substring(range.InnerStart,range.InnerEnd-range.InnerStart);
            var content=ProgressElementRanges(inner,"div").Where(value=>ReferenceAttribute(value.Tag,"data-tasktrace-reference-content")=="1").ToList();
            if(content.Count!=1)continue;
            var snapshot=content[0];var heading=ProgressElementRanges(inner,"p").FirstOrDefault();
            // A damaged wrapper remains ordinary body, including text added outside its snapshot.
            if(heading==null || heading.End>snapshot.Start || inner.Substring(0,heading.Start).Trim()!="" || inner.Substring(heading.End,snapshot.Start-heading.End).Trim()!="" || inner.Substring(snapshot.End).Trim()!="")continue;
            body.Append(html.Substring(cursor,range.Start-cursor));cursor=range.End;
            if(seen.Add(id))result.References.Add(new ProgressReference {Id=id,TaskId=sourceTask,Date=date,CommentIds=ids.Distinct().OrderBy(value=>value).ToList(),Html=inner.Substring(snapshot.InnerStart,snapshot.InnerEnd-snapshot.InnerStart)});
        }
        body.Append(html.Substring(cursor));result.Body=body.ToString();return result;
    }
    static string SerializeProgressReferences(IEnumerable<ProgressReference> references) {
        var seen=new HashSet<string>();var html=new StringBuilder();
        foreach(var item in references??Enumerable.Empty<ProgressReference>()) {
            if(!seen.Add(item.Id))continue;
            string ids=String.Join(",",item.CommentIds.Distinct().OrderBy(value=>value));
            html.Append("<blockquote data-tasktrace-reference=\"1\" data-reference-id=\"").Append(WebUtility.HtmlEncode(item.Id)).Append("\" data-task-id=\"").Append(item.TaskId).Append("\" data-date=\"").Append(WebUtility.HtmlEncode(item.Date)).Append("\" data-comment-ids=\"").Append(ids).Append("\"><p><strong>引用 ").Append(WebUtility.HtmlEncode(item.Date)).Append(" 的进展</strong> <a href=\"/tasks/").Append(item.TaskId).Append("#comment-").Append(item.CommentIds.Max()).Append("\">查看原记录</a></p><div data-tasktrace-reference-content=\"1\">").Append(item.Html).Append("</div></blockquote>");
        }
        return html.ToString();
    }
    static string ProgressTextHtml(string text) { string value=(text??"").Replace("\r\n","\n").Replace("\r","\n");return value==""?"":"<p>"+WebUtility.HtmlEncode(value).Replace("\n","<br>")+"</p>"; }
    static string ProgressDisplayBody(string html) { return Regex.Replace(Regex.Replace(html??"",@"^\s*<h3\b[^>]*>每日进展\s*[·:：]\s*[0-9]{4}-[0-9]{2}-[0-9]{2}</h3>","",RegexOptions.Singleline),@"<p>\s*<strong>遗留问题 / 下一步</strong>\s*</p>\s*<p>.*?</p>","",RegexOptions.Singleline); }
    static string ProgressSnapshotHtml(string html,long taskId) {
        html=SplitProgressReferences(html).Body;
        html=TaskDescriptionEditorHtml(html);
        var output=new StringBuilder();int cursor=0;var attachmentLinks=new Stack<bool>();
        foreach(Match token in ProgressTags.Matches(html)) {
            output.Append(html.Substring(cursor,token.Index-cursor));cursor=token.Index+token.Length;
            var tag=Regex.Match(token.Value,@"^<\s*(/?)\s*([a-z0-9]+)\b",RegexOptions.IgnoreCase);if(!tag.Success)continue;
            bool closing=tag.Groups[1].Value=="/";string name=tag.Groups[2].Value.ToLowerInvariant();
            if(name=="img" && !closing) {
                string src=ReferenceAttribute(token.Value,"data-src")??ReferenceAttribute(token.Value,"src")??"";
                var path=Regex.Match(src,@"^/api/v([12])/tasks/([1-9][0-9]*)/attachments/([1-9][0-9]*)$");long imageTask,imageId;
                if(path.Success && Int64.TryParse(path.Groups[2].Value,out imageTask) && imageTask==taskId && Int64.TryParse(path.Groups[3].Value,out imageId) && imageId<=9007199254740991L)output.Append("<img src=\"").Append(WebUtility.HtmlEncode(src)).Append("\">");
                continue;
            }
            if(name=="a") {
                if(closing){if(attachmentLinks.Count>0 && attachmentLinks.Pop())output.Append("</a>");continue;}
                string href=ReferenceAttribute(token.Value,"href");
                bool allowed=EditorAttachmentHref(href);attachmentLinks.Push(allowed);if(allowed)output.Append("<a href=\"").Append(WebUtility.HtmlEncode(href)).Append("\" download=\"").Append(WebUtility.HtmlEncode(ReferenceAttribute(token.Value,"download")??"附件")).Append("\">");
                continue;
            }
            if(Regex.IsMatch(name,@"^h[1-6]$"))name="div";
            if(!Regex.IsMatch(name,@"^(?:p|div|br|ul|ol|li|blockquote|pre)$"))continue;
            if(name=="br") {if(!closing)output.Append("<br>");continue;}
            output.Append(closing?"</"+name+">":"<"+name+SafeIndentStyle(ReferenceAttribute(token.Value,"style"))+">");
        }
        output.Append(html.Substring(cursor));return output.ToString();
    }
    ProgressReference ReferenceForDay(long taskId,string sourceDay,string targetDay,List<Dictionary<string,object>> history) {
        if(String.CompareOrdinal(sourceDay,targetDay)>=0)throw new Exception("只能引用当前日期之前的进展。");
        var notes=DailyHistory(history).Where(note=>DayOf(note)==sourceDay).OrderBy(note=>Convert.ToInt64(note["id"])).ToList();
        if(notes.Count==0)throw new Exception("该日期没有可引用的进展。");
        string own=String.Join("",notes.Select(note=>ProgressBody((string)note["comment"],taskId)));
        string snapshot=ProgressSnapshotHtml(own,taskId);
        if(String.IsNullOrWhiteSpace(snapshot))throw new Exception("该日期没有可引用的正文或图片。");
        return new ProgressReference {Id="r"+Guid.NewGuid().ToString("N"),TaskId=taskId,Date=sourceDay,Html=snapshot,CommentIds=notes.Select(note=>Convert.ToInt64(note["id"])).Concat(notes.SelectMany(note=>MergedIds((string)note["comment"]))).Distinct().OrderBy(value=>value).ToList()};
    }
    sealed class ProgressCitation { public long Id;public string Date,Html; }
    sealed class ProgressReferenceCandidate { public string Date,Html;public List<ProgressCitation> Citations=new List<ProgressCitation>(); }
    List<ProgressCitation> ProgressCitationsForDay(long taskId,string sourceDay,List<Dictionary<string,object>> history) {
        var sourceIds=new HashSet<long>(DailyHistory(history).Where(note=>DayOf(note)==sourceDay).Select(note=>Convert.ToInt64(note["id"])).Concat(DailyHistory(history).Where(note=>DayOf(note)==sourceDay).SelectMany(note=>MergedIds((string)note["comment"]))));
        var result=new List<ProgressCitation>();var seen=new HashSet<long>();
        foreach(var note in DailyHistory(history)) {
            long id=Convert.ToInt64(note["id"]);var parts=SplitProgressReferences((string)note["comment"],taskId);
            if(parts.References.Any(reference=>reference.CommentIds.Any(sourceIds.Contains)) && seen.Add(id))result.Add(new ProgressCitation {Id=id,Date=DayOf(note),Html=ProgressDisplayBody(parts.Body)});
        }
        return result.OrderByDescending(item=>item.Date).ThenByDescending(item=>item.Id).ToList();
    }
    List<string> SelectProgressReferences(long taskId,string targetDay,List<Dictionary<string,object>> history,IEnumerable<ProgressReference> existing,Form owner,bool verify=false) {
        var used=new HashSet<string>((existing??Enumerable.Empty<ProgressReference>()).Select(item=>item.Date));
        var candidates=DailyHistory(history).Select(note=>DayOf(note)).Distinct().Where(day=>String.CompareOrdinal(day,targetDay)<0 && !used.Contains(day)).OrderByDescending(day=>day).Select(day=>new ProgressReferenceCandidate {
            Date=day,Html=ProgressSnapshotHtml(String.Join("",DailyHistory(history).Where(note=>DayOf(note)==day).OrderBy(note=>Convert.ToInt64(note["id"])).Select(note=>ProgressBody((string)note["comment"],taskId))),taskId),Citations=ProgressCitationsForDay(taskId,day,history)
        }).Where(item=>!String.IsNullOrWhiteSpace(item.Html)).ToList();
        using(var dialog=DpiDialog(new Form {Text="引用历史进展",ClientSize=new Size(760,500),MinimumSize=new Size(560,390),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false})) {
            var layout=new TableLayoutPanel {Dock=DockStyle.Fill,Padding=new Padding(14),ColumnCount=1,RowCount=3};
            layout.RowStyles.Add(new RowStyle(SizeType.Absolute,38));layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,52));
            layout.Controls.Add(new Label {Text="勾选一个或多个历史日期，然后点击确定。",Dock=DockStyle.Fill,TextAlign=ContentAlignment.MiddleLeft},0,0);
            var content=new TableLayoutPanel {Dock=DockStyle.Fill,ColumnCount=1,RowCount=2,Margin=new Padding(0)};
            content.RowStyles.Add(new RowStyle(SizeType.Percent,100));content.RowStyles.Add(new RowStyle(SizeType.Absolute,0));
            var table=new DataGridView {Dock=DockStyle.Fill,AllowUserToAddRows=false,AllowUserToDeleteRows=false,AllowUserToResizeRows=false,AutoGenerateColumns=false,RowHeadersVisible=false,SelectionMode=DataGridViewSelectionMode.FullRowSelect,MultiSelect=false,AccessibleName="历史进展多选表格"};
            table.Columns.Add(new DataGridViewCheckBoxColumn {HeaderText="选择",Width=54,FlatStyle=FlatStyle.Standard});table.Columns.Add(new DataGridViewTextBoxColumn {HeaderText="日期",Width=100,ReadOnly=true});table.Columns.Add(new DataGridViewTextBoxColumn {HeaderText="历史进展信息",AutoSizeMode=DataGridViewAutoSizeColumnMode.Fill,ReadOnly=true});table.Columns.Add(new DataGridViewButtonColumn {HeaderText="引用方",Width=116,ReadOnly=true,FlatStyle=FlatStyle.Standard});
            foreach(var item in candidates){int index=table.Rows.Add(false,item.Date,Plain(item.Html).Replace("\r\n"," ").Replace("\n"," "),item.Citations.Count==0?"无引用方":"查看引用方（"+item.Citations.Count+"）");table.Rows[index].Tag=item;}
            table.CurrentCellDirtyStateChanged+=delegate{if(table.IsCurrentCellDirty)table.CommitEdit(DataGridViewDataErrorContexts.Commit);};
            content.Controls.Add(table,0,0);
            var citationDetails=new TextBox {Dock=DockStyle.Fill,Multiline=true,ReadOnly=true,ScrollBars=ScrollBars.Vertical,Visible=false,AccessibleName="引用方的进展信息"};content.Controls.Add(citationDetails,0,1);layout.Controls.Add(content,0,1);
            ProgressReferenceCandidate shown=null;
            table.CellContentClick+=delegate(object sender,DataGridViewCellEventArgs e){
                if(e.RowIndex<0 || e.ColumnIndex!=3)return;var item=table.Rows[e.RowIndex].Tag as ProgressReferenceCandidate;if(item==null || item.Citations.Count==0)return;
                if(Object.ReferenceEquals(shown,item) && citationDetails.Visible){shown=null;citationDetails.Visible=false;content.RowStyles[1].Height=0;table.Rows[e.RowIndex].Cells[3].Value="查看引用方（"+item.Citations.Count+"）";return;}
                foreach(DataGridViewRow row in table.Rows){var other=row.Tag as ProgressReferenceCandidate;if(other!=null && other.Citations.Count>0)row.Cells[3].Value="查看引用方（"+other.Citations.Count+"）";}
                shown=item;citationDetails.Text=String.Join(Environment.NewLine+Environment.NewLine,item.Citations.Select(citation=>citation.Date+"："+Plain(citation.Html)));citationDetails.Visible=true;content.RowStyles[1].Height=120;table.Rows[e.RowIndex].Cells[3].Value="收起引用方";
            };
            var actions=new TableLayoutPanel {Dock=DockStyle.Fill,ColumnCount=3,RowCount=1,Margin=new Padding(0),Padding=new Padding(0,8,0,0)};
            actions.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,100));actions.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute,96));actions.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute,96));
            var cancel=new Button {Text="取消",DialogResult=DialogResult.Cancel,Dock=DockStyle.Fill,Margin=new Padding(4,0,4,0),AccessibleName="取消引用历史进展"};
            var confirm=new Button {Text="确定",DialogResult=DialogResult.OK,Dock=DockStyle.Fill,Margin=new Padding(4,0,0,0),AccessibleName="确定引用所选历史进展"};
            actions.Controls.Add(cancel,1,0);actions.Controls.Add(confirm,2,0);layout.Controls.Add(actions,0,2);dialog.Controls.Add(layout);dialog.AcceptButton=confirm;dialog.CancelButton=cancel;
            if(verify)dialog.Shown+=delegate{
                if(table.Rows.Count<2)throw new Exception("Reference picker table lacks historical rows");
                var confirmBounds=dialog.RectangleToClient(confirm.RectangleToScreen(confirm.ClientRectangle));var cancelBounds=dialog.RectangleToClient(cancel.RectangleToScreen(cancel.ClientRectangle));
                if(confirm.Height<28 || cancel.Height<28 || !dialog.ClientRectangle.Contains(confirmBounds) || !dialog.ClientRectangle.Contains(cancelBounds))throw new Exception("Reference picker confirm or cancel button is clipped");
                table.Rows[0].Cells[0].Value=true;table.Rows[1].Cells[0].Value=true;table.CurrentCell=null;table.ClearSelection();table.Refresh();Application.DoEvents();using(var bitmap=new Bitmap(dialog.Width,dialog.Height)){dialog.DrawToBitmap(bitmap,new Rectangle(Point.Empty,dialog.Size));bitmap.Save(Path.Combine(data,"floating-progress-reference-picker-test.png"));}dialog.DialogResult=DialogResult.OK;dialog.Close();
            };
            if(dialog.ShowDialog(owner)!=DialogResult.OK)return new List<string>();table.EndEdit();return table.Rows.Cast<DataGridViewRow>().Where(row=>Convert.ToBoolean(row.Cells[0].Value)).Select(row=>((ProgressReferenceCandidate)row.Tag).Date).ToList();
        }
    }
    sealed class ProgressHistoryRow {public string Date,Html,Text;public int ImageCount;}
    List<ProgressHistoryRow> ProgressHistoryRows(long taskId,List<Dictionary<string,object>> history) {
        return DailyHistory(history).GroupBy(note=>DayOf(note)).OrderByDescending(group=>group.Key).Select(group=>{
            var ordered=group.OrderBy(note=>Convert.ToInt64(note["id"])).ToList();
            bool hasTeamAuthors=ordered.Any(note=>ReadFloatingTeamMarker((string)note["comment"])!=null);
            var html=new StringBuilder();var summaries=new List<string>();
            foreach(var note in ordered) {
                string saved=(string)note["comment"],body=ProgressBody(saved,taskId);
                var marker=ReadFloatingTeamMarker(saved);string author=hasTeamAuthors && marker!=null?marker.author:"";
                html.Append("<div>");
                if(!String.IsNullOrWhiteSpace(author))html.Append("<p><strong>").Append(WebUtility.HtmlEncode(author)).Append("：</strong></p>");
                // Keep each author's text, inline pictures and references together in saved order.
                html.Append(body).Append(SerializeProgressReferences(SplitProgressReferences(saved,taskId).References)).Append("</div>");
                string text=Plain(body);
                if(!String.IsNullOrWhiteSpace(author))text=author+"："+(String.IsNullOrWhiteSpace(text)?"（仅包含图片或引用）":text);
                if(!String.IsNullOrWhiteSpace(text))summaries.Add(text);
            }
            var references=ordered.SelectMany(note=>SplitProgressReferences((string)note["comment"],taskId).References).GroupBy(item=>item.Id).Select(items=>items.First()).ToList();
            if(references.Count>0)summaries.Add("引用历史进展：\r\n"+String.Join("\r\n",references.Select(item=>item.Date+"："+Plain(item.Html).Replace("\r\n"," ").Replace("\n"," "))));
            var images=new List<GalleryImage>();CollectImages(images,html.ToString(),"");
            return new ProgressHistoryRow{Date=group.Key,Html=html.ToString(),Text=group.Key+"："+(summaries.Count==0?"（仅包含图片或引用）":String.Join("\r\n\r\n",summaries)),ImageCount=images.Count};
        }).ToList();
    }
    static DataGridView ProgressHistoryTable(List<ProgressHistoryRow> rows) {
        var table=new DataGridView{Dock=DockStyle.Fill,AllowUserToAddRows=false,AllowUserToDeleteRows=false,AllowUserToResizeRows=true,AutoGenerateColumns=false,RowHeadersVisible=false,ReadOnly=true,SelectionMode=DataGridViewSelectionMode.FullRowSelect,MultiSelect=false,AutoSizeRowsMode=DataGridViewAutoSizeRowsMode.AllCellsExceptHeaders,AccessibleName="所有历史进展"};
        table.DefaultCellStyle.WrapMode=DataGridViewTriState.True;
        table.Columns.Add(new DataGridViewTextBoxColumn{HeaderText="日期：进展",AutoSizeMode=DataGridViewAutoSizeColumnMode.Fill,ReadOnly=true});
        table.Columns.Add(new DataGridViewLinkColumn{HeaderText="图片",Width=110,ReadOnly=true,LinkBehavior=LinkBehavior.AlwaysUnderline,LinkColor=Color.FromArgb(0,76,170),ActiveLinkColor=Color.FromArgb(0,76,170),VisitedLinkColor=Color.FromArgb(0,76,170),TrackVisitedState=false});
        table.Columns.Add(new DataGridViewButtonColumn{HeaderText="详情",Width=100,ReadOnly=true,FlatStyle=FlatStyle.Standard});
        // Keep blue links legible when the row is selected.
        table.DefaultCellStyle.SelectionBackColor=Color.FromArgb(225,237,252);table.DefaultCellStyle.SelectionForeColor=Color.Black;
        foreach(var item in rows){int index=table.Rows.Add(item.Text,item.ImageCount>0?"含 "+item.ImageCount+" 张图片":"","完整信息");table.Rows[index].Tag=item;table.Rows[index].MinimumHeight=42;}
        return table;
    }
    async Task<string> ProgressHistoryDocument(string html,Func<string,Task<byte[]>> download) {
        // Allow only passive formatting. Images are fetched through the authenticated local API.
        string safe=TaskDescriptionEditorHtml(html??"");var output=new StringBuilder();int cursor=0;
        var downloaded=new Dictionary<string,string>();
        foreach(Match token in ProgressTags.Matches(safe)) {
            output.Append(safe.Substring(cursor,token.Index-cursor));cursor=token.Index+token.Length;
            var tag=Regex.Match(token.Value,@"^<\s*(/?)\s*([a-z0-9]+)\b",RegexOptions.IgnoreCase);if(!tag.Success)continue;
            bool closing=tag.Groups[1].Value=="/";string name=tag.Groups[2].Value.ToLowerInvariant();
            if(name=="img" && !closing) {
                var images=new List<GalleryImage>();CollectImages(images,token.Value,"");
                if(images.Count==0){output.Append("<span>（图片地址无法识别）</span>");continue;}
                var image=images[0];string source;
                if(!downloaded.TryGetValue(image.Source,out source)) {
                    try{source="data:image/png;base64,"+Convert.ToBase64String(image.Bytes??await download(image.Source));}
                    catch{source="";}
                    downloaded[image.Source]=source;
                }
                output.Append(source==""?"<span>（图片加载失败，请关闭后重试）</span>":"<img src=\""+source+"\" alt=\"进展图片\">");continue;
            }
            if(!Regex.IsMatch(name,@"^(?:p|div|span|br|strong|b|em|i|u|s|strike|ul|ol|li|blockquote|pre|code|table|thead|tbody|tfoot|tr|th|td|h[1-6]|hr|sub|sup)$"))continue;
            output.Append(closing?"</"+name+">":"<"+name+SafeIndentStyle(ReferenceAttribute(token.Value,"style"))+">");
        }
        output.Append(safe.Substring(cursor));
        return "<!doctype html><html><head><meta charset=\"utf-8\"><meta http-equiv=\"X-UA-Compatible\" content=\"IE=edge\"><style>html,body{margin:0;background:#fff;color:#17212d;font:15px/1.6 'Microsoft YaHei UI','Segoe UI',sans-serif}body{padding:16px;overflow-wrap:break-word;word-wrap:break-word}p{margin:.5em 0}img{max-width:100%;height:auto;vertical-align:middle}blockquote{margin:12px 0;padding:8px 12px;background:#f1f5fa}pre{white-space:pre-wrap}table{max-width:100%;border-collapse:collapse}td,th{border:1px solid #cbd3dc;padding:6px}</style></head><body>"+output+"</body></html>";
    }
    async Task ShowProgressHistoryDetails(ProgressHistoryRow item,string taskTitle,Form owner,List<ProgressHistoryRow> rows=null) {
        rows=rows??new List<ProgressHistoryRow>{item};
        int first=Math.Max(0,rows.IndexOf(item)),last=first;bool loading=false;
        using(var dialog=DpiDialog(new Form{Text="完整信息 · "+item.Date+" · "+taskTitle,Size=new Size(760,640),MinimumSize=new Size(440,330),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=true,MinimizeBox=true})) {
            var heading=new Label{Text=item.Date+" · 正在读取完整进展…",Dock=DockStyle.Top,Height=48,Padding=new Padding(14,10,14,0),AccessibleName="完整进展日期"};
            var browser=new WebBrowser{Dock=DockStyle.Fill,AllowWebBrowserDrop=false,IsWebBrowserContextMenuEnabled=true,ScriptErrorsSuppressed=true,AccessibleName="进展完整图文内容"};
            var bridge=new ProgressHistoryBrowserBridge();browser.ObjectForScripting=bridge;
            browser.Navigating+=delegate(object sender,WebBrowserNavigatingEventArgs e){if(e.Url.AbsoluteUri!="about:blank")e.Cancel=true;};
            AttachHistoryImageActions(browser,dialog,heading);
            bridge.ImageRequested=delegate(string source){if(dialog.IsDisposed)return;dialog.BeginInvoke(new Action(async delegate{try{await ShowProgressEditorImage(new ProgressEditorImageEventArgs(source,""),dialog,"进展图片");}catch(Exception error){if(!dialog.IsDisposed)heading.Text="图片预览失败："+error.Message;}}));};
            Action explain=delegate{heading.Text=rows[first].Date+" 至 "+rows[last].Date+" · 滚动到边缘继续滚动，加载相邻日期";};
            bridge.LoadMore=delegate(int direction) {
                if(loading || dialog.IsDisposed || browser.Document==null)return;
                int next=direction<0?first-1:last+1;if(next<0 || next>=rows.Count)return;
                loading=true;heading.Text="正在加载 "+rows[next].Date+"…";
                dialog.BeginInvoke(new Action(async delegate {
                    try {
                        string document=await ProgressHistoryDocument(rows[next].Html,DownloadImage);
                        if(dialog.IsDisposed || browser.Document==null)return;
                        var body=Regex.Match(document,@"<body>([\s\S]*)</body>").Groups[1].Value;
                        browser.Document.InvokeScript("appendHistoryDay",new object[]{rows[next].Date,body,direction<0});
                        if(direction<0)first=next;else last=next;explain();
                    }catch(Exception error){if(!dialog.IsDisposed)heading.Text="加载失败，可继续滚动重试："+error.Message;}
                    finally{loading=false;}
                }));
            };
            dialog.Controls.Add(browser);dialog.Controls.Add(heading);
            dialog.Shown+=async delegate {
                try {
                    loading=true;string document=await ProgressHistoryDocument(item.Html,DownloadImage);if(dialog.IsDisposed)return;
                    document=document.Replace("<body>","<body><section id=\"history-"+item.Date+"\"><h2>"+item.Date+"</h2>").Replace("</body>","</section></body>");
                    browser.DocumentText=document.Replace("</head>","<script>"+ProgressHistoryBrowserBridge.Script+"</script></head>");explain();
                }catch(Exception error){if(!dialog.IsDisposed)heading.Text="完整信息读取失败："+error.Message;}
                finally{loading=false;}
            };
            await ShowEditorWindowAsync(dialog,owner);
        }
    }
    void AttachHistoryImageActions(WebBrowser browser,Form owner,Label feedback) {
        browser.DocumentCompleted+=delegate {
            if(browser.Document==null)return;
            browser.Document.ContextMenuShowing+=delegate(object sender,HtmlElementEventArgs args) {
                var element=browser.Document.GetElementFromPoint(args.ClientMousePosition);
                if(element==null || !String.Equals(element.TagName,"IMG",StringComparison.OrdinalIgnoreCase))return;
                args.ReturnValue=false;
                var menu=new ContextMenuStrip();
                menu.Items.Add("复制图片",null,async delegate {
                    try {
                        var picture=ProgressEditorGalleryImage(new ProgressEditorImageEventArgs(element.GetAttribute("src"),""),"进展图片");
                        if(picture==null)return;
                        byte[] bytes=picture.Bytes??await DownloadImage(picture.Source);
                        using(var stream=new MemoryStream(bytes))using(var image=Image.FromStream(stream))Clipboard.SetImage(image);
                    }catch(Exception error){if(!feedback.IsDisposed)feedback.Text="图片复制失败："+error.Message;}
                });
                menu.Closed+=delegate{menu.Dispose();};menu.Show(browser,args.ClientMousePosition);
            };
        };
    }
    Task ShowProgressHistory(long taskId,string taskTitle,List<Dictionary<string,object>> history,Form owner,bool verify=false) {
        var rows=ProgressHistoryRows(taskId,history);
        using(var dialog=DpiDialog(new Form{Text="所有进展 · "+taskTitle,Size=new Size(760,540),MinimumSize=new Size(480,330),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false})) {
            var layout=new TableLayoutPanel{Dock=DockStyle.Fill,Padding=new Padding(14),ColumnCount=1,RowCount=2};layout.RowStyles.Add(new RowStyle(SizeType.Absolute,40));layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));
            var heading=new Label{Text=rows.Count==0?"暂无每日进展。":"全部每日进展 · 日期越新越靠前",Dock=DockStyle.Fill,TextAlign=ContentAlignment.MiddleLeft,AccessibleName="历史进展说明"};layout.Controls.Add(heading,0,0);
            var table=ProgressHistoryTable(rows);
            table.CellContentClick+=async delegate(object sender,DataGridViewCellEventArgs e){
                if(e.RowIndex<0 || e.ColumnIndex<1)return;var item=table.Rows[e.RowIndex].Tag as ProgressHistoryRow;if(item==null)return;
                try{if(e.ColumnIndex==2)await ShowProgressHistoryDetails(item,taskTitle,dialog,rows);else if(item.ImageCount>0)await ShowImageGallery(taskId,item.Html,dialog,"每日进展 · "+item.Date);}
                catch(Exception error){if(!dialog.IsDisposed)heading.Text="进展查看失败："+error.Message;}
            };
            layout.Controls.Add(table,0,1);dialog.Controls.Add(layout);
            if(verify)dialog.Shown+=delegate{if(rows.Count<3 || !rows.SequenceEqual(rows.OrderByDescending(item=>item.Date)) || table.Rows.Count!=rows.Count || !Convert.ToString(table.Rows[0].Cells[0].Value).StartsWith(rows[0].Date+"："))throw new Exception("All-progress history is missing, unordered, or not formatted as date: progress");using(var bitmap=new Bitmap(dialog.Width,dialog.Height)){dialog.DrawToBitmap(bitmap,new Rectangle(Point.Empty,dialog.Size));bitmap.Save(Path.Combine(data,"floating-progress-history-test.png"));}dialog.Close();};
            dialog.ShowDialog(owner);
        }
        return Task.FromResult(0);
    }
    async Task ShowReferenceSnapshot(ProgressReference reference,Form owner,bool verify=false) {
        using(var dialog=DpiDialog(new Form {Text="引用快照 · "+reference.Date,Size=new Size(520,410),MinimumSize=new Size(390,280),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false})) {
            var layout=new TableLayoutPanel {Dock=DockStyle.Fill,Padding=new Padding(14),ColumnCount=1,RowCount=3};
            layout.RowStyles.Add(new RowStyle(SizeType.Absolute,46));layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,38));
            layout.Controls.Add(new Label{Text="引用内容快照 · "+reference.Date+"；添加引用时固定，后续更正不会改写原记录。",Dock=DockStyle.Fill,TextAlign=ContentAlignment.BottomLeft});
            layout.Controls.Add(new TextBox {Text=Plain(reference.Html),Multiline=true,ReadOnly=true,ScrollBars=ScrollBars.Vertical,Dock=DockStyle.Fill,AccessibleName="引用内容快照",AccessibleDescription="引用内容快照"});
            var images=new Button {Text="查看快照图片",Dock=DockStyle.Fill,Enabled=Regex.IsMatch(reference.Html??"",@"<img\b",RegexOptions.IgnoreCase)};
            images.Click+=async delegate{await ShowImageGallery(reference.TaskId,reference.Html,dialog,"引用快照 · "+reference.Date);};layout.Controls.Add(images);dialog.Controls.Add(layout);
            if(verify)dialog.Shown+=delegate{using(var bitmap=new Bitmap(dialog.Width,dialog.Height)){dialog.DrawToBitmap(bitmap,new Rectangle(Point.Empty,dialog.Size));bitmap.Save(Path.Combine(data,"floating-progress-reference-snapshot-test.png"));}dialog.Close();};
            dialog.ShowDialog(owner);
        }
        await Task.FromResult(0);
    }
    async Task TestProgressReferences() {
        var project=projects.SelectedItem as Project;bool recordingBefore=undoRecording;long taskId=0;
        try {
            undoRecording=false;taskId=Convert.ToInt64((await Api("POST","/projects/"+project.Id+"/tasks",new {title="引用历史进展验收"}))["id"]);
            var images=new List<PastedImage>();using(var bitmap=new Bitmap(56,36))using(var stream=new MemoryStream()){using(var canvas=Graphics.FromImage(bitmap))canvas.Clear(Color.SteelBlue);bitmap.Save(stream,System.Drawing.Imaging.ImageFormat.Png);images.Add(new PastedImage {Bytes=stream.ToArray()});}
            await Task.Delay(120);
            long ancient=await SaveProgress(taskId,DateTime.Today.AddDays(-4),"最早的原记录", "",images);
            await Task.Delay(120);
            long middle=await SaveProgress(taskId,DateTime.Today.AddDays(-2),"另一日期的旧结论", "");
            await Task.Delay(120);
            var nested=ReferenceForDay(taskId,DateTime.Today.AddDays(-4).ToString("yyyy-MM-dd"),DateTime.Today.AddDays(-1).ToString("yyyy-MM-dd"),await ReadHistory(taskId));
            long absorbed=await SaveProgress(taskId,DateTime.Today.AddDays(-1),"同日早先片段", "");
            await Task.Delay(120);
            string imageHtml="<p><img src=\"/api/v1/tasks/"+taskId+"/attachments/"+images[0].Id+"\" alt=\"历史图片\"></p>";
            long source=await SaveProgress(taskId,DateTime.Today.AddDays(-1),"旧结论待更正", "",null,0,"<p><strong>旧结论</strong>待更正</p>"+imageHtml,new List<long>{absorbed},new List<ProgressReference>{nested});
            await Task.Delay(120);
            await SaveProgress(taskId,DateTime.Today.AddDays(2),"未来日期不可引用", "");
            await Task.Delay(120);
            var original=await ReadHistory(taskId);var preserved=original.ToDictionary(note=>Convert.ToInt64(note["id"]),note=>(string)note["comment"]);
            var reference=ReferenceForDay(taskId,DateTime.Today.AddDays(-1).ToString("yyyy-MM-dd"),DateTime.Today.ToString("yyyy-MM-dd"),original);
            if(!reference.CommentIds.SequenceEqual(new List<long>{absorbed,source}) || reference.Html.Contains("最早的原记录") || reference.Html.Contains("<h3") || reference.Html.Contains("<strong") || reference.Html.Contains("data-tasktrace-reference") || !reference.Html.Contains("<img"))throw new Exception("Reference snapshot failed to flatten own content, preserve image or retain merged source IDs");
            string serialized=SerializeProgressReferences(new[]{reference});var roundtrip=SplitProgressReferences("<p>当日正文</p>"+serialized,taskId);
            if(roundtrip.Body!="<p>当日正文</p>" || roundtrip.References.Count!=1 || SerializeProgressReferences(roundtrip.References)!=serialized)throw new Exception("Reference HTML protocol roundtrip failed");
            var duplicate=reference.Copy();duplicate.Id="r"+Guid.NewGuid().ToString("N");
            if(SplitProgressReferences(serialized+serialized+SerializeProgressReferences(new[]{duplicate}),taskId).References.Count!=2)throw new Exception("Reference ID dedup discarded distinct same-date snapshots");
            string invalid=serialized.Replace("data-date=\""+reference.Date+"\"","data-date=\"not-a-date\"");if(SplitProgressReferences(invalid,taskId).Body!=invalid || SplitProgressReferences(serialized,taskId+1).Body!=serialized)throw new Exception("Invalid reference markup was removed from ordinary body");
            string extra=serialized.Replace("</blockquote>","<p>用户额外正文</p></blockquote>");if(SplitProgressReferences(extra,taskId).Body!=extra)throw new Exception("Extra user content in malformed reference was lost");
            string badOuter="<blockquote data-tasktrace-reference=\"2\">"+serialized+"</blockquote>";if(SplitProgressReferences(badOuter,taskId).Body!=badOuter)throw new Exception("Invalid outer wrapper lost valid nested content");
            string unclosed="<blockquote data-tasktrace-reference=\"2\">"+serialized;if(SplitProgressReferences(unclosed,taskId).Body!=unclosed)throw new Exception("Unclosed invalid outer wrapper lost nested content");
            string dangerous=ProgressSnapshotHtml("<script>alert(1)</script><h3>小标题</h3><p>A &amp; B</p><img src=\"/api/v1/tasks/"+taskId+"/attachments/"+images[0].Id+"\"><p>图片后文字</p><img src=\"https://example.invalid/picture.png\">",taskId);if(dangerous.Contains("alert") || dangerous.Contains("<h3") || dangerous.Contains("example.invalid") || dangerous.IndexOf("<img",StringComparison.Ordinal)>dangerous.IndexOf("图片后文字",StringComparison.Ordinal))throw new Exception("Reference snapshot normalization failed");
            string exactLines="<p><br>第一行<br><br>第二行<br></p><div>第三行</div><div><br></div>";if(ProgressSnapshotHtml(exactLines,taskId)!=exactLines || ProgressSnapshotHtml(ProgressSnapshotHtml(exactLines,taskId),taskId)!=exactLines)throw new Exception("Progress editor changed user line breaks");
            if(ProgressTextHtml("\r\n第一行\r\n\r\n第二行\r\n")!="<p><br>第一行<br><br>第二行<br></p>")throw new Exception("Progress text conversion trimmed user line breaks");
            bool blocked=false;try{ReferenceForDay(taskId,DateTime.Today.ToString("yyyy-MM-dd"),DateTime.Today.ToString("yyyy-MM-dd"),original);}catch{blocked=true;}if(!blocked)throw new Exception("Same-day reference was accepted");
            undoRecording=true;await EditProgress(taskId,"引用历史进展验收",true);
            var current=await ReadHistory(taskId);foreach(var note in preserved)if((string)current.First(row=>Convert.ToInt64(row["id"])==note.Key)["comment"]!=note.Value)throw new Exception("Adding a reference rewrote its source record");
            var correction=current.First(note=>DayOf(note)==DateTime.Today.ToString("yyyy-MM-dd"));long correctionId=Convert.ToInt64(correction["id"]);
            if(SplitProgressReferences((string)correction["comment"],taskId).References.Count!=1)throw new Exception("Reference removal did not persist");
            await SaveProgress(taskId,DateTime.Today,"普通后续编辑", "",null,correctionId);
            correction=(await ReadHistory(taskId)).First(note=>Convert.ToInt64(note["id"])==correctionId);if(SplitProgressReferences((string)correction["comment"],taskId).References.Count!=1)throw new Exception("Default save lost existing references");
            await ShowImageGallery(taskId,serialized,this,"引用快照验收");
            if((await DownloadImage("/api/v1/tasks/"+taskId+"/attachments/"+images[0].Id)).Length==0)throw new Exception("Referenced image became inaccessible");
            File.WriteAllText(Path.Combine(data,"floating-progress-references-test.txt"),"PASS: checkbox table multi-select with confirm/cancel, earlier same-task dates, duplicate prevention, same-date distinct snapshots retained, merged source IDs, immutable source records, expandable snapshots and citing progress, snapshot text/images, no nested reference content, malformed markup retained, own body isolation, date draft roundtrip, autosave/edit/remove persistence, marked progress dates, calendar screenshot, reference preview and image download, minimum-size dialog layout.");
        } finally {undoRecording=recordingBefore;}
        if(taskId>0){undoRecording=false;await Api("DELETE","/tasks/"+taskId,null);undoRecording=recordingBefore;}
    }

}
