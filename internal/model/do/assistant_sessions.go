package do

import "github.com/gogf/gf/v2/frame/g"

// AssistantSessions 是 assistant_sessions 表的写入对象。
type AssistantSessions struct {
	g.Meta             `orm:"table:assistant_sessions, do:true"`
	SessionId          interface{} `orm:"session_id"`
	UserCode           interface{} `orm:"user_code"`
	ActiveAgent        interface{} `orm:"active_agent"`
	ActiveCheckpointId interface{} `orm:"active_checkpoint_id"`
	Status             interface{} `orm:"status"`
	Language           interface{} `orm:"language"`
	CreatedAt          interface{} `orm:"created_at"`
	UpdatedAt          interface{} `orm:"updated_at"`
}
