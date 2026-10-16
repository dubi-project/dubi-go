package data

import "errors"

const NAME = "DATA"

const defaultName = "default"

var (
	errInvalidConnection = errors.New("invalid data connection")
)
