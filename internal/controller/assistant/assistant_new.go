package assistant

import "lakeside/api/assistant"

type ControllerV1 struct{}

func NewV1() assistant.IAssistantV1 {
	return &ControllerV1{}
}
