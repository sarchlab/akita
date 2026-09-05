package datarecording

// MustRecord adapts recorder errors to a managed model callback. Standalone
// callers should handle the returned error directly. It never discards failures.
func MustRecord(err error) {
	if err != nil {
		panic(err)
	}
}

// recorderIOError unwinds private SQL helpers to the recorder's public I/O boundary.
// Programming errors retain their original panic semantics.
type recorderIOError struct{ error }

func recoverIO(err *error) {
	if p := recover(); p != nil {
		if failure, ok := p.(recorderIOError); ok {
			*err = failure.error
		} else {
			panic(p)
		}
	}
}
