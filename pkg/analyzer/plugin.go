package analyzer

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("actionlint", newPlugin)
}

func newPlugin(any) (register.LinterPlugin, error) {
	return &actionLintPlugin{}, nil
}

type actionLintPlugin struct{}

func (*actionLintPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return Analyzers, nil
}

func (*actionLintPlugin) GetLoadMode() string {
	return register.LoadModeSyntax
}
