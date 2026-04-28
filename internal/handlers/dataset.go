package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"exeldoctor/internal/models"
	"exeldoctor/internal/services"
	"exeldoctor/internal/services/excel"
	"exeldoctor/internal/services/llm"
	"exeldoctor/internal/services/sandbox"

	"github.com/gofiber/fiber/v2"
	"go.etcd.io/bbolt"
	"gorm.io/gorm"
)

type DatasetHandler struct {
	DB       *gorm.DB
	BoltDB   *bbolt.DB
	Excel    *excel.Service
	LLM      llm.LLMService
	Sandbox  sandbox.PythonSendbox
	Pipeline *services.AIPipeline
}

func NewDatasetHandler(db *gorm.DB, bdb *bbolt.DB, xl *excel.Service, llm llm.LLMService, sb sandbox.PythonSendbox, pipeline *services.AIPipeline) *DatasetHandler {
	return &DatasetHandler{DB: db, BoltDB: bdb, Excel: xl, LLM: llm, Sandbox: sb, Pipeline: pipeline}
}

// ---------------------------------------------------------------------
// EXECUTION CONTRACT + SAFETY
// ---------------------------------------------------------------------

type PythonExecMeta struct {
	ScannedRows       int `json:"scanned_rows"`
	UsedRows          int `json:"used_rows"`
	TotalRowsExpected int `json:"total_rows_expected"`
}

type PythonExecPayload struct {
	Meta    PythonExecMeta           `json:"meta"`
	Summary map[string]interface{}   `json:"summary"`
	Charts  []map[string]interface{} `json:"charts"`
}

var (
	// Ловим присваивание любой из зарезервированных переменных
	reDatasetAssign = regexp.MustCompile(`(?m)^\s*(dataset|data|rows|records)\s*=\s*[\[{(]`)
	reForbiddenImp  = regexp.MustCompile(`(?mi)^\s*(import|from)\s+(pandas|numpy)\b`)

	// Для sanitizeCode — ловим однострочные присваивания
	reHardcodedAssign = regexp.MustCompile(`^(dataset|data|rows|records)\s*=`)
)

const (
	maxCodeLenBytes    = 60_000
	maxRetriesCode     = 3
	maxLastErrorLen    = 1200
	llmAttemptTimeout  = 45 * time.Second
	execAttemptTimeout = 20 * time.Second
	reportTimeout      = 30 * time.Second
)

func truncateError(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLastErrorLen {
		return s
	}
	return s[:maxLastErrorLen] + "...(truncated)"
}

func validateGeneratedCode(code string, maxLen int) string {
	code = strings.TrimSpace(code)

	if code == "" {
		return "Empty code from LLM"
	}

	if len(code) > maxLen {
		return fmt.Sprintf("Generated code too large: %d bytes (limit: %d)", len(code), maxLen)
	}

	if reForbiddenImp.MatchString(code) {
		return "Forbidden import detected (pandas/numpy)"
	}

	if reDatasetAssign.MatchString(code) {
		return "Code defines hardcoded container variable (dataset/data/rows/records)"
	}

	if strings.Contains(code, "open(") {
		return "Forbidden function detected: open()"
	}

	if !strings.Contains(code, "print(") {
		return "Code does not print result JSON"
	}

	return ""
}

func validateExecutionContract(output string, totalRows int) string {
	trimmed := strings.TrimSpace(output)

	if trimmed == "" {
		return "Empty output"
	}

	if !strings.HasPrefix(trimmed, "{") {
		return "Output is not JSON object"
	}

	var payload PythonExecPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return "Invalid JSON: " + err.Error()
	}

	if payload.Meta.TotalRowsExpected != totalRows {
		return fmt.Sprintf("meta.total_rows_expected mismatch: got %d, want %d",
			payload.Meta.TotalRowsExpected, totalRows)
	}

	if payload.Meta.ScannedRows != totalRows {
		return fmt.Sprintf("meta.scanned_rows mismatch: got %d, want %d",
			payload.Meta.ScannedRows, totalRows)
	}

	if payload.Meta.UsedRows < 0 || payload.Meta.UsedRows > totalRows {
		return fmt.Sprintf("meta.used_rows invalid: %d", payload.Meta.UsedRows)
	}

	if payload.Summary == nil {
		return "summary is required"
	}

	// charts может быть пустым — нормальный кейс.
	return ""
}

