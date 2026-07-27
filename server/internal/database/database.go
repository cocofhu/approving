// Package database opens the configured store (SQLite or MySQL), runs
// migrations, and seeds initial demo data so a fresh boot mirrors the frontend
// prototype.
package database

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cocofhu/approving/internal/config"
	"github.com/cocofhu/approving/internal/models"

	"github.com/rs/zerolog/log"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open opens the database for the configured driver and migrates the schema.
// SQLite is the zero-dependency default; MySQL is used when configured (driver
// == "mysql" or a DSN is present, resolved in config.setDefaults).
func Open(cfg config.DatabaseConfig) (*gorm.DB, error) {
	switch cfg.Driver {
	case "mysql":
		return openMySQL(cfg.DSN)
	default:
		return OpenSQLite(cfg.Path)
	}
}

// OpenSQLite opens (or creates) the SQLite database at path. WAL + a busy
// timeout plus a single writer connection keep the concurrent FSM goroutines
// and API readers from tripping over SQLite write locks. Exported so tests can
// spin up a file/memory DB without constructing a full DatabaseConfig.
func OpenSQLite(path string) (*gorm.DB, error) {
	db, err := openSQLiteConn(path)
	if err != nil {
		return nil, err
	}
	return finalize(db)
}

var (
	sqliteTemplateOnce sync.Once
	sqliteTemplate     []byte
	sqliteTemplateErr  error
)

// OpenSQLiteTest opens a migrated SQLite DB at path by cloning a process-wide
// schema template. Unit tests that open a fresh DB per case should prefer this
// over OpenSQLite: AutoMigrate of the full model set is multi-second on cold
// disks and routinely pushes engine/handlers packages near the default 10m
// go-test timeout when every case pays that cost.
func OpenSQLiteTest(path string) (*gorm.DB, error) {
	if err := ensureSQLiteTemplate(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, sqliteTemplate, 0o644); err != nil {
		return nil, err
	}
	return openSQLiteConn(path)
}

func ensureSQLiteTemplate() error {
	sqliteTemplateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "approving-schema-*")
		if err != nil {
			sqliteTemplateErr = err
			return
		}
		defer os.RemoveAll(dir)
		path := filepath.Join(dir, "template.db")
		// Build the template with DELETE journal so the schema lives in a single
		// file (no -wal/-shm) and a byte-for-byte copy is a complete DB.
		db, err := openSQLiteConn(path + "?_journal_mode=DELETE&_busy_timeout=5000&_foreign_keys=on")
		if err != nil {
			sqliteTemplateErr = err
			return
		}
		if _, err := finalize(db); err != nil {
			sqliteTemplateErr = err
			_ = closeGorm(db)
			return
		}
		if err := closeGorm(db); err != nil {
			sqliteTemplateErr = err
			return
		}
		sqliteTemplate, sqliteTemplateErr = os.ReadFile(path)
	})
	return sqliteTemplateErr
}

func closeGorm(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func openSQLiteConn(path string) (*gorm.DB, error) {
	dsn := path
	if !strings.Contains(dsn, "?") {
		dsn += "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on"
	}
	db, err := gorm.Open(sqlite.Open(dsn), gormConfig())
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// Serialize writes: SQLite allows only one writer; a single connection
	// avoids "database is locked" stalls under concurrent runs.
	sqlDB.SetMaxOpenConns(1)
	return db, nil
}

// openMySQL opens the MySQL database at dsn. Unlike SQLite it supports genuine
// concurrent writers, so the FSM goroutines each get their own pooled
// connection instead of being serialized behind a single one.
func openMySQL(dsn string) (*gorm.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("mysql driver selected but database.dsn is empty")
	}
	db, err := gorm.Open(mysql.Open(dsn), gormConfig())
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return finalize(db)
}

func gormConfig() *gorm.Config {
	return &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
}

// finalize runs the shared post-open steps (migrate + backfill) for any driver.
func finalize(db *gorm.DB) (*gorm.DB, error) {
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		return nil, err
	}
	migrateProjectMemoryIndexes(db)
	migrateChannelUniqueProject(db)
	backfillLegacyProjectMemories(db)
	backfillWorkflowIDs(db)
	ensureDefaultProject(db)
	return db, nil
}

