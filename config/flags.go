package config

import (
	"flag"
	"fmt"
	"strings"
	"unicode/utf8"
)

func (c *config) attachFlags(fs *flag.FlagSet) {
	// we must provide either the full name of the flag (e.g. `--version`),
	// or a short and full one at the same time (e.g. `-v` and `--version`)
	fs.BoolVar(&c.showHelp, "h", c.showHelp, "show this help message")
	fs.BoolVar(&c.showHelp, "help", c.showHelp, "show this help message")
	fs.BoolVar(&c.showCurr, "V", c.showCurr, "show current version")
	fs.BoolVar(&c.showCurr, "version", c.showCurr, "show current version")
	fs.BoolVar(&c.validate, "validate", c.validate, "validate config")
	fs.BoolVar(&c.markdown, "markdown", c.markdown, "generate env markdown table")
}

type flagDefinition struct {
	name  string
	short string
	usage string
	value string
}

// nolint: funlen
func (c *config) renderFlags(fs *flag.FlagSet) {
	var flags []*flagDefinition

	fs.VisitAll(func(f *flag.Flag) {
		def := &flagDefinition{
			usage: f.Usage,
			value: f.DefValue,
		}

		switch {
		case len(f.Name) == 1:
			def.short = f.Name
		default:
			def.name = f.Name
		}

		for _, item := range flags {
			if item.usage != def.usage {
				continue
			}

			if item.name == "" {
				item.name = def.name
			}

			return
		}

		flags = append(flags, def)
	})

	maxlen := 0
	lines := make([]string, 0, len(flags))
	for _, item := range flags {
		var line string
		switch {
		case item.short == "":
			line = fmt.Sprintf("      --%s", item.name)
		default:
			line = fmt.Sprintf("  -%s, --%s", item.short, item.name)
		}

		// This special character will be replaced with spacing once the
		// correct alignment is calculated
		line += "\x00"
		if len(line) > maxlen {
			maxlen = utf8.RuneCountInString(line)
		}

		line += item.usage

		lines = append(lines, line)
	}

	for _, line := range lines {
		index := strings.Index(line, "\x00")
		spacing := strings.Repeat(" ", maxlen-index)
		_, _ = fmt.Fprintln(c.out, line[:index], spacing, line[index+1:])
	}
}
