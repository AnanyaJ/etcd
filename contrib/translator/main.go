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

const (
	GetAnnotation = "@get"
	PutAnnotation = "@put"
)

func formatNode(node ast.Node) string {
	buf := new(bytes.Buffer)
	_ = format.Node(buf, token.NewFileSet(), node)
	return buf.String()
}

func findMatchingComment(commGroups []*ast.CommentGroup, text string) *string {
	for _, commGroup := range commGroups {
		for _, comment := range commGroup.List {
			if strings.Contains(comment.Text, text) {
				return &comment.Text
			}
		}

	}
	return nil
}

// signature of input function that performs access
func getAccessInputFuncType() *ast.FuncType {
	return &ast.FuncType{
		Params: &ast.FieldList{},
		Results: &ast.FieldList{List: []*ast.Field{
			&ast.Field{
				Type: ast.NewIdent("any"),
			},
		}},
	}
}

func getAccessCallExpr(input *ast.FuncLit) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  &ast.Ident{Name: "access"},
		Args: []ast.Expr{input},
	}
}

func wrapGetAccess(node ast.Node, getComment string) {
	assignStmt, ok := node.(*ast.AssignStmt)
	if !ok {
		log.Fatalf("%s can only be used to annotate an assignment", GetAnnotation)
	}
	if len(assignStmt.Lhs) != 1 {
		log.Fatalf("%s must perform assignment to exactly one value", GetAnnotation)
	}

	// wrap RHS of assignment in function literal
	accessCallLit := &ast.FuncLit{
		Type: getAccessInputFuncType(),
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ReturnStmt{
				Results: assignStmt.Rhs,
			},
		}},
	}

	// pass function literal to provided access function so RSM library can mediate access
	accessCall := getAccessCallExpr(accessCallLit)

	commentComponents := strings.Split(getComment, " ")
	if len(commentComponents) < 3 {
		log.Fatalf("%s annotation must be followed by type of assignment", GetAnnotation)
	}
	varType := commentComponents[2]

	// assert type to match LHS of assignment
	typedAccessCall := &ast.TypeAssertExpr{
		X:    accessCall,
		Type: &ast.Ident{Name: varType},
	}

	assignStmt.Rhs = []ast.Expr{typedAccessCall}
}

func wrapPutAccess(file *ast.File, node ast.Node) {
	stmt, ok := node.(ast.Stmt)
	if !ok {
		log.Fatalf("%s can only be used to annotate statements", PutAnnotation)
	}

	// wrap put statement in function literal
	accessCallLit := &ast.FuncLit{
		Type: getAccessInputFuncType(),
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				stmt,
				// return some arbitrary value to satisfy required func signature
				&ast.ReturnStmt{
					Results: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "1"}},
				},
			},
		},
	}

	accessCall := getAccessCallExpr(accessCallLit)

	// replace existing put node with access call node
	astutil.Apply(file, func(cr *astutil.Cursor) bool {
		if cr.Node() == node {
			cr.Replace(&ast.ExprStmt{X: accessCall})
			return false
		}
		return true
	}, nil)
}

func modifyAST(file *ast.File, fset *token.FileSet) {
	cmap := ast.NewCommentMap(fset, file, file.Comments)
	for node, cGroup := range cmap {
		getComment := findMatchingComment(cGroup, GetAnnotation)
		if getComment != nil {
			wrapGetAccess(node, *getComment)
		}
		if findMatchingComment(cGroup, PutAnnotation) != nil {
			wrapPutAccess(file, node)
		}
	}
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

	modifyAST(file, fset)

	outFile, err := os.Create(*outputfile)
	if err != nil {
		log.Fatalf("Error opening output file %s: %v", *outputfile, err)
	}
	defer outFile.Close()

	_, err = outFile.WriteString(formatNode(file))
	if err != nil {
		log.Fatalf("Error writing to output file %s: %v", *outputfile, err)
	}
}
