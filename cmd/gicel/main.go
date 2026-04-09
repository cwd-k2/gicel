// Command gicel provides a CLI for the GICEL language.
//
// Usage:
//
//	gicel run     [flags] <file>   compile and execute
//	gicel check   [flags] <file>   type-check only
//	gicel lsp     [flags]          start language server (stdio)
//	gicel docs    [topic]          list topics or show topic
//	gicel example [name]           list examples or show source

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// version is the CLI version, set via -ldflags at build time.
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	switch os.Args[1] {
	case "run":
		os.Exit(cmdRun(os.Args[2:]))
	case "check":
		os.Exit(cmdCheck(os.Args[2:]))
	case "lsp":
		os.Exit(cmdLsp(os.Args[2:]))
	case "docs":
		os.Exit(cmdDocs(os.Args[2:]))
	case "example":
		os.Exit(cmdExample(os.Args[2:]))
	case "help", "-h", "--help":
		printUsage()
		os.Exit(0)
	case "version", "--version":
		fmt.Println("gicel " + version)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage: gicel <command> [flags] [args]

Commands:
  run      Compile and execute a GICEL program
  check    Type-check a GICEL program
  lsp      Start language server (stdio transport)
  docs     List topics, or show a topic (docs <topic>)
  example  List examples, or show source (example <name>)
  version  Print version information

Use 'gicel <command> --help' for flag details.

Packs: prelude,fail,state,io,stream,slice,map,set,array,ref,mmap,mset,json,console`)
}

// normalizeFlagError rewrites Go flag error messages to use -- prefix for
// multi-character flags, preserving single-character flags as -e style.
func normalizeFlagError(msg string) string {
	// "flag provided but not defined: -xyz" → "unknown flag: --xyz"
	if rest, ok := strings.CutPrefix(msg, "flag provided but not defined: -"); ok {
		rest = strings.TrimPrefix(rest, "-") // handle ---xyz edge case
		if len(rest) > 1 {
			return "unknown flag: --" + rest
		}
		return "unknown flag: -" + rest
	}
	// "invalid value ... for flag -xyz" → "invalid value ... for flag --xyz"
	if before, after, ok := strings.Cut(msg, " for flag -"); ok {
		after := after
		after = strings.TrimPrefix(after, "-")
		if len(after) > 1 {
			return before + " for flag --" + after
		}
	}
	return msg
}

func isHelpFlag(s string) bool {
	return s == "--help" || s == "-h" || s == "-help" || s == "help"
}

// catalogEntry is a name–description pair for docs/example listing.
type catalogEntry struct {
	name, desc string
}

// printCatalog groups entries by dot-separated category prefix and prints them.
func printCatalog(entries []catalogEntry, defaultLabel string) {
	groups := map[string][]catalogEntry{}
	var groupOrder []string
	for _, e := range entries {
		category := ""
		if i := strings.LastIndex(e.name, "."); i >= 0 {
			category = e.name[:i]
		}
		if _, seen := groups[category]; !seen {
			groupOrder = append(groupOrder, category)
		}
		groups[category] = append(groups[category], e)
	}
	for _, cat := range groupOrder {
		label := cat
		if label == "" {
			label = defaultLabel
		}
		fmt.Printf("%s:\n", label)
		for _, e := range groups[cat] {
			if e.desc != "" {
				fmt.Printf("  %-30s %s\n", e.name, e.desc)
			} else {
				fmt.Printf("  %s\n", e.name)
			}
		}
		fmt.Println()
	}
}

// warnTrailingFlags warns about flag-like arguments after the positional filename.
func warnTrailingFlags(fs *flag.FlagSet) {
	if fs.NArg() <= 1 {
		return
	}
	for _, a := range fs.Args()[1:] {
		if strings.HasPrefix(a, "-") {
			fmt.Fprintf(os.Stderr, "warning: %q after filename is ignored; place flags before the filename\n", a)
		}
	}
}
