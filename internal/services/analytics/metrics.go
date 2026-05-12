package analytics

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"

	"exeldoctor/internal/models"
	"exeldoctor/internal/services/llm"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ==============================================================================
// 0. СЕРВИС
// ==============================================================================

type Service struct {
	DB  *gorm.DB
	LLM llm.LLMService
}

func NewAnalyticsService(db *gorm.DB, llm llm.LLMService) *Service {
	return &Service{DB: db, LLM: llm}
}

// ==============================================================================
// 1. DTO
// ==============================================================================

type LLMResponse struct {
	Summary    string           `json:"summary"`
	Metrics    MetricsData      `json:"metrics"`
	TimeSeries []TimePeriodData `json:"time_series"`
	Trends     []string         `json:"trends"`
	Anomalies  []string         `json:"anomalies"`
}

type MetricsData struct {
	TotalStudents   int64            `json:"total_students"`
	ActiveStudents  int64            `json:"active_students"`
	AverageScore    float64          `json:"average_score"`
	StatusBreakdown map[string]int64 `json:"status_breakdown"`
}

// Новые поля ScoreSum/ScoreCount нужны для корректного пересчёта среднего.
// Старый API не ломается: старые поля сохранены, новые опциональные.
type TimePeriodData struct {
	Period          string           `json:"period"`
	TotalStudents   int64            `json:"total_students"`
	ActiveStudents  int64            `json:"active_students"`
	AverageScore    float64          `json:"average_score"`
	StatusBreakdown map[string]int64 `json:"status_breakdown"`

	ScoreSum   float64 `json:"score_sum,omitempty"`
	ScoreCount int64   `json:"score_count,omitempty"`
}

type ColumnMapping struct {
	StudentID      *string  `json:"student_id"`
	FullName       *string  `json:"full_name"`
	EnrollmentYear *string  `json:"enrollment_year"`
	GraduationYear *string  `json:"graduation_year"`
	Status         *string  `json:"status"`
	Score          *string  `json:"score"`
	ActiveStatuses []string `json:"active_statuses"`
}

// ==============================================================================
// 2. ОСНОВНОЙ ПАЙПЛАЙН: Маппинг → Вычисления → Мердж
// ==============================================================================

func (s *Service) ProcessUnprocessedDatasets(ctx context.Context) error {
	var datasets []models.Dataset

	// Важно: стабильный порядок обработки.
	// При пересечении периодов владельцем периода станет датасет с меньшим ID.
	if err := s.DB.
		Where("is_processed = ?", false).
		Order("id ASC").
		Find(&datasets).Error; err != nil {
		return err
	}

	for _, ds := range datasets {
		log.Printf("[Analytics] Обработка датасета ID: %d", ds.ID)

		var data []map[string]interface{}
		if err := json.Unmarshal(ds.Data, &data); err != nil {
			log.Printf("[Analytics] Ошибка чтения JSON датасета %d: %v", ds.ID, err)
			continue
		}

		if len(data) == 0 {
			log.Printf("[Analytics] Датасет %d пуст, пропускаю", ds.ID)
			continue
		}

		// Новый путь: агрегированные Excel-отчёты для дашборда.
		// Для них НЕ нужен LLM-маппинг, потому что это не построчные данные студентов,
		// а уже готовые агрегаты: контингент, отчисления, переводы/восстановления.
		reportType := detectDashboardReportType(data)

		if reportType != dashboardReportStudent {
			log.Printf("[Analytics] Датасет %d определён как агрегированный отчёт: %s", ds.ID, reportType)

			result := computeDashboardAggregateReport(data, reportType, ds.Name)

			if err := s.mergeDashboardAggregateIntoGlobalMetrics(ctx, result, reportType); err != nil {
				log.Printf("[Analytics] Ошибка обновления глобальных метрик агрегированного отчёта %d: %v", ds.ID, err)
				continue
			}

			datasetSummary := buildDashboardDatasetSummary(reportType, result)

			if err := s.DB.Model(&ds).Updates(map[string]interface{}{
				"summary":      datasetSummary,
				"is_processed": true,
			}).Error; err != nil {
				log.Printf("[Analytics] Ошибка обновления агрегированного датасета %d: %v", ds.ID, err)
				continue
			}

			log.Printf("[Analytics] Агрегированный датасет %d успешно обработан: %s", ds.ID, datasetSummary)
			continue
		}

		// Старый путь: обычные датасеты студентов.
		// Здесь LLM определяет соответствие колонок: ФИО, ID, статус, балл и т.д.
		mapping, err := s.resolveColumnMapping(ctx, &ds, data)
		if err != nil {
			log.Printf("[Analytics] Ошибка маппинга для датасета %d: %v", ds.ID, err)
			continue
		}

		// 2) Детерминированные метрики в Go
		result := computeMetricsDeterministic(data, mapping)

		// 3) Мердж в global + глобальные инсайты от LLM
		if err := s.mergeIntoGlobalMetrics(ctx, result, ds.ID); err != nil {
			log.Printf("[Analytics] Ошибка обновления глобальных метрик: %v", err)
			continue
		}

		// 4) Summary конкретного датасета — без LLM
		datasetSummary := buildDatasetSummary(result)

		if err := s.DB.Model(&ds).Updates(map[string]interface{}{
			"summary":      datasetSummary,
			"is_processed": true,
		}).Error; err != nil {
			log.Printf("[Analytics] Ошибка обновления датасета %d: %v", ds.ID, err)
			continue
		}

		log.Printf("[Analytics] Датасет %d успешно обработан: %d студентов, %d периодов",
			ds.ID,
			result.Metrics.TotalStudents,
			len(result.TimeSeries),
		)
	}

	return nil
}

