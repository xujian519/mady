package server

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/skill"
)

// --- skill handlers ---

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeSkillAPI(w, r, false) {
		return
	}
	writeJSON(w, http.StatusOK, SkillsResponse{
		Skills: s.skillSummaries(),
	})
}

func (s *Server) handleSkillDiagnostics(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeSkillAPI(w, r, false) {
		return
	}
	cfg := s.snapshotConfig()
	writeJSON(w, http.StatusOK, SkillDiagnosticsResponse{
		Diagnostics: cloneSkillDiagnostics(cfg.SkillDiagnostics),
	})
}

func (s *Server) handleSkillStatus(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeSkillAPI(w, r, false) {
		return
	}
	cfg := s.snapshotConfig()
	threadID := strings.TrimSpace(r.URL.Query().Get("thread_id"))
	selectedSkills := agentcore.CloneStringSlice(cfg.SelectedSkills)
	effectiveSkills := agentcore.CloneStringSlice(cfg.SelectedSkills)
	hasThreadConfig := false
	if threadID != "" {
		threadCfg, ok, err := s.threadCallConfig(r.Context(), threadID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		hasThreadConfig = ok
		effectiveSkills = effectiveSkillSelection(cfg.SelectedSkills, threadCfg)
	}
	skills := skillSummariesFor(cfg.AvailableSkills, selectedSkills)
	diagnostics := cloneSkillDiagnostics(cfg.SkillDiagnostics)
	_, missing := skill.ResolveSelection(cfg.AvailableSkills, effectiveSkills)
	var visible, hidden int
	for _, item := range skills {
		if item.DisableModelInvocation {
			hidden++
		} else {
			visible++
		}
	}
	writeJSON(w, http.StatusOK, SkillRegistryStatusResponse{
		Skills:                  skills,
		ThreadID:                threadID,
		HasThreadConfig:         hasThreadConfig,
		SelectedSkills:          selectedSkills,
		EffectiveSelectedSkills: effectiveSkills,
		MissingSelectedSkills:   missing,
		SkillPaths:              agentcore.CloneStringSlice(cfg.SkillPaths),
		Reloadable:              len(cfg.SkillPaths) > 0,
		Diagnostics:             diagnostics,
		TotalSkills:             len(skills),
		VisibleSkills:           visible,
		HiddenSkills:            hidden,
		DiagnosticsCount:        len(diagnostics),
	})
}

func (s *Server) handleSkillEvents(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeSkillAPI(w, r, false) {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	var dead atomic.Bool
	var mu sync.Mutex
	writeSSE := func(eventType string, data any) {
		if dead.Load() {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if dead.Load() {
			return
		}
		payload, err := json.Marshal(data)
		if err != nil {
			slog.Error("server: SSE marshal error", "event", eventType, "err", err)
			dead.Store(true)
			return
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, payload); err != nil {
			slog.Error("server: SSE write error", "event", eventType, "err", err)
			dead.Store(true)
			return
		}
		flusher.Flush()
	}

	writeSSE("skills_snapshot", skillSnapshotEventPayload(s.snapshotConfig()))

	ch := make(chan agentcore.Event, 8)
	unregister := s.On(agentcore.EventSkillsReloaded, func(e agentcore.Event) {
		select {
		case ch <- e:
		default:
		}
	})
	defer unregister()

	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			writeSSE(string(e.EventKind()), streamEventPayload("", e))
		}
	}
}

func (s *Server) handleReloadSkills(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeSkillAPI(w, r, true) {
		return
	}
	cfg := s.snapshotConfig()
	if len(cfg.SkillPaths) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "skill reload not configured"})
		return
	}
	skills, diagnostics, err := skill.Load(cfg.SkillPaths...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	oldCfg := s.config.Get()
	oldSkills := cloneSkills(oldCfg.AvailableSkills)
	oldDiagnostics := cloneSkillDiagnostics(oldCfg.SkillDiagnostics)
	newCfg := oldCfg
	newCfg.AvailableSkills = cloneSkills(skills)
	newCfg.SkillDiagnostics = cloneSkillDiagnostics(diagnostics)
	s.config.Set(newCfg)
	cfg = s.snapshotConfig()
	skillsSummary := skillSummariesFor(cfg.AvailableSkills, cfg.SelectedSkills)
	oldSkillSummaries := skillSummariesFor(oldSkills, cfg.SelectedSkills)
	addedSkills, removedSkills, updatedSkills := diffSkillSummaries(oldSkillSummaries, skillsSummary)
	addedDiagnostics, removedDiagnostics := diffSkillDiagnostics(oldDiagnostics, cfg.SkillDiagnostics)
	var visible, hidden int
	for _, item := range skillsSummary {
		if item.DisableModelInvocation {
			hidden++
		} else {
			visible++
		}
	}
	_, missing := skill.ResolveSelection(cfg.AvailableSkills, cfg.SelectedSkills)
	s.EmitEvent(agentcore.NewSkillsReloadedEvent(
		cfg.SkillPaths,
		len(skillsSummary),
		visible,
		hidden,
		len(cfg.SkillDiagnostics),
		addedSkills,
		removedSkills,
		updatedSkills,
		addedDiagnostics,
		removedDiagnostics,
	))
	writeJSON(w, http.StatusOK, SkillRegistryStatusResponse{
		Skills:                  skillsSummary,
		SelectedSkills:          agentcore.CloneStringSlice(cfg.SelectedSkills),
		EffectiveSelectedSkills: agentcore.CloneStringSlice(cfg.SelectedSkills),
		MissingSelectedSkills:   missing,
		AddedSkills:             addedSkills,
		RemovedSkills:           removedSkills,
		UpdatedSkills:           updatedSkills,
		AddedDiagnostics:        addedDiagnostics,
		RemovedDiagnostics:      removedDiagnostics,
		SkillPaths:              agentcore.CloneStringSlice(cfg.SkillPaths),
		Reloadable:              true,
		Diagnostics:             cloneSkillDiagnostics(cfg.SkillDiagnostics),
		TotalSkills:             len(skillsSummary),
		VisibleSkills:           visible,
		HiddenSkills:            hidden,
		DiagnosticsCount:        len(cfg.SkillDiagnostics),
	})
}