// buildSandboxGuard генерирует Python-код, который вставляется ПЕРЕД
// пользовательским скриптом. Проверяет, что dataset не был перезаписан.
func buildSandboxGuard(expectedRows int) string {
	return fmt.Sprintf(`
# === SANDBOX GUARD (injected) ===
_expected_len = %d
assert isinstance(dataset, list), "dataset must be a list"
assert len(dataset) == _expected_len, (
    f"dataset was overwritten: expected {_expected_len} rows, got {len(dataset)}"
)
# === END GUARD ===
`, expectedRows)
}

// ---------------------------------------------------------------------
// HISTORY
// ---------------------------------------------------------------------

func (h *DatasetHandler) GetChatHistory(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))

	var messages []models.ChatMessage

	if err := h.DB.Where("dataset_id = ?", id).Order("created_at asc").Find(&messages).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load history"})
	}

	return c.JSON(messages)
}

// ---------------------------------------------------------------------
// HTTP HANDLERS (CRUD)
// ---------------------------------------------------------------------

func (h *DatasetHandler) Upload(c *fiber.Ctx) error {
	start := time.Now()

	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File required"})
	}

	f, err := file.Open()
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}
	defer f.Close()

	headers, data, err := h.Excel.Parse(f, file.Filename)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Parse failed: " + err.Error()})
	}

	headersJSON, _ := json.Marshal(headers)
	dataJSON, _ := json.Marshal(data)

	dataset := models.Dataset{
		Name:      file.Filename,
		Source:    "upload",
		Headers:   headersJSON,
		Data:      dataJSON,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if h.DB != nil {
		if err := h.DB.Create(&dataset).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Database save error"})
		}
	} else {
		dataset.ID = 0
	}

	log.Printf("[Upload] Success. Rows: %d. Time: %v", len(data), time.Since(start))

	return c.JSON(fiber.Map{
		"id":         dataset.ID,
		"rows_count": len(data),
		"filename":   file.Filename,
	})
}

func (h *DatasetHandler) ListDatasets(c *fiber.Ctx) error {
	var datasets []models.Dataset

	if err := h.DB.Select("id", "name", "source", "created_at", "updated_at").Order("created_at desc").Find(&datasets).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch datasets"})
	}

	return c.JSON(datasets)
}

func (h *DatasetHandler) GetDataset(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))

	var dataset models.Dataset
	if err := h.DB.First(&dataset, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Dataset not found"})
	}

	return c.JSON(dataset)
}

func (h *DatasetHandler) RunPython(c *fiber.Ctx) error {
	var req struct {
		Code string `json:"code"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).SendString("Invalid JSON")
	}

	res, err := h.Sandbox.Execute(c.Context(), req.Code, []map[string]interface{}{})
	if err != nil {
		return c.JSON(fiber.Map{"status": "error", "error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "success", "result": res})
}

// ---------------------------------------------------------------------
// CHAT LOGIC
// ---------------------------------------------------------------------

func (h *DatasetHandler) Chat(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))

	var req struct {
		Message string `json:"message"`
		UseCode bool   `json:"use_code"`
		UseNews bool   `json:"use_news"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).SendString("Invalid body")
	}

	userMsg := models.ChatMessage{
		DatasetID: uint(id),
		Role:      "user",
		Content:   req.Message,
		CreatedAt: time.Now(),
	}
	h.DB.Create(&userMsg)

	req.Message += ". БЕЗ ДУБЛИКТОВ СТУДЕНТОВ"

	var dataset models.Dataset
	if err := h.DB.First(&dataset, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Dataset not found"})
	}

	var dataObj []map[string]interface{}
	if err := json.Unmarshal(dataset.Data, &dataObj); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Data corrupt"})
	}

	log.Printf("[Chat] ID: %d | Rows: %d | CodeMode: %v | NewsMode: %v",
		id, len(dataObj), req.UseCode, req.UseNews)

	// В code-режиме новости НЕ подмешиваем — они раздувают prompt и
	// ухудшают качество кодогенерации.
	//
	// Но агрегированные Excel-отчёты для дашборда не отправляем в Python-песочницу:
	// для них графики строятся напрямую из таблицы.
	if req.UseCode {
		if dashboardKind := detectDashboardDatasetKind(dataObj); dashboardKind != dashboardDatasetUnknown {
			return h.handleDashboardAggregateAnalysis(c, req.Message, dataObj, id, dashboardKind)
		}

		return h.handleCodeAnalysis(c, req.Message, dataObj, id)
	}

	enrichedQuery := req.Message

	if req.UseNews && h.BoltDB != nil {
		var newsData []byte

		h.BoltDB.View(func(tx *bbolt.Tx) error {
			b := tx.Bucket([]byte("DashboardCache"))
			if b != nil {
				newsData = b.Get([]byte("official_news"))
			}
			return nil
		})

		if len(newsData) > 0 {
			enrichedQuery = fmt.Sprintf(
				"%s\n\nВАЖНЫЙ КОНТЕКСТ ДЛЯ АНАЛИЗА (Последние новости/законы):\n%s",
				req.Message,
				string(newsData),
			)
			log.Println("[Chat] Новости добавлены в контекст промпта (text mode)")
		}
	}

	return h.handleTextAnalysisSmart(c, enrichedQuery, dataObj, id)
}

