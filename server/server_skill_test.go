package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/iface"
	"github.com/xujian519/mady/session"
	"github.com/xujian519/mady/skill"
)

func TestServerSkillRegistryEndpoints(t *testing.T) {
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
		SkillConfig: agentcore.SkillConfig{
			AvailableSkills: []skill.Skill{
				{
					Name:        "planner",
					Description: "Plans work",
					FilePath:    "/skills/planner/SKILL.md",
					BaseDir:     "/skills/planner",
					Body:        "secret body should not be exposed",
					Metadata: map[string]string{
						"category": "planning",
					},
				},
				{
					Name:                   "debugger",
					Description:            "Debugs failures",
					FilePath:               "/skills/debugger/SKILL.md",
					BaseDir:                "/skills/debugger",
					DisableModelInvocation: true,
				},
			},
			SelectedSkills: []string{"planner"},
			SkillDiagnostics: []skill.Diagnostic{
				{Path: "/skills/debugger/SKILL.md", Message: "name contains invalid characters (must be lowercase a-z, 0-9, hyphens only)"},
				{Path: "/skills/planner/SKILL.md", Message: "description exceeds 1024 characters (1100)"},
			},
		},
	})

	var skillsResp SkillsResponse
	getJSON(t, srv.Handler(), http.MethodGet, "/api/skills", &skillsResp, http.StatusOK)
	if len(skillsResp.Skills) != 2 {
		t.Fatalf("skills = %#v", skillsResp)
	}
	if skillsResp.Skills[0].Name != "planner" || !skillsResp.Skills[0].SelectedByDefault {
		t.Fatalf("planner summary = %#v", skillsResp.Skills[0])
	}
	if skillsResp.Skills[0].Metadata["category"] != "planning" {
		t.Fatalf("planner metadata = %#v", skillsResp.Skills[0].Metadata)
	}
	rawSkillsBody := getRaw(t, srv.Handler(), http.MethodGet, "/api/skills", http.StatusOK)
	if strings.Contains(rawSkillsBody, "secret body should not be exposed") {
		t.Fatalf("skills endpoint leaked body: %s", rawSkillsBody)
	}

	var diagnosticsResp SkillDiagnosticsResponse
	getJSON(t, srv.Handler(), http.MethodGet, "/api/skills/diagnostics", &diagnosticsResp, http.StatusOK)
	if len(diagnosticsResp.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v", diagnosticsResp)
	}

	var statusResp SkillRegistryStatusResponse
	getJSON(t, srv.Handler(), http.MethodGet, "/api/skills/status", &statusResp, http.StatusOK)
	if statusResp.TotalSkills != 2 || statusResp.VisibleSkills != 1 || statusResp.HiddenSkills != 1 {
		t.Fatalf("status counts = %#v", statusResp)
	}
	if statusResp.DiagnosticsCount != 2 {
		t.Fatalf("diagnostics count = %#v", statusResp)
	}
	if len(statusResp.SelectedSkills) != 1 || statusResp.SelectedSkills[0] != "planner" {
		t.Fatalf("selected skills = %#v", statusResp.SelectedSkills)
	}
	if statusResp.Reloadable {
		t.Fatalf("reloadable = %#v", statusResp)
	}
}

