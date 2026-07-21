package registry_test

import (
	"sync"
	"testing"

	"github.com/xujian519/mady/a2a/registry"
)

func newTestReg(name, url string) *registry.Registration {
	return &registry.Registration{
		Name: name, URL: url,
		Capabilities: []string{"streaming", "chat"},
		Skills:       []string{"skill-a", "skill-b"},
	}
}

func TestRegisterAndGet(t *testing.T) {
	r := registry.New()

	err := r.Register(newTestReg("agent-1", "http://localhost:8080"))
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	reg, ok := r.Get("agent-1")
	if !ok {
		t.Fatal("expected agent-1 to be found")
	}
	if reg.Name != "agent-1" {
		t.Errorf("got name %q, want %q", reg.Name, "agent-1")
	}
	if reg.URL != "http://localhost:8080" {
		t.Errorf("got url %q, want %q", reg.URL, "http://localhost:8080")
	}
}

func TestRegisterEmptyName(t *testing.T) {
	r := registry.New()
	err := r.Register(&registry.Registration{Name: "", URL: "http://localhost:8080"})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestRegisterEmptyURL(t *testing.T) {
	r := registry.New()
	err := r.Register(&registry.Registration{Name: "agent-1", URL: ""})
	if err == nil {
		t.Fatal("expected error for empty url")
	}
}

func TestDeregister(t *testing.T) {
	r := registry.New()
	_ = r.Register(newTestReg("agent-1", "http://localhost:8080"))

	r.Deregister("agent-1")
	_, ok := r.Get("agent-1")
	if ok {
		t.Fatal("expected agent-1 to be deregistered")
	}
}

func TestDeregisterNonExistent(t *testing.T) {
	r := registry.New()
	// Should not panic
	r.Deregister("non-existent")
}

func TestRegisterOverwrite(t *testing.T) {
	r := registry.New()
	_ = r.Register(newTestReg("agent-1", "http://localhost:8080"))
	_ = r.Register(&registry.Registration{
		Name: "agent-1", URL: "http://localhost:9090",
		Skills: []string{"skill-c"},
	})

	reg, ok := r.Get("agent-1")
	if !ok {
		t.Fatal("expected agent-1 to be found")
	}
	if reg.URL != "http://localhost:9090" {
		t.Errorf("got url %q, want %q", reg.URL, "http://localhost:9090")
	}

	// Old skills should be gone
	if got := r.ListBySkill("skill-a"); len(got) != 0 {
		t.Errorf("expected 0 agents for old skill-a, got %d", len(got))
	}
	// New skill should be indexed
	if got := r.ListBySkill("skill-c"); len(got) != 1 {
		t.Errorf("expected 1 agent for skill-c, got %d", len(got))
	}
}

func TestList(t *testing.T) {
	r := registry.New()
	_ = r.Register(newTestReg("agent-1", "http://localhost:8080"))
	_ = r.Register(newTestReg("agent-2", "http://localhost:8081"))

	agents := r.List()
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}
}

func TestListByCapability(t *testing.T) {
	r := registry.New()
	_ = r.Register(&registry.Registration{
		Name: "agent-1", URL: "http://localhost:8080",
		Capabilities: []string{"streaming", "chat"},
	})
	_ = r.Register(&registry.Registration{
		Name: "agent-2", URL: "http://localhost:8081",
		Capabilities: []string{"patent"},
	})
	_ = r.Register(&registry.Registration{
		Name: "agent-3", URL: "http://localhost:8082",
		Capabilities: []string{"streaming", "patent"},
	})

	streaming := r.ListByCapability("streaming")
	if len(streaming) != 2 {
		t.Errorf("expected 2 streaming agents, got %d", len(streaming))
	}

	patent := r.ListByCapability("patent")
	if len(patent) != 2 {
		t.Errorf("expected 2 patent agents, got %d", len(patent))
	}

	chat := r.ListByCapability("chat")
	if len(chat) != 1 {
		t.Errorf("expected 1 chat agent, got %d", len(chat))
	}
}

