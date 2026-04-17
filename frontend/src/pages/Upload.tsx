import { useState, useRef } from "preact/hooks";
import { route } from "preact-router";
import { apiFetch } from "../api/client";
import { Icons } from "../components/ui/Icons";

const MAX_FILE_SIZE_MB = 50; // Максимальный размер файла
const ALLOWED_EXTENSIONS = [".xlsx", ".csv"];

export function UploadPage() {
  const [isDragging, setIsDragging] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Валидация файла на клиенте
  const validateFile = (file: File): string | null => {
    const extension = "." + file.name.split(".").pop()?.toLowerCase();

    if (!ALLOWED_EXTENSIONS.includes(extension)) {
      return `Неподдерживаемый формат (${extension}). Пожалуйста, загрузите .xlsx или .csv файл.`;
    }

    if (file.size > MAX_FILE_SIZE_MB * 1024 * 1024) {
      return `Файл слишком большой. Максимальный размер: ${MAX_FILE_SIZE_MB} МБ.`;
    }

    return null; // Ошибок нет
  };

  const handleUpload = async (file: File | undefined) => {
    if (!file) return;

    setErrorMessage(null); // Сбрасываем прошлые ошибки

    // 1. Клиентская защита от дурака
    const validationError = validateFile(file);
    if (validationError) {
      setErrorMessage(validationError);
      return;
    }

    setUploading(true);
    const formData = new FormData();
    formData.append("file", file);

    try {
      const res = await apiFetch("/upload", {
        method: "POST",
        body: formData,
      });

      const data = await res.json();

      if (res.ok) {
        // Успешно загрузили — переходим на страницу датасета
        route(`/dataset/${data.id || data.ID}`);
      } else {
        setErrorMessage(
          data.error || "Произошла ошибка при загрузке на сервер",
        );
      }
    } catch (err: any) {
      setErrorMessage(err.message || "Ошибка сети. Сервер недоступен.");
    } finally {
      setUploading(false);

      // 2. Очищаем скрытый инпут, чтобы можно было выбрать тот же файл повторно после ошибки
      if (fileInputRef.current) {
        fileInputRef.current.value = "";
      }
    }
  };

  // Обработчики Drag & Drop
  const handleDragOver = (e: any) => {
    e.preventDefault();
    if (!uploading) setIsDragging(true);
  };

  const handleDragLeave = (e: any) => {
    e.preventDefault();
    setIsDragging(false);
  };

  const handleDrop = (e: any) => {
    e.preventDefault();
    setIsDragging(false);

    if (uploading) return; // Игнорируем дроп, если уже идет загрузка

    const files = e.dataTransfer?.files;
    if (!files || files.length === 0) return;

    if (files.length > 1) {
      setErrorMessage("Пожалуйста, перетащите только один файл за раз.");
      return;
    }

    handleUpload(files[0]);
  };

  return (
    <div class="flex-1 flex flex-col items-center justify-center p-6 h-full relative">
      <div class="max-w-md w-full text-center mb-8">
        <h1 class="text-3xl font-semibold tracking-tight mb-2 text-white">
          Новый датасет
        </h1>
        <p class="text-gray-400 text-sm">
          Поддерживаются форматы Excel (.xlsx) и .csv (до {MAX_FILE_SIZE_MB} МБ)
        </p>
      </div>

      {/* Вывод красивой ошибки вместо системного alert */}
      {errorMessage && (
        <div class="max-w-xl w-full mb-6 p-4 bg-red-500/10 border border-red-500/20 rounded-xl flex items-start gap-3 text-red-400 animate-in fade-in slide-in-from-top-4">
          <svg
            class="w-5 h-5 flex-shrink-0 mt-0.5"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
            />
          </svg>
          <span class="text-sm font-medium leading-relaxed">
            {errorMessage}
          </span>
          <button
            onClick={() => setErrorMessage(null)}
            class="ml-auto text-red-500/50 hover:text-red-400 transition-colors"
          >
            <svg
              class="w-5 h-5"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </button>
        </div>
      )}

      {/* Зона Drag & Drop */}
      <div
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        class={`w-full max-w-xl border-2 border-dashed rounded-[2rem] p-16 text-center transition-all duration-300 relative overflow-hidden ${
          uploading
            ? "border-border bg-surface/50 cursor-not-allowed opacity-75"
            : isDragging
              ? "border-blue-500 bg-blue-500/5 scale-[1.02]"
              : "border-border bg-surface hover:border-gray-500 hover:bg-surface/80"
        }`}
      >
        {/* Индикатор загрузки поверх зоны */}
        {uploading && (
          <div class="absolute inset-0 bg-background/50 backdrop-blur-sm flex flex-col items-center justify-center z-10">
            <Icons.Spinner class="w-10 h-10 animate-spin text-blue-500 mb-4" />
            <p class="text-sm font-medium text-white animate-pulse">
              Чтение и загрузка данных...
            </p>
          </div>
        )}

        <div
          class={`mb-6 flex justify-center transition-colors ${isDragging ? "text-blue-500" : "text-gray-500"}`}
        >
          <svg
            class="w-16 h-16"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="1.5"
              d="M9 13h6m-3-3v6m5 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
            />
          </svg>
        </div>

        <h3 class="text-lg font-medium text-gray-200 mb-2">
          Перетащите файл сюда
        </h3>
        <p class="text-sm text-gray-500 mb-8">или нажмите кнопку ниже</p>

        <input
          type="file"
          ref={fileInputRef}
          class="hidden"
          accept=".xlsx,.csv"
          disabled={uploading}
          onChange={(e) => {
            const files = (e.target as HTMLInputElement).files;
            if (files && files.length > 0) handleUpload(files[0]);
          }}
        />

        <button
          disabled={uploading}
          onClick={() => fileInputRef.current?.click()}
          class="bg-white text-black px-8 py-2.5 rounded-xl font-medium hover:bg-gray-200 transition-all text-sm disabled:opacity-50 disabled:cursor-not-allowed shadow-sm"
        >
          Выбрать файл
        </button>
      </div>
    </div>
  );
}
