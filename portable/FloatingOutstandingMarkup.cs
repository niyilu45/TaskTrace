// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Linq;
using System.Net;
using System.Text.RegularExpressions;

internal sealed partial class FloatingWindow {
    static string OutstandingAttribute(string tag,string name) {
        // Consume each complete attribute, including its quoted value. Marker-like
        // text inside title/alt must not be interpreted as a second attribute.
        foreach(Match attribute in Regex.Matches(tag??"", @"(?:\s)(?<name>[^\s=/>]+)(?:\s*=\s*(?:""(?<double>[^""]*)""|'(?<single>[^']*)'|(?<bare>[^\s>]+)))?",RegexOptions.Singleline)) {
            if(!String.Equals(attribute.Groups["name"].Value,name,StringComparison.OrdinalIgnoreCase))continue;
            return WebUtility.HtmlDecode(attribute.Groups["double"].Success?attribute.Groups["double"].Value:
                attribute.Groups["single"].Success?attribute.Groups["single"].Value:attribute.Groups["bare"].Value);
        }
        return null;
    }
    static bool IsSharedComment(string html) {
        html=html??"";
        foreach(Match token in ProgressTags.Matches(html)) {
            if(!Regex.IsMatch(token.Value,@"^<[a-z]",RegexOptions.IgnoreCase))continue;
            if(String.Equals(OutstandingAttribute(token.Value,"data-tasktrace-comment-type"),"outstanding",StringComparison.OrdinalIgnoreCase))return true;
            if(Regex.IsMatch(token.Value,@"^<aside\b",RegexOptions.IgnoreCase) &&
                OutstandingAttribute(token.Value,"data-tasktrace-outstanding-note")!=null)return true;
        }
        // A mention in a normal comment must never replace the actual shared list.
        return ProgressElementRanges(html,"h3").Any(heading=>
            WebUtility.HtmlDecode(ProgressTags.Replace(html.Substring(heading.InnerStart,heading.InnerEnd-heading.InnerStart),"")).Trim()==SharedHeading);
    }

    static List<SharedItemMarkup> SharedListItems(string html,HtmlRange list) {
        var items=new List<SharedItemMarkup>();
        string body=html.Substring(list.InnerStart,list.InnerEnd-list.InnerStart);
        int nestedLists=0,bodyStart=-1;string attrs=null;
        foreach(Match token in ProgressTags.Matches(body)) {
            var tag=Regex.Match(token.Value,@"^<(/?)(ul|ol|li)\b",RegexOptions.IgnoreCase);
            if(!tag.Success)continue;
            bool closing=tag.Groups[1].Value=="/";string name=tag.Groups[2].Value.ToLowerInvariant();
            if(name!="li"){nestedLists+=closing?-1:1;continue;}
            if(nestedLists!=0)continue;
            // HTML permits </li> to be omitted before the next sibling or </ul>.
            if(bodyStart>=0)items.Add(new SharedItemMarkup{Attrs=attrs,Body=body.Substring(bodyStart,token.Index-bodyStart)});
            bodyStart=closing?-1:token.Index+token.Length;
            attrs=closing?null:token.Value.Substring(3,token.Value.Length-4);
        }
        if(bodyStart>=0)items.Add(new SharedItemMarkup{Attrs=attrs,Body=body.Substring(bodyStart)});
        return items;
    }

    static IEnumerable<SharedItemMarkup> SharedItemMarkups(string html) {
        html=html??"";
        var lists=ProgressElementRanges(html,"ul");
        var headings=ProgressElementRanges(html,"h3");
        // Same list selection as the web reader: h3 + ul, then an explicit ID list.
        // Restrict to direct items so bullets in descriptions/memos do not become tasks.
        foreach(var list in lists) {
            if(!headings.Any(heading=>heading.End<=list.Start &&
                !ProgressTags.Matches(html.Substring(heading.End,list.Start-heading.End)).Cast<Match>().Any(token=>!token.Value.StartsWith("<!--",StringComparison.Ordinal))))continue;
            return SharedListItems(html,list);
        }
        foreach(var list in lists) {
            var items=SharedListItems(html,list);
            if(items.Any(item=>OutstandingAttribute("<li "+item.Attrs+">","data-id")!=null))return items;
        }
        return new SharedItemMarkup[0];
    }
}
