package sandbox

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	wazerosys "github.com/tetratelabs/wazero/sys"
)

//go:embed python.wasm
var pythonBinary []byte

// WasmRunner реализует изоляцию для запуска Python
type WasmRunner struct {
	runtime      wazero.Runtime
	compiledCode wazero.CompiledModule
}

// NewWasmRunner инициализирует VM. Делать это нужно один раз при старте приложения.
func NewWasmRunner(ctx context.Context) (*WasmRunner, error) {
	log.Println("[Sandbox] Initializing Wasm Runtime...")
	startTime := time.Now()

	// Настройка кэша и лимитов памяти (512 страниц * 64KB = 32MB, можно увеличить)
	runtimeConfig := wazero.NewRuntimeConfig().
		WithCompilationCache(wazero.NewCompilationCache()).
		WithMemoryLimitPages(2048) // ~128MB RAM для Python

	runtime := wazero.NewRuntimeWithConfig(ctx, runtimeConfig)

	// Инициализация WASI (файловая система, env, время, random)
	wasi_snapshot_preview1.MustInstantiate(ctx, runtime)

	log.Println("[Sandbox] Compiling python.wasm binary...")
	compiled, err := runtime.CompileModule(ctx, pythonBinary)
	if err != nil {
		return nil, fmt.Errorf("failed to compile embedded python: %w", err)
	}

	log.Printf("[Sandbox] Ready in %v", time.Since(startTime))
	return &WasmRunner{
		runtime:      runtime,
		compiledCode: compiled,
	}, nil
}

// Execute запускает Python код с переданными данными.
// Вход: code (строка), data (структура/слайс, который станет переменной dataset в Python)
func (r *WasmRunner) Execute(ctx context.Context, userCode string, data interface{}) (string, error) {
	execStart := time.Now()

	// 1. Подготовка полного скрипта (Data Injection + User Code)
	// Мы внедряем данные прямо в код, чтобы не возиться с парсингом stdin внутри Python скрипта,
	// так как сам скрипт мы будем передавать через stdin.
	fullScript, err := r.prepareScript(userCode, data)
	if err != nil {
		return "", fmt.Errorf("script preparation failed: %w", err)
	}

	log.Printf("[Sandbox] Executing code (Length: %d bytes)...", len(fullScript))

	// 2. Настройка потоков ввода/вывода
	// Мы передаем скрипт в STDIN. Python, запущенный с аргументом "-", будет читать код оттуда.
	inputBuffer := bytes.NewBufferString(fullScript)
	var stdout, stderr bytes.Buffer

	moduleConfig := wazero.NewModuleConfig().
		WithStdin(inputBuffer). // <-- Скрипт летит сюда
		WithStdout(&stdout).
		WithStderr(&stderr).
		WithArgs("python", "-"). // <-- Аргумент "-" заставляет Python читать код из stdin
		WithSysWalltime().
		WithSysNanotime()

	// 3. Запуск модуля
	inst, err := r.runtime.InstantiateModule(ctx, r.compiledCode, moduleConfig)

	// Важно: закрываем инстанс (освобождаем память) сразу после выполнения
	if inst != nil {
		defer inst.Close(ctx)
	}

	duration := time.Since(execStart)

	// 4. Обработка результатов
	if err != nil {
		// Python sys.exit(0) вызывает ошибку в wasm, но это штатный выход
		if exitErr, ok := err.(*wazerosys.ExitError); ok && exitErr.ExitCode() == 0 {
			log.Printf("[Sandbox] Success (Took: %v)", duration)
			return stdout.String(), nil
		}

		log.Printf("[Sandbox] Execution Error after %v: %v", duration, err)
		log.Printf("[Sandbox] Stderr Output:\n%s", stderr.String())

		// Возвращаем комбинированную ошибку, чтобы LLM могла её прочитать
		return "", fmt.Errorf("Runtime Error: %v\nDetails:\n%s", err, stderr.String())
	}

	// Если stderr не пустой, но exit code 0 — это могут быть warnings
	if stderr.Len() > 0 {
		log.Printf("[Sandbox] Finished with warnings (Took: %v)", duration)
		// Можно вернуть stdout, но добавить stderr в лог
	} else {
		log.Printf("[Sandbox] Success (Took: %v)", duration)
	}

	return stdout.String(), nil
}

// prepareScript "склеивает" данные и пользовательский код
func (r *WasmRunner) prepareScript(userCode string, data interface{}) (string, error) {
	// Сериализуем данные в JSON
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	// Экранируем тройные кавычки, чтобы не сломать Python string literal
	jsonString := string(dataBytes)
	safeJsonString := strings.ReplaceAll(jsonString, `"""`, `\"\"\"`)

	// Формируем преамбулу.
	// Мы используем try/except, чтобы скрипт не падал, если данные битые,
	// и обеспечиваем наличие переменной dataset.
	preamble := fmt.Sprintf(`
import sys
import json

# --- AUTO-GENERATED PREAMBLE START ---
try:
    raw_data = """%s"""
    dataset = json.loads(raw_data)
except Exception as e:
    print(json.dumps({"error": f"System Error: Failed to load dataset: {e}"}))
    sys.exit(1)
# --- AUTO-GENERATED PREAMBLE END ---

`, safeJsonString)

	return preamble + "\n" + userCode, nil
}

func (r *WasmRunner) Close(ctx context.Context) error {
	log.Println("[Sandbox] Closing runtime...")
	return r.runtime.Close(ctx)
}
