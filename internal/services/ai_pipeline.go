package services

import (
	"context"
	"encoding/json"
	"exeldoctor/internal/services/llm"
	"exeldoctor/internal/services/sandbox"
	"fmt"
	"log"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"
)

type AIPipeline struct {
	LLM     llm.LLMService
	Sandbox sandbox.PythonSendbox
}

func NewAIPipeline(llm llm.LLMService, sb sandbox.PythonSendbox) *AIPipeline {
	return &AIPipeline{LLM: llm, Sandbox: sb}
}

// AnalyzeWithCode - Стратегия 1: Python Code Interpreter
func (p *AIPipeline) AnalyzeWithCode(ctx context.Context, query string, data []map[string]interface{}) (map[string]interface{}, error) {
	previewLen := 2
	if len(data) < previewLen {
		previewLen = len(data)
	}
	dataStructurePreview := data[:previewLen]

	var lastError, executionResultJSON, workingCode string
	maxRetries := 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Printf("[AI-Pipeline] Code Attempt %d/%d...", attempt, maxRetries)

		prompt := p.buildPrompt(query, dataStructurePreview, len(data), lastError)

		llmResp, err := p.LLM.AnalyzeData(ctx, prompt, nil)
		if err != nil {
			continue
		}

		code := p.sanitizeCode(p.extractCode(llmResp))
		execStart := time.Now()
		execResult, err := p.Sandbox.Execute(ctx, code, data)

		failReason := p.detectError(err, execResult)
		if failReason != "" {
			lastError = failReason
			continue
		}

		log.Printf("[AI-Pipeline] Code executed successfully in %v", time.Since(execStart))
		executionResultJSON = execResult
		workingCode = code
		break
	}

	if executionResultJSON == "" {
		return nil, fmt.Errorf("failed after 3 attempts. Last error: %s", lastError)
	}

	finalReport, _ := p.generateReport(ctx, query, executionResultJSON, len(data))

	return map[string]interface{}{
		"reply":       finalReport,
		"code_output": executionResultJSON,
		"source_code": workingCode,
		"mode":        "code_analytics",
	}, nil
}

// AnalyzeWithMapReduce - Стратегия 2: Текстовый анализ по кускам
func (p *AIPipeline) AnalyzeWithMapReduce(ctx context.Context, query string, data []map[string]interface{}) (map[string]interface{}, error) {
	totalRows := len(data)
	const chunkSize = 50

	if totalRows <= chunkSize {
		answer, err := p.LLM.AnalyzeData(ctx, "Проанализируй: "+query, data)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"reply": answer, "mode": "text_simple"}, nil
	}

	log.Printf("[AI-Pipeline] Map-Reduce on %d rows...", totalRows)
	numChunks := int(math.Ceil(float64(totalRows) / float64(chunkSize)))
	if numChunks > 5 {
		numChunks = 5
	} // Ограничение

	var chunksSummaries []string
	var wg sync.WaitGroup
	var mutex sync.Mutex

	for i := 0; i < numChunks; i++ {
		wg.Add(1)
		go func(chunkIdx int) {
			defer wg.Done()
			start := chunkIdx * chunkSize
			end := start + chunkSize
			if end > totalRows {
				end = totalRows
			}

			prompt := fmt.Sprintf("Анализ строк %d-%d. Вопрос: %s. Только факты.", start, end, query)
			summary, err := p.LLM.AnalyzeData(ctx, prompt, data[start:end])
			if err == nil {
				mutex.Lock()
				chunksSummaries = append(chunksSummaries, summary)
				mutex.Unlock()
			}
		}(i)
	}
	wg.Wait()

	finalPrompt := fmt.Sprintf("Объедини выводы частей:\n%s\nВОПРОС: %s", strings.Join(chunksSummaries, "\n"), query)
	finalAnswer, err := p.LLM.AnalyzeData(ctx, finalPrompt, nil)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{"reply": finalAnswer, "mode": "text_map_reduce"}, nil
}

// --- Утилиты (ваши методы из dataset.go, перенесенные сюда) ---
func (p *AIPipeline) sanitizeCode(code string) string {
	code = strings.ReplaceAll(code, "import pandas", "# import pandas")
	lines := strings.Split(code, "\n")
	var clean []string
	for _, l := range lines {
		if (strings.HasPrefix(strings.TrimSpace(l), "dataset =")) && strings.Contains(l, "[") {
			clean = append(clean, "# "+l+"  <-- REMOVED HARDCODE")
		} else {
			clean = append(clean, l)
		}
	}
	return strings.Join(clean, "\n")
}

func (p *AIPipeline) extractCode(text string) string {
	re := regexp.MustCompile("(?s)```(?:python)?(.*?)```")
	if m := re.FindStringSubmatch(text); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return strings.TrimSpace(text)
}

func (p *AIPipeline) detectError(sysErr error, output string) string {
	if sysErr != nil {
		return fmt.Sprintf("System: %v", sysErr)
	}
	if strings.TrimSpace(output) == "" {
		return "Empty output"
	}
	if strings.Contains(strings.ToLower(output), "traceback") {
		return "Script Error: " + output[:min(len(output), 400)]
	}
	if !strings.HasPrefix(strings.TrimSpace(output), "{") {
		return "Not valid JSON"
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (p *AIPipeline) buildPrompt(query string, sample []map[string]interface{}, total int, err string) string {
	keys := "[]"
	if len(sample) > 0 {
		var k []string
		for key := range sample[0] {
			k = append(k, key)
		}
		keys = "['" + strings.Join(k, "', '") + "']"
	}
	sJson, _ := json.Marshal(sample)

	base := fmt.Sprintf(`ROLE: Data Scientist. TASK: Python code. CONTEXT: 'dataset' is loaded. Total: %d. No pandas. Keys: %s. Ex: %s. REQ: "%s"`, total, keys, string(sJson), query)
	if err != "" {
		base += "\nFIX PREVIOUS ERROR: " + err
	}
	return base + "\nPrint JSON result."
}

func (p *AIPipeline) generateReport(ctx context.Context, q, jsonRes string, total int) (string, error) {
	return p.LLM.AnalyzeData(ctx, fmt.Sprintf("Данные (%d строк). Результат: %s. Вопрос: %s. Напиши Markdown отчет по цифрам.", total, jsonRes, q), nil)
}
