package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"log"
	"math/rand"
	"os"
	"strconv"
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

func getAccessReturnType() ast.Expr {
	return &ast.ArrayType{
		Elt: ast.NewIdent("any"),
	}
}

// signature of input function that performs access
func getAccessInputFuncType() *ast.FuncType {
	return &ast.FuncType{
		Params: &ast.FieldList{},
		Results: &ast.FieldList{List: []*ast.Field{
			&ast.Field{
				Type: getAccessReturnType(),
			},
		}},
	}
}

func getAccessCallExpr(input *ast.FuncLit) ast.Expr {
	return &ast.CallExpr{
		Fun:  &ast.Ident{Name: "access"},
		Args: []ast.Expr{input},
	}
}

func wrapGetAccess(file *ast.File, node ast.Node, getComment string) {
	assignStmt, ok := node.(*ast.AssignStmt)
	if !ok {
		log.Fatalf("%s can only be used to annotate an assignment", GetAnnotation)
	}

	// wrap RHS of assignment in function literal
	accessCallLit := &ast.FuncLit{
		Type: getAccessInputFuncType(),
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: assignStmt.Lhs,
				Tok: token.DEFINE,
				Rhs: assignStmt.Rhs,
			},
			&ast.ReturnStmt{
				Results: []ast.Expr{
					&ast.CompositeLit{
						Type: getAccessReturnType(),
						Elts: assignStmt.Lhs,
					},
				},
			},
		}},
	}

	// pass function literal to provided access function so RSM library can mediate access
	accessCall := getAccessCallExpr(accessCallLit)

	// create unique variable name to store return values array
	retValsVar := ast.NewIdent(fmt.Sprintf("retVals%d", rand.Int()))

	// assign access result to return values variable
	retValsAssignment := &ast.AssignStmt{
		Lhs: []ast.Expr{retValsVar},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{accessCall},
	}

	numRetVals := len(assignStmt.Lhs)

	commentComponents := strings.Split(getComment, " ")
	if len(commentComponents) < 2+numRetVals {
		log.Fatalf("%s annotation must be followed by types of assignment", GetAnnotation)
	}

	getStmts := []ast.Stmt{}

	// deconstruct return values
	for i := numRetVals - 1; i >= 0; i-- {
		// assert type to match LHS
		typedAccessCall := &ast.TypeAssertExpr{
			X: &ast.IndexExpr{
				X: retValsVar,
				Index: &ast.BasicLit{
					Kind:  token.INT,
					Value: strconv.Itoa(i),
				},
			},
			Type: &ast.Ident{Name: commentComponents[i+2]},
		}

		retValAssignment := &ast.AssignStmt{
			Lhs: []ast.Expr{assignStmt.Lhs[i]},
			Tok: assignStmt.Tok,
			Rhs: []ast.Expr{typedAccessCall},
		}

		getStmts = append(getStmts, retValAssignment)
	}

	// replace existing put node with access call and assignments
	astutil.Apply(file, func(cr *astutil.Cursor) bool {
		if cr.Node() == node {
			cr.Replace(retValsAssignment)
			for _, stmt := range getStmts {
				cr.InsertAfter(stmt)
			}
			return false
		}
		return true
	}, nil)
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
				&ast.ReturnStmt{
					Results: []ast.Expr{
						&ast.CompositeLit{
							Type: getAccessReturnType(),
							Elts: []ast.Expr{},
						},
					},
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
			wrapGetAccess(file, node, *getComment)
		}
		if findMatchingComment(cGroup, PutAnnotation) != nil {
			wrapPutAccess(file, node)
		}
	}
}

func main() {
	inputfile := flag.String("inputfile", "../lockserver/lockserverrepl.go", "Go RSM application file to translate")
	outputfile := flag.String("outputfile", "../lockserver/lockserverrepltranslated.go", "Path to write generated RSM application file to")
	flag.Parse()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, *inputfile, nil, parser.ParseComments)
	if err != nil {
		log.Fatalf("Could not open file %s for translation", *inputfile)
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