func TestListBySkill(t *testing.T) {
	r := registry.New()
	_ = r.Register(&registry.Registration{
		Name: "agent-1", URL: "http://localhost:8080",
		Skills: []string{"skill-a", "skill-b"},
	})
	_ = r.Register(&registry.Registration{
		Name: "agent-2", URL: "http://localhost:8081",
		Skills: []string{"skill-b"},
	})

	skillA := r.ListBySkill("skill-a")
	if len(skillA) != 1 {
		t.Errorf("expected 1 agent for skill-a, got %d", len(skillA))
	}

	skillB := r.ListBySkill("skill-b")
	if len(skillB) != 2 {
		t.Errorf("expected 2 agents for skill-b, got %d", len(skillB))
	}

	skillNone := r.ListBySkill("non-existent")
	if len(skillNone) != 0 {
		t.Errorf("expected 0 agents for non-existent skill, got %d", len(skillNone))
	}
}

func TestCount(t *testing.T) {
	r := registry.New()
	if c := r.Count(); c != 0 {
		t.Errorf("expected 0, got %d", c)
	}

	_ = r.Register(newTestReg("agent-1", "http://localhost:8080"))
	if c := r.Count(); c != 1 {
		t.Errorf("expected 1, got %d", c)
	}

	_ = r.Register(newTestReg("agent-2", "http://localhost:8081"))
	if c := r.Count(); c != 2 {
		t.Errorf("expected 2, got %d", c)
	}

	r.Deregister("agent-1")
	if c := r.Count(); c != 1 {
		t.Errorf("expected 1, got %d", c)
	}
}

func TestConcurrentAccess(t *testing.T) {
	r := registry.New()

	var wg sync.WaitGroup
	n := 100

	// 并发注册
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := "agent-" + string(rune('A'+i%26))
			_ = r.Register(&registry.Registration{
				Name: name, URL: "http://localhost:8080",
				Skills: []string{"skill-" + string(rune('a'+i%5))},
			})
		}(i)
	}
	wg.Wait()

	// 并发读取
	var readWg sync.WaitGroup
	for i := 0; i < n; i++ {
		readWg.Add(1)
		go func(i int) {
			defer readWg.Done()
			name := "agent-" + string(rune('A'+i%26))
			r.Get(name)
			r.List()
			r.ListByCapability("streaming")
			r.ListBySkill("skill-a")
			r.Count()
		}(i)
	}
	readWg.Wait()

	// 并发注销
	for i := 0; i < n; i++ {
		readWg.Add(1)
		go func(i int) {
			defer readWg.Done()
			r.Count()
			r.List()
		}(i)
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := "agent-" + string(rune('A'+i%26))
			r.Deregister(name)
		}(i)
	}
	wg.Wait()
	readWg.Wait()
}

func TestGetReturnsCopy(t *testing.T) {
	r := registry.New()
	_ = r.Register(newTestReg("agent-1", "http://localhost:8080"))

	reg, ok := r.Get("agent-1")
	if !ok {
		t.Fatal("expected agent-1 to be found")
	}

	// Modify the returned copy
	reg.URL = "http://evil:9999"

	// Verify the original is unchanged
	reg2, _ := r.Get("agent-1")
	if reg2.URL != "http://localhost:8080" {
		t.Errorf("original was mutated, got url %q", reg2.URL)
	}
}

func TestListReturnsCopies(t *testing.T) {
	r := registry.New()
	_ = r.Register(newTestReg("agent-1", "http://localhost:8080"))

	agents := r.List()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}

	// Modify the returned copy
	agents[0].Name = "hacked"

	// Verify the original is unchanged
	reg, _ := r.Get("agent-1")
	if reg.Name != "agent-1" {
		t.Errorf("original was mutated, got name %q", reg.Name)
	}
}
