// lsp subcommand — start the Language Server Protocol server over stdio.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/cwd-k2/gicel/internal/app/engine"
	"github.com/cwd-k2/gicel/internal/lsp"
	"github.com/cwd-k2/gicel/internal/lsp/jsonrpc"
)

func cmdLsp(args []string) int {
	fs := flag.NewFlagSet("lsp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	packs := fs.String("packs", "all", "comma-separated stdlib packs")
	recursion := fs.Bool("recursion", false, "enable recursive definitions (fix/rec)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printLspUsage(os.Stderr)
			return 0
		}
		fmt.Fprintf(os.Stderr, "error: %s\n", normalizeFlagError(err.Error()))
		printLspUsage(os.Stderr)
		return 1
	}

	// Pre-validate packs.
	if _, err := setupEngine(*packs); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	packsCopy := *packs
	recCopy := *recursion
	logger := log.New(os.Stderr, "[gicel-lsp] ", log.LstdFlags)
	transport := jsonrpc.NewTransport(os.Stdin, os.Stdout)

	srv := lsp.NewServer(lsp.ServerConfig{
		Transport: transport,
		Logger:    logger,
		EngineSetup: func() lsp.AnalysisEngine {
			eng, err := setupEngine(packsCopy)
			if err != nil {
				logger.Printf("engine setup: %v", err)
				return engine.NewEngine() // fallback to bare engine
			}
			if recCopy {
				eng.EnableRecursion()
			}
			eng.DenyAssumptions()
			return eng
		},
	})

	if err := srv.Run(context.Background()); err != nil {
		logger.Printf("server error: %v", err)
		return 1
	}
	return srv.ExitCode()
}

func printLspUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: gicel lsp [flags]

Start the Language Server Protocol server (stdio transport).

Flags:
  --packs <list>  Stdlib packs (default: all)
  --recursion     Enable recursive definitions (fix/rec)`)
}
