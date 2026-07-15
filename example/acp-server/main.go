// Command acp-server runs Mady behind the Agent Client Protocol (ACP) so it can
// be used as an agent inside ACP-compatible editors such as Zed.
//
// The server speaks stdio JSON-RPC and is configured entirely through
// environment variables (see package agentconfig). Build it and point your
// editor's agent configuration at the resulting binary.
//
//	$ go build -o mady-acp ./example/acp-server/
//	$ PROVIDER=deepseek API_KEY=sk-... ./mady-acp
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/xujian519/mady/acp"
	"github.com/xujian519/mady/pkg/agentconfig"
)

// commitHash / buildTime are injected via -ldflags at release build time.
var (
	commitHash = "unknown" //nolint:unused // ldflags
	buildTime  = "unknown" //nolint:unused // ldflags
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "mady-acp-server: %v\n", err)
		stop()
		os.Exit(1) //nolint:gocritic // exitAfterDefer: stop() manually called above
	}
}

func run(ctx context.Context) error {
	provider, err := agentconfig.BuildProvider()
	if err != nil {
		return err
	}
	return acp.RunServer(ctx, acp.RunOptions{
		Provider: provider,
		Model:    agentconfig.DefaultModel(),
		Thinking: agentconfig.ThinkingFromEnv(),
		AgentInfo: acp.AgentInfo{
			Name:    "mady",
			Version: "0.1.0",
		},
	})
}
