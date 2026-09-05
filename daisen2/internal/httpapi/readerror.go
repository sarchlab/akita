package httpapi

import "context"

// traceReadError unwinds private SQL decoding helpers to the public reader API.
// Invariant panics remain panics; operational errors never become partial data.
type traceReadError struct{ error }

func readTrace[T any](ctx context.Context, fn func() T) (result T, err error) {
	if err = ctx.Err(); err != nil {
		return result, err
	}
	defer func() {
		if p := recover(); p != nil {
			if e, ok := p.(traceReadError); ok {
				var zero T
				result = zero
				err = e.error
			} else {
				panic(p)
			}
		}
	}()
	result = fn()
	if err = ctx.Err(); err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}
