//go:build ignore

// Generate HTML documentation from markdown files in the docs directory.
// Usage: go run docs/generate.go [-o output_dir]
package main

import (
	"bytes"
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	outputDir = flag.String("o", "docs/site", "output directory")

	// Markdown patterns
	reHeading    = regexp.MustCompile(`^(#{1,6})\s+(.+)`)
	reCodeFence  = regexp.MustCompile("^```(\\w*)")
	reBold       = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalic     = regexp.MustCompile(`\*(.+?)\*`)
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	reLink       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reImage      = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	reHR         = regexp.MustCompile(`^---+\s*$`)
)

type navEntry struct {
	Title string
	Path  string
}

type navSection struct {
	Title   string
	Entries []navEntry
}

func main() {
	flag.Parse()

	docsDir := "docs"
	if _, err := os.Stat(docsDir); err != nil {
		fmt.Fprintf(os.Stderr, "Run from the StarForge project root\n")
		os.Exit(1)
	}

	// Clean and create output directory
	os.RemoveAll(*outputDir)
	os.MkdirAll(*outputDir, 0o755)
	os.MkdirAll(filepath.Join(*outputDir, "actions"), 0o755)
	os.MkdirAll(filepath.Join(*outputDir, "commands"), 0o755)

	// Build navigation
	nav := buildNav(docsDir)

	// Process all markdown files
	err := filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".md" {
			return err
		}

		rel, _ := filepath.Rel(docsDir, path)
		htmlRel := strings.TrimSuffix(rel, ".md") + ".html"
		if rel == "README.md" {
			htmlRel = "index.html"
		}
		if strings.HasSuffix(rel, "/README.md") {
			htmlRel = strings.TrimSuffix(rel, "/README.md") + "/index.html"
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		title := extractTitle(data)
		body := renderMarkdown(data)
		depth := strings.Count(htmlRel, "/")
		prefix := strings.Repeat("../", depth)

		page := buildPage(title, body, nav, prefix, htmlRel)

		outPath := filepath.Join(*outputDir, htmlRel)
		os.MkdirAll(filepath.Dir(outPath), 0o755)
		if err := os.WriteFile(outPath, []byte(page), 0o644); err != nil {
			return err
		}
		fmt.Printf("  %s -> %s\n", rel, htmlRel)
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Write CSS
	cssPath := filepath.Join(*outputDir, "style.css")
	os.WriteFile(cssPath, []byte(cssContent), 0o644)
	fmt.Printf("  style.css\n")
	fmt.Printf("\nGenerated in %s/\n", *outputDir)
}

func buildNav(docsDir string) []navSection {
	var sections []navSection

	// Top-level pages
	top := navSection{Title: "Guide"}
	top.Entries = append(top.Entries, navEntry{"Home", "index.html"})
	if _, err := os.Stat(filepath.Join(docsDir, "guide.md")); err == nil {
		top.Entries = append(top.Entries, navEntry{"Getting Started", "guide.html"})
	}
	sections = append(sections, top)

	// Commands
	cmds := navSection{Title: "Commands"}
	cmds.Entries = append(cmds.Entries, navEntry{"Overview", "commands/index.html"})
	cmdFiles, _ := filepath.Glob(filepath.Join(docsDir, "commands", "*.md"))
	for _, f := range cmdFiles {
		base := filepath.Base(f)
		if base == "README.md" {
			continue
		}
		name := strings.TrimSuffix(base, ".md")
		cmds.Entries = append(cmds.Entries, navEntry{name, "commands/" + name + ".html"})
	}
	sections = append(sections, cmds)

	// Actions
	acts := navSection{Title: "Actions"}
	acts.Entries = append(acts.Entries, navEntry{"Overview", "actions/index.html"})
	actFiles, _ := filepath.Glob(filepath.Join(docsDir, "actions", "*.md"))
	for _, f := range actFiles {
		base := filepath.Base(f)
		if base == "README.md" {
			continue
		}
		name := strings.TrimSuffix(base, ".md")
		acts.Entries = append(acts.Entries, navEntry{name, "actions/" + name + ".html"})
	}
	sections = append(sections, acts)

	return sections
}

func extractTitle(data []byte) string {
	for _, line := range strings.Split(string(data), "\n") {
		if m := reHeading.FindStringSubmatch(line); m != nil && m[1] == "#" {
			return m[2]
		}
	}
	return "StarForge"
}

func renderMarkdown(data []byte) string {
	lines := strings.Split(string(data), "\n")
	var buf bytes.Buffer
	inCode := false
	inList := false
	inTable := false
	listTag := "ul"
	reOrderedItem := regexp.MustCompile(`^\d+\.\s+(.+)`)

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Code fences
		if m := reCodeFence.FindStringSubmatch(line); m != nil {
			if inCode {
				buf.WriteString("</code></pre>\n")
				inCode = false
			} else {
				closeList(&buf, &inList, listTag)
				lang := m[1]
				if lang != "" {
					buf.WriteString(fmt.Sprintf("<pre><code class=\"language-%s\">", lang))
				} else {
					buf.WriteString("<pre><code>")
				}
				inCode = true
			}
			continue
		}
		if inCode {
			buf.WriteString(html.EscapeString(line))
			buf.WriteString("\n")
			continue
		}

		// Blank line
		if strings.TrimSpace(line) == "" {
			closeList(&buf, &inList, listTag)
			closeTable(&buf, &inTable)
			continue
		}

		// Horizontal rule
		if reHR.MatchString(line) {
			closeList(&buf, &inList, listTag)
			buf.WriteString("<hr>\n")
			continue
		}

		// Headings
		if m := reHeading.FindStringSubmatch(line); m != nil {
			closeList(&buf, &inList, listTag)
			level := len(m[1])
			id := slugify(m[2])
			text := renderInline(m[2])
			buf.WriteString(fmt.Sprintf("<h%d id=\"%s\">%s</h%d>\n", level, id, text, level))
			continue
		}

		// Table
		if strings.HasPrefix(line, "|") {
			if !inTable {
				closeList(&buf, &inList, listTag)
				inTable = true
				buf.WriteString("<table>\n")
				// Header row
				cells := parseTableRow(line)
				buf.WriteString("<thead><tr>")
				for _, c := range cells {
					buf.WriteString("<th>" + renderInline(c) + "</th>")
				}
				buf.WriteString("</tr></thead>\n<tbody>\n")
				// Skip separator row
				if i+1 < len(lines) && strings.Contains(lines[i+1], "---") {
					i++
				}
				continue
			}
			cells := parseTableRow(line)
			buf.WriteString("<tr>")
			for _, c := range cells {
				buf.WriteString("<td>" + renderInline(c) + "</td>")
			}
			buf.WriteString("</tr>\n")
			continue
		}

		// Unordered list
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			closeTable(&buf, &inTable)
			if !inList {
				listTag = "ul"
				inList = true
				buf.WriteString("<ul>\n")
			}
			content := strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* ")
			buf.WriteString("<li>" + renderInline(content) + "</li>\n")
			continue
		}

		// Ordered list
		if m := reOrderedItem.FindStringSubmatch(line); m != nil {
			closeTable(&buf, &inTable)
			if !inList {
				listTag = "ol"
				inList = true
				buf.WriteString("<ol>\n")
			}
			buf.WriteString("<li>" + renderInline(m[1]) + "</li>\n")
			continue
		}

		// Paragraph
		closeList(&buf, &inList, listTag)
		closeTable(&buf, &inTable)
		buf.WriteString("<p>" + renderInline(line) + "</p>\n")
	}

	closeList(&buf, &inList, listTag)
	closeTable(&buf, &inTable)
	if inCode {
		buf.WriteString("</code></pre>\n")
	}

	return buf.String()
}

