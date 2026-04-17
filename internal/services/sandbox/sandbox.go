package sandbox

import "context"

type PythonSendbox interface {
	Execute(ctx context.Context, code string, data interface{}) (string, error)
	Close(ctx context.Context) error
}