// --- skill helpers ---

func (s *Server) authorizeSkillAPI(w http.ResponseWriter, r *http.Request, reload bool) bool {
	cfg := s.snapshotConfig()
	if (!reload && cfg.DisableSkillRegistryAPI) || (reload && cfg.DisableSkillReloadAPI) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill API disabled"})
		return false
	}
	token := strings.TrimSpace(cfg.SkillAPIAuthToken)
	if token == "" {
		return true
	}
	if r == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return false
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	expected := "Bearer " + token
	if subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) == 1 {
		return true
	}
	w.Header().Set("WWW-Authenticate", `Bearer realm="skills"`)
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid authorization"})
	return false
}

func (s *Server) skillSummaries() []SkillSummary {
	cfg := s.snapshotConfig()
	return skillSummariesFor(cfg.AvailableSkills, cfg.SelectedSkills)
}

func cloneSkillMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneSkillDiagnostics(in []skill.Diagnostic) []skill.Diagnostic {
	if len(in) == 0 {
		return nil
	}
	out := make([]skill.Diagnostic, len(in))
	copy(out, in)
	return out
}

func cloneSkills(in []skill.Skill) []skill.Skill {
	if len(in) == 0 {
		return nil
	}
	out := make([]skill.Skill, 0, len(in))
	for _, item := range in {
		cp := item
		cp.AllowedTools = agentcore.CloneStringSlice(item.AllowedTools)
		cp.Metadata = cloneSkillMetadata(item.Metadata)
		out = append(out, cp)
	}
	return out
}

func skillSummariesFor(skills []skill.Skill, selected []string) []SkillSummary {
	if len(skills) == 0 {
		return nil
	}
	selectedSet := make(map[string]bool, len(selected))
	for _, name := range selected {
		selectedSet[name] = true
	}
	out := make([]SkillSummary, 0, len(skills))
	for _, item := range skills {
		out = append(out, SkillSummary{
			Name:                   item.Name,
			Description:            item.Description,
			FilePath:               item.FilePath,
			BaseDir:                item.BaseDir,
			DisableModelInvocation: item.DisableModelInvocation,
			Metadata:               cloneSkillMetadata(item.Metadata),
			SelectedByDefault:      selectedSet[item.Name],
		})
	}
	return out
}

