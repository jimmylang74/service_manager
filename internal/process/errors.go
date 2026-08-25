package process

import "errors"

// ErrNotFound is returned when a service name is not found.
var ErrNotFound = errors.New("service not found")