func TestServerSkillStatusReflectsThreadOverride(t *testing.T) {
	sessionFS, err := session.NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	threadStore := session.NewAgentStore(sessionFS, "/project")
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
		SkillConfig: agentcore.SkillConfig{
			AvailableSkills: []skill.Skill{
				{Name: "planner", Description: "Plans work", FilePath: "/skills/planner/SKILL.md", BaseDir: "/skills/planner"},
				{Name: "debugger", Description: "Debugs failures", FilePath: "/skills/debugger/SKILL.md", BaseDir: "/skills/debugger"},
			},
			SelectedSkills: []string{"planner"},
		},
		Store: threadStore,
	})

	thread := postChat(t, srv.Handler(), ChatRequest{Message: "hello"})
	var putResp ThreadConfigResponse
	putJSON(t, srv.Handler(), "/api/threads/"+thread.ThreadID+"/config", ThreadConfigRequest{
		Config: &agentcore.CallConfig{
			Skills: []string{"debugger"},
		},
	}, &putResp, http.StatusOK)

	var statusResp SkillRegistryStatusResponse
	getJSON(t, srv.Handler(), http.MethodGet, "/api/skills/status?thread_id="+thread.ThreadID, &statusResp, http.StatusOK)
	if !statusResp.HasThreadConfig || statusResp.ThreadID != thread.ThreadID {
		t.Fatalf("thread status = %#v", statusResp)
	}
	if len(statusResp.SelectedSkills) != 1 || statusResp.SelectedSkills[0] != "planner" {
		t.Fatalf("default selected = %#v", statusResp.SelectedSkills)
	}
	if len(statusResp.EffectiveSelectedSkills) != 1 || statusResp.EffectiveSelectedSkills[0] != "debugger" {
		t.Fatalf("effective selected = %#v", statusResp.EffectiveSelectedSkills)
	}
	if len(statusResp.MissingSelectedSkills) != 0 {
		t.Fatalf("missing selected = %#v", statusResp.MissingSelectedSkills)
	}
}

func TestServerSkillReloadEndpoint(t *testing.T) {
	root := t.TempDir()
	mustWriteSkillFixture(t, filepath.Join(root, "planner", "SKILL.md"), `---
name: planner
description: Plans work
---
Planner body`)
	initialSkills, initialDiagnostics, err := skill.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
		SkillConfig: agentcore.SkillConfig{
			AvailableSkills:  initialSkills,
			SkillDiagnostics: initialDiagnostics,
			SelectedSkills:   []string{"planner"},
			SkillPaths:       []string{root},
		},
	})

	mustWriteSkillFixture(t, filepath.Join(root, "debugger", "SKILL.md"), `---
name: debugger
description: Debugs failures
disable-model-invocation: true
---
Debugger body`)
	mustWriteSkillFixture(t, filepath.Join(root, "broken", "SKILL.md"), `---
name: broken
---
Missing description`)

	var reloadResp SkillRegistryStatusResponse
	postJSON(t, srv.Handler(), "/api/skills/reload", nil, &reloadResp, http.StatusOK)
	if !reloadResp.Reloadable {
		t.Fatalf("reload response = %#v", reloadResp)
	}
	if reloadResp.TotalSkills != 2 || reloadResp.VisibleSkills != 1 || reloadResp.HiddenSkills != 1 {
		t.Fatalf("reload counts = %#v", reloadResp)
	}
	if reloadResp.DiagnosticsCount != 1 {
		t.Fatalf("reload diagnostics = %#v", reloadResp)
	}
	if len(reloadResp.AddedSkills) != 1 || reloadResp.AddedSkills[0] != "debugger" {
		t.Fatalf("reload added skills = %#v", reloadResp)
	}
	if len(reloadResp.RemovedSkills) != 0 || len(reloadResp.UpdatedSkills) != 0 {
		t.Fatalf("reload diff = %#v", reloadResp)
	}
	if len(reloadResp.AddedDiagnostics) != 1 || len(reloadResp.RemovedDiagnostics) != 0 {
		t.Fatalf("reload diagnostics diff = %#v", reloadResp)
	}
	if !strings.Contains(reloadResp.AddedDiagnostics[0].Path, "broken") || !strings.Contains(reloadResp.AddedDiagnostics[0].Message, "description") {
		t.Fatalf("reload added diagnostics = %#v", reloadResp.AddedDiagnostics)
	}
	if len(reloadResp.SkillPaths) != 1 || reloadResp.SkillPaths[0] != root {
		t.Fatalf("reload skill paths = %#v", reloadResp.SkillPaths)
	}

	var statusResp SkillRegistryStatusResponse
	getJSON(t, srv.Handler(), http.MethodGet, "/api/skills/status", &statusResp, http.StatusOK)
	if statusResp.TotalSkills != 2 || statusResp.DiagnosticsCount != 1 {
		t.Fatalf("status after reload = %#v", statusResp)
	}
}

