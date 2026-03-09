package itsmagent

import "github.com/cloudwego/eino/schema"

func init() {
	schema.Register[*TicketInterruptInfo]()
	schema.Register[*TicketAgentState]()
	schema.Register[*ResumeCollectData]()
	schema.Register[*ResumeConfirmData]()
	schema.Register[*TicketExecutionResult]()
}
