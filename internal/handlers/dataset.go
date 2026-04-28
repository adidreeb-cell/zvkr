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
	reDatasetAssign  = regexp.MustCompile(`(?m)^\s*(dataset|data|rows|records)\s*=\s*[\[{(]`)
	reForbiddenImp   = regexp.MustCompile(`(?mi)^\s*(import|from)\s+(pandas|numpy)\b`)
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
		return fmt.Sprintf(
			"meta.total_rows_expected mismatch: got %d, want %d",
			payload.Meta.TotalRowsExpected,
			totalRows,
		)
	}

	if payload.Meta.ScannedRows != totalRows {
		return fmt.Sprintf(
			"meta.scanned_rows mismatch: got %d, want %d",
			payload.Meta.ScannedRows,
			totalRows,
		)
	}

	if payload.Meta.UsedRows < 0 || payload.Meta.UsedRows > totalRows {
		return fmt.Sprintf("meta.used_rows invalid: %d", payload.Meta.UsedRows)
	}

	if payload.Summary == nil {
		return "summary is required"
	}

	return ""
}

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
// HTTP HANDLERS
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

	originalMessage := strings.TrimSpace(req.Message)
	if originalMessage == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Message is required"})
	}

	userMsg := models.ChatMessage{
		DatasetID: uint(id),
		Role:      "user",
		Content:   originalMessage,
		CreatedAt: time.Now(),
	}
	h.DB.Create(&userMsg)

	req.Message = originalMessage + ". БЕЗ ДУБЛИКТОВ СТУДЕНТОВ"

	var dataset models.Dataset
	if err := h.DB.First(&dataset, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Dataset not found"})
	}

	var dataObj []map[string]interface{}
	if err := json.Unmarshal(dataset.Data, &dataObj); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Data corrupt"})
	}

	log.Printf("[Chat] ID: %d | Rows: %d | CodeMode: %v | NewsMode: %v",
		id,
		len(dataObj),
		req.UseCode,
		req.UseNews,
	)

	if req.UseCode {
		if dashboardKind := detectDashboardDatasetKind(dataObj); dashboardKind != dashboardDatasetUnknown {
			if isChartRequest(originalMessage) {
				return h.handleDashboardAggregateChart(c, originalMessage, dataObj, id, dashboardKind)
			}

			return h.handleDashboardAggregateText(c, originalMessage, dataObj, id, dashboardKind)
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
// STRATEGY 0: FLEXIBLE DASHBOARD AGGREGATE ANALYSIS
// ---------------------------------------------------------------------

const (
	dashboardDatasetUnknown    = "unknown"
	dashboardDatasetContingent = "contingent"
	dashboardDatasetMovement   = "movement"
	dashboardDatasetDeduction  = "deduction"
)

type DashboardChartPlan struct {
	Type     string   `json:"type"`
	Title    string   `json:"title"`
	XColumn  string   `json:"x_column"`
	YColumns []string `json:"y_columns"`
	SortBy   string   `json:"sort_by"`
	SortDesc bool     `json:"sort_desc"`
	Limit    int      `json:"limit"`
}

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

func isChartRequest(message string) bool {
	msg := strings.ToLower(strings.TrimSpace(message))

	keywords := []string{
		"график",
		"диаграмм",
		"визуализ",
		"chart",
		"plot",
		"bar",
		"line",
		"pie",
		"столбц",
		"кругов",
		"линейн",
		"построй",
		"нарисуй",
		"покажи",
		"сравни",
		"распределение",
		"динамик",
		"топ",
		"top",
	}

	for _, keyword := range keywords {
		if strings.Contains(msg, keyword) {
			return true
		}
	}

	return false
}

func (h *DatasetHandler) handleDashboardAggregateText(
	c *fiber.Ctx,
	query string,
	data []map[string]interface{},
	id int,
	kind string,
) error {
	summaryData := buildDashboardAggregateSummary(data, kind)

	prompt := fmt.Sprintf(`Ты аналитик данных образовательной организации.

Пользователь спрашивает:
%s

Тип агрегированного отчёта:
%s

Данные уже агрегированы, это НЕ список студентов.
Используй только эти рассчитанные значения:
%s

Ответь по запросу пользователя на русском языке.
Если пользователь просит число — дай число.
Если просит вывод — дай краткий аналитический вывод.
Если данных недостаточно — прямо скажи, каких колонок не хватает.
Не придумывай ФИО, ID студентов, средний балл или статусы, которых нет в отчёте.`,
		query,
		kind,
		toPrettyJSON(summaryData),
	)

	answer, err := h.LLM.AnalyzeData(c.Context(), prompt, nil)
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
		Content:   answer,
		IsError:   false,
		CreatedAt: time.Now(),
	})

	return c.JSON(fiber.Map{
		"reply":      answer,
		"dataset_id": id,
		"mode":       "dashboard_aggregate_text",
	})
}

