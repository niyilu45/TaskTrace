// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Net;
using System.Text;
using System.Text.RegularExpressions;
using System.Windows.Forms;

internal static class FloatingClipboardMedia {
    const int MaxBytes=64*1024*1024;
    static string ImageHtml(byte[] bytes) {
        if(bytes==null || bytes.Length==0 || bytes.Length>MaxBytes)throw new Exception("剪贴板图片无法读取或超过 64 MB。");
        using(var stream=new MemoryStream(bytes))using(var image=Image.FromStream(stream))return "<img src=\"data:image/png;base64,"+Convert.ToBase64String(FloatingClipboard.PngBytes(image))+"\">";
    }
    static string Source(string tag) {
        var match=Regex.Match(tag,@"\bsrc\s*=\s*(?:""(?<value>[^""]*)""|'(?<value>[^']*)'|(?<value>[^\s>]+))",RegexOptions.IgnoreCase);
        return WebUtility.HtmlDecode(match.Groups["value"].Value);
    }
    static byte[] LocalImage(string source) {
        Uri uri;string path=null;
        if(Uri.TryCreate(source,UriKind.Absolute,out uri) && uri.IsFile)path=uri.LocalPath;
        else if(Path.IsPathRooted(source) && !source.StartsWith("/api/",StringComparison.OrdinalIgnoreCase))path=source;
        if(path==null || !File.Exists(path))return null;
        if(new FileInfo(path).Length>MaxBytes)throw new Exception("剪贴板图片超过 64 MB，请分次添加。");
        return File.ReadAllBytes(path);
    }
    static byte[] ReadRemote(Uri source) {
        ServicePointManager.SecurityProtocol|=SecurityProtocolType.Tls12;
        var request=(HttpWebRequest)WebRequest.Create(source);request.Proxy=WebRequest.DefaultWebProxy;request.Timeout=8000;request.ReadWriteTimeout=8000;request.UserAgent="Mozilla/5.0 TaskTrace";
        using(var response=request.GetResponse())using(var input=response.GetResponseStream())using(var output=new MemoryStream()) {
            if(response.ContentLength>MaxBytes)throw new Exception("图片超过 64 MB。");
            byte[] buffer=new byte[16384];int count;while((count=input.Read(buffer,0,buffer.Length))>0){if(output.Length+count>MaxBytes)throw new Exception("图片超过 64 MB。");output.Write(buffer,0,count);}return output.ToArray();
        }
    }
    internal static string Expand(string html,IDataObject data,Func<Uri,byte[]> readRemote=null) {
        string rtf=Convert.ToString(data.GetData(DataFormats.Rtf,false));
        if(rtf.IndexOf(@"\pict",StringComparison.OrdinalIgnoreCase)<0)rtf="";
        var raw=data.GetData(DataFormats.Html,false);string envelope=raw as string;
        if(envelope==null){var bytes=FloatingClipboard.StreamBytes(raw as Stream);if(bytes!=null)envelope=Encoding.UTF8.GetString(bytes,0,Math.Min(bytes.Length,4096));}
        var sourceUrl=Regex.Match(envelope??"",@"(?im)^SourceURL:\s*(?<url>[^\r\n]+)");Uri origin=null;
        if(sourceUrl.Success)Uri.TryCreate(sourceUrl.Groups["url"].Value.Trim(),UriKind.Absolute,out origin);
        var remoteCache=new Dictionary<string,string>();
        List<string> rtfImages=null;
        string rtfHtml=null;
        Func<List<string>> images=delegate{if(rtfImages==null){rtfImages=new List<string>();rtfHtml=FromRtf(rtf,rtfImages);}return rtfImages;};
        if(!Regex.IsMatch(html??"",@"<img\b",RegexOptions.IgnoreCase) && Regex.IsMatch(html??"",@"<v:imagedata\b",RegexOptions.IgnoreCase))html=Regex.Replace(html,@"<v:imagedata\b[^>]*>",match=>"<img src=\""+WebUtility.HtmlEncode(Source(match.Value))+"\">",RegexOptions.IgnoreCase);
        var tags=Regex.Matches(html??"",@"<img\b[^>]*>",RegexOptions.IgnoreCase);
        if(tags.Count==0){if(!String.IsNullOrEmpty(rtf) && images().Count>0)return rtfHtml;return html??"";}
        int index=0;
        return Regex.Replace(html,@"<img\b[^>]*>",delegate(Match match){
            int position=index++;string source=Source(match.Value);
            if(Regex.IsMatch(source,@"^(?:data:image/(?:png|jpe?g|gif|bmp);base64,|/?api/v[12]/tasks/[1-9][0-9]*/attachments/[1-9][0-9]*)",RegexOptions.IgnoreCase))return match.Value;
            Uri resource=null;if(origin!=null)Uri.TryCreate(origin,source,out resource);else Uri.TryCreate(source,UriKind.Absolute,out resource);
            byte[] local=resource!=null&&!resource.IsFile?null:LocalImage(resource!=null?resource.AbsoluteUri:source);if(local!=null)return ImageHtml(local);
            var alternatives=images();if(position<alternatives.Count)return alternatives[position];
            // A single bitmap cannot represent several distinct inline images.
            if(tags.Count==1){var bitmap=FloatingClipboard.ImageBytes(data);if(bitmap!=null)return ImageHtml(bitmap);}
            if(resource!=null && (resource.Scheme=="http" || resource.Scheme=="https")) {
                string cached;if(remoteCache.TryGetValue(resource.AbsoluteUri,out cached))return cached;
                try{cached=ImageHtml((readRemote??ReadRemote)(resource));remoteCache[resource.AbsoluteUri]=cached;return cached;}catch(Exception error){throw new Exception("无法读取第 "+(position+1)+" 张网页图片，请检查网络或在原软件中复制原图。"+error.Message,error);}
            }
            throw new Exception("无法读取复制内容中的第 "+(position+1)+" 张图片，未粘贴不完整内容。请从原软件重新复制，或先将图片保存到本地再粘贴。");
        },RegexOptions.IgnoreCase);
    }

