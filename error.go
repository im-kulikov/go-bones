package bones

// Error allows to define constant errors.
type Error string

// Error used to implement error interface.
func (e Error) Error() string { return string(e) }

// ExtractError extracts the last argument as an error, if it is of type error.
// It is useful for ignoring return values while still handling errors in functions
// with multiple return values.
//
// If the last argument is not an error or no arguments are passed, it returns nil.
//
// Should be used in tests.
//
// Example:
//
//	 require.ErrorIs(t, ExtractError(shouldCatchAnError()), ErrTest)
//
//		// example func SomeFunction() (float, error)
//		if err := ExtractError(SomeFunction()); err != nil {
//		    log.Println("error occurred:", err)
//		}
//
//		// example func AnotherFunction() (int, string, error)
//		if err := ExtractError(AnotherFunction()); err != nil {
//		    log.Println("error occurred:", err)
//		}
func ExtractError(args ...any) error {
	if len(args) == 0 {
		return nil
	}

	if err, ok := args[len(args)-1].(error); ok {
		return err
	}

	return nil
}
