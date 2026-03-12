package agentplatform

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/mssql/v2"
	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
)

const (
	agentDBGroup        = "agentplatform"
	tableAgentSessions  = "agent_sessions"
	tableAgentMessages  = "agent_messages"
	tableAgentMemories  = "agent_memories"
	tableAgentRuns      = "agent_runs"
	tableAgentRunEvents = "agent_run_events"
)

type sqliteRepository struct {
	db     gdb.DB
	dbType string
}

func newRepository(ctx context.Context, cfg *config) (Repository, error) {
	if cfg == nil {
		return nil, fmt.Errorf("agent platform config is nil")
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.Storage.Provider))
	if provider == "" {
		provider = "sqlite"
	}
	switch provider {
	case "sqlite":
		path := strings.TrimSpace(cfg.Storage.SQLitePath)
		if path == "" {
			path = "./runtime/agent.db"
		}
		if err := gfile.Mkdir(filepath.Dir(path)); err != nil {
			return nil, err
		}
		gdb.SetConfigGroup(agentDBGroup, gdb.ConfigGroup{{
			Type:    "sqlite",
			Link:    fmt.Sprintf("sqlite::@file(%s)", path),
			Charset: "utf8",
		}})
	case "mssql":
		host := strings.TrimSpace(cfg.Storage.MSSQL.Host)
		user := strings.TrimSpace(cfg.Storage.MSSQL.User)
		pass := cfg.Storage.MSSQL.Pass
		dbName := strings.TrimSpace(cfg.Storage.MSSQL.DBName)
		port := cfg.Storage.MSSQL.Port
		if host == "" || user == "" || pass == "" || dbName == "" {
			return nil, fmt.Errorf("agents.storage.mssql.host/user/password/database are required when provider=mssql")
		}
		if port <= 0 {
			port = 1433
		}
		link := buildMSSQLLink(host, port, user, pass, dbName)
		gdb.SetConfigGroup(agentDBGroup, gdb.ConfigGroup{{
			Type:    "mssql",
			Link:    link,
			Charset: "utf8",
		}})
	default:
		return nil, fmt.Errorf("unsupported agents.storage.provider: %s", provider)
	}
	db, err := gdb.NewByGroup(agentDBGroup)
	if err != nil {
		return nil, err
	}
	repo := &sqliteRepository{db: db, dbType: provider}
	if err := repo.initTables(); err != nil {
		return nil, err
	}
	return repo, nil
}

func buildMSSQLLink(host string, port int, user, pass, dbName string) string {
	userEsc := url.QueryEscape(user)
	passEsc := url.QueryEscape(pass)
	dbEsc := url.QueryEscape(dbName)
	return fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s&encrypt=disable", userEsc, passEsc, host, port, dbEsc)
}

func (r *sqliteRepository) initTables() error {
	ctx := context.Background()
	sqls := sqliteInitSQLs()
	if strings.EqualFold(strings.TrimSpace(r.dbType), "mssql") {
		sqls = mssqlInitSQLs()
	}
	for _, sql := range sqls {
		if _, err := r.db.Exec(ctx, sql); err != nil {
			return err
		}
	}
	return nil
}

