import json, subprocess, sys
import tree_sitter_language_pack as p
refname={"bash":"bash","c":"c","cpp":"cpp","css":"css","diff":"diff","elisp":"elisp","fortran":"fortran","go":"go","html":"html","java":"java","javascript":"javascript","json":"json","lua":"lua","markdown":"markdown","python":"python","regex":"regex","ruby":"ruby","rust":"rust","swift":"swift","toml":"toml","tsx":"tsx","typescript":"typescript","yaml":"yaml"}
def ref_nodes(lang, src):
    t=p.get_parser(refname[lang]).parse(src)
    out=[]
    def walk(n):
        if n.is_named or n.type=="ERROR": out.append((n.type,n.start_point[0],n.start_point[1],n.end_point[0],n.end_point[1]))
        for c in n.children: walk(c)
    walk(t.root_node); return out
def anchors(nodes, lines):
    a={}
    for (ty,sr,sc,er,ec) in nodes[1:]:
        if er==sr or sr>=len(lines): continue
        ind=len(lines[sr])-len(lines[sr].lstrip(b" \t"))
        if sc!=ind: continue
        a.setdefault(sr,(ty,er))
    return a
tot_same=tot_ref=0
for line in open(sys.argv[1]):
    lang,path=line.strip().split(":",1)
    if lang=="java" and "bison" in path: continue
    src=open(path,"rb").read()
    ref=ref_nodes(lang,src)
    got=[tuple(x) for x in json.loads(subprocess.check_output(["/tmp/wb-backends/syntax-dump",lang,path]))]
    rs,gs=set(ref),set(got)
    lines=src.split(b"\n")
    ra,ga=anchors(ref,lines),anchors(got,lines)
    agree=sum(1 for k in ra if ga.get(k)==ra[k]);
    rerr=sum(1 for n in ref if n[0]=="ERROR"); gerr=sum(1 for n in got if n[0]=="ERROR")
    diff=[k for k in sorted(set(ra)|set(ga)) if ra.get(k)!=ga.get(k)][:2]
    ex=[(k+1,ra.get(k),ga.get(k)) for k in diff]
    print(f"{lang:10} nodes ref={len(ref):6} same={len(rs&gs)/max(1,len(rs)):.3f}  anchors ref={len(ra):4} agree={agree/max(1,len(ra)):.3f}  errors ref={rerr} got={gerr}  {ex if ex else ''}")
