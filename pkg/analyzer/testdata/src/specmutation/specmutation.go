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

func (a *myAction) Handle(ctx context.Context, instance *MyInstance) *Result { // want Handle:"mutatesParam"
	instance.Spec.Host = "bad" // want "action type must not mutate instance.Spec; only Status is persisted by the action framework"

	instance.Status.Ready = true // OK — Status is persisted

	return a.Return()
}

type deepAction struct {
	BaseAction
}

func (a *deepAction) Handle(ctx context.Context, instance *MyInstance) *Result { // want Handle:"mutatesParam"
	instance.Spec.Address = "also bad" // want "action type must not mutate instance.Spec; only Status is persisted by the action framework"
	return a.Return()
}

// Helper method on action type — SHOULD be flagged.
func (a *myAction) helper(instance *MyInstance) { // want helper:"mutatesParam"
	instance.Spec.Host = "bad in helper" // want "action type must not mutate instance.Spec; only Status is persisted by the action framework"
}

// Handle with wrong return type — not an action type, should not be flagged.
type notAnAction struct {
	BaseAction
}

func (a *notAnAction) HandleOther(ctx context.Context, instance *MyInstance) error { // want HandleOther:"mutatesParam"
	instance.Spec.Host = "not an action Handle"
	return nil
}

// Method on non-action type — should not be flagged.
type plainType struct{}

func (p *plainType) DoSomething(instance *MyInstance) { // want DoSomething:"mutatesParam"
	instance.Spec.Host = "ok on non-action type"
}

// --- Indirect spec mutation tests ---

// Free function that mutates its pointer parameter.
func mutateSpec(spec *MySpec) { // want mutateSpec:"mutatesParam"
	spec.Host = "mutated"
}

// Free function that only reads its pointer parameter.
func readSpec(spec *MySpec) string {
	return spec.Host
}

// Action that calls a mutating free function with &instance.Spec — SHOULD be flagged.
type indirectAction struct {
	BaseAction
}

func (a *indirectAction) Handle(ctx context.Context, instance *MyInstance) *Result {
	mutateSpec(&instance.Spec) // want "action type must not mutate instance.Spec; only Status is persisted by the action framework"

	readSpec(&instance.Spec) // OK — readSpec does not mutate

	return a.Return()
}

// Action helper method that mutates via pointer param — calling it SHOULD be flagged.
func (a *indirectAction) mutateHelper(spec *MySpec) { // want mutateHelper:"mutatesParam"
	spec.Address = "mutated by helper"
}

func (a *indirectAction) HandleWithHelper(ctx context.Context, instance *MyInstance) *Result {
	a.mutateHelper(&instance.Spec) // want "action type must not mutate instance.Spec; only Status is persisted by the action framework"
	return a.Return()
}

// Non-action type calling a mutating function — NOT flagged.
func (p *plainType) CallMutate(instance *MyInstance) {
	mutateSpec(&instance.Spec) // OK — plainType is not an action
}

// Action passing a spec sub-field pointer to a mutating function.
type SubField struct {
	Value string
}

type InstanceWithPtrSpec struct {
	Spec   SpecWithPtr
	Status MyStatus
}

type SpecWithPtr struct {
	Sub *SubField
}

func mutateSubField(s *SubField) { // want mutateSubField:"mutatesParam"
	s.Value = "mutated"
}

type subFieldAction struct {
	BaseAction
}

func (a *subFieldAction) Handle(ctx context.Context, instance *InstanceWithPtrSpec) *Result {
	mutateSubField(instance.Spec.Sub) // want "action type must not mutate instance.Spec; only Status is persisted by the action framework"
	return a.Return()
}

// --- Whole-object passing tests ---

// Helper that receives the whole object and mutates its Spec.
func mutateViaObject(obj *MyInstance) { // want mutateViaObject:"mutatesParam"
	obj.Spec.Host = "mutated through object"
}

// Helper that receives the whole object but only mutates Status.
func mutateStatusOnly(obj *MyInstance) { // want mutateStatusOnly:"mutatesParam"
	obj.Status.Ready = true
}

type wholeObjectAction struct {
	BaseAction
}

func (a *wholeObjectAction) Handle(ctx context.Context, instance *MyInstance) *Result {
	mutateViaObject(instance) // want "action type must not mutate instance.Spec; only Status is persisted by the action framework"

	mutateStatusOnly(instance) // OK — only Status is mutated, not Spec

	return a.Return()
}
