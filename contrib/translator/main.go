package main

import (
	"bytes"
	"flag"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"log"
	"os"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

func formatNode(node ast.Node) string {
	buf := new(bytes.Buffer)
	_ = format.Node(buf, token.NewFileSet(), node)
	return buf.String()
}

func findMatch(commGroups []*ast.CommentGroup, text string) *string {
	for _, commGroup := range commGroups {
		for _, comment := range commGroup.List {
			if strings.Contains(comment.Text, text) {
				return &comment.Text
			}
		}

	}
	return nil
}

func main() {
	inputfile := flag.String("inputfile", "../lockserver/lockservertranslate.go", "Go RSM application file to translate")
	outputfile := flag.String("outputfile", "../lockserver/lockservertranslated.go", "Path to write generated RSM application file to")
	flag.Parse()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, *inputfile, nil, parser.ParseComments)
	if err != nil {
		log.Fatalf("Could not open file %s for translation", inputfile)
	}

	funcType := &ast.FuncType{
		Params: &ast.FieldList{},
		Results: &ast.FieldList{List: []*ast.Field{
			&ast.Field{
				Type: ast.NewIdent("any"),
			},
		}},
	}

	cmap := ast.NewCommentMap(fset, file, file.Comments)
	for node, cGroup := range cmap {
		matchingComment := findMatch(cGroup, "@get")
		if matchingComment != nil {
			assignStmt, ok := node.(*ast.AssignStmt)
			if !ok {
				log.Fatalf("@get can only be used to annotate an assignment")
			}

			returnStmt := &ast.ReturnStmt{
				Results: assignStmt.Rhs,
			}

			funcLit := &ast.FuncLit{
				Type: funcType,
				Body: &ast.BlockStmt{List: []ast.Stmt{returnStmt}},
			}

			accessCall := &ast.CallExpr{
				Fun:  &ast.Ident{Name: "access"},
				Args: []ast.Expr{funcLit},
			}

			if len(assignStmt.Lhs) != 1 {
				log.Fatalf("@get must perform assignment to exactly one value")
			}

			commentComponents := strings.Split(*matchingComment, " ")
			if len(commentComponents) < 3 {
				log.Fatalf("@get must be followed by type of assignment")
			}

			typedAccessCall := &ast.TypeAssertExpr{
				X:    accessCall,
				Type: &ast.Ident{Name: commentComponents[2]},
			}

			assignStmt.Rhs = []ast.Expr{typedAccessCall}
		}

		if findMatch(cGroup, "@put") != nil {
			stmt, ok := node.(ast.Stmt)
			if !ok {
				log.Fatalf("@put can only be used to annotate statements")
			}

			returnStmt := &ast.ReturnStmt{
				Results: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "1"}},
			}

			funcBody := &ast.BlockStmt{List: []ast.Stmt{stmt, returnStmt}}

			funcLit := &ast.FuncLit{
				Type: funcType,
				Body: funcBody,
			}

			accessCall := &ast.CallExpr{
				Fun:  &ast.Ident{Name: "access"},
				Args: []ast.Expr{funcLit},
			}

			astutil.Apply(file, func(cr *astutil.Cursor) bool {
				if cr.Node() == node {
					cr.Replace(&ast.ExprStmt{X: accessCall})
					return false
				}
				return true
			}, nil)
		}
	}

	outFile, err := os.Create(*outputfile)
	if err != nil {
		log.Fatalf("Error opening output file %s: %v", *outputfile, err)
	}
	defer outFile.Close() // Ensure the file is closed when we're done.

	_, err = outFile.WriteString(formatNode(file))
	if err != nil {
		log.Fatalf("Error writing to output file %s: %v", *outputfile, err)
	}
}
