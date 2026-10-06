package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"github.com/im-kulikov/go-bones"
	"github.com/im-kulikov/go-bones/health"
	"github.com/im-kulikov/go-bones/logger"
)

// ErrDependency is wrapped by the error Build returns when a constructor asks Get
// for a dependency that is missing or ambiguous.
const ErrDependency bones.Error = "unresolved dependency"

// ErrNilComponent is wrapped by the error Build returns when a constructor
// returns a typed nil, such as a nil *T: Get could not tell it from a real
// dependency. A nil interface means a disabled component and is not an error.
const ErrNilComponent bones.Error = "constructor returned a typed nil"

// ErrNilConstructor is wrapped by the error Build returns for a nil
// constructor, instead of a nil pointer panic without the call site.
const ErrNilConstructor bones.Error = "nil constructor"

// Env is what an application gives every component constructor.
//
// Components get their dependencies from Env with Get, by type, from the values
// built before them. Get calls belong at the top of a constructor, so the
// dependencies of a component are visible at a glance.
type Env struct {
	// Context is canceled on SIGINT/SIGTERM. Use it while building a component
	// (fetching data, connecting); runtime work uses the context passed to Start.
	Context context.Context

	// Logger is the application logger.
	Logger *logger.Logger

	// Health is the application health monitor, for components that register
	// their own checks, statuses or heartbeats.
	Health *health.Monitor

	values *[]any
}

// Constructor builds a component from its configuration and the application
// environment. The configuration type should be concrete: with an interface,
// Go cannot infer the type parameters of the call site.
type Constructor[C, T any] func(C, Env) (T, error)

// dependencyError is the panic value of Get; Build turns it back into an error.
type dependencyError struct{ err error }

func (e dependencyError) Error() string { return e.err.Error() }

func (e dependencyError) Unwrap() error { return e.err }

// NewEnv returns an environment whose Get resolves dependencies from values.
func NewEnv(ctx context.Context, log *logger.Logger, hc *health.Monitor, values ...any) Env {
	return Env{Context: ctx, Logger: log, Health: hc, values: &values}
}

// Get returns the only value in env that is assignable to T: a value built by an
// earlier Build or passed to NewEnv. T is usually an interface the component
// needs, such as a storage it reads from.
//
// A missing or ambiguous dependency panics with an error wrapping ErrDependency;
// Build recovers it and returns it as the constructor's error, naming the type.
func Get[T any](env Env) T {
	var (
		out   T
		found []string
	)

	if env.values != nil {
		for _, v := range *env.values {
			if t, ok := v.(T); ok {
				out = t
				found = append(found, fmt.Sprintf("%T", v))
			}
		}
	}

	name := reflect.TypeFor[T]().String()

	switch len(found) {
	case 1:
		return out
	case 0:
		panic(
			dependencyError{
				fmt.Errorf("%w: needs %s, nothing built before provides it", ErrDependency, name),
			},
		)
	default:
		panic(dependencyError{fmt.Errorf("%w: needs %s, ambiguous: %s",
			ErrDependency, name, strings.Join(found, ", "))})
	}
}

// Build calls ctor with cfg and env, and adds the result to env, so the
// constructors built after it can Get it. A nil interface result (a disabled
// component) is not added; a typed nil is an error wrapping ErrNilComponent.
// The error names the constructor.
func Build[C, T any](env Env, cfg C, ctor Constructor[C, T]) (_ T, err error) {
	if ctor == nil {
		var zero T

		return zero, fmt.Errorf("%w for %s", ErrNilConstructor, reflect.TypeFor[T]())
	}

	name := runtime.FuncForPC(reflect.ValueOf(ctor).Pointer()).Name()

	defer func() {
		if r := recover(); r != nil {
			var dep dependencyError
			if e, ok := r.(error); !ok || !errors.As(e, &dep) {
				panic(r)
			}

			err = fmt.Errorf("%s: %w", name, dep.err)
		}
	}()

	v, err := ctor(cfg, env)
	if err != nil {
		return v, fmt.Errorf("%s: %w", name, err)
	}

	if typedNil(v) {
		return v, fmt.Errorf("%s: %w: %T", name, ErrNilComponent, v)
	}

	if env.values != nil && any(v) != nil {
		*env.values = append(*env.values, v)
	}

	return v, nil
}

// typedNil reports a nil pointer, map, slice, func or chan held by a non-nil
// interface.
func typedNil(v any) bool {
	rv := reflect.ValueOf(v)

	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan,
		reflect.UnsafePointer:
		return rv.IsNil()
	default:
		return false
	}
}