func buildDatasetSummary(result *LLMResponse) string {
	if result == nil {
		return "Датасет обработан."
	}

	return fmt.Sprintf(
		"Датасет обработан: студентов=%d, активных=%d, средний балл=%.2f, периодов=%d.",
		result.Metrics.TotalStudents,
		result.Metrics.ActiveStudents,
		result.Metrics.AverageScore,
		len(result.TimeSeries),
	)
}

// ==============================================================================
// 2.1. АГРЕГИРОВАННЫЕ ОТЧЁТЫ ДЛЯ ДАШБОРДА
// ==============================================================================

const (
	dashboardReportStudent    = "student"
	dashboardReportContingent = "contingent"
	dashboardReportMovement   = "movement"
	dashboardReportDeduction  = "deduction"
)

func detectDashboardReportType(data []map[string]interface{}) string {
	headers := collectHeaderIndex(data)

	if hasHeader(headers, "контингент обучающихся") {
		return dashboardReportContingent
	}

	if hasHeader(headers,
		"восстановлены (чел.)",
		"зачислены переводом из другого вуза/филиала (чел.)",
		"зачислены переводом из другого вуза/ филиала (чел.)",
		"переведены в другой вуз/филиал (чел.)",
		"переведены в другой вуз/ филиал (чел.)",
		"переведены в другогой вуз/филиал (чел.)",
		"переведены в другогой вуз/ филиал (чел.)",
	) {
		return dashboardReportMovement
	}

	if hasHeader(headers,
		"отчислено всего (чел.)",
		"отчислено ВСЕГО (чел.)",
		"отчислены за неуспеваемость (чел.)",
		"отчислены за не оплату обучения (чел.)",
		"отчислены за неоплату обучения (чел.)",
		"отчислены по собственному желанию (чел.)",
		"выпуск (получили образование)(чел.)",
		"ВЫПУСК (получили образование)(чел.)",
	) {
		return dashboardReportDeduction
	}

	return dashboardReportStudent
}

func computeDashboardAggregateReport(data []map[string]interface{}, reportType string, datasetName string) *LLMResponse {
	headers := collectHeaderIndex(data)
	period := extractPeriodFromDatasetName(datasetName)

	statuses := make(map[string]int64)

	var totalStudents int64
	var activeStudents int64

	switch reportType {
	case dashboardReportContingent:
		contingent := sumColumn(data, headers, "контингент обучающихся")

		totalStudents = contingent
		activeStudents = contingent

		statuses["контингент обучающихся"] = contingent

	case dashboardReportMovement:
		restored := sumColumn(data, headers, "восстановлены (чел.)")

		transferIn := sumColumn(data, headers,
			"зачислены переводом из другого вуза/филиала (чел.)",
			"зачислены переводом из другого вуза/ филиала (чел.)",
		)

		transferOut := sumColumn(data, headers,
			"переведены в другой вуз/филиал (чел.)",
			"переведены в другой вуз/ филиал (чел.)",
			"переведены в другогой вуз/филиал (чел.)",
			"переведены в другогой вуз/ филиал (чел.)",
		)

		statuses["восстановлены"] = restored
		statuses["зачислены переводом"] = transferIn
		statuses["переведены в другой вуз/филиал"] = transferOut

	case dashboardReportDeduction:
		deductedTotal := sumColumn(data, headers, "отчислено всего (чел.)", "отчислено ВСЕГО (чел.)")

		deductedBadProgress := sumColumn(data, headers,
			"отчислены за неуспеваемость (чел.)",
		)

		deductedNonPayment := sumColumn(data, headers,
			"отчислены за не оплату обучения (чел.)",
			"отчислены за неоплату обучения (чел.)",
		)

		deductedVoluntary := sumColumn(data, headers,
			"отчислены по собственному желанию (чел.)",
		)

		graduated := sumColumn(data, headers,
			"выпуск (получили образование)(чел.)",
			"ВЫПУСК (получили образование)(чел.)",
		)

		statuses["отчислено всего"] = deductedTotal
		statuses["отчислены за неуспеваемость"] = deductedBadProgress
		statuses["отчислены за неоплату обучения"] = deductedNonPayment
		statuses["отчислены по собственному желанию"] = deductedVoluntary
		statuses["выпуск"] = graduated
	}

	timeSeries := []TimePeriodData{
		{
			Period:          period,
			TotalStudents:   totalStudents,
			ActiveStudents:  activeStudents,
			AverageScore:    0,
			StatusBreakdown: statuses,
			ScoreSum:        0,
			ScoreCount:      0,
		},
	}

	return &LLMResponse{
		Metrics: MetricsData{
			TotalStudents:   totalStudents,
			ActiveStudents:  activeStudents,
			AverageScore:    0,
			StatusBreakdown: statuses,
		},
		TimeSeries: timeSeries,
	}
}

