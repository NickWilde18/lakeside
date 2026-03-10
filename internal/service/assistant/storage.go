package assistant

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"lakeside/internal/dao"
	"lakeside/internal/model/do"
	"lakeside/internal/model/entity"

	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
)

const assistantDBGroup = "assistant"

type sqliteRepository struct {
	db gdb.DB
}

func newRepository(ctx context.Context) (Repository, error) {
	path := g.Cfg().MustGet(ctx, "assistant.storage.sqlitePath", "./runtime/assistant.db").String()
	if err := gfile.Mkdir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	gdb.SetConfigGroup(assistantDBGroup, gdb.ConfigGroup{{
		Type:    "sqlite",
		Link:    fmt.Sprintf("sqlite::@file(%s)", path),
		Charset: "utf8",
	}})
	db, err := gdb.NewByGroup(assistantDBGroup)
	if err != nil {
		return nil, err
	}
	repo := &sqliteRepository{db: db}
	if err := repo.initTables(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *sqliteRepository) initTables() error {
	ctx := context.Background()
	sqls := []string{
		`CREATE TABLE IF NOT EXISTS assistant_sessions (
			session_id TEXT PRIMARY KEY,
			user_code TEXT NOT NULL,
			active_agent TEXT NOT NULL,
			active_checkpoint_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			language TEXT NOT NULL DEFAULT 'zh',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS assistant_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			user_code TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			payload_json TEXT NOT NULL DEFAULT '',
			active_agent TEXT NOT NULL,
			checkpoint_id TEXT NOT NULL DEFAULT '',
			language TEXT NOT NULL DEFAULT 'zh',
			created_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_assistant_messages_session_id_id ON assistant_messages(session_id, id);`,
		`CREATE TABLE IF NOT EXISTS assistant_summaries (
			session_id TEXT PRIMARY KEY,
			summary_text TEXT NOT NULL,
			last_message_id INTEGER NOT NULL DEFAULT 0,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS assistant_memories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_code TEXT NOT NULL,
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
			UNIQUE(user_code, category, canonical_key)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_assistant_memories_user_code_updated_at ON assistant_memories(user_code, updated_at DESC);`,
	}
	for _, sql := range sqls {
		if _, err := r.db.Exec(ctx, sql); err != nil {
			return err
		}
	}
	return nil
}

func (r *sqliteRepository) SaveSession(ctx context.Context, session SessionRecord) error {
	_, err := dao.AssistantSessions.Ctx(ctx).
		OnConflict(dao.AssistantSessions.Columns().SessionId).
		Save(do.AssistantSessions{
			SessionId:          session.SessionID,
			UserCode:           session.UserCode,
			ActiveAgent:        session.ActiveAgent,
			ActiveCheckpointId: session.ActiveCheckpointID,
			Status:             session.Status,
			Language:           session.Language,
			CreatedAt:          session.CreatedAt,
			UpdatedAt:          session.UpdatedAt,
		})
	return err
}

func (r *sqliteRepository) GetSession(ctx context.Context, sessionID string) (*SessionRecord, error) {
	item, err := dao.AssistantSessions.Ctx(ctx).
		Where(dao.AssistantSessions.Columns().SessionId, sessionID).
		One()
	if err != nil {
		return nil, err
	}
	if item.IsEmpty() {
		return nil, nil
	}
	var entityItem entity.AssistantSessions
	if err = item.Struct(&entityItem); err != nil {
		return nil, err
	}
	record := sessionFromEntity(entityItem)
	return &record, nil
}

func (r *sqliteRepository) AppendMessage(ctx context.Context, message MessageRecord) (int64, error) {
	result, err := dao.AssistantMessages.Ctx(ctx).Data(do.AssistantMessages{
		SessionId:    message.SessionID,
		UserCode:     message.UserCode,
		Role:         message.Role,
		Content:      message.Content,
		PayloadJson:  message.PayloadJSON,
		ActiveAgent:  message.ActiveAgent,
		CheckpointId: message.CheckpointID,
		Language:     message.Language,
		CreatedAt:    message.CreatedAt,
	}).Insert()
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *sqliteRepository) ListMessagesAfter(ctx context.Context, sessionID string, lastMessageID int64, limit int) ([]MessageRecord, error) {
	var result []entity.AssistantMessages
	err := dao.AssistantMessages.Ctx(ctx).
		Where(dao.AssistantMessages.Columns().SessionId, sessionID).
		WhereGT(dao.AssistantMessages.Columns().Id, lastMessageID).
		OrderAsc(dao.AssistantMessages.Columns().Id).
		Limit(limit).
		Scan(&result)
	if err != nil {
		return nil, err
	}
	return messagesFromEntities(result), nil
}

func (r *sqliteRepository) ListRecentMessages(ctx context.Context, sessionID string, limit int) ([]MessageRecord, error) {
	var result []entity.AssistantMessages
	err := dao.AssistantMessages.Ctx(ctx).
		Where(dao.AssistantMessages.Columns().SessionId, sessionID).
		OrderDesc(dao.AssistantMessages.Columns().Id).
		Limit(limit).
		Scan(&result)
	if err != nil {
		return nil, err
	}
	items := messagesFromEntities(result)
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	return items, nil
}

func (r *sqliteRepository) UpsertSummary(ctx context.Context, summary SummaryRecord) error {
	_, err := dao.AssistantSummaries.Ctx(ctx).
		OnConflict(dao.AssistantSummaries.Columns().SessionId).
		Save(do.AssistantSummaries{
			SessionId:     summary.SessionID,
			SummaryText:   summary.SummaryText,
			LastMessageId: summary.LastMessageID,
			UpdatedAt:     summary.UpdatedAt,
		})
	return err
}

func (r *sqliteRepository) GetSummary(ctx context.Context, sessionID string) (*SummaryRecord, error) {
	item, err := dao.AssistantSummaries.Ctx(ctx).
		Where(dao.AssistantSummaries.Columns().SessionId, sessionID).
		One()
	if err != nil {
		return nil, err
	}
	if item.IsEmpty() {
		return nil, nil
	}
	var entityItem entity.AssistantSummaries
	if err = item.Struct(&entityItem); err != nil {
		return nil, err
	}
	record := summaryFromEntity(entityItem)
	return &record, nil
}

func (r *sqliteRepository) UpsertMemories(ctx context.Context, userCode string, sessionID string, sourceMessageID int64, memories []MemoryItem) error {
	for _, memory := range memories {
		now := time.Now()
		_, err := dao.AssistantMemories.Ctx(ctx).
			OnConflict("user_code,category,canonical_key").
			Save(do.AssistantMemories{
				UserCode:        userCode,
				Category:        memory.Category,
				CanonicalKey:    memory.CanonicalKey,
				Content:         memory.Content,
				ValueJson:       memory.ValueJSON,
				Confidence:      memory.Confidence,
				SourceSessionId: sessionID,
				SourceMessageId: sourceMessageID,
				Status:          statusActive,
				CreatedAt:       now,
				UpdatedAt:       now,
			})
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *sqliteRepository) ListMemories(ctx context.Context, userCode string, limit int) ([]MemoryRecord, error) {
	model := dao.AssistantMemories.Ctx(ctx).
		Where(dao.AssistantMemories.Columns().UserCode, userCode).
		Where(dao.AssistantMemories.Columns().Status, statusActive).
		OrderDesc(dao.AssistantMemories.Columns().UpdatedAt)
	if limit > 0 {
		model = model.Limit(limit)
	}
	var result []entity.AssistantMemories
	err := model.Scan(&result)
	if err != nil {
		return nil, err
	}
	return memoriesFromEntities(result), nil
}

func (r *sqliteRepository) DeleteMemories(ctx context.Context, userCode, category, canonicalKey string) (int64, error) {
	model := dao.AssistantMemories.Ctx(ctx).
		Where(dao.AssistantMemories.Columns().UserCode, userCode).
		Where(dao.AssistantMemories.Columns().Status, statusActive)
	if category = strings.TrimSpace(category); category != "" {
		model = model.Where(dao.AssistantMemories.Columns().Category, category)
	}
	if canonicalKey = strings.TrimSpace(canonicalKey); canonicalKey != "" {
		model = model.Where(dao.AssistantMemories.Columns().CanonicalKey, canonicalKey)
	}
	result, err := model.Delete()
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func sessionFromEntity(item entity.AssistantSessions) SessionRecord {
	return SessionRecord{
		SessionID:          item.SessionId,
		UserCode:           item.UserCode,
		ActiveAgent:        item.ActiveAgent,
		ActiveCheckpointID: item.ActiveCheckpointId,
		Status:             item.Status,
		Language:           item.Language,
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}
}

func messagesFromEntities(result []entity.AssistantMessages) []MessageRecord {
	items := make([]MessageRecord, 0, len(result))
	for _, item := range result {
		items = append(items, MessageRecord{
			ID:           item.Id,
			SessionID:    item.SessionId,
			UserCode:     item.UserCode,
			Role:         item.Role,
			Content:      item.Content,
			PayloadJSON:  item.PayloadJson,
			ActiveAgent:  item.ActiveAgent,
			CheckpointID: item.CheckpointId,
			Language:     item.Language,
			CreatedAt:    item.CreatedAt,
		})
	}
	return items
}

func summaryFromEntity(item entity.AssistantSummaries) SummaryRecord {
	return SummaryRecord{
		SessionID:     item.SessionId,
		SummaryText:   item.SummaryText,
		LastMessageID: item.LastMessageId,
		UpdatedAt:     item.UpdatedAt,
	}
}

func memoriesFromEntities(result []entity.AssistantMemories) []MemoryRecord {
	items := make([]MemoryRecord, 0, len(result))
	for _, item := range result {
		items = append(items, MemoryRecord{
			ID:              item.Id,
			UserCode:        item.UserCode,
			Category:        item.Category,
			CanonicalKey:    item.CanonicalKey,
			Content:         item.Content,
			ValueJSON:       item.ValueJson,
			Confidence:      item.Confidence,
			SourceSessionID: item.SourceSessionId,
			SourceMessageID: item.SourceMessageId,
			Status:          item.Status,
			CreatedAt:       item.CreatedAt,
			UpdatedAt:       item.UpdatedAt,
		})
	}
	return items
}