func TestServerSkillReloadReportsDiffs(t *testing.T) {
	root := t.TempDir()
	mustWriteSkillFixture(t, filepath.Join(root, "planner", "SKILL.md"), `---
name: planner
description: Plans work
---
Planner body`)
	mustWriteSkillFixture(t, filepath.Join(root, "debugger", "SKILL.md"), `---
name: debugger
description: Debugs failures
---
Debugger body`)
	initialSkills, initialDiagnostics, err := skill.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
		SkillConfig: agentcore.SkillConfig{
			AvailableSkills:  initialSkills,
			SkillDiagnostics: initialDiagnostics,
			SkillPaths:       []string{root},
		},
	})
	defer srv.Close()

	mustWriteSkillFixture(t, filepath.Join(root, "planner", "SKILL.md"), `---
name: planner
description: Plans work with checklists
---
Updated planner body`)
	if err := os.RemoveAll(filepath.Join(root, "debugger")); err != nil {
		t.Fatal(err)
	}
	mustWriteSkillFixture(t, filepath.Join(root, "reviewer", "SKILL.md"), `---
name: reviewer
description: Reviews changes
---
Reviewer body`)

	var reloadResp SkillRegistryStatusResponse
	postJSON(t, srv.Handler(), "/api/skills/reload", nil, &reloadResp, http.StatusOK)
	if len(reloadResp.AddedSkills) != 1 || reloadResp.AddedSkills[0] != "reviewer" {
		t.Fatalf("added skills = %#v", reloadResp)
	}
	if len(reloadResp.RemovedSkills) != 1 || reloadResp.RemovedSkills[0] != "debugger" {
		t.Fatalf("removed skills = %#v", reloadResp)
	}
	if len(reloadResp.UpdatedSkills) != 1 || reloadResp.UpdatedSkills[0] != "planner" {
		t.Fatalf("updated skills = %#v", reloadResp)
	}
	if len(reloadResp.AddedDiagnostics) != 0 || len(reloadResp.RemovedDiagnostics) != 0 {
		t.Fatalf("diagnostics diff = %#v", reloadResp)
	}
}

func TestServerSkillReloadReportsDiagnosticDiffs(t *testing.T) {
	root := t.TempDir()
	mustWriteSkillFixture(t, filepath.Join(root, "planner", "SKILL.md"), `---
name: planner
description: Plans work
---
Planner body`)
	mustWriteSkillFixture(t, filepath.Join(root, "broken", "SKILL.md"), `---
name: broken
---
Missing description`)
	initialSkills, initialDiagnostics, err := skill.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(initialDiagnostics) != 1 {
		t.Fatalf("initial diagnostics = %#v", initialDiagnostics)
	}
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
		SkillConfig: agentcore.SkillConfig{
			AvailableSkills:  initialSkills,
			SkillDiagnostics: initialDiagnostics,
			SkillPaths:       []string{root},
		},
	})
	defer srv.Close()

	mustWriteSkillFixture(t, filepath.Join(root, "broken", "SKILL.md"), `---
name: broken
description: Fixed description
---
Recovered body`)

	var reloadResp SkillRegistryStatusResponse
	postJSON(t, srv.Handler(), "/api/skills/reload", nil, &reloadResp, http.StatusOK)
	if len(reloadResp.AddedDiagnostics) != 0 {
		t.Fatalf("unexpected added diagnostics = %#v", reloadResp.AddedDiagnostics)
	}
	if len(reloadResp.RemovedDiagnostics) != 1 {
		t.Fatalf("removed diagnostics = %#v", reloadResp.RemovedDiagnostics)
	}
	if !strings.Contains(reloadResp.RemovedDiagnostics[0].Path, "broken") || !strings.Contains(reloadResp.RemovedDiagnostics[0].Message, "description") {
		t.Fatalf("removed diagnostics payload = %#v", reloadResp.RemovedDiagnostics)
	}
}

