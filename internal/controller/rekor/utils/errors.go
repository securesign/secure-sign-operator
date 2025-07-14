package utils

import "errors"

var (
	ErrServerConfigNotSpecified    = errors.New("server config name not specified")
	ErrTreeNotSpecified            = errors.New("tree not specified")
	ErrTrillianAddressNotSpecified = errors.New("trillian address not specified")
	ErrTrillianPortNotSpecified    = errors.New("trillian port not specified")
	ErrSignerKeyNotSpecified       = errors.New("signer key reference not specified")
)