    static int GroupEnd(string rtf,int start) {
        int depth=0;
        for(int i=start;i<rtf.Length;i++) {
            if(rtf[i]=='\\'){if(i+1<rtf.Length && (rtf[i+1]=='\\'||rtf[i+1]=='{'||rtf[i+1]=='}')){i++;continue;}var binary=Regex.Match(rtf.Substring(i,Math.Min(30,rtf.Length-i)),@"^\\bin(\d+) ?");int size;if(binary.Success && Int32.TryParse(binary.Groups[1].Value,out size)){i=Math.Min(rtf.Length-1,i+binary.Length+size-1);continue;}}
            if(rtf[i]=='{')depth++;else if(rtf[i]=='}' && --depth==0)return i;
        }
        return rtf.Length-1;
    }
    static int Hex(char value){return value<='9'?value-'0':Char.ToUpperInvariant(value)-'A'+10;}
    static string Picture(string rtf,int start,int end) {
        var hex=new StringBuilder();int cursor=start+1;
        while(cursor<end) {
            char value=rtf[cursor];
            if(value=='{'){cursor=GroupEnd(rtf,cursor)+1;continue;}
            if(value=='\\') {
                var control=Regex.Match(rtf.Substring(cursor,Math.Min(48,end-cursor)),@"^\\[a-z]+-?\d* ?",RegexOptions.IgnoreCase);
                if(!control.Success)throw new Exception("剪贴板中的 RTF 图片格式不受支持。");
                cursor+=control.Length;continue;
            }
            if(Uri.IsHexDigit(value))hex.Append(value);else if(!Char.IsWhiteSpace(value))throw new Exception("剪贴板中的 RTF 图片格式不受支持。");
            cursor++;if(hex.Length>MaxBytes*2)throw new Exception("剪贴板图片超过 64 MB。");
        }
        if(hex.Length==0 || hex.Length%2!=0)throw new Exception("剪贴板中的图片数据不完整。");
        byte[] bytes=new byte[hex.Length/2];for(int i=0;i<bytes.Length;i++)bytes[i]=(byte)((Hex(hex[i*2])<<4)|Hex(hex[i*2+1]));
        return ImageHtml(bytes);
    }
    sealed class RtfState {internal int UnicodeFallback=1;}
    internal static string FromRtf(string rtf,List<string> images) {
        if(String.IsNullOrEmpty(rtf) || !rtf.StartsWith(@"{\rtf",StringComparison.Ordinal))return "";
        var html=new StringBuilder();var paragraph=new StringBuilder();var states=new Stack<RtfState>();var state=new RtfState();var encoding=Encoding.GetEncoding(1252);int fallback=0,indent=0;
        Action flush=delegate{html.Append(indent==0?"<p>":"<p style=\"margin-left:"+(indent/20.0).ToString(System.Globalization.CultureInfo.InvariantCulture)+"pt\">").Append(paragraph).Append("</p>");paragraph.Clear();};
        Action<string> text=delegate(string value){paragraph.Append(WebUtility.HtmlEncode(value));};
        for(int i=0;i<rtf.Length;) {
            char value=rtf[i++];
            if(value=='{') {
                var group=Regex.Match(rtf.Substring(i,Math.Min(64,rtf.Length-i)),@"^(?:\\\*\s*)?\\(?<name>[a-z]+)",RegexOptions.IgnoreCase);string name=group.Groups["name"].Value.ToLowerInvariant();
                if(name=="pict"){int end=GroupEnd(rtf,i-1);string image=Picture(rtf,i-1,end);images.Add(image);paragraph.Append(image);i=end+1;continue;}
                if((group.Value.StartsWith(@"\*") && name!="shppict") || Regex.IsMatch(name,@"^(?:fonttbl|colortbl|stylesheet|info|nonshppict|header\w*|footer\w*|object|fldinst)$")){i=GroupEnd(rtf,i-1)+1;continue;}
                states.Push(state);state=new RtfState{UnicodeFallback=state.UnicodeFallback};continue;
            }
            if(value=='}'){if(states.Count>0)state=states.Pop();continue;}
            if(value=='\r'||value=='\n')continue;
            if(value!='\\'){if(fallback>0)fallback--;else text(value.ToString());continue;}
            if(i>=rtf.Length)break;value=rtf[i];
            if(value=='\\'||value=='{'||value=='}'){i++;if(fallback>0)fallback--;else text(value.ToString());continue;}
            if(value=='\'') {
                var bytes=new List<byte>();while(i+2<rtf.Length && rtf[i]=='\'' && Uri.IsHexDigit(rtf[i+1]) && Uri.IsHexDigit(rtf[i+2])){byte item=Convert.ToByte(rtf.Substring(i+1,2),16);if(fallback>0)fallback--;else bytes.Add(item);i+=3;if(i+1<rtf.Length && rtf[i]=='\\' && rtf[i+1]=='\'')i++;else break;}
                text(encoding.GetString(bytes.ToArray()));continue;
            }
            if(!Char.IsLetter(value)){i++;if(fallback>0){fallback--;continue;}if(value=='~')text("\u00a0");else if(value=='_')text("\u2011");continue;}
            int start=i;while(i<rtf.Length && Char.IsLetter(rtf[i]))i++;string word=rtf.Substring(start,i-start);start=i;if(i<rtf.Length && rtf[i]=='-')i++;while(i<rtf.Length && Char.IsDigit(rtf[i]))i++;int number;Int32.TryParse(rtf.Substring(start,i-start),out number);if(i<rtf.Length && rtf[i]==' ')i++;
            if(word=="uc")state.UnicodeFallback=Math.Max(0,number);
            else if(word=="u"){text(((char)(number&65535)).ToString());fallback=state.UnicodeFallback;}
            else if(word=="ansicpg"){try{encoding=Encoding.GetEncoding(number);}catch(ArgumentException){}}
            else if(word=="par")flush();else if(word=="line")paragraph.Append("<br>");else if(word=="tab")paragraph.Append("&emsp;");else if(word=="li")indent=number;else if(word=="pard")indent=0;
            else if(word=="emdash")text("\u2014");else if(word=="endash")text("\u2013");else if(word=="lquote"||word=="rquote")text(word=="lquote"?"\u2018":"\u2019");else if(word=="ldblquote"||word=="rdblquote")text(word=="ldblquote"?"\u201c":"\u201d");
        }
        if(paragraph.Length>0)flush();return html.ToString();
    }
}