func TestServerSkillReloadEmitsEvent(t *testing.T) {
	root := t.TempDir()
	mustWriteSkillFixture(t, filepath.Join(root, "planner", "SKILL.md"), `---
name: planner
description: Plans work
---
Planner body`)
	initialSkills, initialDiagnostics, err := skill.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
		SkillConfig: agentcore.SkillConfig{
			AvailableSkills:  initialSkills,
			SkillDiagnostics: initialDiagnostics,
			SkillPaths:       []string{root},
		},
	})
	defer srv.Close()

	events := make(chan agentcore.SkillsReloadedEvent, 1)
	srv.On(iface.EventType(agentcore.EventSkillsReloaded), func(e iface.Event) {
		if raw := e.Payload(); raw != nil {
			if ev, ok := raw.(agentcore.SkillsReloadedEvent); ok {
				events <- ev
			}
			if ev, ok := raw.(*agentcore.SkillsReloadedEvent); ok {
				events <- *ev
			}
		}
	})

	mustWriteSkillFixture(t, filepath.Join(root, "debugger", "SKILL.md"), `---
name: debugger
description: Debugs failures
---
Debugger body`)
	mustWriteSkillFixture(t, filepath.Join(root, "broken", "SKILL.md"), `---
name: broken
---
Missing description`)

	var reloadResp SkillRegistryStatusResponse
	postJSON(t, srv.Handler(), "/api/skills/reload", nil, &reloadResp, http.StatusOK)

	select {
	case ev := <-events:
		if ev.EventKind() != agentcore.EventSkillsReloaded || ev.TotalSkills != 2 || ev.VisibleSkills != 2 || ev.DiagnosticsCount != 1 {
			t.Fatalf("reload event = %#v", ev)
		}
		if len(ev.AddedSkills) != 1 || ev.AddedSkills[0] != "debugger" {
			t.Fatalf("reload event diff = %#v", ev)
		}
		if len(ev.AddedDiagnostics) != 1 || len(ev.RemovedDiagnostics) != 0 {
			t.Fatalf("reload event diagnostics diff = %#v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for skills_reloaded event")
	}
}

