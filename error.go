package bones

// Error allows to define constant errors.
type Error string

// Error used to implement error interface.
func (e Error) Error() string { return string(e) }
