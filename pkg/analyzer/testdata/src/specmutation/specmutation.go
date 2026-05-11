package specmutation

import "context"

type Result struct{}

type BaseAction struct{}

func (*BaseAction) PersistStatus(ctx context.Context, obj any) (bool, error) {
	return false, nil
}
func (*BaseAction) Return() *Result { return nil }

type MySpec struct {
	Host    string
	Address string
}

type MyStatus struct {
	Ready bool
}

type MyInstance struct {
	Spec   MySpec
	Status MyStatus
}

type myAction struct {
	BaseAction
}

func (a *myAction) Handle(ctx context.Context, instance *MyInstance) *Result {
	instance.Spec.Host = "bad" // want "action type must not mutate instance.Spec; only Status is persisted by the action framework"

	instance.Status.Ready = true // OK — Status is persisted

	return a.Return()
}

type deepAction struct {
	BaseAction
}

func (a *deepAction) Handle(ctx context.Context, instance *MyInstance) *Result {
	instance.Spec.Address = "also bad" // want "action type must not mutate instance.Spec; only Status is persisted by the action framework"
	return a.Return()
}

// Helper method on action type — SHOULD be flagged.
func (a *myAction) helper(instance *MyInstance) {
	instance.Spec.Host = "bad in helper" // want "action type must not mutate instance.Spec; only Status is persisted by the action framework"
}

// Handle with wrong return type — not an action type, should not be flagged.
type notAnAction struct {
	BaseAction
}

func (a *notAnAction) HandleOther(ctx context.Context, instance *MyInstance) error {
	instance.Spec.Host = "not an action Handle"
	return nil
}

// Method on non-action type — should not be flagged.
type plainType struct{}

func (p *plainType) DoSomething(instance *MyInstance) {
	instance.Spec.Host = "ok on non-action type"
}
