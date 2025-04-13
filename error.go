package bones

// Error allows to define constant errors.
type Error string

// Error used to implement error interface.
func (e Error) Error() string { return string(e) }

// OnlyError catch an error.
func OnlyError[T any](_ T, err error) error { return err }
