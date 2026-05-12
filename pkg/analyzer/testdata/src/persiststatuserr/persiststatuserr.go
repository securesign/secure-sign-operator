package persiststatuserr

import "context"

type Result struct{}

type BaseAction struct{}

func (*BaseAction) PersistStatus(ctx context.Context, obj any) (bool, error) {
	return false, nil
}

func (*BaseAction) Error(ctx context.Context, err error, obj any) *Result { return nil }
func (*BaseAction) Return() *Result                                       { return nil }
func (*BaseAction) Requeue() *Result                                      { return nil }

type myAction struct {
	BaseAction
}

func good_both(a *myAction) *Result {
	changed, err := a.PersistStatus(context.TODO(), nil)
	if err != nil {
		return a.Error(context.TODO(), err, nil)
	}
	_ = changed
	return a.Return()
}

func good_err_named(a *myAction) *Result {
	_, err := a.PersistStatus(context.TODO(), nil)
	if err != nil {
		return a.Error(context.TODO(), err, nil)
	}
	return a.Requeue()
}

func bad_err_blank(a *myAction) *Result {
	_, _ = a.PersistStatus(context.TODO(), nil) // want "PersistStatus error return value must not be discarded"
	return a.Requeue()
}

func bad_all_discarded(a *myAction) *Result {
	a.PersistStatus(context.TODO(), nil) // want "PersistStatus return values must not be discarded"
	return a.Return()
}

// Unrelated method with same name but different signature — should NOT be flagged.
type otherService struct{}

func (*otherService) PersistStatus(ctx context.Context) error { return nil }

func no_false_positive(s *otherService) {
	_ = s.PersistStatus(context.TODO())
}
