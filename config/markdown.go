package config

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/cristalhq/aconfig"
)

const cellSeparator = "|"

// nolint: funlen
func (c *config) generateMarkdown(l *aconfig.Loader) {
	var table [][]string

	table = append(table, []string{
		"Name", "Required", "Default value", "Usage", "Example",
	})

	sizes := make([]int, len(table[0]))

	var lineSize int
	for i, cell := range table[0] {
		sizes[i] = utf8.RuneCountInString(cell) + 2
	}

	l.WalkFields(func(f aconfig.Field) bool {
		names := f.Tag("env")
		usage := f.Tag("usage")
		value := f.Tag("default")

		required := f.Tag("required")
		if required == "" {
			required = "false"
		}

		examples := f.Tag("example")

		field := f
		var ok bool
		for {
			if field, ok = field.Parent(); !ok {
				break
			}

			names = fmt.Sprintf("%s_%s", field.Tag("env"), names)
		}

		cell := []string{names, required, value, usage, examples}
		table = append(table, cell)

		lineSize = 0
		for i, item := range cell {
			if size := utf8.RuneCountInString(item); size+2 > sizes[i] {
				sizes[i] = size + 2
			}

			lineSize += sizes[i] // recalculate line size
		}

		return true
	})

	var out strings.Builder
	_, _ = out.WriteString("### Envs\n\n")
	for i, row := range table {
		_, _ = out.WriteString(cellSeparator)

		for j, cell := range row {
			size := utf8.RuneCountInString(" " + cell + " ")

			data := strings.Repeat(" ", sizes[j]-size)

			_, _ = out.WriteString(" " + cell + " ")
			_, _ = out.WriteString(data)

			if len(row)-1 != j {
				_, _ = out.WriteString(cellSeparator)
			}
		}

		if i == 0 {
			_, _ = out.WriteString(cellSeparator)
			_, _ = out.WriteRune('\n')

			_, _ = out.WriteString(cellSeparator)
			for j, item := range sizes {
				dashes := strings.Repeat("-", item)
				_, _ = out.WriteString(dashes)

				if len(sizes)-1 != j {
					_, _ = out.WriteString(cellSeparator)
				}
			}
		}

		_, _ = out.WriteString(cellSeparator)
		_, _ = out.WriteRune('\n')
	}

	_, _ = fmt.Fprintln(c.out, out.String())
}