// migrateProjectMemoryIndexes drops legacy unique indexes that predate
// agent_name scoping (project_id, title) so AutoMigrate can create
// idx_pm_mem_proj_agent_title cleanly.
func migrateProjectMemoryIndexes(db *gorm.DB) {
	legacy := []string{
		"idx_pm_mem_proj_title",
		"idx_project_memory_items_project_id_title",
		"udx_project_memory_items_project_id_title",
	}
	switch db.Dialector.Name() {
	case "mysql":
		for _, name := range legacy {
			_ = db.Exec("DROP INDEX `" + name + "` ON `project_memory_items`").Error
		}
	default:
		for _, name := range legacy {
			_ = db.Exec("DROP INDEX IF EXISTS `" + name + "`").Error
		}
	}
}

// migrateChannelUniqueProject enforces the "one channel per project" model at
// the DB level. Legacy rows (created when the old admin UI allowed several
// channels per project) are collapsed to the earliest-created row before the
// unique index is added, otherwise index creation would fail.
func migrateChannelUniqueProject(db *gorm.DB) {
	var rows []models.ChannelConfig
	if err := db.Order("project_id asc, created_at asc, id asc").Find(&rows).Error; err != nil {
		return
	}
	seen := map[string]bool{}
	var dupeIDs []string
	for _, r := range rows {
		if seen[r.ProjectID] {
			dupeIDs = append(dupeIDs, r.ID)
			continue
		}
		seen[r.ProjectID] = true
	}
	if len(dupeIDs) > 0 {
		if err := db.Where("id IN ?", dupeIDs).Delete(&models.ChannelConfig{}).Error; err != nil {
			log.Warn().Err(err).Int("count", len(dupeIDs)).Msg("channel dedup: failed to drop duplicate per-project channels")
			return
		}
		log.Warn().Int("count", len(dupeIDs)).Msg("channel dedup: collapsed duplicate per-project channels to the earliest-created row")
	}
	const stmt = "CREATE UNIQUE INDEX udx_channel_configs_project_id ON channel_configs (project_id)"
	switch db.Dialector.Name() {
	case "mysql":
		_ = db.Exec(stmt).Error // no-op if it already exists
	default:
		_ = db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS udx_channel_configs_project_id ON channel_configs (project_id)").Error
	}
}

// backfillLegacyProjectMemories assigns empty agent_name memories to each
// project's bound PM Leader agent once at startup (not on every GetBinding).
func backfillLegacyProjectMemories(db *gorm.DB) {
	var projects []models.Project
	if err := db.Where("pm_leader_agent != ? AND pm_leader_agent IS NOT NULL", "").Find(&projects).Error; err != nil {
		return
	}
	for _, p := range projects {
		agent := strings.TrimSpace(p.PmLeaderAgent)
		if agent == "" {
			continue
		}
		_ = db.Model(&models.ProjectMemoryItem{}).
			Where("project_id = ? AND (agent_name = '' OR agent_name IS NULL)", p.ID).
			Update("agent_name", agent).Error
	}
}

// ensureDefaultProject creates「默认项目」when none exist and backfills any
// workflow_defs rows still missing a project_id. The default project has no
// special protection afterward (rename/delete-when-empty like any other).
func ensureDefaultProject(db *gorm.DB) {
	var count int64
	db.Model(&models.Project{}).Count(&count)
	if count == 0 {
		now := time.Now()
		_ = db.Create(&models.Project{
			ID:           models.DefaultProjectID,
			Name:         models.DefaultProjectName,
			SandboxEnv:   []models.EnvEntry{},
			Variables:    []models.ProjectVariable{},
			NotifyPolicy: models.DefaultProjectNotifyPolicy(),
			CreatedAt:    now,
			UpdatedAt:    now,
		}).Error
	}
	var defaultID string
	var p models.Project
	if err := db.Where("id = ?", models.DefaultProjectID).First(&p).Error; err == nil {
		defaultID = p.ID
	} else if err := db.Order("created_at asc").First(&p).Error; err == nil {
		defaultID = p.ID
	}
	if defaultID == "" {
		return
	}
	db.Exec("UPDATE workflow_defs SET project_id = ? WHERE project_id IS NULL OR project_id = ''", defaultID)
}

// backfillWorkflowIDs populates the workflow_id column on gates/artifacts rows
// created before the column existed, resolving it from each row's run. Best
// effort: errors are ignored (a missing run simply leaves the field empty).
func backfillWorkflowIDs(db *gorm.DB) {
	for _, table := range []string{"gates", "artifacts"} {
		db.Exec("UPDATE " + table + " SET workflow_id = (SELECT workflow_id FROM runs WHERE runs.id = " + table + ".run_id) " +
			"WHERE (workflow_id IS NULL OR workflow_id = '') AND run_id IN (SELECT id FROM runs)")
	}
}
