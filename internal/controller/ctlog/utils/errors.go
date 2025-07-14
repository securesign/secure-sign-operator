package utils

import "errors"

var (
	ErrServerConfigNotSpecified    = errors.New("server config name not specified")
	ErrTreeNotSpecified            = errors.New("tree not specified")
	ErrTrillianAddressNotSpecified = errors.New("trillian address not specified")
	ErrTrillianPortNotSpecified    = errors.New("trillian port not specified")
	ErrPrivateKeyNotSpecified      = errors.New("private key not specified")
)
