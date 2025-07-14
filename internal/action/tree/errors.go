package tree

import (
	"errors"
)

var (
	ErrTrillianPortNotSpecified = errors.New("trillian port not specified")
	ErrJobFailed                = errors.New("createtree job failed")
)