// ---------------------------------------------------------------------
// STRATEGY 0: DETERMINISTIC DASHBOARD AGGREGATE ANALYSIS
// ---------------------------------------------------------------------

const (
	dashboardDatasetUnknown    = "unknown"
	dashboardDatasetContingent = "contingent"
	dashboardDatasetMovement   = "movement"
	dashboardDatasetDeduction  = "deduction"
)

func detectDashboardDatasetKind(data []map[string]interface{}) string {
	headers := make(map[string]bool)

	for _, row := range data {
		for key := range row {
			headers[normalizeDashboardHeader(key)] = true
		}
	}

	if headers["контингент обучающихся"] {
		return dashboardDatasetContingent
	}

	if headers["восстановлены (чел.)"] ||
		headers["зачислены переводом из другого вуза/филиала (чел.)"] ||
		headers["переведены в другой вуз/филиал (чел.)"] {
		return dashboardDatasetMovement
	}

	if headers["отчислено всего (чел.)"] ||
		headers["отчислены за неуспеваемость (чел.)"] ||
		headers["отчислены за неоплату обучения (чел.)"] ||
		headers["отчислены по собственному желанию (чел.)"] ||
		headers["выпуск (получили образование)(чел.)"] {
		return dashboardDatasetDeduction
	}

	return dashboardDatasetUnknown
}

func (h *DatasetHandler) handleDashboardAggregateAnalysis(
	c *fiber.Ctx,
	query string,
	data []map[string]interface{},
	id int,
	kind string,
) error {
	chartData := buildDashboardChartData(data, kind)

	if len(chartData) == 0 {
		msg := "Не удалось построить график: в агрегированном отчёте не найдено строк с числовыми показателями."

		h.DB.Create(&models.ChatMessage{
			DatasetID: uint(id),
			Role:      "bot",
			Content:   msg,
			IsError:   true,
			CreatedAt: time.Now(),
		})

		return c.Status(400).JSON(fiber.Map{
			"error": msg,
		})
	}

	sort.Slice(chartData, func(i, j int) bool {
		return maxDashboardPointValue(chartData[i]) > maxDashboardPointValue(chartData[j])
	})

	if len(chartData) > 20 {
		chartData = chartData[:20]
	}

	yKeys := collectDashboardYKeys(chartData)

	title := "Показатели по образовательным программам"
	switch kind {
	case dashboardDatasetContingent:
		title = "Контингент обучающихся по образовательным программам"
	case dashboardDatasetMovement:
		title = "Переводы и восстановления по образовательным программам"
	case dashboardDatasetDeduction:
		title = "Отчисления и выпуск по образовательным программам"
	}

	codeOutput := map[string]interface{}{
		"meta": map[string]interface{}{
			"scanned_rows":          len(data),
			"used_rows":             len(chartData),
			"total_rows_expected":   len(data),
			"mode":                  "dashboard_aggregate",
			"dashboard_report_type": kind,
		},
		"summary": map[string]interface{}{
			"message": "График построен напрямую по агрегированному Excel-отчёту без запуска Python-песочницы.",
			"query":   query,
		},
		"charts": []map[string]interface{}{
			{
				"type":  "bar",
				"title": title,

				// Два варианта ключей оставлены для совместимости с разными ChartRenderer.
				"xKey":  "name",
				"x_key": "name",
				"yKeys": yKeys,
				"y_keys": yKeys,

				"data": chartData,
			},
		},
	}

	codeOutputJSON, _ := json.Marshal(codeOutput)

	reply := fmt.Sprintf(
		"График построен по агрегированному отчёту. Использовано строк: %d. Тип отчёта: %s.",
		len(chartData),
		kind,
	)

	h.DB.Create(&models.ChatMessage{
		DatasetID:  uint(id),
		Role:       "bot",
		Content:    reply,
		CodeOutput: string(codeOutputJSON),
		SourceCode: "",
		IsError:    false,
		CreatedAt:  time.Now(),
	})

	return c.JSON(fiber.Map{
		"reply":       reply,
		"code_output": string(codeOutputJSON),
		"source_code": "",
		"dataset_id":  id,
		"mode":        "dashboard_aggregate_chart",
	})
}

