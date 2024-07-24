package utils

import "errors"

var (
	ServerConfigNotSpecified    = errors.New("server config name not specified")
	TreeNotSpecified            = errors.New("tree not specified")
	TrillianAddressNotSpecified = errors.New("trillian address not specified")
	TrillianPortNotSpecified    = errors.New("trillian port not specified")
)
