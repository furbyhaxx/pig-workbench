package main

import (
	"fmt"
	"os"
	ts "github.com/odvcencio/gotreesitter"
	l_go "github.com/odvcencio/gotreesitter/grammars/go"
	l_rust "github.com/odvcencio/gotreesitter/grammars/rust"
	l_python "github.com/odvcencio/gotreesitter/grammars/python"
	l_typescript "github.com/odvcencio/gotreesitter/grammars/typescript"
	l_tsx "github.com/odvcencio/gotreesitter/grammars/tsx"
	l_javascript "github.com/odvcencio/gotreesitter/grammars/javascript"
	l_c "github.com/odvcencio/gotreesitter/grammars/c"
	l_cpp "github.com/odvcencio/gotreesitter/grammars/cpp"
	l_c_sharp "github.com/odvcencio/gotreesitter/grammars/c_sharp"
	l_java "github.com/odvcencio/gotreesitter/grammars/java"
	l_kotlin "github.com/odvcencio/gotreesitter/grammars/kotlin"
	l_swift "github.com/odvcencio/gotreesitter/grammars/swift"
	l_ruby "github.com/odvcencio/gotreesitter/grammars/ruby"
	l_php "github.com/odvcencio/gotreesitter/grammars/php"
	l_bash "github.com/odvcencio/gotreesitter/grammars/bash"
	l_lua "github.com/odvcencio/gotreesitter/grammars/lua"
	l_toml "github.com/odvcencio/gotreesitter/grammars/toml"
	l_yaml "github.com/odvcencio/gotreesitter/grammars/yaml"
	l_json "github.com/odvcencio/gotreesitter/grammars/json"
	l_markdown "github.com/odvcencio/gotreesitter/grammars/markdown"
	l_html "github.com/odvcencio/gotreesitter/grammars/html"
	l_css "github.com/odvcencio/gotreesitter/grammars/css"
	l_scss "github.com/odvcencio/gotreesitter/grammars/scss"
	l_sql "github.com/odvcencio/gotreesitter/grammars/sql"
	l_zig "github.com/odvcencio/gotreesitter/grammars/zig"
	l_elixir "github.com/odvcencio/gotreesitter/grammars/elixir"
	l_haskell "github.com/odvcencio/gotreesitter/grammars/haskell"
	l_scala "github.com/odvcencio/gotreesitter/grammars/scala"
	l_ocaml "github.com/odvcencio/gotreesitter/grammars/ocaml"
	l_dart "github.com/odvcencio/gotreesitter/grammars/dart"
	l_nix "github.com/odvcencio/gotreesitter/grammars/nix"
	l_dockerfile "github.com/odvcencio/gotreesitter/grammars/dockerfile"
	l_make "github.com/odvcencio/gotreesitter/grammars/make"
	l_cmake "github.com/odvcencio/gotreesitter/grammars/cmake"
	l_hcl "github.com/odvcencio/gotreesitter/grammars/hcl"
	l_proto "github.com/odvcencio/gotreesitter/grammars/proto"
	l_graphql "github.com/odvcencio/gotreesitter/grammars/graphql"
	l_vue "github.com/odvcencio/gotreesitter/grammars/vue"
	l_svelte "github.com/odvcencio/gotreesitter/grammars/svelte"
	l_r "github.com/odvcencio/gotreesitter/grammars/r"
	l_julia "github.com/odvcencio/gotreesitter/grammars/julia"
	l_perl "github.com/odvcencio/gotreesitter/grammars/perl"
	l_powershell "github.com/odvcencio/gotreesitter/grammars/powershell"
	l_xml "github.com/odvcencio/gotreesitter/grammars/xml"
	l_ini "github.com/odvcencio/gotreesitter/grammars/ini"
)

func main() {
	langs := map[string]*ts.Language{
		"go": l_go.Language(),
		"rust": l_rust.Language(),
		"python": l_python.Language(),
		"typescript": l_typescript.Language(),
		"tsx": l_tsx.Language(),
		"javascript": l_javascript.Language(),
		"c": l_c.Language(),
		"cpp": l_cpp.Language(),
		"c_sharp": l_c_sharp.Language(),
		"java": l_java.Language(),
		"kotlin": l_kotlin.Language(),
		"swift": l_swift.Language(),
		"ruby": l_ruby.Language(),
		"php": l_php.Language(),
		"bash": l_bash.Language(),
		"lua": l_lua.Language(),
		"toml": l_toml.Language(),
		"yaml": l_yaml.Language(),
		"json": l_json.Language(),
		"markdown": l_markdown.Language(),
		"html": l_html.Language(),
		"css": l_css.Language(),
		"scss": l_scss.Language(),
		"sql": l_sql.Language(),
		"zig": l_zig.Language(),
		"elixir": l_elixir.Language(),
		"haskell": l_haskell.Language(),
		"scala": l_scala.Language(),
		"ocaml": l_ocaml.Language(),
		"dart": l_dart.Language(),
		"nix": l_nix.Language(),
		"dockerfile": l_dockerfile.Language(),
		"make": l_make.Language(),
		"cmake": l_cmake.Language(),
		"hcl": l_hcl.Language(),
		"proto": l_proto.Language(),
		"graphql": l_graphql.Language(),
		"vue": l_vue.Language(),
		"svelte": l_svelte.Language(),
		"r": l_r.Language(),
		"julia": l_julia.Language(),
		"perl": l_perl.Language(),
		"powershell": l_powershell.Language(),
		"xml": l_xml.Language(),
		"ini": l_ini.Language(),
	}
	src, _ := os.ReadFile(os.Args[2])
	tree, err := ts.NewParser(langs[os.Args[1]]).Parse(src)
	if err != nil { fmt.Println("ERR", err); os.Exit(1) }
	fmt.Println(os.Args[1], tree.RootNode().Type(langs[os.Args[1]]), tree.RootNode().ChildCount())
}