func buildDashboardDatasetSummary(reportType string, result *LLMResponse) string {
	if result == nil || len(result.TimeSeries) == 0 {
		return "Агрегированный отчёт обработан."
	}

	period := result.TimeSeries[0].Period
	breakdown := formatStatusBreakdown(result.Metrics.StatusBreakdown)

	switch reportType {
	case dashboardReportContingent:
		return fmt.Sprintf(
			"Отчёт по контингенту обработан: период=%s, контингент=%d.",
			period,
			result.Metrics.TotalStudents,
		)

	case dashboardReportMovement:
		return fmt.Sprintf(
			"Отчёт по переводам и восстановлениям обработан: период=%s, %s.",
			period,
			breakdown,
		)

	case dashboardReportDeduction:
		return fmt.Sprintf(
			"Отчёт по отчислениям обработан: период=%s, %s.",
			period,
			breakdown,
		)

	default:
		return fmt.Sprintf(
			"Агрегированный отчёт обработан: период=%s, %s.",
			period,
			breakdown,
		)
	}
}

func formatStatusBreakdown(statuses map[string]int64) string {
	if len(statuses) == 0 {
		return "показателей нет"
	}

	keys := make([]string, 0, len(statuses))
	for k := range statuses {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, statuses[k]))
	}

	return strings.Join(parts, ", ")
}

func collectHeaderIndex(data []map[string]interface{}) map[string]string {
	headers := make(map[string]string)

	for _, row := range data {
		for original := range row {
			normalized := normalizeHeaderName(original)
			if normalized == "" {
				continue
			}

			if _, exists := headers[normalized]; !exists {
				headers[normalized] = original
			}
		}
	}

	return headers
}

func hasHeader(headers map[string]string, aliases ...string) bool {
	return findHeader(headers, aliases...) != ""
}

func findHeader(headers map[string]string, aliases ...string) string {
	for _, alias := range aliases {
		normalized := normalizeHeaderName(alias)
		if original, ok := headers[normalized]; ok {
			return original
		}
	}

	return ""
}

func normalizeHeaderName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "ё", "е")
	s = strings.Join(strings.Fields(s), " ")

	s = strings.ReplaceAll(s, " /", "/")
	s = strings.ReplaceAll(s, "/ ", "/")

	// Опечатки и варианты написания из реальных файлов
	s = strings.ReplaceAll(s, "реализвции", "реализации")
	s = strings.ReplaceAll(s, "другогой", "другой")
	s = strings.ReplaceAll(s, "не оплату", "неоплату")

	return strings.TrimSpace(s)
}

func sumColumn(data []map[string]interface{}, headers map[string]string, aliases ...string) int64 {
	col := findHeader(headers, aliases...)
	if col == "" {
		return 0
	}

	var total int64

	for _, row := range data {
		if isDashboardServiceRow(row, headers) {
			continue
		}

		total += toInt64(row[col])
	}

	return total
}

func isDashboardServiceRow(row map[string]interface{}, headers map[string]string) bool {
	directionCol := findHeader(headers, "направление подготовки, специальность")
	programCol := findHeader(headers, "образовательная программа")

	if directionCol == "" && programCol == "" {
		return isEmptyDataRow(row)
	}

	direction := ""
	program := ""

	if directionCol != "" {
		direction = normalizeString(row[directionCol])
	}

	if programCol != "" {
		program = normalizeString(row[programCol])
	}

	// В файле "Контингент" есть итоговые числа вне таблицы.
	// Такие строки обычно без направления и без программы.
	if direction == "" && program == "" {
		return true
	}

	lowerDirection := strings.ToLower(direction)
	if strings.Contains(lowerDirection, "итого") || strings.Contains(lowerDirection, "всего") {
		return true
	}

	return false
}

func isEmptyDataRow(row map[string]interface{}) bool {
	for _, v := range row {
		if normalizeString(v) != "" {
			return false
		}
	}

	return true
}

func toInt64(val interface{}) int64 {
	f, ok := toFloat64(val)
	if !ok {
		return 0
	}

	return int64(math.Round(f))
}

var monthNames = map[string]string{
	"янв": "01", "фев": "02", "мар": "03", "апр": "04",
	"май": "05", "мая": "05", "июн": "06", "июл": "07",
	"авг": "08", "сен": "09", "окт": "10", "ноя": "11", "дек": "12",
}

func extractPeriodFromDatasetName(name string) string {
	lowerName := strings.ToLower(name)
	year := ""
	month := ""

	// 1. Ищем текстовое упоминание месяца в названии
	for key, val := range monthNames {
		if strings.Contains(lowerName, key) {
			month = val
			break
		}
	}

	// 2. Ищем год и/или цифровой месяц (например: report_2023_09.xlsx)
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r < '0' || r > '9'
	})

	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]

		// Ищем 4 цифры (год)
		if len(part) == 4 {
			if y, err := strconv.Atoi(part); err == nil && y >= 2000 && y <= 2100 {
				year = part

				// Если месяц текстом не нашли, смотрим на соседнее число (возможно, это номер месяца)
				if month == "" && i > 0 && len(parts[i-1]) <= 2 {
					m, _ := strconv.Atoi(parts[i-1])
					if m >= 1 && m <= 12 {
						month = fmt.Sprintf("%02d", m)
					}
				}
				break
			}
		} else if len(part) == 2 && year == "" {
			// Ищем 2 цифры как год, если 4 цифры не найдены
			if y, err := strconv.Atoi(part); err == nil && y >= 16 && y <= 40 {
				year = fmt.Sprintf("20%02d", y)
				break
			}
		}
	}

	if year != "" {
		if month == "" {
			month = "01" // Fallback: если месяц не определен, ставим январь
		}
		return fmt.Sprintf("%s-%s", year, month)
	}

	return "unknown"
}

