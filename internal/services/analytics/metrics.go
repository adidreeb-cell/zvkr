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

		// 1) Маппинг (LLM вызывается только если маппинг ещё не сохранён)
		mapping, err := s.resolveColumnMapping(ctx, &ds, data)
		if err != nil {
			log.Printf("[Analytics] Ошибка маппинга для датасета %d: %v", ds.ID, err)
			continue
		}

		// 2) Детерминированные метрики в Go
		result := computeMetricsDeterministic(data, mapping)

		// 3) Мердж в global + глобальные инсайты от LLM (один вызов после мерджа)
		if err := s.mergeIntoGlobalMetrics(ctx, result, ds.ID); err != nil {
			log.Printf("[Analytics] Ошибка обновления глобальных метрик: %v", err)
			continue
		}

		// 4) Summary конкретного датасета — без LLM (дешево и детерминированно)
		datasetSummary := buildDatasetSummary(result)
		if err := s.DB.Model(&ds).Updates(map[string]interface{}{
			"summary":      datasetSummary,
			"is_processed": true,
		}).Error; err != nil {
			log.Printf("[Analytics] Ошибка обновления датасета %d: %v", ds.ID, err)
			continue
		}

		log.Printf("[Analytics] Датасет %d успешно обработан: %d студентов, %d периодов",
			ds.ID, result.Metrics.TotalStudents, len(result.TimeSeries))
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
// 3. МАППИНГ КОЛОНОК (LLM вызывается ОДИН раз, результат кэшируется в Dataset)
// ==============================================================================

func (s *Service) resolveColumnMapping(ctx context.Context, ds *models.Dataset, data []map[string]interface{}) (*ColumnMapping, error) {
	if len(ds.ColumnMapping) > 0 {
		var mapping ColumnMapping
		if err := json.Unmarshal(ds.ColumnMapping, &mapping); err == nil {
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
- enrollment_year: год поступления (или дата, из которой можно извлечь год)
- graduation_year: год выпуска
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
		if err := json.Unmarshal([]byte(cleaned), &mapping); err != nil {
			lastErr = fmt.Errorf("парсинг ответа (попытка %d): %w, raw: %s", attempt, err, cleaned)
			continue
		}

		if mapping.StudentID == nil && mapping.FullName == nil {
			lastErr = fmt.Errorf("попытка %d: не найден ни student_id, ни full_name", attempt)
			continue
		}

		return &mapping, nil
	}

	return nil, fmt.Errorf("не удалось получить маппинг за %d попыток: %w", maxRetries, lastErr)
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
// 4. ДЕТЕРМИНИРОВАННЫЕ ВЫЧИСЛЕНИЯ (Go)
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
		len(data), len(result), skippedDuplicates, usedHashFallback,
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

	// yyyy....
	if len(raw) >= 4 {
		yearStr := raw[:4]
		if _, err := strconv.Atoi(yearStr); err == nil {
			return yearStr
		}
	}

	// dd.mm.yyyy / yyyy-mm-dd / ...
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '.' || r == '/' || r == '-'
	})
	for i := len(parts) - 1; i >= 0; i-- {
		if len(parts[i]) == 4 {
			if _, err := strconv.Atoi(parts[i]); err == nil {
				return parts[i]
			}
		}
	}

	return "unknown"
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
// 5. ТЕКСТОВЫЕ ИНСАЙТЫ (LLM на финальных merged данных)
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
			Summary: "Автоматический анализ временно недоступен.",
			Trends:  []string{},
			Anomalies: []string{},
		}
	}

	var insights textInsightsResult
	if err := json.Unmarshal([]byte(cleanLLMJSON(resp)), &insights); err != nil {
		log.Printf("[Analytics] Ошибка парсинга инсайтов: %v", err)
		return textInsightsResult{
			Summary: "Ошибка разбора текстовых инсайтов.",
			Trends:  []string{},
			Anomalies: []string{},
		}
	}

	return insights
}

// ==============================================================================
// 6. ГЛОБАЛЬНЫЕ МЕТРИКИ + ownership period
// ==============================================================================

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

	// Инсайты считаем по финальному merged набору (один вызов LLM).
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

		// Корректный путь (новые данные)
		if ts.ScoreCount > 0 {
			totalScoreSum += ts.ScoreSum
			totalScoreCount += ts.ScoreCount
			continue
		}

		// Backward-compat для старых записей
		if ts.TotalStudents > 0 {
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
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		cleaned := strings.Replace(strings.TrimSpace(v), ",", ".", 1)
		f, err := strconv.ParseFloat(cleaned, 64)
		return f, err == nil
	default:
		s := fmt.Sprintf("%v", v)
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
