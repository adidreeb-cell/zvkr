package llm

import "context"

type LLMService interface {
	AnalyzeData(ctx context.Context, question string, dataPreview interface{}) (string, error)
}
