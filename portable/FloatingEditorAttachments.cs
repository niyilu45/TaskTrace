// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.IO;
using System.Net;
using System.Net.Http;
using System.Text;
using System.Text.RegularExpressions;
using System.Threading.Tasks;
using System.Collections;
using System.Collections.Generic;

internal sealed partial class FloatingWindow {
    internal static bool EditorAttachmentHref(string href){return Regex.IsMatch(href??"",@"^(?:/api/v[12]/tasks/[1-9][0-9]*/attachments/[1-9][0-9]*|data:application/octet-stream;base64,[A-Za-z0-9+/=\s]+)$",RegexOptions.IgnoreCase);}
    async Task<string> PrepareInlineEditorFiles(string html) {
        var output=new StringBuilder();int cursor=0;
        foreach(Match link in Regex.Matches(html??"",@"<a\b[^>]*>",RegexOptions.IgnoreCase)) {
            output.Append(html.Substring(cursor,link.Index-cursor));cursor=link.Index+link.Length;
            string href=ReferenceAttribute(link.Value,"href")??"",path=AttachmentPath(href);
            if(path==null){output.Append(link.Value);continue;}
            try {
                byte[] bytes=await DownloadImage(path);
                if(bytes.Length>64*1024*1024){output.Append(link.Value);continue;}
                string name=ReferenceAttribute(link.Value,"download")??"附件";
                output.Append("<a href=\"data:application/octet-stream;base64,").Append(Convert.ToBase64String(bytes)).Append("\" download=\"").Append(WebUtility.HtmlEncode(name)).Append("\" data-tasktrace-file=\"").Append(WebUtility.HtmlEncode(path)).Append("\">");
            }catch{output.Append(link.Value);}
        }
        output.Append(html.Substring(cursor));return output.ToString();
    }
    async Task<string> PersistInlineEditorFiles(long taskId,string html) {
        var output=new StringBuilder();int cursor=0;
        foreach(Match link in Regex.Matches(html??"",@"<a\b[^>]*>",RegexOptions.IgnoreCase)) {
            output.Append(html.Substring(cursor,link.Index-cursor));cursor=link.Index+link.Length;
            string href=ReferenceAttribute(link.Value,"href")??"",retained=ReferenceAttribute(link.Value,"data-tasktrace-file")??"",name=ReferenceAttribute(link.Value,"download")??"附件";
            if(Regex.IsMatch(retained,@"^/api/v[12]/tasks/"+taskId+@"/attachments/[1-9][0-9]*$"))href=retained;
            else if(href.StartsWith("data:application/octet-stream;base64,",StringComparison.OrdinalIgnoreCase)) {
                byte[] bytes=Convert.FromBase64String(href.Substring(href.IndexOf(',')+1));
                long attachmentId=await UploadEditorFile(taskId,bytes,name,"application/octet-stream");href="/api/v1/tasks/"+taskId+"/attachments/"+attachmentId;
            }
            if(EditorAttachmentHref(href))output.Append("<a href=\"").Append(WebUtility.HtmlEncode(href)).Append("\" download=\"").Append(WebUtility.HtmlEncode(name)).Append("\">");
            else output.Append(link.Value);
        }
        output.Append(html.Substring(cursor));return output.ToString();
    }
    async Task<long> UploadEditorFile(long taskId,byte[] bytes,string name,string contentType) {
        if(bytes==null || bytes.Length==0)throw new Exception("附件内容为空，当前输入已保留，请重新选择图片或文件。");
        if(bytes.Length>64*1024*1024)throw new Exception("单个附件不能超过 64 MB，请选择较小的文件。");
        await Api("GET","/tasks/"+taskId,null);
        using(var body=new MultipartFormDataContent()) {
            var file=new ByteArrayContent(bytes);file.Headers.ContentType=new System.Net.Http.Headers.MediaTypeHeaderValue(contentType);
            body.Add(file,"files",Path.GetFileName(name));
            using(var request=new HttpRequestMessage(HttpMethod.Post,url+"/api/v2/tasks/"+taskId+"/attachments")) {
                request.Headers.Authorization=new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer",token);request.Content=body;
                using(var response=await http.SendAsync(request)) {
                    string responseBody=await response.Content.ReadAsStringAsync();
                    if(!response.IsSuccessStatusCode)throw new Exception(ApiFailureMessage(response.StatusCode,"/tasks/"+taskId+"/attachments",responseBody,"附件上传失败，当前输入已保留，请重试。"));
                    return UploadedEditorAttachmentId(responseBody);
                }
            }
        }
    }
    long UploadedEditorAttachmentId(string responseBody) {
        var result=ReadObject(responseBody);object values;
        if(result!=null && result.TryGetValue("success",out values) && values is IEnumerable) {
            foreach(object raw in (IEnumerable)values) {
                var attached=raw as Dictionary<string,object>;object value;long id;
                if(attached!=null && attached.TryGetValue("id",out value) && Int64.TryParse(Convert.ToString(value),out id) && id>0)return id;
            }
        }
        var errors=new List<string>();
        if(result!=null && result.TryGetValue("errors",out values) && values is IEnumerable) {
            foreach(object raw in (IEnumerable)values) {
                var error=raw as Dictionary<string,object>;object message;
                if(error!=null && error.TryGetValue("message",out message) && !String.IsNullOrWhiteSpace(Convert.ToString(message)))
                    errors.Add(Convert.ToString(message));
            }
        }
        throw new Exception("附件未能保存，当前输入和图片已保留。"+(errors.Count==0?"请检查文件大小、写入权限后重试。":String.Join("；",errors)));
    }
}