func buildDashboardChartData(data []map[string]interface{}, kind string) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)

	for _, row := range data {
		if isDashboardServiceRow(row) {
			continue
		}

		direction := getDashboardString(row, "Направление подготовки, специальность")
		program := getDashboardString(row, "Образовательная программа", "разовательная программа")

		label := program
		if label == "" {
			label = direction
		}
		if label == "" {
			continue
		}

		point := map[string]interface{}{
			"name": label,
		}

		hasValue := false

		switch kind {
		case dashboardDatasetContingent:
			contingent := getDashboardNumber(row, "контингент обучающихся")
			if contingent > 0 {
				point["контингент"] = contingent
				hasValue = true
			}

		case dashboardDatasetMovement:
			restored := getDashboardNumber(row, "восстановлены (чел.)")

			transferIn := getDashboardNumber(row,
				"зачислены переводом из другого вуза/филиала (чел.)",
				"зачислены переводом из другого вуза/ филиала (чел.)",
			)

			transferOut := getDashboardNumber(row,
				"переведены в другой вуз/филиал (чел.)",
				"переведены в другой вуз/ филиал (чел.)",
				"переведены в другогой вуз/филиал (чел.)",
				"переведены в другогой вуз/ филиал (чел.)",
			)

			if restored > 0 {
				point["восстановлены"] = restored
				hasValue = true
			}
			if transferIn > 0 {
				point["зачислены переводом"] = transferIn
				hasValue = true
			}
			if transferOut > 0 {
				point["переведены"] = transferOut
				hasValue = true
			}

		case dashboardDatasetDeduction:
			deductedTotal := getDashboardNumber(row,
				"отчислено всего (чел.)",
				"отчислено ВСЕГО (чел.)",
			)

			badProgress := getDashboardNumber(row,
				"отчислены за неуспеваемость (чел.)",
			)

			nonPayment := getDashboardNumber(row,
				"отчислены за не оплату обучения (чел.)",
				"отчислены за неоплату обучения (чел.)",
			)

			voluntary := getDashboardNumber(row,
				"отчислены по собственному желанию (чел.)",
			)

			graduated := getDashboardNumber(row,
				"выпуск (получили образование)(чел.)",
				"ВЫПУСК (получили образование)(чел.)",
			)

			if deductedTotal > 0 {
				point["отчислено всего"] = deductedTotal
				hasValue = true
			}
			if badProgress > 0 {
				point["неуспеваемость"] = badProgress
				hasValue = true
			}
			if nonPayment > 0 {
				point["неоплата"] = nonPayment
				hasValue = true
			}
			if voluntary > 0 {
				point["собственное желание"] = voluntary
				hasValue = true
			}
			if graduated > 0 {
				point["выпуск"] = graduated
				hasValue = true
			}
		}

		if hasValue {
			result = append(result, point)
		}
	}

	return result
}