func closeList(buf *bytes.Buffer, inList *bool, tag string) {
	if *inList {
		buf.WriteString(fmt.Sprintf("</%s>\n", tag))
		*inList = false
	}
}

func closeTable(buf *bytes.Buffer, inTable *bool) {
	if *inTable {
		buf.WriteString("</tbody></table>\n")
		*inTable = false
	}
}

func parseTableRow(line string) []string {
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	var cells []string
	for _, p := range parts {
		cells = append(cells, strings.TrimSpace(p))
	}
	return cells
}

func renderInline(s string) string {
	s = html.EscapeString(s)

	// Restore markdown constructs that got escaped
	// Process in order: images, links, code, bold, italic

	// Inline code (must decode backticks)
	s = strings.ReplaceAll(s, "&#96;", "`")
	s = reInlineCode.ReplaceAllStringFunc(s, func(m string) string {
		inner := reInlineCode.FindStringSubmatch(m)[1]
		return "<code>" + inner + "</code>"
	})

	// Links and images (must decode brackets)
	s = strings.ReplaceAll(s, "&#91;", "[")
	s = strings.ReplaceAll(s, "&#93;", "]")
	s = strings.ReplaceAll(s, "&#40;", "(")
	s = strings.ReplaceAll(s, "&#41;", ")")

	// Images
	s = reImage.ReplaceAllStringFunc(s, func(m string) string {
		parts := reImage.FindStringSubmatch(m)
		return fmt.Sprintf("<img src=\"%s\" alt=\"%s\">", parts[2], parts[1])
	})

	// Links — rewrite .md to .html
	s = reLink.ReplaceAllStringFunc(s, func(m string) string {
		parts := reLink.FindStringSubmatch(m)
		href := parts[2]
		if strings.HasSuffix(href, ".md") {
			href = strings.TrimSuffix(href, ".md") + ".html"
		}
		if strings.HasSuffix(href, "/README.html") {
			href = strings.TrimSuffix(href, "/README.html") + "/index.html"
		}
		if href == "README.html" {
			href = "index.html"
		}
		return fmt.Sprintf("<a href=\"%s\">%s</a>", href, parts[1])
	})

	// Bold/italic (must decode asterisks)
	s = strings.ReplaceAll(s, "&#42;", "*")
	s = reBold.ReplaceAllString(s, "<strong>$1</strong>")
	s = reItalic.ReplaceAllString(s, "<em>$1</em>")

	return s
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = regexp.MustCompile(`[^a-z0-9\s-]`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`[\s]+`).ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func buildPage(title, body string, nav []navSection, prefix, currentPath string) string {
	var sidebar bytes.Buffer
	for _, sec := range nav {
		sidebar.WriteString(fmt.Sprintf("<h3>%s</h3>\n<ul>\n", sec.Title))
		for _, entry := range sec.Entries {
			class := ""
			if entry.Path == currentPath {
				class = ` class="active"`
			}
			sidebar.WriteString(fmt.Sprintf("<li><a href=\"%s%s\"%s>%s</a></li>\n",
				prefix, entry.Path, class, entry.Title))
		}
		sidebar.WriteString("</ul>\n")
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s — StarForge</title>
<link rel="stylesheet" href="%sstyle.css">
</head>
<body>
<nav class="sidebar">
<div class="logo"><a href="%sindex.html">StarForge</a></div>
%s
</nav>
<main>
<article>
%s
</article>
</main>
</body>
</html>`, html.EscapeString(title), prefix, prefix, sidebar.String(), body)
}

const cssContent = `
:root {
  --bg: #0d1117;
  --bg-sidebar: #161b22;
  --fg: #e6edf3;
  --fg-dim: #8b949e;
  --fg-heading: #f0f6fc;
  --accent: #58a6ff;
  --accent-hover: #79c0ff;
  --border: #30363d;
  --code-bg: #1c2128;
  --table-stripe: #161b22;
  --active-bg: #1f2937;
}

* { margin: 0; padding: 0; box-sizing: border-box; }

body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
  background: var(--bg);
  color: var(--fg);
  display: flex;
  min-height: 100vh;
  line-height: 1.6;
}

.sidebar {
  width: 260px;
  min-width: 260px;
  background: var(--bg-sidebar);
  border-right: 1px solid var(--border);
  padding: 1.5rem 1rem;
  position: fixed;
  top: 0;
  left: 0;
  bottom: 0;
  overflow-y: auto;
}

.sidebar .logo {
  margin-bottom: 1.5rem;
  padding-bottom: 1rem;
  border-bottom: 1px solid var(--border);
}

.sidebar .logo a {
  color: var(--fg-heading);
  text-decoration: none;
  font-size: 1.25rem;
  font-weight: 700;
  letter-spacing: -0.01em;
}

.sidebar h3 {
  color: var(--fg-dim);
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin: 1.25rem 0 0.5rem 0;
}

.sidebar ul {
  list-style: none;
}

.sidebar li a {
  display: block;
  padding: 0.2rem 0.75rem;
  color: var(--fg);
  text-decoration: none;
  font-size: 0.875rem;
  border-radius: 4px;
  transition: background 0.15s;
}

.sidebar li a:hover {
  background: var(--active-bg);
  color: var(--accent);
}

.sidebar li a.active {
  background: var(--active-bg);
  color: var(--accent);
  font-weight: 600;
}

main {
  margin-left: 260px;
  flex: 1;
  max-width: 52rem;
  padding: 2.5rem 3rem;
}

article h1 { font-size: 2rem; color: var(--fg-heading); margin: 0 0 1rem; border-bottom: 1px solid var(--border); padding-bottom: 0.5rem; }
article h2 { font-size: 1.5rem; color: var(--fg-heading); margin: 2rem 0 0.75rem; border-bottom: 1px solid var(--border); padding-bottom: 0.3rem; }
article h3 { font-size: 1.2rem; color: var(--fg-heading); margin: 1.5rem 0 0.5rem; }
article h4 { font-size: 1rem; color: var(--fg-heading); margin: 1.25rem 0 0.5rem; }

article p { margin: 0.75rem 0; }

article a { color: var(--accent); text-decoration: none; }
article a:hover { color: var(--accent-hover); text-decoration: underline; }

article code {
  background: var(--code-bg);
  padding: 0.15em 0.35em;
  border-radius: 4px;
  font-size: 0.875em;
  font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
}

article pre {
  background: var(--code-bg);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 1rem;
  overflow-x: auto;
  margin: 1rem 0;
  line-height: 1.5;
}

article pre code {
  background: none;
  padding: 0;
  font-size: 0.85rem;
}

article table {
  width: 100%;
  border-collapse: collapse;
  margin: 1rem 0;
  font-size: 0.9rem;
}

article th, article td {
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--border);
  text-align: left;
}

article th {
  background: var(--bg-sidebar);
  font-weight: 600;
  color: var(--fg-heading);
}

article tr:nth-child(even) { background: var(--table-stripe); }

article ul, article ol { margin: 0.75rem 0; padding-left: 1.5rem; }
article li { margin: 0.25rem 0; }

article hr {
  border: none;
  border-top: 1px solid var(--border);
  margin: 2rem 0;
}

article strong { color: var(--fg-heading); }

@media (max-width: 768px) {
  .sidebar { position: static; width: 100%; min-width: auto; border-right: none; border-bottom: 1px solid var(--border); }
  body { flex-direction: column; }
  main { margin-left: 0; padding: 1.5rem; }
}
`
