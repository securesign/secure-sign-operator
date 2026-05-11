package specmutation_crosspackage

import (
	"context"

	"specmutation_crosspackage/helpers"
)

type Result struct{}

type BaseAction struct{}

func (*BaseAction) Return() *Result { return nil }

type MySpec struct {
	Config *helpers.Config
}

type MyStatus struct {
	Ready bool
}

type MyInstance struct {
	Spec   MySpec
	Status MyStatus
}

type crossPkgAction struct {
	BaseAction
}

func (a *crossPkgAction) Handle(ctx context.Context, instance *MyInstance) *Result {
	helpers.MutateConfig(instance.Spec.Config) // want "action type must not mutate instance.Spec; only Status is persisted by the action framework"

	helpers.ReadConfig(instance.Spec.Config) // OK — ReadConfig does not mutate

	return a.Return()
}

// Whole-object cross-package test: action type uses helpers.Instance.
type crossPkgObjectAction struct {
	BaseAction
}

func (a *crossPkgObjectAction) Handle(ctx context.Context, instance *helpers.Instance) *Result {
	helpers.MutateSpecViaObject(instance) // want "action type must not mutate instance.Spec; only Status is persisted by the action framework"

	helpers.MutateStatusViaObject(instance) // OK — only Status is mutated

	return a.Return()
}