// ==============================================================================
// 3. МАППИНГ КОЛОНОК
// ==============================================================================

func (s *Service) resolveColumnMapping(ctx context.Context, ds *models.Dataset, data []map[string]interface{}) (*ColumnMapping, error) {
	if len(ds.ColumnMapping) > 0 {
		var mapping ColumnMapping
		if err := json.Unmarshal(ds.ColumnMapping, &mapping); err == nil {
			normalizeMappingNulls(&mapping)
			log.Printf("[Analytics] Используем сохранённый маппинг для датасета %d", ds.ID)
			return &mapping, nil
		}
	}

	mapping, err := s.detectColumnMapping(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("не удалось определить маппинг: %w", err)
	}

	mappingJSON, _ := json.Marshal(mapping)
	if err := s.DB.Model(ds).Update("column_mapping", mappingJSON).Error; err != nil {
		log.Printf("[Analytics] Не удалось сохранить column_mapping для датасета %d: %v", ds.ID, err)
	}

	log.Printf("[Analytics] Маппинг определён и сохранён для датасета %d: %s", ds.ID, string(mappingJSON))

	return mapping, nil
}

func (s *Service) detectColumnMapping(ctx context.Context, data []map[string]interface{}) (*ColumnMapping, error) {
	previewLen := 5
	if len(data) < previewLen {
		previewLen = len(data)
	}

	sampleJSON, _ := json.MarshalIndent(data[:previewLen], "", "  ")
	statusHints := collectUniqueValues(data, 50)

	prompt := fmt.Sprintf(`Ты аналитик данных. Вот пример строк из CSV-датасета студентов вуза:
%s

Уникальные значения по колонкам (до 50 первых):
%s

Определи, какие колонки соответствуют следующим понятиям:
- student_id: уникальный идентификатор студента (номер зачётки, ID, и т.д.)
- full_name: ФИО или имя студента
- enrollment_year: ДАТА зачисления/поступления (колонка с датой, из которой можно извлечь год и месяц)
- graduation_year: год или дата выпуска
- status: текущий статус студента (обучается, отчислен, выпущен, академ и т.д.)
- score: средний балл или оценка
- active_statuses: список значений поля status, которые означают "студент сейчас активно учится"

ВЕРНИ СТРОГО JSON без пояснений:
{
  "student_id": "точное_имя_колонки_или_null",
  "full_name": "точное_имя_колонки_или_null",
  "enrollment_year": "точное_имя_колонки_или_null",
  "graduation_year": "точное_имя_колонки_или_null",
  "status": "точное_имя_колонки_или_null",
  "score": "точное_имя_колонки_или_null",
  "active_statuses": ["Значение1", "Значение2"]
}

Имена колонок бери ТОЧНО как в данных, с учётом регистра и пробелов.
Если подходящей колонки нет — ставь null.`, string(sampleJSON), statusHints)

	var mapping ColumnMapping
	maxRetries := 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := s.LLM.AnalyzeData(ctx, prompt, nil)
		if err != nil {
			lastErr = fmt.Errorf("LLM ошибка (попытка %d): %w", attempt, err)
			continue
		}

		cleaned := cleanLLMJSON(resp)
		if cleaned == "" {
			lastErr = fmt.Errorf("пустой ответ LLM (попытка %d)", attempt)
			continue
		}

		var attemptMapping ColumnMapping
		if err := json.Unmarshal([]byte(cleaned), &attemptMapping); err != nil {
			lastErr = fmt.Errorf("парсинг ответа (попытка %d): %w, raw: %s", attempt, err, cleaned)
			continue
		}

		normalizeMappingNulls(&attemptMapping)

		if attemptMapping.StudentID == nil && attemptMapping.FullName == nil {
			lastErr = fmt.Errorf("попытка %d: не найден ни student_id, ни full_name", attempt)
			continue
		}

		mapping = attemptMapping
		return &mapping, nil
	}

	return nil, fmt.Errorf("не удалось получить маппинг за %d попыток: %w", maxRetries, lastErr)
}

func normalizeMappingNulls(mapping *ColumnMapping) {
	normalizePtr := func(p **string) {
		if p == nil || *p == nil {
			return
		}

		v := strings.TrimSpace(**p)
		if v == "" || strings.EqualFold(v, "null") || strings.EqualFold(v, "nil") {
			*p = nil
			return
		}

		**p = v
	}

	normalizePtr(&mapping.StudentID)
	normalizePtr(&mapping.FullName)
	normalizePtr(&mapping.EnrollmentYear)
	normalizePtr(&mapping.GraduationYear)
	normalizePtr(&mapping.Status)
	normalizePtr(&mapping.Score)
}

