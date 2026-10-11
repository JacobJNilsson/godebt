package inventory

import (
	"go/ast"
	"go/scanner"
	"go/token"
	"strings"
)

// complexity returns 1 plus the decision points of spec 002 in a body, without nested function literals.
func complexity(body ast.Node) int {
	cc := 1
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			cc++
		case *ast.SwitchStmt:
			if n.Tag != nil {
				cc += flatAdjust(n.Body)
			}
		case *ast.TypeSwitchStmt:
			cc += flatAdjust(n.Body)
		case *ast.CaseClause, *ast.CommClause:
			cc += clauseCount(n)
		case *ast.BinaryExpr:
			if n.Op == token.LAND || n.Op == token.LOR {
				cc++
			}
		}
		return true
	})
	return cc
}

// clauseCount returns 1 for a case or select clause that is not the default.
func clauseCount(n ast.Node) int {
	switch n := n.(type) {
	case *ast.CaseClause:
		// A nil list marks the default clause.
		if n.List != nil {
			return 1
		}
	case *ast.CommClause:
		if n.Comm != nil {
			return 1
		}
	}
	return 0
}

// flatAdjust returns the cc correction that makes a flat switch count 1 in total, or 0 when the switch is not flat.
func flatAdjust(body *ast.BlockStmt) int {
	counted := 0
	for _, stmt := range body.List {
		clause, ok := stmt.(*ast.CaseClause)
		if !ok || !trivialBody(clause.Body) {
			return 0
		}
		if clause.List != nil {
			counted++
		}
	}
	if counted == 0 {
		return 0
	}
	return 1 - counted
}

// trivialBody reports whether a clause body is empty or one return, assignment, or call without a decision point.
func trivialBody(stmts []ast.Stmt) bool {
	if len(stmts) == 0 {
		return true
	}
	if len(stmts) > 1 {
		return false
	}
	switch stmt := stmts[0].(type) {
	case *ast.ReturnStmt, *ast.AssignStmt:
	case *ast.ExprStmt:
		if _, ok := ast.Unparen(stmt.X).(*ast.CallExpr); !ok {
			return false
		}
	default:
		return false
	}
	return !hasDecision(stmts[0])
}

// hasDecision reports whether a node holds a function literal, && or ||.
func hasDecision(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			found = true
		case *ast.BinaryExpr:
			found = found || n.Op == token.LAND || n.Op == token.LOR
		}
		return !found
	})
	return found
}

// nesting returns the deepest chain of if, for, switch, and select statements below a node at the given depth.
func nesting(node ast.Node, depth int) int {
	deepest := depth
	ast.Inspect(node, func(n ast.Node) bool {
		if n == node {
			return true
		}
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.IfStmt:
			deepest = max(deepest, ifNesting(n, depth))
			return false
		case *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			deepest = max(deepest, nesting(n, depth+1))
			return false
		}
		return true
	})
	return deepest
}

// ifNesting keeps an else-if chain at the depth of its first if.
func ifNesting(stmt *ast.IfStmt, depth int) int {
	deepest := depth + 1
	for _, part := range []ast.Node{stmt.Init, stmt.Cond, stmt.Body} {
		if part != nil {
			deepest = max(deepest, nesting(part, depth+1))
		}
	}
	switch els := stmt.Else.(type) {
	case *ast.IfStmt:
		deepest = max(deepest, ifNesting(els, depth))
	case *ast.BlockStmt:
		deepest = max(deepest, nesting(els, depth+1))
	}
	return deepest
}

// codeLines marks the lines of a file that hold at least one token that is not a comment, indexed by line number.
// A multi-line string literal marks every line it spans.
func codeLines(file *token.File, src []byte) []bool {
	var lines []bool
	var s scanner.Scanner
	s.Init(file, src, nil, 0)
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			return lines
		}
		start := file.PositionFor(pos, false).Line
		end := start
		if tok == token.STRING {
			end += strings.Count(lit, "\n")
		}
		for len(lines) <= end {
			lines = append(lines, false)
		}
		for line := start; line <= end; line++ {
			lines[line] = true
		}
	}
}
