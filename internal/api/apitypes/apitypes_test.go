package apitypes

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// TestNullableFieldsAreRequiredInTS checks the tstype tags that tygo needs to
// match what encoding/json sends (ADR 0001). tygo makes every pointer field
// optional (field?: T), but a pointer without omitempty is always sent, as
// null when unset, so it must be tagged tstype:"T | null,required".
func TestNullableFieldsAreRequiredInTS(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return false
			}
			for _, f := range st.Fields.List {
				if _, ok := f.Type.(*ast.StarExpr); !ok || f.Tag == nil {
					continue
				}
				raw, err := strconv.Unquote(f.Tag.Value)
				if err != nil {
					t.Fatal(err)
				}
				tag := reflect.StructTag(raw)
				if strings.Contains(tag.Get("json"), "omitempty") {
					continue
				}
				checked++
				tstype := tag.Get("tstype")
				name := ts.Name.Name + "." + f.Names[0].Name
				if !strings.Contains(tstype, "| null") || !strings.HasSuffix(tstype, ",required") {
					t.Errorf("%s: always-sent pointer field has tstype %q, want \"T | null,required\"", name, tstype)
				}
			}
			return false
		})
	}
	if checked == 0 {
		t.Error("found no pointer fields to check")
	}
}
