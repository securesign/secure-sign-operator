package analyzer

import "golang.org/x/tools/go/analysis"

var Analyzers = []*analysis.Analyzer{
	PersistStatusErr,
	SpecMutation,
}