func collectUniqueValues(data []map[string]interface{}, limit int) string {
	uniquePerCol := make(map[string]map[string]bool)

	for _, row := range data {
		for col, val := range row {
			if _, ok := uniquePerCol[col]; !ok {
				uniquePerCol[col] = make(map[string]bool)
			}

			if len(uniquePerCol[col]) < limit {
				uniquePerCol[col][fmt.Sprintf("%v", val)] = true
			}
		}
	}

	var sb strings.Builder

	cols := make([]string, 0, len(uniquePerCol))
	for col := range uniquePerCol {
		cols = append(cols, col)
	}
	sort.Strings(cols)

	for _, col := range cols {
		vals := make([]string, 0, len(uniquePerCol[col]))
		for v := range uniquePerCol[col] {
			vals = append(vals, v)
		}

		sort.Strings(vals)
		sb.WriteString(fmt.Sprintf("  %s: %s\n", col, strings.Join(vals, ", ")))
	}

	return sb.String()
}

// ==============================================================================
// 4. ДЕТЕРМИНИРОВАННЫЕ ВЫЧИСЛЕНИЯ
// ==============================================================================

func computeMetricsDeterministic(data []map[string]interface{}, mapping *ColumnMapping) *LLMResponse {
	uniqueData := deduplicateRows(data, mapping)

	periodMap := make(map[string]*periodAccumulator)
	globalAcc := &periodAccumulator{statusBreakdown: make(map[string]int64)}

	for _, row := range uniqueData {
		period := extractPeriod(row, mapping)

		acc, ok := periodMap[period]
		if !ok {
			acc = &periodAccumulator{statusBreakdown: make(map[string]int64)}
			periodMap[period] = acc
		}

		acc.totalStudents++
		globalAcc.totalStudents++

		if mapping.Status != nil {
			status := normalizeString(row[*mapping.Status])
			if status != "" {
				acc.statusBreakdown[status]++
				globalAcc.statusBreakdown[status]++

				if isActiveStatus(status, mapping.ActiveStatuses) {
					acc.activeStudents++
					globalAcc.activeStudents++
				}
			}
		}

		if mapping.Score != nil {
			if score, ok := toFloat64(row[*mapping.Score]); ok {
				acc.scoreSum += score
				acc.scoreCount++

				globalAcc.scoreSum += score
				globalAcc.scoreCount++
			}
		}
	}

	var timeSeries []TimePeriodData

	periods := make([]string, 0, len(periodMap))
	for p := range periodMap {
		periods = append(periods, p)
	}
	sort.Strings(periods)

	for _, period := range periods {
		acc := periodMap[period]

		timeSeries = append(timeSeries, TimePeriodData{
			Period:          period,
			TotalStudents:   acc.totalStudents,
			ActiveStudents:  acc.activeStudents,
			AverageScore:    acc.averageScore(),
			StatusBreakdown: acc.statusBreakdown,
			ScoreSum:        acc.scoreSum,
			ScoreCount:      acc.scoreCount,
		})
	}

	return &LLMResponse{
		Metrics: MetricsData{
			TotalStudents:   globalAcc.totalStudents,
			ActiveStudents:  globalAcc.activeStudents,
			AverageScore:    globalAcc.averageScore(),
			StatusBreakdown: globalAcc.statusBreakdown,
		},
		TimeSeries: timeSeries,
	}
}

type periodAccumulator struct {
	totalStudents   int64
	activeStudents  int64
	scoreSum        float64
	scoreCount      int64
	statusBreakdown map[string]int64
}

func (a *periodAccumulator) averageScore() float64 {
	if a.scoreCount == 0 {
		return 0
	}

	return math.Round((a.scoreSum/float64(a.scoreCount))*100) / 100
}

// Дедуп по составному ключу student + period.
func deduplicateRows(data []map[string]interface{}, mapping *ColumnMapping) []map[string]interface{} {
	seen := make(map[string]bool, len(data))
	result := make([]map[string]interface{}, 0, len(data))

	var usedHashFallback int
	var skippedDuplicates int

	for _, row := range data {
		period := extractPeriod(row, mapping)
		studentKey := buildStudentKey(row, mapping)

		if studentKey == "" {
			studentKey = "rowhash:" + makeStableRowSignature(row)
			usedHashFallback++
		}

		compositeKey := studentKey + "|" + period
		if seen[compositeKey] {
			skippedDuplicates++
			continue
		}

		seen[compositeKey] = true
		result = append(result, row)
	}

	log.Printf(
		"[Analytics] Дедупликация (student+period): %d → %d, дубликатов: %d, hash-fallback: %d",
		len(data),
		len(result),
		skippedDuplicates,
		usedHashFallback,
	)

	return result
}

func buildStudentKey(row map[string]interface{}, mapping *ColumnMapping) string {
	if mapping.StudentID != nil {
		id := strings.ToLower(normalizeString(row[*mapping.StudentID]))
		if id != "" {
			return "id:" + id
		}
	}

	if mapping.FullName != nil {
		name := strings.ToLower(normalizeString(row[*mapping.FullName]))
		if name != "" {
			return "name:" + name
		}
	}

	return ""
}