func normalizeDashboardHeader(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "ё", "е")
	s = strings.Join(strings.Fields(s), " ")

	s = strings.ReplaceAll(s, " /", "/")
	s = strings.ReplaceAll(s, "/ ", "/")

	s = strings.ReplaceAll(s, "реализвции", "реализации")
	s = strings.ReplaceAll(s, "другогой", "другой")
	s = strings.ReplaceAll(s, "не оплату", "неоплату")

	return strings.TrimSpace(s)
}

func getDashboardString(row map[string]interface{}, aliases ...string) string {
	for key, value := range row {
		normalizedKey := normalizeDashboardHeader(key)

		for _, alias := range aliases {
			if normalizedKey == normalizeDashboardHeader(alias) {
				return strings.TrimSpace(fmt.Sprintf("%v", value))
			}
		}
	}

	return ""
}

func getDashboardNumber(row map[string]interface{}, aliases ...string) float64 {
	for key, value := range row {
		normalizedKey := normalizeDashboardHeader(key)

		for _, alias := range aliases {
			if normalizedKey == normalizeDashboardHeader(alias) {
				return dashboardValueToFloat(value)
			}
		}
	}

	return 0
}

func dashboardValueToFloat(value interface{}) float64 {
	if value == nil {
		return 0
	}

	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	case json.Number:
		f, err := v.Float64()
		if err == nil {
			return f
		}
		return 0
	default:
		s := strings.TrimSpace(fmt.Sprintf("%v", value))
		s = strings.ReplaceAll(s, "\u00a0", "")
		s = strings.ReplaceAll(s, " ", "")
		s = strings.ReplaceAll(s, ",", ".")

		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0
		}

		return f
	}
}

func isDashboardServiceRow(row map[string]interface{}) bool {
	direction := getDashboardString(row, "Направление подготовки, специальность")
	program := getDashboardString(row, "Образовательная программа", "разовательная программа")

	if direction == "" && program == "" {
		return true
	}

	lowerDirection := strings.ToLower(direction)
	if strings.Contains(lowerDirection, "итого") || strings.Contains(lowerDirection, "всего") {
		return true
	}

	return false
}

func maxDashboardPointValue(point map[string]interface{}) float64 {
	var maxValue float64

	for key, value := range point {
		if key == "name" {
			continue
		}

		switch v := value.(type) {
		case float64:
			if v > maxValue {
				maxValue = v
			}
		case int:
			if float64(v) > maxValue {
				maxValue = float64(v)
			}
		case int64:
			if float64(v) > maxValue {
				maxValue = float64(v)
			}
		case int32:
			if float64(v) > maxValue {
				maxValue = float64(v)
			}
		}
	}

	return maxValue
}

