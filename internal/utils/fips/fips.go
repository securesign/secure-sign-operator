package fips

import (
	"crypto/fips140"
	"errors"
)

var Enabled = fips140.Enabled

var ErrPasswordRefInFIPS = errors.New("password-protected private keys are not allowed in FIPS mode: remove the password reference and provide an unencrypted key")
