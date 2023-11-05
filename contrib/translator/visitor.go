package main

import (
	"go/ast"
)

type Visitor struct {
}

func (v Visitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}

	return v
}
