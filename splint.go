// splint is a little Go application to analyze Go source files.  It finds any functions that are
// too long or have too many parameters or results.
//
// find . -name "*.go" -exec splint {} \;
// By default, splint will inform you of any functions that are more than 30 statements long, have more than five parameters, or have more than five results.
//
// You can change these values with command line flags. -s sets the statement count threshold, -p sets the parameter count threshold, and -r sets the result count threshold.
// Check for all functions with more than 50 statements, 10 parameters, 7 results:
// splint -s=50 -p=10 -r=7 **/*.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
)

var statementThreshold = flag.Int("s", 30, "function statement count threshold")
var paramThreshold = flag.Int("p", 5, "parameter list length threshold")
var resultThreshold = flag.Int("r", 5, "result list length threshold")
var ifChainThreshold = flag.Int("c", 2, "if/else chain length threshold")
var ifBodyThreshold = flag.Int("f", 20, "if body statement count threshold")
var skipBoolParamCheck = flag.Bool("b", false, "don't warn on bool function params")
var outputJSON = flag.Bool("j", false, "output results as json")
var ignoreTestFiles = flag.Bool("i", true, "ignore test files")
var outputSummary = flag.Bool("sum", false, "output summary")

// Parser parses go source files, looking for potentially complex
// code.
type Parser struct {
	filename string
	first    bool
	summary  *Summary
	fileset  *token.FileSet
}

// Offender contains the file, function, position, and count of
// a block of code that splint has recognized as an issue.
type Offender struct {
	Filename string
	Function string
	Count    int
	Position token.Position
}

func (o *Offender) warning(msg string) {
	if *outputJSON {
		return
	}
	fmt.Printf("%s:\tfunction %s %s: %d\n", o.Position, o.Function, msg, o.Count)
}

func (o *Offender) warnNoCount(msg string) {
	if *outputJSON {
		return
	}
	fmt.Printf("%s:\tfunction %s %s\n", o.Position, o.Function, msg)
}

// Summary is a collection of Offenders for all the different
// checks that splint performs.
type Summary struct {
	Statement  []*Offender
	Param      []*Offender
	Result     []*Offender
	EmptyIfs   []*Offender
	IfChains   []*Offender
	BoolParams []*Offender
	LongIfs    []*Offender

	// redundant, but using these for easy json output
	NumAboveStatementThreshold int
	NumAboveParamThreshold     int
	NumAboveResultThreshold    int
	NumIfChains                int
	NumEmptyIfs                int
	NumWithBoolParams          int
	NumLongIfs                 int
}

func (s *Summary) IsClean() bool {
	base := len(s.Statement) == 0 && len(s.Param) == 0 && len(s.Result) == 0 && len(s.EmptyIfs) == 0 && len(s.IfChains) == 0 && len(s.LongIfs) == 0
	if *skipBoolParamCheck {
		return base
	}
	return base && len(s.BoolParams) == 0
}

func (s *Summary) addStatement(o *Offender) {
	s.Statement = append(s.Statement, o)
	s.NumAboveStatementThreshold++
	o.warning("too long")
}

func (s *Summary) addParam(o *Offender) {
	s.Param = append(s.Param, o)
	s.NumAboveParamThreshold++
	o.warning("too many params")
}

func (s *Summary) addBoolParam(o *Offender) {
	s.BoolParams = append(s.BoolParams, o)
	s.NumWithBoolParams++
	o.warnNoCount("bool function param")
}

func (s *Summary) addResult(o *Offender) {
	s.Result = append(s.Result, o)
	s.NumAboveResultThreshold++
	o.warning("too many results")
}

func (s *Summary) addEmptyIfBody(o *Offender) {
	s.EmptyIfs = append(s.EmptyIfs, o)
	s.NumEmptyIfs++
	o.warnNoCount("if with empty body")
}

func (s *Summary) addLongIfBody(o *Offender) {
	s.LongIfs = append(s.LongIfs, o)
	s.NumLongIfs++
	o.warnNoCount("if with long body")
}

func (s *Summary) addIfChain(o *Offender) {
	s.IfChains = append(s.IfChains, o)
	s.NumIfChains++
	o.warning("long if/else chain")
}

// NewParser creates a splint parser for a file.
func NewParser(filename string, summary *Summary) *Parser {
	return &Parser{filename: filename, first: true, summary: summary}
}

func statementCount(n ast.Node) int {
	total := 0
	counter := func(node ast.Node) bool {
		switch node.(type) {
		case ast.Stmt:
			total++
		}
		return true
	}
	ast.Inspect(n, counter)
	return total
}

func (p *Parser) offender(function string, count int, pos token.Pos) *Offender {
	return &Offender{
		Filename: p.filename,
		Function: function,
		Count:    count,
		Position: p.fileset.Position(pos),
	}
}

