package main

import (
	"fmt"

	"github.com/ncruces/go-sqlite3"
	_ "github.com/ncruces/go-sqlite3/embed"
)

func main() {
	db, err := sqlite3.OpenFlags("file:/tmp/wb-backends/t.db?mode=ro&immutable=0", sqlite3.OPEN_READONLY|sqlite3.OPEN_URI)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	db.SetAuthorizer(func(action sqlite3.AuthorizerActionCode, a3, a4, schema, inner string) sqlite3.AuthorizerReturnCode {
		switch action {
		case sqlite3.AUTH_SELECT, sqlite3.AUTH_READ:
			return sqlite3.AUTH_OK
		case sqlite3.AUTH_FUNCTION:
			switch a4 {
			case "load_extension", "readfile", "writefile", "edit", "fts3_tokenizer":
				return sqlite3.AUTH_DENY
			}
			return sqlite3.AUTH_OK
		}
		return sqlite3.AUTH_DENY
	})
	for _, q := range []string{
		"SELECT id, name, big, price FROM users WHERE id = 2",
		"SELECT count(*) FROM users",
		"DELETE FROM users",
		"ATTACH DATABASE '/tmp/x.db' AS x",
		"SELECT load_extension('/tmp/evil.so')",
		"PRAGMA writable_schema=1",
		"SELECT 1; DELETE FROM users",
	} {
		stmt, tail, err := db.Prepare(q)
		if err != nil {
			fmt.Printf("%-45s -> rejected: %v\n", q, err)
			continue
		}
		row := []string{}
		if stmt.Step() {
			for i := 0; i < stmt.ColumnCount(); i++ {
				row = append(row, fmt.Sprintf("%s=%s", stmt.ColumnName(i), stmt.ColumnRawText(i)))
			}
		}
		fmt.Printf("%-45s -> %v tail=%q err=%v\n", q, row, tail, stmt.Err())
		stmt.Close()
	}
}
