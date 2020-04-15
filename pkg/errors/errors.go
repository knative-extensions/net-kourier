package errors

import "errors"

var (
	ErrDomainConflict = errors.New("ingress has a conflicting domain with another ingress")
)
