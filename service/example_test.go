package service_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

type (
	Store     interface{ Lookup(key string) string }
	memStore  map[string]string
	StoreConf struct{ Seed map[string]string }
	APIConf   struct{ Key string }
	api       struct{ answer string }
)

func (m memStore) Lookup(key string) string { return m[key] }

func newStore(cfg StoreConf, _ service.Env) (memStore, error) { return cfg.Seed, nil }

// newAPI gets the store built before it by the interface it needs.
func newAPI(cfg APIConf, env service.Env) (*api, error) {
	store := service.Get[Store](env)

	return &api{answer: store.Lookup(cfg.Key)}, nil
}

// Components are built in order; each one finds its dependencies by type.
func ExampleGet() {
	env := service.NewEnv(context.Background(), logger.ForTests(), nil)

	if _, err := service.Build(
		env,
		StoreConf{Seed: map[string]string{"greeting": "hello"}},
		newStore,
	); err != nil {
		panic(err)
	}

	a, err := service.Build(env, APIConf{Key: "greeting"}, newAPI)
	fmt.Println(a.answer, err)

	empty := service.NewEnv(context.Background(), logger.ForTests(), nil)
	_, err = service.Build(empty, APIConf{}, newAPI)
	fmt.Println(errors.Is(err, service.ErrDependency))

	// Output:
	// hello <nil>
	// true
}

// A periodic task: runs on start, then every minute, each run bounded by 10s.
func ExampleNewTicker() {
	cleanup := service.NewTicker("cleanup", time.Minute, deleteExpired,
		service.WithTickerTimeout(10*time.Second), service.WithTickerJitter(0.1))

	_ = service.Run(logger.Default(), service.WithService(cleanup))
}

func deleteExpired(context.Context) error { return nil }