func makeStableRowSignature(row map[string]interface{}) string {
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder

	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(strings.ToLower(normalizeString(row[k])))
		b.WriteString(";")
	}

	sum := sha1.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func extractPeriod(row map[string]interface{}, mapping *ColumnMapping) string {
	if mapping.EnrollmentYear == nil {
		return "unknown"
	}

	raw := normalizeString(row[*mapping.EnrollmentYear])
	if raw == "" {
		return "unknown"
	}

	// Разбиваем строку по типичным разделителям дат
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '.' || r == '/' || r == '-' || r == ' '
	})

	if len(parts) >= 2 {
		year := ""
		month := ""

		// Проверяем первый элемент (формат YYYY-MM-DD)
		if len(parts[0]) == 4 {
			year = parts[0]
			month = fmt.Sprintf("%02d", parseMonth(parts[1]))
		} else if len(parts[len(parts)-1]) == 4 {
			// Проверяем последний элемент (формат DD.MM.YYYY)
			year = parts[len(parts)-1]
			if len(parts) >= 3 {
				month = fmt.Sprintf("%02d", parseMonth(parts[len(parts)-2]))
			} else { // MM.YYYY
				month = fmt.Sprintf("%02d", parseMonth(parts[0]))
			}
		} else if len(parts[len(parts)-1]) == 2 {
			// Формат DD.MM.YY
			y, _ := strconv.Atoi(parts[len(parts)-1])
			if y >= 0 && y <= 99 {
				// Упрощенная логика: < 50 это 2000-е
				year = fmt.Sprintf("20%02d", y)
				if len(parts) >= 3 {
					month = fmt.Sprintf("%02d", parseMonth(parts[len(parts)-2]))
				} else {
					month = fmt.Sprintf("%02d", parseMonth(parts[0]))
				}
			}
		}

		if year != "" && month != "00" {
			return fmt.Sprintf("%s-%s", year, month)
		}
	}

	// Fallback: если указан только год (4 цифры)
	if len(raw) >= 4 {
		yearStr := raw[:4]
		if _, err := strconv.Atoi(yearStr); err == nil {
			return yearStr + "-01" // Если месяца нет, по умолчанию берем январь
		}
	}

	return "unknown"
}

// Вспомогательная функция для парсинга месяца
func parseMonth(m string) int {
	v, _ := strconv.Atoi(m)
	if v >= 1 && v <= 12 {
		return v
	}
	return 0
}

func isActiveStatus(status string, activeStatuses []string) bool {
	statusLower := strings.ToLower(strings.TrimSpace(status))

	for _, as := range activeStatuses {
		if strings.ToLower(strings.TrimSpace(as)) == statusLower {
			return true
		}
	}

	return false
}

// ==============================================================================
// 5. ТЕКСТОВЫЕ ИНСАЙТЫ
// ==============================================================================

type textInsightsResult struct {
	Summary   string   `json:"summary"`
	Trends    []string `json:"trends"`
	Anomalies []string `json:"anomalies"`
}

func (s *Service) generateTextInsights(ctx context.Context, result *LLMResponse) textInsightsResult {
	metricsJSON, _ := json.MarshalIndent(struct {
		Metrics    MetricsData      `json:"metrics"`
		TimeSeries []TimePeriodData `json:"time_series"`
	}{
		Metrics:    result.Metrics,
		TimeSeries: result.TimeSeries,
	}, "", "  ")

	prompt := fmt.Sprintf(`Вот точные метрики по студентам вуза (посчитаны программно, 100%% точные):
%s

На основе этих данных:
1) Краткое summary (2-3 предложения)
2) Ключевые trends
3) anomalies (скачки/отклонения)

Верни СТРОГО JSON без пояснений:
{"summary":"...", "trends":["..."], "anomalies":["..."]}`, string(metricsJSON))

	resp, err := s.LLM.AnalyzeData(ctx, prompt, nil)
	if err != nil {
		log.Printf("[Analytics] Ошибка получения текстовых инсайтов: %v", err)

		return textInsightsResult{
			Summary:   "Автоматический анализ временно недоступен.",
			Trends:    []string{},
			Anomalies: []string{},
		}
	}

	cleaned := cleanLLMJSON(resp)
	if cleaned == "" {
		log.Printf("[Analytics] Пустой ответ LLM при генерации инсайтов")

		return textInsightsResult{
			Summary:   "Автоматический анализ временно недоступен.",
			Trends:    []string{},
			Anomalies: []string{},
		}
	}

	var insights textInsightsResult
	if err := json.Unmarshal([]byte(cleaned), &insights); err != nil {
		log.Printf("[Analytics] Ошибка парсинга инсайтов: %v", err)

		return textInsightsResult{
			Summary:   "Ошибка разбора текстовых инсайтов.",
			Trends:    []string{},
			Anomalies: []string{},
		}
	}

	return insights
}

// ==============================================================================
// 6. ГЛОБАЛЬНЫЕ МЕТРИКИ + ownership period
// ==============================================================================

