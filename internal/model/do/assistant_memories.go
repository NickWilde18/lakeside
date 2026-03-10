package do

import "github.com/gogf/gf/v2/frame/g"

// AssistantMemories 是 assistant_memories 表的写入对象。
type AssistantMemories struct {
	g.Meta          `orm:"table:assistant_memories, do:true"`
	Id              interface{} `orm:"id"`
	UserCode        interface{} `orm:"user_code"`
	Category        interface{} `orm:"category"`
	CanonicalKey    interface{} `orm:"canonical_key"`
	Content         interface{} `orm:"content"`
	ValueJson       interface{} `orm:"value_json"`
	Confidence      interface{} `orm:"confidence"`
	SourceSessionId interface{} `orm:"source_session_id"`
	SourceMessageId interface{} `orm:"source_message_id"`
	Status          interface{} `orm:"status"`
	CreatedAt       interface{} `orm:"created_at"`
	UpdatedAt       interface{} `orm:"updated_at"`
}
