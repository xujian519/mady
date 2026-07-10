package domains

import (
	"context"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// funcStep adapts a plain function to the agentcore.Step interface.
type funcStep struct {
	name string
	fn   func(ctx context.Context, input string) (string, error)
}

func (s *funcStep) Run(ctx context.Context, input string) (string, error) {
	return s.fn(ctx, input)
}

// BuildDomainGraph constructs the top-level domain routing DAG.
// The graph structure:
//
//	             ┌──────────┐
//	             │  router  │ (classify intent)
//	             └────┬─────┘
//	      ┌───────────┼───────────┬───────────┐
//	      ▼           ▼           ▼           ▼
//	┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐
//	│  chat   │ │assistant│ │ patent  │ │  legal  │
//	└─────────┘ └─────────┘ └─────────┘ └─────────┘
//
// The router uses conditional edges to auto-route to the correct domain.
func BuildDomainGraph(chatStep, assistantStep, patentStep, legalStep agentcore.Step) (*graph.CompiledGraph, error) {
	return BuildDomainGraphWithClassifier(chatStep, assistantStep, patentStep, legalStep, nil)
}

// BuildDomainGraphWithClassifier constructs the top-level domain routing DAG
// with an optional IntentClassifier. If classifier is nil, KeywordClassifier is used.
func BuildDomainGraphWithClassifier(chatStep, assistantStep, patentStep, legalStep agentcore.Step, classifier IntentClassifier) (*graph.CompiledGraph, error) {
	if classifier == nil {
		classifier = &KeywordClassifier{}
	}
	g := graph.NewGraph()

	// Router node: pass through the input; conditional edge handles routing.
	g.AddNode("router", &funcStep{name: "router", fn: func(_ context.Context, input string) (string, error) {
		return input, nil
	}})
	g.AddNode(DomainChat, chatStep)
	g.AddNode(DomainAssistant, assistantStep)
	g.AddNode(DomainPatent, patentStep)
	g.AddNode(DomainLegal, legalStep)

	// Conditional edge: route based on intent classification.
	g.AddConditionalEdge("router", func(ctx context.Context, output string) string {
		domain, _, err := classifier.Classify(ctx, output)
		if err != nil {
			return DomainChat
		}
		return domain
	}, []string{DomainChat, DomainAssistant, DomainPatent, DomainLegal})

	return g.Compile(graph.CompileOptions{EntryNode: "router"})
}