func (s *Service) mergeDashboardAggregateIntoGlobalMetrics(ctx context.Context, newData *LLMResponse, reportType string) error {
	if newData == nil || len(newData.TimeSeries) == 0 {
		return nil
	}

	var (
		mergedMetrics MetricsData
		mergedTS      []TimePeriodData
	)

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var global models.GlobalMetric
		if err := tx.FirstOrCreate(&global, models.GlobalMetric{ID: 1}).Error; err != nil {
			return err
		}

		var currentTimeSeries []TimePeriodData
		if len(global.TimeSeries) > 0 {
			_ = json.Unmarshal(global.TimeSeries, &currentTimeSeries)
		}

		tsMap := make(map[string]TimePeriodData, len(currentTimeSeries))

		for _, ts := range currentTimeSeries {
			if ts.StatusBreakdown == nil {
				ts.StatusBreakdown = make(map[string]int64)
			}

			tsMap[ts.Period] = ts
		}

		incoming := newData.TimeSeries[0]
		if incoming.StatusBreakdown == nil {
			incoming.StatusBreakdown = make(map[string]int64)
		}

		existing, ok := tsMap[incoming.Period]
		if !ok {
			existing = TimePeriodData{
				Period:          incoming.Period,
				StatusBreakdown: make(map[string]int64),
			}
		}

		if existing.StatusBreakdown == nil {
			existing.StatusBreakdown = make(map[string]int64)
		}

		// Только отчёт "Контингент" меняет KPI "Всего студентов" и "Активные".
		// Отчёты "Отчисление" и "Перевод/восстановление" добавляют показатели
		// в StatusBreakdown, но не увеличивают total_students.
		if reportType == dashboardReportContingent {
			existing.TotalStudents = incoming.TotalStudents
			existing.ActiveStudents = incoming.ActiveStudents
			existing.AverageScore = incoming.AverageScore
			existing.ScoreSum = incoming.ScoreSum
			existing.ScoreCount = incoming.ScoreCount
		}

		for key, value := range incoming.StatusBreakdown {
			existing.StatusBreakdown[key] = value
		}

		tsMap[incoming.Period] = existing

		mergedTS = make([]TimePeriodData, 0, len(tsMap))
		for _, ts := range tsMap {
			mergedTS = append(mergedTS, ts)
		}

		sort.Slice(mergedTS, func(i, j int) bool {
			return mergedTS[i].Period < mergedTS[j].Period
		})

		mergedMetrics = recalcMetricsFromTimeSeries(mergedTS)

		global.TotalStudents = mergedMetrics.TotalStudents
		global.ActiveStudents = mergedMetrics.ActiveStudents
		global.AverageScore = mergedMetrics.AverageScore
		global.StatusBreakdown, _ = json.Marshal(mergedMetrics.StatusBreakdown)
		global.TimeSeries, _ = json.Marshal(mergedTS)

		return tx.Save(&global).Error
	})
	if err != nil {
		return err
	}

	insights := s.generateTextInsights(ctx, &LLMResponse{
		Metrics:    mergedMetrics,
		TimeSeries: mergedTS,
	})

	trendsJSON, _ := json.Marshal(insights.Trends)
	anomaliesJSON, _ := json.Marshal(insights.Anomalies)

	return s.DB.Model(&models.GlobalMetric{}).
		Where("id = ?", 1).
		Updates(map[string]interface{}{
			"trends":    trendsJSON,
			"anomalies": anomaliesJSON,
		}).Error
}

func (s *Service) mergeIntoGlobalMetrics(ctx context.Context, newData *LLMResponse, datasetID uint) error {
	var (
		mergedMetrics MetricsData
		mergedTS      []TimePeriodData
	)

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var global models.GlobalMetric
		if err := tx.FirstOrCreate(&global, models.GlobalMetric{ID: 1}).Error; err != nil {
			return err
		}

		var currentTimeSeries []TimePeriodData
		if len(global.TimeSeries) > 0 {
			_ = json.Unmarshal(global.TimeSeries, &currentTimeSeries)
		}

		tsMap := make(map[string]TimePeriodData, len(currentTimeSeries))
		for _, ts := range currentTimeSeries {
			tsMap[ts.Period] = ts
		}

		var skippedPeriods []string

		for _, newTs := range newData.TimeSeries {
			// Пытаемся занять период текущим датасетом.
			// При unique(period) в processed_periods это конкурентно-безопасно.
			pp := models.ProcessedPeriod{
				Period:    newTs.Period,
				DatasetID: datasetID,
			}

			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "period"}},
				DoNothing: true,
			}).Create(&pp).Error; err != nil {
				return err
			}

			var owner models.ProcessedPeriod
			if err := tx.Where("period = ?", newTs.Period).First(&owner).Error; err != nil {
				return err
			}

			if owner.DatasetID != datasetID {
				// Период уже принадлежит другому датасету — пропускаем.
				skippedPeriods = append(skippedPeriods, newTs.Period)
				continue
			}

			// Этот датасет — владелец периода.
			tsMap[newTs.Period] = newTs
		}

		if len(skippedPeriods) > 0 {
			log.Printf("[Analytics] Пропущены конфликтующие периоды (owner другой): %v", skippedPeriods)
		}

		mergedTS = make([]TimePeriodData, 0, len(tsMap))
		for _, ts := range tsMap {
			mergedTS = append(mergedTS, ts)
		}

		sort.Slice(mergedTS, func(i, j int) bool {
			return mergedTS[i].Period < mergedTS[j].Period
		})

		mergedMetrics = recalcMetricsFromTimeSeries(mergedTS)

		global.TotalStudents = mergedMetrics.TotalStudents
		global.ActiveStudents = mergedMetrics.ActiveStudents
		global.AverageScore = mergedMetrics.AverageScore
		global.StatusBreakdown, _ = json.Marshal(mergedMetrics.StatusBreakdown)
		global.TimeSeries, _ = json.Marshal(mergedTS)

		return tx.Save(&global).Error
	})
	if err != nil {
		return err
	}

	// Инсайты считаем по финальному merged набору.
	insights := s.generateTextInsights(ctx, &LLMResponse{
		Metrics:    mergedMetrics,
		TimeSeries: mergedTS,
	})

	trendsJSON, _ := json.Marshal(insights.Trends)
	anomaliesJSON, _ := json.Marshal(insights.Anomalies)

	return s.DB.Model(&models.GlobalMetric{}).
		Where("id = ?", 1).
		Updates(map[string]interface{}{
			"trends":    trendsJSON,
			"anomalies": anomaliesJSON,
		}).Error
}

