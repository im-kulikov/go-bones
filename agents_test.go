package bones

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

const (
	agentsIndex   = "AGENTS.md"
	agentsDocsDir = "docs/agents"

	// Line budgets keep the docs readable in portions: AGENTS.md is an index,
	// details live one level down, in pages small enough to read whole.
	maxIndexLines = 100
	maxPageLines  = 150
)

// TestAgentDocs is the guard behind the self-documentation rule of AGENTS.md:
// every Go file and package is documented for agents and no documented file is
// gone, every agent page is reachable from AGENTS.md, links resolve and pages
// stay small. It checks structure, not whether the prose is true.
func TestAgentDocs(t *testing.T) {
	docs, pages := agentDocs(t)
	files := goSources(t)

	t.Run("files", func(t *testing.T) { checkFiles(t, docs, pages, files) })
	t.Run("stale", func(t *testing.T) { checkStale(t, docs) })
	t.Run("packages", func(t *testing.T) { checkPackages(t, docs[agentsIndex], files) })
	t.Run("pages", func(t *testing.T) { checkPages(t, docs[agentsIndex], pages) })
	t.Run("links", func(t *testing.T) { checkLinks(t, docs) })
	t.Run("budgets", func(t *testing.T) { checkBudgets(t, docs) })
}

// agentDocs reads AGENTS.md and docs/agents/*.md, keyed by slash path.
func agentDocs(t *testing.T) (docs map[string]string, pages []string) {
	t.Helper()

	found, err := filepath.Glob(filepath.Join(agentsDocsDir, "*.md"))
	if err != nil || len(found) == 0 {
		t.Fatalf("no agent pages in %s: %v", agentsDocsDir, err)
	}

	docs = map[string]string{agentsIndex: readText(t, agentsIndex)}
	for _, page := range found {
		page = filepath.ToSlash(page)
		docs[page] = readText(t, page)
		pages = append(pages, page)
	}

	return docs, pages
}

func readText(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}

	return string(data)
}

// skipDir reports directories that hold no module code: the ones the go tool
// ignores, vendor/, and the owner's local scratch tmp/ and temp/, which are
// excluded from git.
func skipDir(name string) bool {
	switch name {
	case "testdata", "vendor", "tmp", "temp":
		return true
	}

	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// goSources returns the non-test Go files as slash paths relative to the root.
func goSources(t *testing.T) []string {
	t.Helper()

	var files []string

	err := filepath.WalkDir(".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			if name != "." && skipDir(entry.Name()) {
				return filepath.SkipDir
			}

			return nil
		}

		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			files = append(files, filepath.ToSlash(name))
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk sources: %v", err)
	}

	return files
}

// packageDirs returns the directories of files, except the module root.
func packageDirs(files []string) []string {
	var dirs []string

	seen := make(map[string]bool)
	for _, file := range files {
		if dir := path.Dir(file); dir != "." && !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}

	return dirs
}

func checkFiles(t *testing.T, docs map[string]string, pages, files []string) {
	var all strings.Builder
	for _, page := range pages {
		all.WriteString(docs[page])
	}

	text := all.String()
	for _, file := range files {
		if !strings.Contains(text, "`"+file+"`") {
			t.Errorf("%s is not named in %s/*.md: add it to the Files table of its package page",
				file, agentsDocsDir)
		}
	}
}

// checkStale fails on a backticked Go path, such as a Files row, whose file is
// gone.
func checkStale(t *testing.T, docs map[string]string) {
	for name, text := range docs {
		for _, file := range stalePaths(text) {
			t.Errorf("%s names %s, which does not exist: drop or rename it", name, file)
		}
	}
}

// stalePaths returns the backticked Go paths of text that do not exist. A bare
// name such as `doc.go` is a root file; `_test.go` or `<package>/doc.go` are
// patterns, not paths.
func stalePaths(text string) []string {
	var out []string

	goPath := regexp.MustCompile("`((?:[\\w.-]+/)*[A-Za-z][\\w.-]*\\.go)`")
	for _, match := range goPath.FindAllStringSubmatch(text, -1) {
		if _, err := os.Stat(match[1]); err != nil {
			out = append(out, match[1])
		}
	}

	return out
}

