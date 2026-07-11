package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/xujian519/mady/a2a"
	"github.com/xujian519/mady/agentcore"
)

// stubProvider is a minimal provider for demonstration purposes.
type stubProvider struct{}

func (stubProvider) Complete(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	return &agentcore.ProviderResponse{
		Content: "Hello! I'm a demo A2A agent.",
	}, nil
}

func (stubProvider) Stream(ctx context.Context, req *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	ch := make(chan agentcore.StreamDelta, 1)
	ch <- agentcore.StreamDelta{Content: "Hello! I'm a demo A2A agent."}
	close(ch)
	return ch, nil
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "demo-a2a-agent",
			Model:    "stub",
			Provider: stubProvider{},
		},
		SystemPrompt: "You are a helpful demo agent exposed via the A2A protocol.",
	}

	// Create a simple agent
	agent := agentcore.New(cfg)
	defer agent.Close()

	// Define agent card
	card := a2a.AgentCard{
		Name:        "demo-a2a-agent",
		Description: "A demonstration A2A agent built with Mady",
		URL:         fmt.Sprintf("http://localhost:%s", port),
		Version:     "1.0.0",
		Capabilities: a2a.AgentCapabilities{
			Streaming:         true,
			PushNotifications: false,
		},
		Skills: []a2a.AgentSkill{
			{
				ID:          "echo",
				Name:        "Echo",
				Description: "Echo back the user's message",
				Tags:        []string{"demo", "echo"},
			},
			{
				ID:          "greet",
				Name:        "Greeting",
				Description: "Greet the user",
				Tags:        []string{"demo", "greeting"},
			},
		},
	}

	// Create A2A handler and server
	handler := a2a.NewDefaultAgentHandler(card, agent, cfg)
	server := a2a.NewServer(handler)

	log.Printf("A2A server starting on :%s", port)
	log.Printf("Agent Card: http://localhost:%s/.well-known/agent.json", port)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Println("shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	if err := server.ListenAndServe(":" + port); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
	}
}