func collectDashboardYKeys(data []map[string]interface{}) []string {
	set := make(map[string]bool)

	for _, row := range data {
		for key := range row {
			if key != "name" {
				set[key] = true
			}
		}
	}

	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

// ---------------------------------------------------------------------
// STRATEGY 1: PYTHON CODE INTERPRETER
// ---------------------------------------------------------------------

func (h *DatasetHandler) handleCodeAnalysis(c *fiber.Ctx, query string, data []map[string]interface{}, id int) error {
	previewLen := 2
	if len(data) < previewLen {
		previewLen = len(data)
	}

	dataStructurePreview := data[:previewLen]

	var lastError string
	var executionResultJSON string
	var workingCode string

	for attempt := 1; attempt <= maxRetriesCode; attempt++ {
		log.Printf("[Code-Step] Attempt %d/%d generating code...", attempt, maxRetriesCode)

		prompt := buildAnalyticalPythonPrompt(query, dataStructurePreview, len(data), lastError)

		genCtx, cancelGen := context.WithTimeout(c.Context(), llmAttemptTimeout)
		llmResp, err := h.LLM.AnalyzeData(genCtx, prompt, nil)
		cancelGen()

		if err != nil {
			lastError = truncateError("LLM error: " + err.Error())
			log.Printf("[Code-Step] %s", lastError)
			continue
		}

		code := extractCode(llmResp)
		if code == "" {
			code = llmResp
		}

		code = sanitizeCode(code)

		if reason := validateGeneratedCode(code, maxCodeLenBytes); reason != "" {
			lastError = truncateError(reason)
			log.Printf("[Code-Step] Validation failed: %s", lastError)
			continue
		}

		// Инжектируем guard перед пользовательским кодом.
		// Guard проверяет, что dataset не был переопределён внутри скрипта.
		guardedCode := buildSandboxGuard(len(data)) + "\n" + code

		execStart := time.Now()

		execCtx, cancelExec := context.WithTimeout(c.Context(), execAttemptTimeout)
		execResult, err := h.Sandbox.Execute(execCtx, guardedCode, data)
		cancelExec()

		if reason := detectExecutionError(err, execResult); reason != "" {
			lastError = truncateError(reason)
			log.Printf("[Code-Step] Exec failed: %s", lastError)
			continue
		}

		if reason := validateExecutionContract(execResult, len(data)); reason != "" {
			lastError = truncateError("Logic/contract error: " + reason)
			log.Printf("[Code-Step] %s", lastError)
			continue
		}

		log.Printf("[Code-Step] Success in %v", time.Since(execStart))

		executionResultJSON = execResult
		workingCode = code // Сохраняем без guard — для отображения пользователю
		break
	}

	if executionResultJSON == "" {
		msg := fmt.Sprintf("Failed after %d attempts. Last error: %s", maxRetriesCode, lastError)

		h.DB.Create(&models.ChatMessage{
			DatasetID:  uint(id),
			Role:       "bot",
			Content:    msg,
			SourceCode: workingCode,
			IsError:    true,
			CreatedAt:  time.Now(),
		})

		return c.Status(500).JSON(fiber.Map{
			"error":       "Failed to analyze data after retries",
			"last_error":  lastError,
			"failed_code": workingCode,
		})
	}

	log.Println("[Report-Step] Generating text report...")

	repCtx, cancelRep := context.WithTimeout(c.Context(), reportTimeout)
	finalReport, repErr := h.generateAnalyticalReport(repCtx, query, executionResultJSON, len(data))
	cancelRep()

	if repErr != nil || strings.TrimSpace(finalReport) == "" {
		finalReport = "Анализ данных выполнен. Отчёт не удалось сгенерировать, но JSON-результат доступен в code_output."
	}

	h.DB.Create(&models.ChatMessage{
		DatasetID:  uint(id),
		Role:       "bot",
		Content:    finalReport,
		SourceCode: workingCode,
		CodeOutput: executionResultJSON,
		CreatedAt:  time.Now(),
	})

	return c.JSON(fiber.Map{
		"reply":       finalReport,
		"code_output": executionResultJSON,
		"source_code": workingCode,
		"dataset_id":  id,
		"mode":        "code_analytics",
	})
}

// ---------------------------------------------------------------------
// STRATEGY 2: MAP-REDUCE TEXT
// ---------------------------------------------------------------------

func (h *DatasetHandler) handleTextAnalysisSmart(c *fiber.Ctx, query string, data []map[string]interface{}, id int) error {
	totalRows := len(data)

	const chunkSize = 50

	if totalRows <= chunkSize {
		answer, err := h.LLM.AnalyzeData(c.Context(), "Проанализируй: "+query, data)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		h.DB.Create(&models.ChatMessage{
			DatasetID: uint(id),
			Role:      "bot",
			Content:   answer,
			CreatedAt: time.Now(),
		})

		return c.JSON(fiber.Map{
			"reply":      answer,
			"dataset_id": id,
			"mode":       "text_simple",
		})
	}

	log.Printf("[Text-Step] Map-Reduce on %d rows...", totalRows)

	maxChunks := 5
	numChunks := int(math.Ceil(float64(totalRows) / float64(chunkSize)))
	if numChunks > maxChunks {
		numChunks = maxChunks
	}

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

			chunkData := data[start:end]

			prompt := fmt.Sprintf(
				"Проанализируй этот ФРАГМЕНТ данных (строки %d-%d из %d). Вопрос: %s. Кратко, только факты.",
				start,
				end,
				totalRows,
				query,
			)

			summary, err := h.LLM.AnalyzeData(c.Context(), prompt, chunkData)
			if err == nil {
				mutex.Lock()
				chunksSummaries = append(chunksSummaries,
					fmt.Sprintf("--- Fragment %d ---\n%s", chunkIdx+1, summary))
				mutex.Unlock()
			}
		}(i)
	}

	wg.Wait()

	log.Println("[Text-Step] Reducing...")

	combinedSummaries := strings.Join(chunksSummaries, "\n\n")

	finalPrompt := fmt.Sprintf(`
Я разбил большой файл на части. Вот выводы по частям:
%s

ВОПРОС: "%s"

Объедини выводы в один связный ответ.
`, combinedSummaries, query)

	finalAnswer, err := h.LLM.AnalyzeData(c.Context(), finalPrompt, nil)
	if err != nil {
		h.DB.Create(&models.ChatMessage{
			DatasetID: uint(id),
			Role:      "bot",
			Content:   err.Error(),
			IsError:   true,
			CreatedAt: time.Now(),
		})

		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	h.DB.Create(&models.ChatMessage{
		DatasetID: uint(id),
		Role:      "bot",
		Content:   finalAnswer,
		CreatedAt: time.Now(),
	})

	return c.JSON(fiber.Map{
		"reply":      finalAnswer,
		"dataset_id": id,
		"mode":       "text_map_reduce",
	})
}

