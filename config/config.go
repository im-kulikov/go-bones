package config

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path"
	"reflect"
	"strings"

	"github.com/cristalhq/aconfig"
	"github.com/cristalhq/aconfig/aconfigdotenv"

	"github.com/im-kulikov/go-bones/logger"
)

// Config interface that allows to set and validate
// project configuration.
type Config interface {
	Validate(context.Context) error
}

func (c *config) checkEnvPath() error {
	if c.envPath != "" {
		return nil
	}

	var err error
	if c.envPath, err = c.pwd(); err != nil {
		return err
	}

	return nil
}

var (
	errVersion      = errors.New("show version")
	errShowHelp     = errors.New("show help")
	errValidate     = errors.New("validate")
	errMarkdown     = errors.New("markdown")
	errFailValidate = errors.New("could not validate config")
)

func (c *config) generateDefaultEnvs(field aconfig.Field) bool {
	value := field.Tag("default")
	names := field.Tag("env")
	usage := field.Tag("usage")

	current := field
	if value == "" {
		value = "<empty>"
	}

	pad := 50

	var ok bool
	for {
		if current, ok = current.Parent(); !ok {
			break
		}

		names = fmt.Sprintf("%s_%s", current.Tag("env"), names)
	}

	var line strings.Builder
	_, _ = line.WriteString(names)
	_, _ = line.WriteString("=")
	_, _ = line.WriteString(value)

	if usage != "" {
		_, _ = line.WriteString(strings.Repeat(" ", pad-line.Len()))
		_, _ = line.WriteString("# " + usage)
	}

	_, _ = fmt.Fprintln(c.out, line.String())

	return true
}

func (c *config) renderHelp(l *aconfig.Loader, fs *flag.FlagSet) {
	output := fs.Output()

	_, _ = fmt.Fprintln(output, "Usage:")
	_, _ = fmt.Fprintln(output)

	c.renderFlags(fs)

	var out strings.Builder

	_, _ = fmt.Fprintf(output, "\nDefault envs:\n%s\n", out.String())

	l.WalkFields(c.generateDefaultEnvs)
}

func (c *config) loadConfig(ctx context.Context, cfg Config) (err error) {
	if err = c.checkEnvPath(); err != nil {
		return fmt.Errorf("could not get current directory: %w", err)
	}

	c.envs = append(os.Environ(), c.envs...)

	loader := aconfig.LoaderFor(cfg, aconfig.Config{
		AllowUnknownFields: true,
		SkipFlags:          true,
		Envs:               c.envs,
		Files:              []string{path.Join(c.envPath, ".env")},
		FileDecoders: map[string]aconfig.FileDecoder{
			".env": aconfigdotenv.New(),
		},
	})

	flags := loader.Flags()
	flags.SetOutput(c.out)
	flags.Usage = func() { c.renderHelp(loader, flags) }

	c.attachFlags(flags)

	if err = flags.Parse(c.args); err != nil && !errors.Is(err, flag.ErrHelp) {
		return fmt.Errorf("could not parse flags: %w", err)
	}

	defer func() {
		switch {
		default:
		case c.showCurr:
			// on version requested
			_, _ = fmt.Fprintln(c.out, c.version)

			c.exit(0)

			err = errVersion

		case c.markdown:
			// on markdown requested
			c.generateMarkdown(loader)

			c.exit(0)

			err = errMarkdown
		case c.validate:
			// on validate requested
			if err = cfg.Validate(ctx); err != nil {
				c.fatalf("could not validate config: %s", err)

				c.exit(2)

				err = errFailValidate

				return
			}

			_, _ = fmt.Fprintln(c.out, "OK")

			c.exit(0)

			err = errValidate
		case c.showHelp:
			// on help requested
			c.renderHelp(loader, flags)

			c.exit(0)

			err = errShowHelp
		}
	}()

	err = loader.Load()

	return
}

// Load returns an error if
// - Config is not a pointer to struct
// - could not load configuration from env
// - could not validate config
//
// otherwise it pass configuration to Config.
func Load(ctx context.Context, cfg Config, opts ...Option) error {
	if reflect.ValueOf(cfg).Kind() != reflect.Ptr {
		return fmt.Errorf("config variable must be a pointer")
	}

	options := config{
		pwd:  os.Getwd,
		out:  os.Stdout,
		exit: os.Exit,
		args: os.Args[1:],
		envs: os.Environ(),

		fatalf: logger.Default().Fatalf,
	}

	for _, o := range opts {
		o(&options)
	}

	if err := options.loadConfig(ctx, cfg); err != nil {
		return fmt.Errorf("could not load config: %w", err)
	}

	return cfg.Validate(ctx)
}
