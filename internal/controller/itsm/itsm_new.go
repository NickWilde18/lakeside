package itsm

import "lakeside/api/itsm"

type ControllerV1 struct{}

func NewV1() itsm.IItsmV1 {
	return &ControllerV1{}
}
