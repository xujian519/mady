package main

// This file implements the `mady acp` subcommand: run as an ACP
// (Agent Client Protocol) server over stdio JSON-RPC, for editors like Zed.

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/xujian519/mady/acp"
	"github.com/xujian519/mady/pkg/agentconfig"
)

func runAcp(ctx context.Context) {
	fs := flag.NewFlagSet("mady acp", flag.ExitOnError)
	if err := fs.Parse(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "mady acp: %v\n", err)
		os.Exit(1)
	}

	fc := setupFrameworkContext(ctx)

	err := acp.RunServer(ctx, acp.RunOptions{
		Provider:   fc.Provider,
		Model:      agentconfig.DefaultModel(),
		Thinking:   agentconfig.ThinkingFromEnv(),
		Lifecycle:  fc.WikiHook,
		Extensions: extSlice(fc.KnowledgeExt),
		AgentInfo: acp.AgentInfo{
			Name:    "mady",
			Version: "0.1.0",
		},
	})
	if err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "mady acp: %v\n", err)
		os.Exit(1)
	}
}