// ---------------------------------------------------------------------
// UTILS & PROMPTS
// ---------------------------------------------------------------------

// sanitizeCode — первый проход: комментирует очевидные нарушения.
// Не является единственной защитой — validateGeneratedCode проверяет строже.
func sanitizeCode(code string) string {
	// Убираем запрещённые импорты
	code = strings.ReplaceAll(code, "import pandas", "# import pandas")
	code = strings.ReplaceAll(code, "import numpy", "# import numpy")
	code = strings.ReplaceAll(code, "from pandas", "# from pandas")

	lines := strings.Split(code, "\n")

	var cleanLines []string

	inMultilineAssign := false
	bracketDepth := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Обработка многострочного присваивания
		if inMultilineAssign {
			cleanLines = append(cleanLines, "# "+line+" # REMOVED")

			bracketDepth += strings.Count(trimmed, "[") + strings.Count(trimmed, "{") + strings.Count(trimmed, "(")
			bracketDepth -= strings.Count(trimmed, "]") + strings.Count(trimmed, "}") + strings.Count(trimmed, ")")

			if bracketDepth <= 0 {
				inMultilineAssign = false
				bracketDepth = 0
			}

			continue
		}

		// Однострочное/начало многострочного: dataset =, data =, rows =, records =
		if reHardcodedAssign.MatchString(trimmed) &&
			(strings.Contains(trimmed, "[") || strings.Contains(trimmed, "{") || strings.Contains(trimmed, "(")) {

			cleanLines = append(cleanLines, "# "+line+" # REMOVED HARDCODED DATA")

			// Проверяем, закрылось ли на этой же строке
			bracketDepth = strings.Count(trimmed, "[") + strings.Count(trimmed, "{") + strings.Count(trimmed, "(")
			bracketDepth -= strings.Count(trimmed, "]") + strings.Count(trimmed, "}") + strings.Count(trimmed, ")")

			if bracketDepth > 0 {
				inMultilineAssign = true
			} else {
				bracketDepth = 0
			}

			continue
		}

		cleanLines = append(cleanLines, line)
	}

	return strings.Join(cleanLines, "\n")
}

// buildSchemaHint показывает тип + укороченный пример значения для каждого ключа.
// LLM получает формат данных (нужен для парсинга дат/чисел), но не может
// скопировать полную строку.
func buildSchemaHint(sample []map[string]interface{}) string {
	if len(sample) == 0 {
		return "{}"
	}

	row := sample[0]

	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	lines := make([]string, 0, len(keys))

	for _, k := range keys {
		v := row[k]

		typeHint := "unknown"
		exampleHint := ""

		switch val := v.(type) {
		case nil:
			typeHint = "nullable"
			exampleHint = "null"

		case bool:
			typeHint = "bool"
			exampleHint = fmt.Sprintf("%v", val)

		case float64:
			typeHint = "number"
			exampleHint = fmt.Sprintf("%.2f", val)
			if len(exampleHint) > 12 {
				exampleHint = exampleHint[:12] + "…"
			}

		case string:
			typeHint = "string"
			exampleHint = val
			if len(exampleHint) > 24 {
				exampleHint = exampleHint[:24] + "…"
			}

		default:
			typeHint = fmt.Sprintf("%T", v)
			exampleHint = "…"
		}

		lines = append(lines, fmt.Sprintf("  %q: <%s> e.g. %q", k, typeHint, exampleHint))
	}

	return "{\n" + strings.Join(lines, ",\n") + "\n}"
}

