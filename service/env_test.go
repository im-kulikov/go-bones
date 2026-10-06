package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type (
	envStore    interface{ Lookup() string }
	envRepo     struct{ name string }
	envConfig   struct{ Name string }
	envConsumer struct{ store envStore }
)

func (r *envRepo) Lookup() string { return r.name }

func newEnvRepo(cfg envConfig, _ Env) (*envRepo, error) { return &envRepo{name: cfg.Name}, nil }

func newEnvConsumer(_ envConfig, env Env) (*envConsumer, error) {
	return &envConsumer{store: Get[envStore](env)}, nil
}

func TestBuild_ResolvesEarlierValuesByType(t *testing.T) {
	env := TestEnv(t)

	repo, err := Build(env, envConfig{Name: "db"}, newEnvRepo)
	require.NoError(t, err)

	consumer, err := Build(env, envConfig{}, newEnvConsumer)
	require.NoError(t, err)
	require.Same(t, repo, consumer.store, "found through the interface it implements")
}

func TestBuild_MissingDependency(t *testing.T) {
	_, err := Build(TestEnv(t), envConfig{}, newEnvConsumer)

	require.ErrorIs(t, err, ErrDependency)
	require.ErrorContains(t, err, "service.newEnvConsumer")
	require.ErrorContains(t, err, "needs service.envStore, nothing built before provides it")
}

func TestBuild_AmbiguousDependency(t *testing.T) {
	env := TestEnv(t, &envRepo{name: "a"}, &envRepo{name: "b"})

	_, err := Build(env, envConfig{}, newEnvConsumer)
	require.ErrorIs(t, err, ErrDependency)
	require.ErrorContains(t, err, "ambiguous: *service.envRepo, *service.envRepo")
}

func TestBuild_ConstructorError(t *testing.T) {
	env := TestEnv(t)

	_, err := Build(
		env,
		envConfig{},
		func(envConfig, Env) (*envRepo, error) { return nil, errTest },
	)
	require.ErrorIs(t, err, errTest)
	require.ErrorContains(t, err, "TestBuild_ConstructorError")

	_, err = Build(env, envConfig{}, newEnvConsumer)
	require.ErrorIs(t, err, ErrDependency, "a failed constructor provides nothing")
}

func TestBuild_NilConstructor(t *testing.T) {
	_, err := Build[envConfig, *envRepo](TestEnv(t), envConfig{}, nil)
	require.ErrorIs(t, err, ErrNilConstructor)
	require.ErrorContains(t, err, "*service.envRepo")
}

func TestBuild_NilResultIsNotAdded(t *testing.T) {
	env := TestEnv(t)

	_, err := Build(env, envConfig{}, func(envConfig, Env) (envStore, error) { return nil, nil })
	require.NoError(t, err)

	_, err = Build(env, envConfig{}, newEnvConsumer)
	require.ErrorIs(t, err, ErrDependency)
}

func TestBuild_TypedNilIsAnError(t *testing.T) {
	env := TestEnv(t)

	_, err := Build(env, envConfig{}, func(envConfig, Env) (*envRepo, error) { return nil, nil })
	require.ErrorIs(t, err, ErrNilComponent)
	require.ErrorContains(t, err, "*service.envRepo")

	_, err = Build(env, envConfig{}, func(envConfig, Env) (envStore, error) {
		return (*envRepo)(nil), nil
	})
	require.ErrorIs(t, err, ErrNilComponent)

	_, err = Build(env, envConfig{}, newEnvConsumer)
	require.ErrorIs(t, err, ErrDependency, "a typed nil is not stored")
}

func TestBuild_OtherPanicsPropagate(t *testing.T) {
	require.PanicsWithValue(t, "boom", func() {
		_, _ = Build(TestEnv(t), envConfig{}, func(envConfig, Env) (int, error) { panic("boom") })
	})

	require.PanicsWithError(t, errTest.Error(), func() {
		_, _ = Build(TestEnv(t), envConfig{}, func(envConfig, Env) (int, error) { panic(errTest) })
	})
}

func TestGet_OutsideBuild(t *testing.T) {
	require.Equal(t, "db", Get[envStore](TestEnv(t, &envRepo{name: "db"})).Lookup())

	var zero Env
	require.Panics(t, func() { Get[envStore](zero) })

	func() {
		defer func() {
			err, ok := recover().(error)
			require.True(t, ok)
			require.True(t, errors.Is(err, ErrDependency))
			require.Contains(t, err.Error(), "needs service.envStore")
		}()

		Get[envStore](zero)
	}()
}

func TestBuild_ZeroEnvDoesNotStore(t *testing.T) {
	var zero Env

	repo, err := Build(zero, envConfig{Name: "db"}, newEnvRepo)
	require.NoError(t, err)
	require.Equal(t, "db", repo.name)
}
