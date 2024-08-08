package utils

import "errors"

var (
	CtlogAddressNotSpecified = errors.New("ctlog address not specified")
	CtlogPortNotSpecified    = errors.New("ctlog port not specified")
	CtlogPrefixNotSpecified  = errors.New("ctlog prefix not specified")
)
