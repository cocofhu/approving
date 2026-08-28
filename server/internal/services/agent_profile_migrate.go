package services

import (
	"fmt"
	"strings"

	"github.com/cocofhu/approving/internal/models"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// Legacy node config key for Agent identity references. Kept only for the
// one-shot startup/import migrator; runtime hot paths must not read it.
const legacyAgentProfileKey = "skill_profile"

// agentProfileConfigKey is the current node config key for Agent identity refs.
const agentProfileConfigKey = "agent_profile"

// MigrateAgentProfileInGraph rewrites nodes[].config from the legacy
// skill_profile key to agent_profile in place:
//   - only legacy key → copy value to agent_profile, delete legacy
//   - both keys → keep agent_profile, delete legacy
//   - only agent_profile → no-op
// Empty-string values are preserved (empty still means "skip validation").
// Returns whether any node config map was changed. Idempotent / reentrant.
func MigrateAgentProfileInGraph(g *models.Graph) bool {
	if g == nil {
		return false
	}
	changed := false
	for i := range g.Nodes {
		cfg := g.Nodes[i].Config
		if cfg == nil {
			continue
		}
		_, hasNew := cfg[agentProfileConfigKey]
		oldRaw, hasOld := cfg[legacyAgentProfileKey]
		if !hasOld {
			continue
		}
		if !hasNew {
			cfg[agentProfileConfigKey] = legacyProfileString(oldRaw)
		}
		delete(cfg, legacyAgentProfileKey)
		changed = true
	}
	return changed
}

func legacyProfileString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// MigrateAgentProfilesOnce rewrites persisted WorkflowDef / WorkflowVersion /
// Run graphs so nodes only carry agent_profile. Idempotent: graphs that already
// use the new key alone are left unchanged. Failures on individual rows are
// logged and skipped so one bad row does not block boot.
func MigrateAgentProfilesOnce(db *gorm.DB) {
	if db == nil {
		return
	}
	migrateAgentProfilesDefs(db)
	migrateAgentProfilesVersions(db)
	migrateAgentProfilesRuns(db)
}

func migrateAgentProfilesDefs(db *gorm.DB) {
	var defs []models.WorkflowDef
	// LIKE catches either key; reentrant rows with only agent_profile are no-ops.
	if err := db.Where("graph LIKE ?", "%skill_profile%").Find(&defs).Error; err != nil {
		log.Warn().Err(err).Msg("agent_profile migrate: list workflow defs failed")
		return
	}
	n := 0
	for i := range defs {
		wf := &defs[i]
		if !MigrateAgentProfileInGraph(&wf.Graph) {
			continue
		}
		if err := db.Model(wf).Update("graph", wf.Graph).Error; err != nil {
			log.Warn().Err(err).Str("workflow", wf.ID).Msg("agent_profile migrate: save workflow def failed")
			continue
		}
		n++
	}
	if n > 0 {
		log.Info().Int("count", n).Msg("agent_profile migrate: WorkflowDef graphs rewritten")
	}
}

func migrateAgentProfilesVersions(db *gorm.DB) {
	var versions []models.WorkflowVersion
	if err := db.Where("graph LIKE ?", "%skill_profile%").Find(&versions).Error; err != nil {
		log.Warn().Err(err).Msg("agent_profile migrate: list workflow versions failed")
		return
	}
	n := 0
	for i := range versions {
		snap := &versions[i]
		if !MigrateAgentProfileInGraph(&snap.Graph) {
			continue
		}
		if err := db.Model(snap).Update("graph", snap.Graph).Error; err != nil {
			log.Warn().Err(err).Uint("version_id", snap.ID).Msg("agent_profile migrate: save workflow version failed")
			continue
		}
		n++
	}
	if n > 0 {
		log.Info().Int("count", n).Msg("agent_profile migrate: WorkflowVersion graphs rewritten")
	}
}

func migrateAgentProfilesRuns(db *gorm.DB) {
	var runs []models.Run
	if err := db.Where("graph LIKE ?", "%skill_profile%").Find(&runs).Error; err != nil {
		log.Warn().Err(err).Msg("agent_profile migrate: list runs failed")
		return
	}
	n := 0
	for i := range runs {
		run := &runs[i]
		if !MigrateAgentProfileInGraph(&run.Graph) {
			continue
		}
		if err := db.Model(run).Update("graph", run.Graph).Error; err != nil {
			log.Warn().Err(err).Str("run", run.ID).Msg("agent_profile migrate: save run graph failed")
			continue
		}
		n++
	}
	if n > 0 {
		log.Info().Int("count", n).Msg("agent_profile migrate: Run graphs rewritten")
	}
}
