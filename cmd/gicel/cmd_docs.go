// docs and example subcommands — browse built-in documentation and samples.

package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cwd-k2/gicel"
)

func printDocsUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: gicel docs [topic]

List available documentation topics, or show a specific topic.`)
}

func printExampleUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: gicel example [name]

List available examples, or show example source.`)
}

// normalizeFlagError rewrites Go flag error messages to use -- prefix for
// multi-character flags, preserving single-character flags as -e style.

func cmdDocs(args []string) int {
	if len(args) > 0 && isHelpFlag(args[0]) {
		printDocsUsage(os.Stderr)
		return 0
	}
	if len(args) == 0 {
		topics := gicel.DocTopics()
		if len(topics) == 0 {
			fmt.Fprintln(os.Stderr, "no documentation available")
			return 1
		}
		entries := make([]catalogEntry, len(topics))
		for i, name := range topics {
			entries[i] = catalogEntry{name, gicel.DocDesc(name)}
		}
		printCatalog(entries, "general")
		fmt.Println("Run 'gicel docs <topic>' for details.")
		return 0
	}
	topic := args[0]
	content := gicel.Doc(topic)
	if content == "" {
		fmt.Fprintf(os.Stderr, "unknown topic: %s\n\nAvailable topics:\n", topic)
		for _, t := range gicel.DocTopics() {
			fmt.Fprintf(os.Stderr, "  %s\n", t)
		}
		return 1
	}
	fmt.Print(content)
	return 0
}

func cmdExample(args []string) int {
	if len(args) > 0 && isHelpFlag(args[0]) {
		printExampleUsage(os.Stderr)
		return 0
	}
	if len(args) == 0 {
		examples := gicel.Examples()
		if len(examples) == 0 {
			fmt.Fprintln(os.Stderr, "no examples available")
			return 1
		}
		entries := make([]catalogEntry, len(examples))
		for i, name := range examples {
			entries[i] = catalogEntry{name, exampleDesc(gicel.Example(name))}
		}
		printCatalog(entries, "Other")
		fmt.Println("Run 'gicel example <name>' to view source.")
		return 0
	}
	name := args[0]
	content := gicel.Example(name)
	if content == "" {
		fmt.Fprintf(os.Stderr, "unknown example: %s\n\nAvailable examples:\n", name)
		for _, e := range gicel.Examples() {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		return 1
	}
	fmt.Print(content)
	return 0
}

// exampleDesc extracts the title from "-- GICEL Example: <title>" and
// appends any required flags from "-- Flags: <flags>" lines.

// exampleDesc extracts the title from "-- GICEL Example: <title>" and
// appends any required flags from "-- Flags: <flags>" lines.
func exampleDesc(source string) string {
	const titlePrefix = "-- GICEL Example: "
	const flagsPrefix = "-- Flags: "
	var title, flags string
	for line := range strings.SplitSeq(source, "\n") {
		if t, ok := strings.CutPrefix(line, titlePrefix); ok && title == "" {
			title = t
		}
		if f, ok := strings.CutPrefix(line, flagsPrefix); ok && flags == "" {
			flags = strings.TrimSpace(f)
		}
		// Stop scanning after the header comment block.
		if !strings.HasPrefix(line, "--") && strings.TrimSpace(line) != "" {
			break
		}
	}
	if flags != "" && title != "" {
		return title + " [" + flags + "]"
	}
	return title
}

// packMap maps pack names to their Pack functions.