func (p *Parser) checkFuncLength(x *ast.FuncDecl) {
	numStatements := statementCount(x)
	if numStatements <= *statementThreshold {
		return
	}

	p.summary.addStatement(p.offender(x.Name.String(), numStatements, x.Pos()))
}

func (p *Parser) checkParamCount(x *ast.FuncDecl) {
	numFields := x.Type.Params.NumFields()
	if numFields <= *paramThreshold {
		return
	}

	p.summary.addParam(p.offender(x.Name.String(), numFields, x.Pos()))
}

func (p *Parser) checkBoolParams(x *ast.FuncDecl) {
	if *skipBoolParamCheck {
		return
	}
	for _, f := range x.Type.Params.List {
		// this is ugly, but:
		if fmt.Sprintf("%s", f.Type) != "bool" {
			continue
		}
		p.summary.addBoolParam(p.offender(x.Name.String(), 0, x.Pos()))
	}
}

func (p *Parser) checkResultCount(x *ast.FuncDecl) {
	numResults := x.Type.Results.NumFields()
	if numResults <= *resultThreshold {
		return
	}

	p.summary.addResult(p.offender(x.Name.String(), numResults, x.Pos()))
}

func (p *Parser) checkEmptyIfs(x *ast.FuncDecl) {
	findIf := func(node ast.Node) bool {
		switch y := node.(type) {
		case *ast.IfStmt:
			if y.Body == nil || len(y.Body.List) == 0 {
				p.summary.addEmptyIfBody(p.offender(x.Name.String(), 0, y.Pos()))
			} else if statementCount(y.Body) > *ifBodyThreshold {
				p.summary.addLongIfBody(p.offender(x.Name.String(), 0, y.Pos()))
			}
		}
		return true
	}
	ast.Inspect(x, findIf)
}

func chainLength(x *ast.IfStmt) int {
	if x.Else == nil {
		return 0
	}
	if ifst, ok := x.Else.(*ast.IfStmt); ok {
		return 1 + chainLength(ifst)
	}
	return 1
}

func (p *Parser) checkIfChains(x *ast.FuncDecl) {
	findIf := func(node ast.Node) bool {
		switch y := node.(type) {
		case *ast.IfStmt:
			n := chainLength(y)
			if n > *ifChainThreshold {
				p.summary.addIfChain(p.offender(x.Name.String(), n, y.Pos()))
			}
			return false // don't go any deeper
		}
		return true
	}
	ast.Inspect(x, findIf)
}

func (p *Parser) examineFunc(x *ast.FuncDecl) {
	p.checkFuncLength(x)
	p.checkParamCount(x)
	p.checkBoolParams(x)
	p.checkResultCount(x)
	p.checkEmptyIfs(x)
	p.checkIfChains(x)
}

func (p *Parser) examineDecls(tree *ast.File) {
	for _, v := range tree.Decls {
		switch x := v.(type) {
		case *ast.FuncDecl:
			p.examineFunc(x)
		}
	}
}

// Parse parses a file, looking for issues in functions.
func (p *Parser) Parse() {
	p.fileset = token.NewFileSet()
	tree, err := parser.ParseFile(p.fileset, p.filename, nil, 0)
	if err != nil {
		fmt.Printf("error parsing %s: %s\n", p.filename, err)
		return
	}

	p.examineDecls(tree)
}

func isTestFile(filename string) bool {
	base := path.Base(filename)
	match, err := path.Match("*_test.go", base)
	if err != nil {
		fmt.Println("match error:", err)
		return false
	}
	return match
}

func parseFile(filename string, summary *Summary) {
	if *ignoreTestFiles && isTestFile(filename) {
		return
	}
	parser := NewParser(filename, summary)
	parser.Parse()
}

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		fmt.Println("Usage: splint [options] <go file>...")
		flag.PrintDefaults()
		os.Exit(1)
	}

	summary := new(Summary)

	for _, v := range args {
		parseFile(v, summary)
	}

	if *outputJSON {
		data, err := json.MarshalIndent(summary, "", "\t")
		if err != nil {
			fmt.Println("json encode error:", err)
		}
		fmt.Println(string(data))

	} else if *outputSummary {
		fmt.Println()
		fmt.Println("Number of functions above statement threshold:", summary.NumAboveStatementThreshold)
		fmt.Println("Number of functions above param threshold:", summary.NumAboveParamThreshold)
		fmt.Println("Number of functions above result threshold:", summary.NumAboveResultThreshold)
		fmt.Println("Number of long if/else chains:", summary.NumIfChains)
		fmt.Println("Number of empty if bodies:", summary.NumEmptyIfs)
		fmt.Println("Number of long if bodies:", summary.NumLongIfs)
		if !*skipBoolParamCheck {
			fmt.Println("Number of functions with bool params:", summary.NumWithBoolParams)
		}
		if !summary.IsClean() {
			os.Exit(1)
		}
	}
}
