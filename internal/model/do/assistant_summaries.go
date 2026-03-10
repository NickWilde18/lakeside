package do

import "github.com/gogf/gf/v2/frame/g"

// AssistantSummaries 是 assistant_summaries 表的写入对象。
type AssistantSummaries struct {
	g.Meta        `orm:"table:assistant_summaries, do:true"`
	SessionId     interface{} `orm:"session_id"`
	SummaryText   interface{} `orm:"summary_text"`
	LastMessageId interface{} `orm:"last_message_id"`
	UpdatedAt     interface{} `orm:"updated_at"`
}
