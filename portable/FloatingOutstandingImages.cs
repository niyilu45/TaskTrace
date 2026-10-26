// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Linq;
using System.Net;
using System.Text.RegularExpressions;

internal sealed partial class FloatingWindow {
    static string ImageAttachmentPath(string source,string localOrigin) {
        string value=WebUtility.HtmlDecode(source??"").Trim();Uri address;
        if(value.StartsWith("/api/",StringComparison.OrdinalIgnoreCase) || value.StartsWith("api/",StringComparison.OrdinalIgnoreCase)) {
            if(!Uri.TryCreate("http://tasktrace.invalid/"+value.TrimStart('/'),UriKind.Absolute,out address))return null;
        } else {
            if(!Uri.TryCreate(value,UriKind.Absolute,out address) || (address.Scheme!="http" && address.Scheme!="https"))return null;
            if(!String.IsNullOrWhiteSpace(localOrigin)){Uri origin;if(!Uri.TryCreate(localOrigin,UriKind.Absolute,out origin) || address.Scheme!=origin.Scheme || address.Host!=origin.Host || address.Port!=origin.Port)return null;}
        }
        var match=Regex.Match(address.AbsolutePath,@"^/api/v[12]/tasks/([0-9]+)/attachments/([0-9]+)$",RegexOptions.IgnoreCase);
        return match.Success?"/api/v2/tasks/"+match.Groups[1].Value+"/attachments/"+match.Groups[2].Value:null;
    }
    static IEnumerable<string> VisibleImageTags(string html) {
        string ignored=null;int depth=0;bool raw=false;
        foreach(System.Text.RegularExpressions.Match token in ProgressTags.Matches(html??"")) {
            var tag=Regex.Match(token.Value,@"^<\s*(/?)\s*([a-z][a-z0-9:_-]*)\b",RegexOptions.IgnoreCase);if(!tag.Success)continue;
            string name=tag.Groups[2].Value.ToLowerInvariant();bool closing=tag.Groups[1].Value=="/";
            if(ignored!=null){if(name==ignored){if(closing)depth--;else if(!raw && !token.Value.EndsWith("/>"))depth++;if(depth==0)ignored=null;}continue;}
            bool review=Regex.IsMatch(token.Value,@"\sdata-tasktrace-review-change(?:\s|=|/?>)",RegexOptions.IgnoreCase);
            if(!closing && (review || Regex.IsMatch(name,@"^(?:script|style|textarea|template|noscript|head)$"))){if(!token.Value.EndsWith("/>")){ignored=name;depth=1;raw=Regex.IsMatch(name,@"^(?:script|style|textarea|noscript|head)$");}continue;}
            if(!closing && name=="img")yield return token.Value;
        }
    }
    static void CollectImages(List<GalleryImage> images,string html,string caption,string localOrigin) {
        var known=new HashSet<string>(images.Select(image=>image.Source),StringComparer.Ordinal);
        foreach(string tag in VisibleImageTags(html)) {
            foreach(string raw in new[]{ReferenceAttribute(tag,"data-tasktrace-src"),ReferenceAttribute(tag,"data-src"),ReferenceAttribute(tag,"src")}.Where(value=>!String.IsNullOrWhiteSpace(value))) {
                string source=ImageAttachmentPath(raw,localOrigin);
                if(source!=null){if(known.Add(source))images.Add(new GalleryImage{Source=source,Caption=caption});break;}
                var data=Regex.Match(raw,@"^data:image/(?:png|jpe?g|gif|bmp);base64,(?<bytes>[A-Za-z0-9+/=\s]+)$",RegexOptions.IgnoreCase);if(!data.Success)continue;
                try{byte[] bytes=Convert.FromBase64String(Regex.Replace(data.Groups["bytes"].Value,@"\s",""));string key="data:"+Convert.ToBase64String(bytes);if(known.Add(key))images.Add(new GalleryImage{Source=key,Bytes=bytes,Caption=caption});break;}catch(FormatException){}
            }
        }
    }
    static string OutstandingText(string html,string noteHtml=null,string origin=null) {
        string text=Plain(html).Replace("\r\n"," ");var images=new List<GalleryImage>();
        CollectImages(images,html,"",origin);CollectImages(images,noteHtml,"",origin);
        return text+(images.Count>0?(text.Length==0?"":" · ")+images.Count+" 张图片":"");
    }
    static string OutstandingImageHtml(OutstandingLeaf leaf){return (leaf.Html??"")+(leaf.NoteHtml??"");}
    int OutstandingImageCount(OutstandingLeaf leaf) {
        if(leaf.CachedImageCount<0 || !Object.ReferenceEquals(leaf.ImageSummaryHtml,leaf.Html) || !Object.ReferenceEquals(leaf.ImageSummaryNote,leaf.NoteHtml)) {
            var images=new List<GalleryImage>();CollectImages(images,leaf.Html,"");CollectImages(images,leaf.NoteHtml,"");
            leaf.CachedImageCount=images.Count;leaf.ImageSummaryHtml=leaf.Html;leaf.ImageSummaryNote=leaf.NoteHtml;
        }
        return leaf.CachedImageCount;
    }
}
