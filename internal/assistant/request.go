package assistant

import "context"

type Request struct {
	Question       string
	PreviousAnswer string
	Deep           bool
}

type Service interface {
	Answer(ctx context.Context, request Request) (string, error)
}
