import { h } from "preact";
import { useState, useEffect } from "preact/hooks";
import { route } from "preact-router";
import { apiFetch } from "../api/client";
import { Icons } from "../components/ui/Icons";

export function SetupPage() {
  const [loading, setLoading] = useState(true);
  const [credentials, setCredentials] = useState<{
    username: string;
    password: string;
  } | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Состояния для защиты от дурака
  const [isCopied, setIsCopied] = useState(false);
  const [isAcknowledged, setIsAcknowledged] = useState(false);
  const [isConfirming, setIsConfirming] = useState(false);

  useEffect(() => {
    // При загрузке страницы проверяем, доступен ли первичный сетап
    const fetchSetupData = async () => {
      try {
        const res = await apiFetch("/system/setup", { method: "GET" });
        const data = await res.json();

        if (res.ok && data.password) {
          setCredentials(data);
        } else {
          // Если система уже настроена (пароль уже показывали), выкидываем на логин
          route("/login", true);
        }
      } catch (err) {
        setError(
          "Ошибка подключения к серверу. Убедитесь, что бэкенд запущен.",
        );
      } finally {
        setLoading(false);
      }
    };

    fetchSetupData();
  }, []);

  const handleCopyPassword = async () => {
    if (credentials?.password) {
      try {
        await navigator.clipboard.writeText(credentials.password);
        setIsCopied(true);
        setIsAcknowledged(true); // Автоматически ставим галочку при копировании
      } catch (err) {
        alert(
          "Не удалось скопировать. Пожалуйста, выделите текст и скопируйте вручную.",
        );
      }
    }
  };

  const handleCompleteSetup = async () => {
    if (!isAcknowledged) return;

    setIsConfirming(true);
    try {
      // Сообщаем серверу, что мы сохранили пароль.
      // Сервер должен удалить его из памяти и заблокировать этот роут навсегда.
      const res = await apiFetch("/system/setup/complete", { method: "POST" });

      if (res.ok) {
        route("/login", true);
      } else {
        setError("Не удалось завершить настройку на сервере.");
      }
    } catch (err) {
      setError("Ошибка сети при подтверждении.");
    } finally {
      setIsConfirming(false);
    }
  };

  if (loading) {
    return (
      <div class="min-h-screen bg-background flex items-center justify-center text-gray-400">
        <Icons.Spinner class="w-10 h-10 animate-spin mb-4" />
      </div>
    );
  }

  // Если произошла ошибка или роут заблокирован
  if (error || !credentials) {
    return (
      <div class="min-h-screen bg-background flex items-center justify-center p-6">
        <div class="bg-surface border border-border p-8 rounded-2xl max-w-md w-full text-center">
          <div class="text-red-500 mb-4 flex justify-center">
            <svg
              class="w-12 h-12"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
              />
            </svg>
          </div>
          <h2 class="text-xl text-white font-semibold mb-2">
            Ошибка установки
          </h2>
          <p class="text-sm text-gray-400 mb-6">
            {error || "Система уже инициализирована."}
          </p>
          <button
            onClick={() => route("/login", true)}
            class="bg-[#3291ff] hover:bg-[#2075d6] text-white px-6 py-2.5 rounded-xl font-medium transition w-full"
          >
            Перейти к авторизации
          </button>
        </div>
      </div>
    );
  }

  return (
    <div class="min-h-screen bg-background flex items-center justify-center p-6 relative overflow-hidden">
      {/* Декоративный фон */}
      <div class="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[600px] h-[600px] bg-blue-500/20 blur-[120px] rounded-full pointer-events-none" />

      <div class="bg-surface border border-border p-8 sm:p-10 rounded-[2rem] max-w-lg w-full shadow-2xl relative z-10 animate-in fade-in zoom-in-95 duration-500">
        <div class="w-16 h-16 bg-blue-500/10 border border-blue-500/20 rounded-2xl flex items-center justify-center mb-6 text-blue-400">
          <svg
            class="w-8 h-8"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"
            />
          </svg>
        </div>

        <h1 class="text-3xl font-bold text-white tracking-tight mb-2">
          Успешный запуск!
        </h1>
        <p class="text-gray-400 text-sm mb-8 leading-relaxed">
          Система инициализирована. Был автоматически создан главный аккаунт
          администратора.{" "}
          <strong class="text-gray-200">
            Пароль отображается только один раз!
          </strong>
        </p>

        {/* Карточка с доступами */}
        <div class="bg-black/50 border border-border rounded-2xl p-6 mb-8 relative group">
          <div class="flex flex-col space-y-4">
            <div>
              <div class="text-[10px] text-gray-500 uppercase tracking-wider font-semibold mb-1">
                Логин
              </div>
              <div class="text-white font-mono text-lg">
                {credentials.username}
              </div>
            </div>

            <div class="h-px w-full bg-border" />

            <div>
              <div class="text-[10px] text-gray-500 uppercase tracking-wider font-semibold mb-1">
                Пароль
              </div>
              <div class="text-white font-mono text-2xl tracking-widest break-all">
                {credentials.password}
              </div>
            </div>
          </div>

          <button
            onClick={handleCopyPassword}
            class={`absolute right-4 bottom-4 flex items-center gap-2 px-4 py-2 rounded-xl text-sm font-medium transition-all ${
              isCopied
                ? "bg-green-500/10 text-green-400 border border-green-500/20"
                : "bg-surface hover:bg-white/10 text-gray-300 border border-border"
            }`}
          >
            {isCopied ? (
              <>
                <svg
                  class="w-4 h-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M5 13l4 4L19 7"
                  />
                </svg>
                Скопировано
              </>
            ) : (
              <>
                <svg
                  class="w-4 h-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                  />
                </svg>
                Скопировать
              </>
            )}
          </button>
        </div>

        {/* Чекбокс защиты от дурака */}
        <label class="flex items-start gap-3 cursor-pointer mb-6 group">
          <div class="relative flex items-center justify-center mt-0.5">
            <input
              type="checkbox"
              class="peer sr-only"
              checked={isAcknowledged}
              onChange={(e) => setIsAcknowledged(e.currentTarget.checked)}
            />
            <div class="w-5 h-5 border-2 border-gray-600 rounded peer-checked:bg-blue-500 peer-checked:border-blue-500 transition-all flex items-center justify-center">
              <svg
                class={`w-3.5 h-3.5 text-white transition-transform ${isAcknowledged ? "scale-100" : "scale-0"}`}
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="3"
                  d="M5 13l4 4L19 7"
                />
              </svg>
            </div>
          </div>
          <span class="text-sm text-gray-300 group-hover:text-white transition-colors leading-snug">
            Я надежно сохранил этот пароль. Я понимаю, что восстановить его
            будет невозможно.
          </span>
        </label>

        {/* Кнопка входа */}
        <button
          onClick={handleCompleteSetup}
          disabled={!isAcknowledged || isConfirming}
          class={`w-full py-3.5 rounded-xl text-sm font-semibold transition-all flex items-center justify-center gap-2 ${
            isAcknowledged
              ? "bg-[#3291ff] hover:bg-[#2075d6] text-white shadow-[0_0_20px_rgba(50,145,255,0.3)]"
              : "bg-surface border border-border text-gray-500 cursor-not-allowed"
          }`}
        >
          {isConfirming ? (
            <Icons.Spinner class="w-5 h-5 animate-spin" />
          ) : (
            <>
              Продолжить работу
              <svg
                class="w-4 h-4"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M14 5l7 7m0 0l-7 7m7-7H3"
                />
              </svg>
            </>
          )}
        </button>
      </div>
    </div>
  );
}
