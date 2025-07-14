package utils

import "errors"

var (
	ErrCtlogAddressNotSpecified = errors.New("ctlog address not specified")
	ErrCtlogPortNotSpecified    = errors.New("ctlog port not specified")
	ErrCtlogPrefixNotSpecified  = errors.New("ctlog prefix not specified")
)