func (h *DatasetHandler) handleDashboardAggregateChart(
	c *fiber.Ctx,
	query string,
	data []map[string]interface{},
	id int,
	kind string,
) error {
	plan, err := h.buildDashboardChartPlan(c.Context(), query, data, kind)
	if err != nil {
		msg := "Не удалось построить график по запросу: " + err.Error()

		h.DB.Create(&models.ChatMessage{
			DatasetID: uint(id),
			Role:      "bot",
			Content:   msg,
			IsError:   true,
			CreatedAt: time.Now(),
		})

		return c.Status(500).JSON(fiber.Map{"error": msg})
	}

	chartData, yKeys, err := buildDashboardChartDataFromPlan(data, plan)
	if err != nil {
		msg := "Не удалось подготовить данные графика: " + err.Error()

		h.DB.Create(&models.ChatMessage{
			DatasetID: uint(id),
			Role:      "bot",
			Content:   msg,
			IsError:   true,
			CreatedAt: time.Now(),
		})

		return c.Status(400).JSON(fiber.Map{"error": msg})
	}

	if len(chartData) == 0 {
		msg := "Не удалось построить график: по выбранным колонкам не найдено числовых значений."

		h.DB.Create(&models.ChatMessage{
			DatasetID: uint(id),
			Role:      "bot",
			Content:   msg,
			IsError:   true,
			CreatedAt: time.Now(),
		})

		return c.Status(400).JSON(fiber.Map{"error": msg})
	}

	title := strings.TrimSpace(plan.Title)
	if title == "" {
		title = "График по данным отчёта"
	}

	chartType := normalizeChartType(plan.Type)

	codeOutput := map[string]interface{}{
		"meta": map[string]interface{}{
			"scanned_rows":          len(data),
			"used_rows":             len(chartData),
			"total_rows_expected":   len(data),
			"mode":                  "dashboard_aggregate_llm_planned",
			"dashboard_report_type": kind,
			"chart_plan":            plan,
		},
		"summary": map[string]interface{}{
			"message":   "График построен по плану LLM. Значения посчитаны детерминированно по данным файла.",
			"query":     query,
			"x_column":  plan.XColumn,
			"y_columns": plan.YColumns,
		},
		"charts": []map[string]interface{}{
			{
				"type":  chartType,
				"title": title,

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
		"График построен по вашему запросу. X: %s. Y: %s. Использовано строк: %d.",
		plan.XColumn,
		strings.Join(plan.YColumns, ", "),
		len(chartData),
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

func (h *DatasetHandler) buildDashboardChartPlan(
	ctx context.Context,
	query string,
	data []map[string]interface{},
	kind string,
) (DashboardChartPlan, error) {
	headers := collectDashboardHeaders(data)
	numericHeaders := collectDashboardNumericHeaders(data)
	sample := collectDashboardSampleRows(data, 15)

	prompt := fmt.Sprintf(`Ты аналитик данных и проектировщик графиков.

Пользователь просит:
%s

Тип отчёта:
%s

Это агрегированный Excel-отчёт, НЕ список студентов.

Доступные колонки:
%s

Колонки, где есть числовые значения:
%s

Пример строк:
%s

Твоя задача — выбрать, какой график построить по запросу пользователя.
НЕ считай числа сам. Только выбери колонки и параметры графика.
Имена колонок бери ТОЧНО из списка доступных колонок.

Верни СТРОГО JSON без markdown и пояснений:
{
  "type": "bar | line | pie",
  "title": "название графика",
  "x_column": "точное имя колонки для подписей по оси X",
  "y_columns": ["точное имя числовой колонки 1", "точное имя числовой колонки 2"],
  "sort_by": "точное имя числовой колонки для сортировки или пустая строка",
  "sort_desc": true,
  "limit": 20
}

Правила:
- Если пользователь просит сравнение программ/направлений — x_column обычно образовательная программа или направление.
- Если пользователь просит форму обучения — x_column должна быть колонка формы обучения.
- Если пользователь просит уровень образования — x_column должна быть колонка уровня образования.
- Если пользователь просит причины отчислений — y_columns должны быть колонки причин отчисления.
- Если пользователь просит переводы — y_columns должны быть колонки переводов/восстановлений.
- Если пользователь просит контингент — y_columns должна включать колонку контингента.
- Для круговой диаграммы используй только одну y_column.
- limit поставь разумно: 10, 15, 20 или 30.`,
		query,
		kind,
		toPrettyJSON(headers),
		toPrettyJSON(numericHeaders),
		toPrettyJSON(sample),
	)

	resp, err := h.LLM.AnalyzeData(ctx, prompt, nil)
	if err != nil {
		return DashboardChartPlan{}, err
	}

	cleaned := cleanDashboardLLMJSON(resp)
	if cleaned == "" {
		return DashboardChartPlan{}, fmt.Errorf("LLM вернула пустой план графика")
	}

	var plan DashboardChartPlan
	if err := json.Unmarshal([]byte(cleaned), &plan); err != nil {
		return DashboardChartPlan{}, fmt.Errorf("не удалось разобрать JSON-план графика: %w; raw=%s", err, cleaned)
	}

	if err := validateDashboardChartPlan(plan, headers, numericHeaders); err != nil {
		return DashboardChartPlan{}, err
	}

	plan.Type = normalizeChartType(plan.Type)

	if plan.Title == "" {
		plan.Title = "График по данным отчёта"
	}

	if plan.Limit <= 0 {
		plan.Limit = 20
	}
	if plan.Limit > 50 {
		plan.Limit = 50
	}

	if plan.SortBy == "" && len(plan.YColumns) > 0 {
		plan.SortBy = plan.YColumns[0]
	}

	return plan, nil
}

func validateDashboardChartPlan(plan DashboardChartPlan, headers []string, numericHeaders []string) error {
	if resolveDashboardColumnName(headers, plan.XColumn) == "" {
		return fmt.Errorf("LLM выбрала неизвестную x_column: %q", plan.XColumn)
	}

	if len(plan.YColumns) == 0 {
		return fmt.Errorf("LLM не выбрала y_columns")
	}

	for _, yCol := range plan.YColumns {
		if resolveDashboardColumnName(numericHeaders, yCol) == "" {
			return fmt.Errorf("LLM выбрала неизвестную или нечисловую y_column: %q", yCol)
		}
	}

	if plan.SortBy != "" {
		if resolveDashboardColumnName(numericHeaders, plan.SortBy) == "" {
			return fmt.Errorf("LLM выбрала неизвестную или нечисловую sort_by: %q", plan.SortBy)
		}
	}

	return nil
}

func buildDashboardChartDataFromPlan(
	data []map[string]interface{},
	plan DashboardChartPlan,
) ([]map[string]interface{}, []string, error) {
	headers := collectDashboardHeaders(data)

	xCol := resolveDashboardColumnName(headers, plan.XColumn)
	if xCol == "" {
		return nil, nil, fmt.Errorf("колонка X не найдена: %s", plan.XColumn)
	}

	resolvedYCols := make([]string, 0, len(plan.YColumns))
	for _, yCol := range plan.YColumns {
		resolved := resolveDashboardColumnName(headers, yCol)
		if resolved == "" {
			return nil, nil, fmt.Errorf("колонка Y не найдена: %s", yCol)
		}
		resolvedYCols = append(resolvedYCols, resolved)
	}

	yKeys := make([]string, 0, len(resolvedYCols))
	yKeyByColumn := make(map[string]string)

	for _, col := range resolvedYCols {
		key := shortDashboardMetricName(col)
		if key == "" {
			key = col
		}

		originalKey := key
		idx := 2
		for containsString(yKeys, key) {
			key = fmt.Sprintf("%s_%d", originalKey, idx)
			idx++
		}

		yKeys = append(yKeys, key)
		yKeyByColumn[col] = key
	}

	type accRow struct {
		Name   string
		Values map[string]float64
	}

	grouped := make(map[string]*accRow)

	for _, row := range data {
		if isDashboardServiceRow(row) {
			continue
		}

		label := strings.TrimSpace(fmt.Sprintf("%v", getValueByColumnName(row, xCol)))
		if label == "" || label == "<nil>" {
			continue
		}

		acc, ok := grouped[label]
		if !ok {
			acc = &accRow{
				Name:   label,
				Values: make(map[string]float64),
			}
			grouped[label] = acc
		}

		for _, col := range resolvedYCols {
			value := dashboardValueToFloat(getValueByColumnName(row, col))
			key := yKeyByColumn[col]
			acc.Values[key] += value
		}
	}

	chartData := make([]map[string]interface{}, 0, len(grouped))

	for _, acc := range grouped {
		point := map[string]interface{}{
			"name": acc.Name,
		}

		hasValue := false

		for _, key := range yKeys {
			value := acc.Values[key]
			point[key] = value
			if value != 0 {
				hasValue = true
			}
		}

		if hasValue {
			chartData = append(chartData, point)
		}
	}

	sortKey := ""
	if plan.SortBy != "" {
		resolvedSort := resolveDashboardColumnName(headers, plan.SortBy)
		sortKey = yKeyByColumn[resolvedSort]
	}
	if sortKey == "" && len(yKeys) > 0 {
		sortKey = yKeys[0]
	}

	if sortKey != "" {
		sort.Slice(chartData, func(i, j int) bool {
			left := dashboardValueToFloat(chartData[i][sortKey])
			right := dashboardValueToFloat(chartData[j][sortKey])

			if plan.SortDesc {
				return left > right
			}

			return left < right
		})
	}

	if plan.Limit > 0 && len(chartData) > plan.Limit {
		chartData = chartData[:plan.Limit]
	}

	return chartData, yKeys, nil
}

func buildDashboardAggregateSummary(data []map[string]interface{}, kind string) map[string]interface{} {
	headers := collectDashboardHeaders(data)
	numericHeaders := collectDashboardNumericHeaders(data)

	result := map[string]interface{}{
		"report_type":     kind,
		"rows_total":      len(data),
		"headers":         headers,
		"numeric_headers": numericHeaders,
		"totals":          map[string]float64{},
		"top_by_metric":   map[string][]map[string]interface{}{},
	}

	totals := make(map[string]float64)
	topByMetric := make(map[string][]map[string]interface{})

	labelColumns := []string{
		"Образовательная программа",
		"разовательная программа",
		"Направление подготовки, специальность",
		"Форма обучения",
		"уровень образования",
	}

	for _, metricCol := range numericHeaders {
		resolvedMetric := resolveDashboardColumnName(headers, metricCol)
		if resolvedMetric == "" {
			continue
		}

		rowsForMetric := make([]map[string]interface{}, 0)

		for _, row := range data {
			if isDashboardServiceRow(row) {
				continue
			}

			value := dashboardValueToFloat(getValueByColumnName(row, resolvedMetric))
			if value == 0 {
				continue
			}

			label := ""
			for _, labelCol := range labelColumns {
				resolvedLabel := resolveDashboardColumnName(headers, labelCol)
				if resolvedLabel == "" {
					continue
				}

				label = strings.TrimSpace(fmt.Sprintf("%v", getValueByColumnName(row, resolvedLabel)))
				if label != "" && label != "<nil>" {
					break
				}
			}

			if label == "" {
				label = "Без названия"
			}

			totals[resolvedMetric] += value

			rowsForMetric = append(rowsForMetric, map[string]interface{}{
				"name":  label,
				"value": value,
			})
		}

		sort.Slice(rowsForMetric, func(i, j int) bool {
			return dashboardValueToFloat(rowsForMetric[i]["value"]) > dashboardValueToFloat(rowsForMetric[j]["value"])
		})

		if len(rowsForMetric) > 10 {
			rowsForMetric = rowsForMetric[:10]
		}

		topByMetric[resolvedMetric] = rowsForMetric
	}

	result["totals"] = totals
	result["top_by_metric"] = topByMetric

	return result
}

func collectDashboardHeaders(data []map[string]interface{}) []string {
	set := make(map[string]string)

	for _, row := range data {
		for key := range row {
			normalized := normalizeDashboardHeader(key)
			if normalized == "" {
				continue
			}

			if _, exists := set[normalized]; !exists {
				set[normalized] = key
			}
		}
	}

	headers := make([]string, 0, len(set))
	for _, original := range set {
		headers = append(headers, original)
	}

	sort.Strings(headers)

	return headers
}

func collectDashboardNumericHeaders(data []map[string]interface{}) []string {
	headers := collectDashboardHeaders(data)
	numericSet := make(map[string]bool)

	for _, header := range headers {
		for _, row := range data {
			if isDashboardServiceRow(row) {
				continue
			}

			value := getValueByColumnName(row, header)
			if dashboardValueToFloat(value) != 0 {
				numericSet[header] = true
				break
			}
		}
	}

	result := make([]string, 0, len(numericSet))
	for header := range numericSet {
		result = append(result, header)
	}

	sort.Strings(result)

	return result
}

func collectDashboardSampleRows(data []map[string]interface{}, limit int) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)

	for _, row := range data {
		if isDashboardServiceRow(row) {
			continue
		}

		result = append(result, row)
		if len(result) >= limit {
			break
		}
	}

	return result
}

func resolveDashboardColumnName(headers []string, requested string) string {
	requestedNorm := normalizeDashboardHeader(requested)
	if requestedNorm == "" {
		return ""
	}

	for _, header := range headers {
		if normalizeDashboardHeader(header) == requestedNorm {
			return header
		}
	}

	for _, header := range headers {
		headerNorm := normalizeDashboardHeader(header)
		if strings.Contains(headerNorm, requestedNorm) || strings.Contains(requestedNorm, headerNorm) {
			return header
		}
	}

	return ""
}

func getValueByColumnName(row map[string]interface{}, column string) interface{} {
	normalizedColumn := normalizeDashboardHeader(column)

	for key, value := range row {
		if normalizeDashboardHeader(key) == normalizedColumn {
			return value
		}
	}

	return nil
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
	headers := collectDashboardHeaders([]map[string]interface{}{row})

	for _, alias := range aliases {
		resolved := resolveDashboardColumnName(headers, alias)
		if resolved == "" {
			continue
		}

		value := getValueByColumnName(row, resolved)
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}

	return ""
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

func shortDashboardMetricName(column string) string {
	normalized := normalizeDashboardHeader(column)

	replacer := strings.NewReplacer(
		"(чел.)", "",
		"чел.", "",
		"обучающихся", "",
		"обучения", "",
		"образование", "",
	)

	result := replacer.Replace(normalized)
	result = strings.TrimSpace(result)
	result = strings.Join(strings.Fields(result), " ")

	if result == "" {
		return column
	}

	return result
}

func normalizeChartType(chartType string) string {
	t := strings.ToLower(strings.TrimSpace(chartType))

	switch t {
	case "line", "линейный", "линейная":
		return "line"
	case "pie", "круговой", "круговая":
		return "pie"
	default:
		return "bar"
	}
}

func cleanDashboardLLMJSON(raw string) string {
	s := strings.TrimSpace(raw)

	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		s = s[start : end+1]
	}

	return strings.TrimSpace(s)
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}

	return false
}

func toPrettyJSON(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}

	return string(b)
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
		workingCode = code
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

func sanitizeCode(code string) string {
	code = strings.ReplaceAll(code, "import pandas", "# import pandas")
	code = strings.ReplaceAll(code, "import numpy", "# import numpy")
	code = strings.ReplaceAll(code, "from pandas", "# from pandas")

	lines := strings.Split(code, "\n")

	var cleanLines []string

	inMultilineAssign := false
	bracketDepth := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

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

		if reHardcodedAssign.MatchString(trimmed) &&
			(strings.Contains(trimmed, "[") || strings.Contains(trimmed, "{") || strings.Contains(trimmed, "(")) {

			cleanLines = append(cleanLines, "# "+line+" # REMOVED HARDCODED DATA")

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