func effectiveSkillSelection(defaultSkills []string, threadCfg *agentcore.CallConfig) []string {
	base := &agentcore.CallConfig{Skills: agentcore.CloneStringSlice(defaultSkills)}
	merged := agentcore.MergeCallConfig(base, threadCfg)
	if merged == nil {
		return nil
	}
	return agentcore.CloneStringSlice(merged.Skills)
}

func skillSnapshotEventPayload(cfg agentcore.Config) SkillsSnapshotStreamEvent {
	skills := skillSummariesFor(cfg.AvailableSkills, cfg.SelectedSkills)
	var visible, hidden int
	for _, item := range skills {
		if item.DisableModelInvocation {
			hidden++
		} else {
			visible++
		}
	}
	return SkillsSnapshotStreamEvent{
		Schema:    streamSchemaSkillsSnapshot,
		Type:      "skills_snapshot",
		Timestamp: time.Now(),
		Payload: SkillRegistryStatusResponse{
			Skills:                  skills,
			SelectedSkills:          agentcore.CloneStringSlice(cfg.SelectedSkills),
			EffectiveSelectedSkills: agentcore.CloneStringSlice(cfg.SelectedSkills),
			SkillPaths:              agentcore.CloneStringSlice(cfg.SkillPaths),
			Reloadable:              len(cfg.SkillPaths) > 0,
			Diagnostics:             cloneSkillDiagnostics(cfg.SkillDiagnostics),
			TotalSkills:             len(skills),
			VisibleSkills:           visible,
			HiddenSkills:            hidden,
			DiagnosticsCount:        len(cfg.SkillDiagnostics),
		},
	}
}

func diffSkillSummaries(oldSkills, newSkills []SkillSummary) (added, removed, updated []string) {
	oldByName := make(map[string]SkillSummary, len(oldSkills))
	newByName := make(map[string]SkillSummary, len(newSkills))
	for _, item := range oldSkills {
		oldByName[item.Name] = item
	}
	for _, item := range newSkills {
		newByName[item.Name] = item
	}
	for name, newItem := range newByName {
		oldItem, ok := oldByName[name]
		if !ok {
			added = append(added, name)
			continue
		}
		if !skillSummaryEqual(oldItem, newItem) {
			updated = append(updated, name)
		}
	}
	for name := range oldByName {
		if _, ok := newByName[name]; !ok {
			removed = append(removed, name)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(updated)
	return added, removed, updated
}

func diffSkillDiagnostics(oldDiagnostics, newDiagnostics []skill.Diagnostic) (added, removed []skill.Diagnostic) {
	oldByKey := make(map[string]skill.Diagnostic, len(oldDiagnostics))
	newByKey := make(map[string]skill.Diagnostic, len(newDiagnostics))
	for _, item := range oldDiagnostics {
		oldByKey[item.Path+"\x00"+item.Message] = item
	}
	for _, item := range newDiagnostics {
		newByKey[item.Path+"\x00"+item.Message] = item
	}
	for key, item := range newByKey {
		if _, ok := oldByKey[key]; !ok {
			added = append(added, item)
		}
	}
	for key, item := range oldByKey {
		if _, ok := newByKey[key]; !ok {
			removed = append(removed, item)
		}
	}
	sort.Slice(added, func(i, j int) bool {
		if added[i].Path == added[j].Path {
			return added[i].Message < added[j].Message
		}
		return added[i].Path < added[j].Path
	})
	sort.Slice(removed, func(i, j int) bool {
		if removed[i].Path == removed[j].Path {
			return removed[i].Message < removed[j].Message
		}
		return removed[i].Path < removed[j].Path
	})
	return added, removed
}

func skillSummaryEqual(a, b SkillSummary) bool {
	if a.Name != b.Name ||
		a.Description != b.Description ||
		a.FilePath != b.FilePath ||
		a.BaseDir != b.BaseDir ||
		a.DisableModelInvocation != b.DisableModelInvocation {
		return false
	}
	if len(a.Metadata) != len(b.Metadata) {
		return false
	}
	for key, value := range a.Metadata {
		if b.Metadata[key] != value {
			return false
		}
	}
	return true
}