func buildAnalyticalPythonPrompt(userRequest string, dataSample []map[string]interface{}, totalRows int, lastError string) string {
	var keys []string

	if len(dataSample) > 0 {
		for k := range dataSample[0] {
			keys = append(keys, k)
		}
	}

	sort.Strings(keys)

	keysStr := "['" + strings.Join(keys, "', '") + "']"
	schemaHint := buildSchemaHint(dataSample)

	base := fmt.Sprintf(`ROLE: Senior Python Data Analyst.
TASK: Generate ONLY Python code.

STRICT RULES:
1) Variable 'dataset' is preloaded as List[Dict] with %d rows. NEVER redefine dataset/data/rows/records.
2) FORBIDDEN: pandas, numpy, open(), files, network.
3) Use only: json, math, statistics, datetime, collections.
4) All values may be strings — always use try/except for int()/float() conversions.
5) Output MUST be valid JSON via print(json.dumps(..., ensure_ascii=False)).
6) Keep code under 300 lines. No long comments.
7) DO NOT hardcode example values. Iterate over ALL rows in dataset.

DATA SCHEMA (example format only — DO NOT copy values):
- total rows: %d
- keys: %s
- field types: %s

USER REQUEST:
"%s"

OUTPUT CONTRACT (REQUIRED — validation will reject non-conforming output):
{
  "meta": {
    "scanned_rows": <must equal %d>,
    "used_rows": <int: rows that contributed to metrics>,
    "total_rows_expected": <must equal len(dataset)>
  },
  "summary": { <your analysis results> },
  "charts": [ <optional chart data> ]
}

TEMPLATE:
`+"```python"+`
import json
import math
import statistics
import datetime
from collections import Counter

def analyze_data():
    results = {
        "meta": {
            "scanned_rows": 0,
            "used_rows": 0,
            "total_rows_expected": len(dataset)
        },
        "summary": {},
        "charts": []
    }

    for row in dataset:
        results["meta"]["scanned_rows"] += 1
        # parse fields safely:
        # try:
        #     val = float(row["field"])
        # except (ValueError, TypeError, KeyError):
        #     continue
        # results["meta"]["used_rows"] += 1

    return results

print(json.dumps(analyze_data(), ensure_ascii=False))
`+"```",
		totalRows,
		totalRows,
		keysStr,
		schemaHint,
		userRequest,
		totalRows,
	)

	if lastError != "" {
		base += fmt.Sprintf(`

PREVIOUS ATTEMPT FAILED WITH ERROR:
%s

Fix the issue. Return ONLY valid Python code.`, lastError)
	}

	return base
}

func (h *DatasetHandler) generateAnalyticalReport(ctx context.Context, userQuery, pythonOutputJSON string, totalRows int) (string, error) {
	prompt := fmt.Sprintf(`Ты — бизнес-аналитик. Данные (%d строк) обработаны скриптом.
Результат (JSON):
%s

Вопрос пользователя: "%s"

Напиши отчёт в Markdown:
1. Опирайся только на цифры из JSON.
2. Не упоминай код/скрипт. Пиши «Анализ данных показал…».
3. Если в charts есть данные, опиши графики.
`, totalRows, pythonOutputJSON, userQuery)

	return h.LLM.AnalyzeData(ctx, prompt, nil)
}

func detectExecutionError(sysErr error, output string) string {
	if sysErr != nil {
		return fmt.Sprintf("System Error: %v", sysErr)
	}

	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "Empty output. Did you forget print(json.dumps(...))?"
	}

	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "traceback") || strings.Contains(lower, "error:") {
		if len(trimmed) > 400 {
			return "Script Error: " + trimmed[:400] + "..."
		}

		return "Script Error: " + trimmed
	}

	if !strings.HasPrefix(trimmed, "{") {
		return "Output is not valid JSON. Must start with '{'"
	}

	return ""
}

func extractCode(text string) string {
	re := regexp.MustCompile("(?s)```(?:python)?(.*?)```")
	matches := re.FindStringSubmatch(text)

	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	return strings.TrimSpace(text)
}
