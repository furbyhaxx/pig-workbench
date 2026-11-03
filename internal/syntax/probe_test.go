package syntax

import (
 "os"
 "path/filepath"
 "testing"
 ts "github.com/odvcencio/gotreesitter"
 "github.com/odvcencio/gotreesitter/grammars"
)
func TestProbe(t *testing.T) {
 fixtures:=map[string]string{
 "sample.go":"package sample\n\nfunc first(\n\targ int,\n) int {\n\tif true {\n\t\tprintln(1)\n\t}\n\treturn arg\n}\n\nfunc second() {}\n",
 "broken.go":"package sample\n\nfunc good() {\n\tprintln(1)\n}\n\nfunc broken() {\n\tif true {\n\t\tx :=\n\t}\n}\n",
 "sample.py":"@decorator\ndef outer():\n    if True:\n        print(1)\n    print(2)\n\ndef other():\n    return 3\n",
 "sample.md":"# One\nintro\n```go\n# Not a heading\n```\n## Nested\ntext\n\n# Two\nend\n",
 }
 for name, src:=range fixtures {
  p:=filepath.Join(t.TempDir(),name); if err:=os.WriteFile(p,[]byte(src),0600);err!=nil {t.Fatal(err)}; data,err:=os.ReadFile(p); if err!=nil {t.Fatal(err)}
  entry:=grammars.DetectLanguage(p); lang:=entry.Language(); tree,err:=ts.NewParser(lang).ParseStrict(data); if err!=nil {t.Fatal(err)}; if tree==nil {t.Fatal("nil tree")}
  t.Logf("--- %s (%s) ---",name,entry.Name)
  var walk func(*ts.Node,int); walk=func(n *ts.Node,d int){ if n==nil{return}; if n.IsNamed()||n.IsError()||n.IsMissing() {t.Logf("%*s%s %v-%v bytes=%d-%d err=%v missing=%v",d*2,"",n.Type(lang),n.StartPoint(),n.EndPoint(),n.StartByte(),n.EndByte(),n.IsError(),n.IsMissing())}; for i:=0;i<n.ChildCount();i++ {walk(n.Child(i),d+1)} }; walk(tree.RootNode(),0); tree.Release()
 }
}