func TestAgentDocs_StalePaths(t *testing.T) {
	text := "`error.go` `config/base.go` `missing.go` `nested/missing.go` `_test.go` `<package>/doc.go`"
	if got := strings.Join(stalePaths(text), " "); got != "missing.go nested/missing.go" {
		t.Errorf("stale paths: got %q", got)
	}
}

func checkPackages(t *testing.T, index string, files []string) {
	for _, dir := range packageDirs(files) {
		if !strings.Contains(index, "`"+dir+"/`") {
			t.Errorf("package %s/ is not routed in %s: add a routing row", dir, agentsIndex)
		}
	}
}

func checkPages(t *testing.T, index string, pages []string) {
	for _, page := range pages {
		if !strings.Contains(index, "]("+page) {
			t.Errorf("%s is not linked from %s", page, agentsIndex)
		}
	}
}

func checkLinks(t *testing.T, docs map[string]string) {
	link := regexp.MustCompile(`\]\(([^)\s]+)\)`)

	for name, text := range docs {
		for _, match := range link.FindAllStringSubmatch(prose(text), -1) {
			if problem := linkProblem(name, match[1]); problem != "" {
				t.Errorf("%s: broken link %s (%s)", name, match[1], problem)
			}
		}
	}
}

func checkBudgets(t *testing.T, docs map[string]string) {
	for name, text := range docs {
		limit := maxPageLines
		if name == agentsIndex {
			limit = maxIndexLines
		}

		if lines := lineCount(text); lines > limit {
			t.Errorf("%s has %d lines, budget %d: split it or move details one level down",
				name, lines, limit)
		}
	}
}

// lineCount counts lines as an editor shows them: the last one may lack "\n".
func lineCount(text string) int {
	n := strings.Count(text, "\n")
	if text != "" && !strings.HasSuffix(text, "\n") {
		n++
	}

	return n
}

// prose drops fenced code blocks and inline code, where `Get[T](env)` is Go,
// not a link.
func prose(text string) string {
	var out strings.Builder

	code := regexp.MustCompile("`[^`]*`")
	fenced := false

	for line := range strings.Lines(text) {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
		}

		if !fenced {
			out.WriteString(code.ReplaceAllString(line, ""))
		}
	}

	return out.String()
}

// linkProblem checks a link of doc: a relative target must exist, and its
// #anchor must match a heading. External links are not checked.
func linkProblem(doc, target string) string {
	if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
		return ""
	}

	file, anchor, _ := strings.Cut(target, "#")
	if file == "" {
		file = doc
	} else {
		file = path.Join(path.Dir(doc), file)
	}

	info, err := os.Stat(file)

	switch {
	case err != nil:
		return "no such file"
	case info.IsDir() || anchor == "":
		return ""
	}

	data, err := os.ReadFile(file)
	if err != nil || !anchors(string(data))[anchor] {
		return "no such heading"
	}

	return ""
}

// anchors returns the heading anchors GitHub generates for a markdown text.
func anchors(text string) map[string]bool {
	out := make(map[string]bool)
	count := make(map[string]int)
	fenced := false

	for line := range strings.Lines(text) {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
		}

		if fenced || !strings.HasPrefix(line, "#") {
			continue
		}

		base := headingSlug(strings.TrimLeft(line, "#"))
		slug := base

		if n := count[base]; n > 0 {
			slug += "-" + strconv.Itoa(n)
		}

		out[slug] = true
		count[base]++
	}

	return out
}

// headingSlug lowercases a heading, keeps letters, digits, '-' and '_', turns
// spaces into '-' and drops everything else, like GitHub does.
func headingSlug(heading string) string {
	var out strings.Builder

	for _, r := range strings.ToLower(strings.TrimSpace(heading)) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
			out.WriteRune(r)
		case r == ' ':
			out.WriteByte('-')
		}
	}

	return out.String()
}

func TestLineCount(t *testing.T) {
	for text, want := range map[string]int{"": 0, "a\n": 1, "a\nb": 2, "a\nb\n": 2} {
		if got := lineCount(text); got != want {
			t.Errorf("lineCount(%q) = %d, want %d", text, got, want)
		}
	}
}