func sqliteInitSQLs() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS agent_sessions (
			assistant_key TEXT NOT NULL,
			session_id TEXT PRIMARY KEY,
			user_upn TEXT NOT NULL,
			active_path_json TEXT NOT NULL DEFAULT '[]',
			active_checkpoint_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			language TEXT NOT NULL DEFAULT 'zh',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_sessions_assistant_user_updated_at ON agent_sessions(assistant_key, user_upn, updated_at DESC);`,
		`CREATE TABLE IF NOT EXISTS agent_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			assistant_key TEXT NOT NULL,
			session_id TEXT NOT NULL,
			user_upn TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			payload_json TEXT NOT NULL DEFAULT '',
			active_path_json TEXT NOT NULL DEFAULT '[]',
			checkpoint_id TEXT NOT NULL DEFAULT '',
			language TEXT NOT NULL DEFAULT 'zh',
			created_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_messages_session_id_id ON agent_messages(session_id, id);`,
		`CREATE TABLE IF NOT EXISTS agent_runs (
			run_id TEXT PRIMARY KEY,
			assistant_key TEXT NOT NULL,
			session_id TEXT NOT NULL,
			user_upn TEXT NOT NULL,
			kind TEXT NOT NULL,
			status TEXT NOT NULL,
			parent_run_id TEXT NOT NULL DEFAULT '',
			request_json TEXT NOT NULL DEFAULT '{}',
			response_json TEXT NOT NULL DEFAULT '{}',
			checkpoint_id TEXT NOT NULL DEFAULT '',
			error_message TEXT NOT NULL DEFAULT '',
			started_at DATETIME NOT NULL,
			finished_at DATETIME NOT NULL,
			last_event_id INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_runs_assistant_user_started_at ON agent_runs(assistant_key, user_upn, started_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_runs_session_started_at ON agent_runs(session_id, started_at DESC);`,
		`CREATE TABLE IF NOT EXISTS agent_run_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL,
			assistant_key TEXT NOT NULL,
			session_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			path_json TEXT NOT NULL DEFAULT '[]',
			message TEXT NOT NULL DEFAULT '',
			payload_json TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_run_events_run_id_id ON agent_run_events(run_id, id);`,
		`CREATE TABLE IF NOT EXISTS agent_memories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			assistant_key TEXT NOT NULL,
			user_upn TEXT NOT NULL,
			category TEXT NOT NULL,
			canonical_key TEXT NOT NULL,
			content TEXT NOT NULL,
			value_json TEXT NOT NULL DEFAULT '',
			confidence REAL NOT NULL DEFAULT 0,
			source_session_id TEXT NOT NULL DEFAULT '',
			source_message_id INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE(assistant_key, user_upn, category, canonical_key)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_memories_assistant_user_updated_at ON agent_memories(assistant_key, user_upn, updated_at DESC);`,
	}
}

func mssqlInitSQLs() []string {
	return []string{
		`IF OBJECT_ID('agent_sessions', 'U') IS NULL
BEGIN
	CREATE TABLE agent_sessions (
		assistant_key NVARCHAR(128) NOT NULL,
		session_id NVARCHAR(128) NOT NULL PRIMARY KEY,
		user_upn NVARCHAR(256) NOT NULL,
		active_path_json NVARCHAR(MAX) NOT NULL DEFAULT '[]',
		active_checkpoint_id NVARCHAR(128) NOT NULL DEFAULT '',
		status NVARCHAR(32) NOT NULL,
		language NVARCHAR(16) NOT NULL DEFAULT 'zh',
		created_at DATETIME2 NOT NULL,
		updated_at DATETIME2 NOT NULL
	)
END`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_agent_sessions_assistant_user_updated_at' AND object_id = OBJECT_ID('agent_sessions'))
CREATE INDEX idx_agent_sessions_assistant_user_updated_at ON agent_sessions(assistant_key, user_upn, updated_at DESC)`,
		`IF OBJECT_ID('agent_messages', 'U') IS NULL
BEGIN
	CREATE TABLE agent_messages (
		id BIGINT IDENTITY(1,1) NOT NULL PRIMARY KEY,
		assistant_key NVARCHAR(128) NOT NULL,
		session_id NVARCHAR(128) NOT NULL,
		user_upn NVARCHAR(256) NOT NULL,
		role NVARCHAR(32) NOT NULL,
		content NVARCHAR(MAX) NOT NULL,
		payload_json NVARCHAR(MAX) NOT NULL DEFAULT '',
		active_path_json NVARCHAR(MAX) NOT NULL DEFAULT '[]',
		checkpoint_id NVARCHAR(128) NOT NULL DEFAULT '',
		language NVARCHAR(16) NOT NULL DEFAULT 'zh',
		created_at DATETIME2 NOT NULL
	)
END`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_agent_messages_session_id_id' AND object_id = OBJECT_ID('agent_messages'))
CREATE INDEX idx_agent_messages_session_id_id ON agent_messages(session_id, id)`,
		`IF OBJECT_ID('agent_runs', 'U') IS NULL
BEGIN
	CREATE TABLE agent_runs (
		run_id NVARCHAR(128) NOT NULL PRIMARY KEY,
		assistant_key NVARCHAR(128) NOT NULL,
		session_id NVARCHAR(128) NOT NULL,
		user_upn NVARCHAR(256) NOT NULL,
		kind NVARCHAR(32) NOT NULL,
		status NVARCHAR(32) NOT NULL,
		parent_run_id NVARCHAR(128) NOT NULL DEFAULT '',
		request_json NVARCHAR(MAX) NOT NULL DEFAULT '{}',
		response_json NVARCHAR(MAX) NOT NULL DEFAULT '{}',
		checkpoint_id NVARCHAR(128) NOT NULL DEFAULT '',
		error_message NVARCHAR(MAX) NOT NULL DEFAULT '',
		started_at DATETIME2 NOT NULL,
		finished_at DATETIME2 NOT NULL,
		last_event_id BIGINT NOT NULL DEFAULT 0
	)
END`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_agent_runs_assistant_user_started_at' AND object_id = OBJECT_ID('agent_runs'))
CREATE INDEX idx_agent_runs_assistant_user_started_at ON agent_runs(assistant_key, user_upn, started_at DESC)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_agent_runs_session_started_at' AND object_id = OBJECT_ID('agent_runs'))
CREATE INDEX idx_agent_runs_session_started_at ON agent_runs(session_id, started_at DESC)`,
		`IF OBJECT_ID('agent_run_events', 'U') IS NULL
BEGIN
	CREATE TABLE agent_run_events (
		id BIGINT IDENTITY(1,1) NOT NULL PRIMARY KEY,
		run_id NVARCHAR(128) NOT NULL,
		assistant_key NVARCHAR(128) NOT NULL,
		session_id NVARCHAR(128) NOT NULL,
		event_type NVARCHAR(64) NOT NULL,
		path_json NVARCHAR(MAX) NOT NULL DEFAULT '[]',
		message NVARCHAR(MAX) NOT NULL DEFAULT '',
		payload_json NVARCHAR(MAX) NOT NULL DEFAULT '{}',
		created_at DATETIME2 NOT NULL
	)
END`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_agent_run_events_run_id_id' AND object_id = OBJECT_ID('agent_run_events'))
CREATE INDEX idx_agent_run_events_run_id_id ON agent_run_events(run_id, id)`,
		`IF OBJECT_ID('agent_memories', 'U') IS NULL
BEGIN
	CREATE TABLE agent_memories (
		id BIGINT IDENTITY(1,1) NOT NULL PRIMARY KEY,
		assistant_key NVARCHAR(128) NOT NULL,
		user_upn NVARCHAR(256) NOT NULL,
		category NVARCHAR(64) NOT NULL,
		canonical_key NVARCHAR(256) NOT NULL,
		content NVARCHAR(MAX) NOT NULL,
		value_json NVARCHAR(MAX) NOT NULL DEFAULT '',
		confidence FLOAT NOT NULL DEFAULT 0,
		source_session_id NVARCHAR(128) NOT NULL DEFAULT '',
		source_message_id BIGINT NOT NULL DEFAULT 0,
		status NVARCHAR(32) NOT NULL DEFAULT 'active',
		created_at DATETIME2 NOT NULL,
		updated_at DATETIME2 NOT NULL
	)
END`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'uidx_agent_memories_main' AND object_id = OBJECT_ID('agent_memories'))
CREATE UNIQUE INDEX uidx_agent_memories_main ON agent_memories(assistant_key, user_upn, category, canonical_key)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_agent_memories_assistant_user_updated_at' AND object_id = OBJECT_ID('agent_memories'))
CREATE INDEX idx_agent_memories_assistant_user_updated_at ON agent_memories(assistant_key, user_upn, updated_at DESC)`,
	}
}

func (r *sqliteRepository) SaveSession(ctx context.Context, session SessionRecord) error {
	updateResult, err := r.db.Model(tableAgentSessions).
		Ctx(ctx).
		Where("session_id", session.SessionID).
		Data(g.Map{
			"assistant_key":        session.AssistantKey,
			"user_upn":             session.UserUPN,
			"active_path_json":     session.ActivePathJSON,
			"active_checkpoint_id": session.ActiveCheckpoint,
			"status":               session.Status,
			"language":             session.Language,
			"updated_at":           session.UpdatedAt,
		}).Update()
	if err != nil {
		return err
	}
	rows, err := updateResult.RowsAffected()
	if err != nil {
		return err
	}
	if rows > 0 {
		return nil
	}
	_, err = r.db.Model(tableAgentSessions).
		Ctx(ctx).
		Data(g.Map{
			"assistant_key":        session.AssistantKey,
			"session_id":           session.SessionID,
			"user_upn":             session.UserUPN,
			"active_path_json":     session.ActivePathJSON,
			"active_checkpoint_id": session.ActiveCheckpoint,
			"status":               session.Status,
			"language":             session.Language,
			"created_at":           session.CreatedAt,
			"updated_at":           session.UpdatedAt,
		}).Insert()
	return err
}

func (r *sqliteRepository) GetSession(ctx context.Context, sessionID string) (*SessionRecord, error) {
	item, err := r.db.Model(tableAgentSessions).Ctx(ctx).Where("session_id", sessionID).One()
	if err != nil {
		return nil, err
	}
	if item.IsEmpty() {
		return nil, nil
	}
	var record SessionRecord
	if err := item.Struct(&record); err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *sqliteRepository) ListSessions(ctx context.Context, assistantKey, userUPN string, limit int) ([]SessionRecord, error) {
	model := r.db.Model(tableAgentSessions).Ctx(ctx).
		Where("assistant_key", assistantKey).
		Where("user_upn", userUPN).
		WhereNot("status", statusDeleted).
		OrderDesc("updated_at")
	if limit > 0 {
		model = model.Limit(limit)
	}
	var result []SessionRecord
	if err := model.Scan(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *sqliteRepository) DeleteSession(ctx context.Context, assistantKey, sessionID, userUPN string, deletedAt time.Time) error {
	_, err := r.db.Model(tableAgentSessions).Ctx(ctx).
		Where("assistant_key", assistantKey).
		Where("session_id", sessionID).
		Where("user_upn", userUPN).
		Data(g.Map{
			"status":     statusDeleted,
			"updated_at": deletedAt,
		}).Update()
	return err
}

func (r *sqliteRepository) AppendMessage(ctx context.Context, message MessageRecord) (int64, error) {
	result, err := r.db.Model(tableAgentMessages).Ctx(ctx).Data(g.Map{
		"assistant_key":    message.AssistantKey,
		"session_id":       message.SessionID,
		"user_upn":         message.UserUPN,
		"role":             message.Role,
		"content":          message.Content,
		"payload_json":     message.PayloadJSON,
		"active_path_json": message.ActivePathJSON,
		"checkpoint_id":    message.CheckpointID,
		"language":         message.Language,
		"created_at":       message.CreatedAt,
	}).Insert()
	if err != nil {
		return 0, err
	}
	id, idErr := result.LastInsertId()
	if idErr != nil {
		// MSSQL 驱动通常不支持 LastInsertId；消息写入成功时返回 0 即可。
		if strings.EqualFold(strings.TrimSpace(r.dbType), "mssql") {
			return 0, nil
		}
		return 0, idErr
	}
	return id, nil
}

func (r *sqliteRepository) ListRecentMessages(ctx context.Context, sessionID string, limit int) ([]MessageRecord, error) {
	var result []MessageRecord
	err := r.db.Model(tableAgentMessages).Ctx(ctx).
		Where("session_id", sessionID).
		OrderDesc("id").
		Limit(limit).
		Scan(&result)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result, nil
}

func (r *sqliteRepository) ListMessages(ctx context.Context, sessionID string) ([]MessageRecord, error) {
	var result []MessageRecord
	err := r.db.Model(tableAgentMessages).Ctx(ctx).
		Where("session_id", sessionID).
		OrderAsc("id").
		Scan(&result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *sqliteRepository) CreateRun(ctx context.Context, run RunRecord) error {
	_, err := r.db.Model(tableAgentRuns).Ctx(ctx).Data(g.Map{
		"run_id":        run.RunID,
		"assistant_key": run.AssistantKey,
		"session_id":    run.SessionID,
		"user_upn":      run.UserUPN,
		"kind":          run.Kind,
		"status":        run.Status,
		"parent_run_id": run.ParentRunID,
		"request_json":  run.RequestJSON,
		"response_json": run.ResponseJSON,
		"checkpoint_id": run.CheckpointID,
		"error_message": run.ErrorMessage,
		"started_at":    formatDBTime(run.StartedAt),
		"finished_at":   formatDBTime(run.FinishedAt),
		"last_event_id": run.LastEventID,
	}).Insert()
	return err
}

func (r *sqliteRepository) UpdateRunStatus(ctx context.Context, runID, status string) error {
	_, err := r.db.Model(tableAgentRuns).Ctx(ctx).Where("run_id", runID).Data(g.Map{
		"status": status,
	}).Update()
	return err
}

func (r *sqliteRepository) TryStartRun(ctx context.Context, runID string) (bool, error) {
	result, err := r.db.Model(tableAgentRuns).Ctx(ctx).Where("run_id", runID).Where("status", runStatusQueued).Data(g.Map{
		"status": runStatusRunning,
	}).Update()
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (r *sqliteRepository) TryCancelQueuedRun(ctx context.Context, runID, responseJSON, errorMessage string, finishedAt time.Time) (bool, error) {
	result, err := r.db.Model(tableAgentRuns).Ctx(ctx).
		Where("run_id", runID).
		Where("status", runStatusQueued).
		Data(g.Map{
			"status":        runStatusCancelled,
			"response_json": responseJSON,
			"checkpoint_id": "",
			"error_message": strings.TrimSpace(errorMessage),
			"finished_at":   formatDBTime(finishedAt),
		}).Update()
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (r *sqliteRepository) FinishRun(ctx context.Context, runID, status, responseJSON, checkpointID, errorMessage string, finishedAt time.Time) error {
	_, err := r.db.Model(tableAgentRuns).Ctx(ctx).Where("run_id", runID).Data(g.Map{
		"status":        status,
		"response_json": responseJSON,
		"checkpoint_id": checkpointID,
		"error_message": errorMessage,
		"finished_at":   formatDBTime(finishedAt),
	}).Update()
	return err
}

func (r *sqliteRepository) GetRun(ctx context.Context, runID string) (*RunRecord, error) {
	item, err := r.db.Model(tableAgentRuns).Ctx(ctx).Where("run_id", runID).One()
	if err != nil {
		return nil, err
	}
	if item.IsEmpty() {
		return nil, nil
	}
	var record RunRecord
	if err := item.Struct(&record); err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *sqliteRepository) ListRunsBySession(ctx context.Context, sessionID string) ([]RunRecord, error) {
	var result []RunRecord
	err := r.db.Model(tableAgentRuns).Ctx(ctx).
		Where("session_id", sessionID).
		OrderAsc("started_at").
		Scan(&result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *sqliteRepository) AppendRunEvent(ctx context.Context, event RunEventRecord) (int64, error) {
	var id int64
	err := r.db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		insert, err := tx.Model(tableAgentRunEvents).Ctx(ctx).Data(g.Map{
			"run_id":        event.RunID,
			"assistant_key": event.AssistantKey,
			"session_id":    event.SessionID,
			"event_type":    event.EventType,
			"path_json":     event.PathJSON,
			"message":       event.Message,
			"payload_json":  event.PayloadJSON,
			"created_at":    event.CreatedAt,
		}).Insert()
		if err != nil {
			return err
		}
		id, err = insert.LastInsertId()
		if err != nil {
			id, err = queryMaxIDInTx(ctx, tx, tableAgentRunEvents, "run_id", event.RunID)
			if err != nil {
				return err
			}
		}
		_, err = tx.Model(tableAgentRuns).Ctx(ctx).Where("run_id", event.RunID).Data(g.Map{"last_event_id": id}).Update()
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *sqliteRepository) ListRunEventsAfter(ctx context.Context, runID string, afterID int64) ([]RunEventRecord, error) {
	model := r.db.Model(tableAgentRunEvents).Ctx(ctx).Where("run_id", runID).OrderAsc("id")
	if afterID > 0 {
		model = model.WhereGT("id", afterID)
	}
	var result []RunEventRecord
	if err := model.Scan(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *sqliteRepository) MarkStaleRunsFailed(ctx context.Context, errorMessage string, finishedAt time.Time) error {
	_, err := r.db.Model(tableAgentRuns).Ctx(ctx).
		WhereIn("status", []string{runStatusQueued, runStatusRunning}).
		Data(g.Map{
			"status":        runStatusFailed,
			"error_message": strings.TrimSpace(errorMessage),
			"finished_at":   formatDBTime(finishedAt),
		}).Update()
	return err
}

func formatDBTime(value time.Time) string {
	if value.IsZero() {
		return "0001-01-01 00:00:00"
	}
	return value.Format("2006-01-02 15:04:05")
}

func (r *sqliteRepository) UpsertMemories(ctx context.Context, assistantKey, userUPN, sessionID string, sourceMessageID int64, memories []MemoryItem) error {
	for _, memory := range memories {
		now := time.Now()
		updateResult, err := r.db.Model(tableAgentMemories).Ctx(ctx).
			Where("assistant_key", assistantKey).
			Where("user_upn", userUPN).
			Where("category", memory.Category).
			Where("canonical_key", memory.CanonicalKey).
			Data(g.Map{
				"content":           memory.Content,
				"value_json":        memory.ValueJSON,
				"confidence":        memory.Confidence,
				"source_session_id": sessionID,
				"source_message_id": sourceMessageID,
				"status":            statusActive,
				"updated_at":        now,
			}).Update()
		if err != nil {
			return err
		}
		rows, err := updateResult.RowsAffected()
		if err != nil {
			return err
		}
		if rows > 0 {
			continue
		}
		_, err = r.db.Model(tableAgentMemories).Ctx(ctx).Data(g.Map{
			"assistant_key":     assistantKey,
			"user_upn":          userUPN,
			"category":          memory.Category,
			"canonical_key":     memory.CanonicalKey,
			"content":           memory.Content,
			"value_json":        memory.ValueJSON,
			"confidence":        memory.Confidence,
			"source_session_id": sessionID,
			"source_message_id": sourceMessageID,
			"status":            statusActive,
			"created_at":        now,
			"updated_at":        now,
		}).Insert()
		if err != nil {
			_, updateErr := r.db.Model(tableAgentMemories).Ctx(ctx).
				Where("assistant_key", assistantKey).
				Where("user_upn", userUPN).
				Where("category", memory.Category).
				Where("canonical_key", memory.CanonicalKey).
				Data(g.Map{
					"content":           memory.Content,
					"value_json":        memory.ValueJSON,
					"confidence":        memory.Confidence,
					"source_session_id": sessionID,
					"source_message_id": sourceMessageID,
					"status":            statusActive,
					"updated_at":        now,
				}).Update()
			if updateErr != nil {
				return err
			}
		}
	}
	return nil
}

func (r *sqliteRepository) ListMemories(ctx context.Context, assistantKey, userUPN string, limit int) ([]MemoryRecord, error) {
	model := r.db.Model(tableAgentMemories).Ctx(ctx).
		Where("assistant_key", assistantKey).
		Where("user_upn", userUPN).
		Where("status", statusActive).
		OrderDesc("updated_at")
	if limit > 0 {
		model = model.Limit(limit)
	}
	var result []MemoryRecord
	if err := model.Scan(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *sqliteRepository) DeleteMemories(ctx context.Context, assistantKey, userUPN, category, canonicalKey string) (int64, error) {
	model := r.db.Model(tableAgentMemories).Ctx(ctx).
		Where("assistant_key", assistantKey).
		Where("user_upn", userUPN)
	if strings.TrimSpace(category) != "" {
		model = model.Where("category", strings.TrimSpace(category))
	}
	if strings.TrimSpace(canonicalKey) != "" {
		model = model.Where("canonical_key", strings.TrimSpace(canonicalKey))
	}
	result, err := model.Delete()
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return rows, nil
}

func queryMaxIDInTx(ctx context.Context, tx gdb.TX, table, keyField, keyValue string) (int64, error) {
	item, err := tx.Model(table).Ctx(ctx).Fields("MAX(id) AS id").Where(keyField, keyValue).One()
	if err != nil {
		return 0, err
	}
	if item.IsEmpty() {
		return 0, nil
	}
	return item["id"].Int64(), nil
}
