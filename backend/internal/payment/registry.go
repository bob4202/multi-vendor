package payment

import (
	"errors"
	"fmt"
)

var ErrUnknownProvider = errors.New("payment: provider not found")

func (r Registry) Build(name string, c Credentials) (Provider, error) {
	f, ok := r[name]
	if !ok {
		return nil, fmt.Errorf("%q: %w", name, ErrUnknownProvider)
	}

	return f(c)
}

func (r Registry) Has(name string) bool {
	_, ok := r[name]
	return ok
}