func TestServerSkillEventsEndpointStreamsReloadEvents(t *testing.T) {
	root := t.TempDir()
	mustWriteSkillFixture(t, filepath.Join(root, "planner", "SKILL.md"), `---
name: planner
description: Plans work
---
Planner body`)
	initialSkills, initialDiagnostics, err := skill.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
		SkillConfig: agentcore.SkillConfig{
			AvailableSkills:  initialSkills,
			SkillDiagnostics: initialDiagnostics,
			SkillPaths:       []string{root},
		},
	})
	defer srv.Close()

	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	stream := openSSEStream(t, httpSrv.URL+"/api/skills/events", nil)
	defer stream.cancel()
	<-stream.ready

	snapshot := nextSSEEvent(t, stream, 3*time.Second)
	if snapshot.Event != "skills_snapshot" {
		t.Fatalf("snapshot event = %#v", snapshot)
	}
	var snapshotPayload SkillsSnapshotStreamEvent
	if err := json.Unmarshal(snapshot.Data, &snapshotPayload); err != nil {
		t.Fatalf("decode snapshot payload: %v", err)
	}
	if snapshotPayload.Schema != streamSchemaSkillsSnapshot || snapshotPayload.Type != "skills_snapshot" {
		t.Fatalf("snapshot payload = %#v", snapshotPayload)
	}
	if snapshotPayload.Payload.TotalSkills != 1 || snapshotPayload.Payload.DiagnosticsCount != 0 {
		t.Fatalf("snapshot payload body = %#v", snapshotPayload.Payload)
	}

	mustWriteSkillFixture(t, filepath.Join(root, "debugger", "SKILL.md"), `---
name: debugger
description: Debugs failures
---
Debugger body`)
	mustWriteSkillFixture(t, filepath.Join(root, "broken", "SKILL.md"), `---
name: broken
---
Missing description`)

	var reloadResp SkillRegistryStatusResponse
	postJSON(t, srv.Handler(), "/api/skills/reload", nil, &reloadResp, http.StatusOK)

	ev := nextSSEEvent(t, stream, 3*time.Second)
	if ev.Event != "skills_reloaded" {
		t.Fatalf("event = %#v", ev)
	}
	var payload AgentStreamEvent
	if err := json.Unmarshal(ev.Data, &payload); err != nil {
		t.Fatalf("decode sse payload: %v", err)
	}
	if payload.Schema != streamSchemaAgentEvent || payload.Type != string(agentcore.EventSkillsReloaded) {
		t.Fatalf("payload = %#v", payload)
	}
	body, _ := json.Marshal(payload.Payload)
	if !strings.Contains(string(body), `"added_skills":["debugger"]`) {
		t.Fatalf("payload body = %s", body)
	}
	if !strings.Contains(string(body), `"added_diagnostics":[`) {
		t.Fatalf("payload diagnostics body = %s", body)
	}
}

func TestServerSkillAPIAuthorizationAndDisableSwitches(t *testing.T) {
	root := t.TempDir()
	mustWriteSkillFixture(t, filepath.Join(root, "planner", "SKILL.md"), `---
name: planner
description: Plans work
---
Planner body`)
	loadedSkills, loadedDiagnostics, err := skill.Load(root)
	if err != nil {
		t.Fatal(err)
	}

	disabled := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
		SkillConfig: agentcore.SkillConfig{
			AvailableSkills:         loadedSkills,
			SkillDiagnostics:        loadedDiagnostics,
			DisableSkillRegistryAPI: true,
			DisableSkillReloadAPI:   true,
			SkillPaths:              []string{root},
		},
	})
	resp := doRequest(t, disabled.Handler(), http.MethodGet, "/api/skills", nil, nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("registry status = %d body=%s", resp.Code, resp.Body.String())
	}
	resp = doRequest(t, disabled.Handler(), http.MethodGet, "/api/skills/events", nil, nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("events status = %d body=%s", resp.Code, resp.Body.String())
	}
	resp = doRequest(t, disabled.Handler(), http.MethodPost, "/api/skills/reload", nil, nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("reload status = %d body=%s", resp.Code, resp.Body.String())
	}

	protected := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
		SkillConfig: agentcore.SkillConfig{
			AvailableSkills:   loadedSkills,
			SkillDiagnostics:  loadedDiagnostics,
			SkillPaths:        []string{root},
			SkillAPIAuthToken: "secret-token",
		},
	})
	resp = doRequest(t, protected.Handler(), http.MethodGet, "/api/skills/status", nil, nil)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d body=%s", resp.Code, resp.Body.String())
	}
	resp = doRequest(t, protected.Handler(), http.MethodGet, "/api/skills/events", nil, nil)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized events status = %d body=%s", resp.Code, resp.Body.String())
	}
	resp = doRequest(t, protected.Handler(), http.MethodGet, "/api/skills/status", nil, map[string]string{
		"Authorization": "Bearer secret-token",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("authorized status = %d body=%s", resp.Code, resp.Body.String())
	}
	resp = doRequest(t, protected.Handler(), http.MethodPost, "/api/skills/reload", nil, map[string]string{
		"Authorization": "Bearer secret-token",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("authorized reload status = %d body=%s", resp.Code, resp.Body.String())
	}
}