// ==============================================================================
// 7. ВЫБОРКА С ФИЛЬТРАЦИЕЙ
// ==============================================================================

func (s *Service) GetAdvancedMetrics(startDate, endDate string) (*LLMResponse, error) {
	var global models.GlobalMetric
	if err := s.DB.First(&global, 1).Error; err != nil {
		return nil, err
	}

	response := &LLMResponse{}

	_ = json.Unmarshal(global.TimeSeries, &response.TimeSeries)
	_ = json.Unmarshal(global.Trends, &response.Trends)
	_ = json.Unmarshal(global.Anomalies, &response.Anomalies)

	if startDate != "" || endDate != "" {
		filtered := make([]TimePeriodData, 0, len(response.TimeSeries))

		for _, ts := range response.TimeSeries {
			if startDate != "" && ts.Period < startDate {
				continue
			}

			if endDate != "" && ts.Period > endDate {
				continue
			}

			filtered = append(filtered, ts)
		}

		response.TimeSeries = filtered
	}

	response.Metrics = recalcMetricsFromTimeSeries(response.TimeSeries)

	return response, nil
}

func recalcMetricsFromTimeSeries(timeSeries []TimePeriodData) MetricsData {
	var total, active int64
	statuses := make(map[string]int64)

	var totalScoreSum float64
	var totalScoreCount int64

	for _, ts := range timeSeries {
		total += ts.TotalStudents
		active += ts.ActiveStudents

		for status, count := range ts.StatusBreakdown {
			statuses[status] += count
		}

		// Корректный путь для новых данных.
		if ts.ScoreCount > 0 {
			totalScoreSum += ts.ScoreSum
			totalScoreCount += ts.ScoreCount
			continue
		}

		// Backward-compat для старых записей.
		// Важно: не считаем AverageScore=0 как настоящий средний балл.
		// Это защищает агрегированные отчёты "Контингент" без оценок.
		if ts.AverageScore > 0 && ts.TotalStudents > 0 {
			totalScoreSum += ts.AverageScore * float64(ts.TotalStudents)
			totalScoreCount += ts.TotalStudents
		}
	}

	var avgScore float64
	if totalScoreCount > 0 {
		avgScore = math.Round((totalScoreSum/float64(totalScoreCount))*100) / 100
	}

	return MetricsData{
		TotalStudents:   total,
		ActiveStudents:  active,
		AverageScore:    avgScore,
		StatusBreakdown: statuses,
	}
}

// ==============================================================================
// 8. УТИЛИТЫ
// ==============================================================================

func normalizeString(val interface{}) string {
	if val == nil {
		return ""
	}

	s := strings.TrimSpace(fmt.Sprintf("%v", val))
	if s == "<nil>" {
		return ""
	}

	return s
}

func toFloat64(val interface{}) (float64, bool) {
	if val == nil {
		return 0, false
	}

	switch v := val.(type) {
	case float64:
		return v, true

	case float32:
		return float64(v), true

	case int:
		return float64(v), true

	case int64:
		return float64(v), true

	case int32:
		return float64(v), true

	case json.Number:
		f, err := v.Float64()
		return f, err == nil

	case string:
		cleaned := strings.TrimSpace(v)
		cleaned = strings.Replace(cleaned, "\u00a0", "", -1)
		cleaned = strings.Replace(cleaned, " ", "", -1)
		cleaned = strings.Replace(cleaned, ",", ".", 1)

		f, err := strconv.ParseFloat(cleaned, 64)
		return f, err == nil

	default:
		s := fmt.Sprintf("%v", v)
		s = strings.TrimSpace(s)
		s = strings.Replace(s, "\u00a0", "", -1)
		s = strings.Replace(s, " ", "", -1)
		s = strings.Replace(s, ",", ".", 1)

		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	}
}

func cleanLLMJSON(raw string) string {
	s := strings.TrimSpace(raw)

	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")

	return strings.TrimSpace(s)
}
