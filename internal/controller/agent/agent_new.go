package agent

import "lakeside/api/agent"

type ControllerV1 struct{}

func NewV1() agent.IAgentV1 {
	return &ControllerV1{}
}
