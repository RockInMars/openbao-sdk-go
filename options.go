package bao

import "github.com/RockInMars/openbao-sdk-go/observe"

// Option can only be created through SDK-defined helpers.
type Option struct{ apply func(*clientOptions) error }
type clientOptions struct{ observer observe.Observer }

func WithObserver(o observe.Observer) Option {
	return Option{apply: func(c *clientOptions) error {
		if nilInterface(o) {
			return invalid("OBSERVER")
		}
		c.observer = o
		return nil
	}}
}
