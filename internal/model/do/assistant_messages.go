package do

import "github.com/gogf/gf/v2/frame/g"

// AssistantMessages 是 assistant_messages 表的写入对象。
type AssistantMessages struct {
	g.Meta       `orm:"table:assistant_messages, do:true"`
	Id           interface{} `orm:"id"`
	SessionId    interface{} `orm:"session_id"`
	UserCode     interface{} `orm:"user_code"`
	Role         interface{} `orm:"role"`
	Content      interface{} `orm:"content"`
	PayloadJson  interface{} `orm:"payload_json"`
	ActiveAgent  interface{} `orm:"active_agent"`
	CheckpointId interface{} `orm:"checkpoint_id"`
	Language     interface{} `orm:"language"`
	CreatedAt    interface{} `orm:"created_at"`
}
