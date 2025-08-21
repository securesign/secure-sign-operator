package pvc

import (
	"errors"
)

var (
	ErrPVCSizeNotSet = errors.New("PVC size is not set")
)
