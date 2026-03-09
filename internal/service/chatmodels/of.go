package chatmodels

func of[T any](v T) *T {
	return &v
}
