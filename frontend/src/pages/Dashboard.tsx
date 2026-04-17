import { useState, useEffect, useRef } from "preact/hooks";
import { Link } from "preact-router";
import { apiFetch } from "../api/client";
import { Icons } from "../components/ui/Icons";
import type { Dataset, Metrics } from "../types";

// Расширяем тип NewsItem для поддержки источника
export interface NewsItem {
  title?: string;
  Title?: string;
  link?: string;
  Link?: string;
  source?: string;
  Source?: string;
}

export function Dashboard() {
  const [datasets, setDatasets] = useState<Dataset[]>([]);
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  const [metricsPeriod, setMetricsPeriod] = useState<string>(""); // Год последней статистики
  const [news, setNews] = useState<NewsItem[]>([]);

  // Состояния для статуса синхронизации
  const [lastUpdated, setLastUpdated] = useState<string>("");
  const [pendingCount, setPendingCount] = useState<number>(0);

  const [loading, setLoading] = useState(true);
  const [isSyncing, setIsSyncing] = useState(false); // Идет ли HTTP-запрос синхронизации прямо сейчас
  const [isRefreshing, setIsRefreshing] = useState(false);

  const role = localStorage.getItem("role");
  const canForceSync =
    role === "admin" || role === "analyst" || role === "аналитик";

  // Функция загрузки данных + статуса
  const fetchDashboardData = async (silent = false) => {
    if (!silent) setIsRefreshing(true);

    try {
      const [dsRes, metRes, newsRes, statusRes] = await Promise.allSettled([
        apiFetch("/datasets").then((r) => r.json()),
        apiFetch("/analytics/metrics").then((r) => r.json()),
        apiFetch("/news").then((r) => r.json()),
        apiFetch("/analytics/status").then((r) => r.json()), // Запрос статуса
      ]);

      if (dsRes.status === "fulfilled") setDatasets(dsRes.value || []);

      if (metRes.status === "fulfilled" && metRes.value) {
        const d = metRes.value;
        const payload = d.data || d;
        setMetrics(payload.metrics ? payload.metrics : payload);
        if (d.period) setMetricsPeriod(d.period);
      }

      if (newsRes.status === "fulfilled" && newsRes.value) {
        setNews(newsRes.value.slice ? newsRes.value.slice(0, 4) : []);
      }

      // Обработка статуса синхронизации
      if (statusRes.status === "fulfilled" && statusRes.value) {
        const val = statusRes.value;
        setPendingCount(val.pending_count || 0);

        // Форматируем дату, отсекая пустые значения из базы (0001-01-01)
        if (val.last_updated && !val.last_updated.startsWith("0001")) {
          const date = new Date(val.last_updated);
          setLastUpdated(
            date.toLocaleString("ru-RU", {
              day: "2-digit",
              month: "2-digit",
              year: "numeric",
              hour: "2-digit",
              minute: "2-digit",
            }),
          );
        }
      }
    } catch (err) {
      console.error("Ошибка при получении данных дашборда:", err);
    } finally {
      if (!silent) setIsRefreshing(false);
    }
  };

  // 1. Первичная загрузка
  useEffect(() => {
    fetchDashboardData().finally(() => setLoading(false));
  }, []);

  // 2. Механизм фонового обновления (Real-time polling) каждые 15 секунд
  useEffect(() => {
    // Поллинг работает постоянно, чтобы вовремя заметить завершение чужой синхронизации
    const interval = setInterval(() => {
      fetchDashboardData(true); // silent = true
    }, 15000);

    return () => clearInterval(interval);
  }, []);

  // Запуск тяжелого парсинга (Force Sync)
  const handleForceSync = async () => {
    if (pendingCount === 0) {
      alert("Все датасеты уже обработаны! Метрики актуальны.");
      return;
    }

    if (
      !confirm(
        `Запустить ИИ-анализ для новых датасетов (${pendingCount} шт.)? Это может занять некоторое время.`,
      )
    ) {
      return;
    }

    setIsSyncing(true);
    try {
      const res = await apiFetch("/analytics/sync", { method: "GET" });
      const data = await res.json();

      if (res.ok) {
        alert("✅ " + (data.message || "Метрики успешно обновлены!"));
        await fetchDashboardData(); // Сразу подтягиваем свежие данные
      } else {
        alert("❌ Ошибка: " + (data.error || "Не удалось обновить метрики"));
      }
    } catch (err) {
      alert("❌ Ошибка сети при попытке синхронизации");
    } finally {
      setIsSyncing(false);
    }
  };

  // Ручное быстрое обновление статистики
  const handleRefresh = () => {
    fetchDashboardData(false);
  };

  if (loading) {
    return (
      <div class="h-full flex items-center justify-center text-gray-400">
        <Icons.Spinner class="w-8 h-8 animate-spin" />
      </div>
    );
  }

  return (
    <div class="container mx-auto px-6 py-8 overflow-y-auto h-full custom-scrollbar">
      {/* --- ШАПКА ДАШБОРДА --- */}
      <div class="flex flex-col md:flex-row md:items-center justify-between mb-8 gap-4">
        <div>
          <h1 class="text-3xl font-semibold tracking-tight text-white flex items-center gap-3">
            Обзор показателей
            {metricsPeriod && (
              <span class="text-sm font-medium bg-blue-500/10 text-blue-400 border border-blue-500/20 px-2 py-1 rounded-md">
                за {metricsPeriod} год
              </span>
            )}
          </h1>
          <div class="mt-2 space-y-1">
            <p class="text-sm text-gray-400">
              Сводка данных на основе последнего доступного периода
            </p>
            {lastUpdated && (
              <p class="text-xs text-gray-500 flex items-center">
                <svg
                  class="w-3.5 h-3.5 mr-1"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                  />
                </svg>
                Последнее обновление: {lastUpdated}
              </p>
            )}
          </div>
        </div>

        {/* Блок с кнопками управления */}
        <div class="flex items-center space-x-2 md:space-x-3">
          {/* Кнопка ручного обновления */}
          <button
            onClick={handleRefresh}
            disabled={isRefreshing || isSyncing}
            title="Обновить данные на экране"
            class="flex items-center justify-center p-2.5 bg-surface border border-border text-gray-300 hover:text-white hover:bg-white/5 rounded-xl transition-all disabled:opacity-50 disabled:cursor-not-allowed shadow-sm"
          >
            <svg
              class={`w-4 h-4 ${isRefreshing ? "animate-spin" : ""}`}
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
              />
            </svg>
          </button>

          {/* AI-Аналитика */}
          <Link
            href="/analytics/advanced"
            class="flex items-center px-4 py-2.5 bg-gradient-to-r from-indigo-500/10 to-purple-500/10 hover:from-indigo-500/20 hover:to-purple-500/20 border border-indigo-500/30 text-indigo-300 rounded-xl text-sm font-medium transition-all shadow-sm whitespace-nowrap"
          >
            <span class="mr-2">✨</span>
            AI-Аналитика
          </Link>

          {/* Принудительная синхронизация датасетов */}
          {canForceSync && (
            <button
              onClick={handleForceSync}
              // Блокируем, если идет процесс ИЛИ если нет файлов для синхронизации
              disabled={isSyncing || pendingCount === 0}
              class="relative flex items-center space-x-2 bg-white text-black hover:bg-gray-200 px-4 py-2.5 rounded-xl text-sm font-medium transition-all disabled:opacity-50 disabled:cursor-not-allowed shadow-sm whitespace-nowrap"
            >
              {/* Красный бейдж, если есть файлы, ожидающие обработки */}
              {pendingCount > 0 && !isSyncing && (
                <span class="absolute -top-2 -right-2 flex h-5 w-5 items-center justify-center rounded-full bg-red-500 text-[11px] text-white font-bold border-2 border-background animate-pulse shadow-sm">
                  {pendingCount}
                </span>
              )}

              {isSyncing ? (
                <>
                  <Icons.Spinner class="w-4 h-4 animate-spin text-black" />
                  <span>Анализ...</span>
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
                      d="M19.428 15.428a2 2 0 00-1.022-.547l-2.387-.477a6 6 0 00-3.86.517l-.318.158a6 6 0 01-3.86.517L6.05 15.21a2 2 0 00-1.806.547M8 4h8l-1 1v5.172a2 2 0 00.586 1.414l5 5c1.26 1.26.367 3.414-1.415 3.414H4.828c-1.782 0-2.674-2.154-1.414-3.414l5-5A2 2 0 009 10.172V5L8 4z"
                    />
                  </svg>
                  <span class="hidden sm:inline">Синхронизация</span>
                </>
              )}
            </button>
          )}
        </div>
      </div>

      {/* --- КАРТОЧКИ KPI --- */}
      {metrics ? (
        <div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-12">
          <div class="bg-surface border border-border p-6 rounded-2xl shadow-sm relative overflow-hidden group">
            <div class="absolute inset-0 bg-gradient-to-b from-white/[0.03] to-transparent opacity-0 group-hover:opacity-100 transition-opacity" />
            <div class="text-gray-500 text-xs uppercase tracking-wider font-mono mb-2">
              Контингент
            </div>
            <div class="text-4xl font-medium tracking-tight text-white">
              {metrics.total_students?.toLocaleString("ru-RU") || 0}
            </div>
          </div>

          <div class="bg-surface border border-border p-6 rounded-2xl shadow-sm relative overflow-hidden group">
            <div class="absolute inset-0 bg-gradient-to-b from-green-500/[0.03] to-transparent opacity-0 group-hover:opacity-100 transition-opacity" />
            <div class="text-gray-500 text-xs uppercase tracking-wider font-mono mb-2">
              Активные студенты
            </div>
            <div class="text-4xl font-medium tracking-tight text-emerald-500">
              {metrics.active_students?.toLocaleString("ru-RU") || 0}
            </div>
          </div>

          <div class="bg-surface border border-border p-6 rounded-2xl shadow-sm relative overflow-hidden group">
            <div class="absolute inset-0 bg-gradient-to-b from-blue-500/[0.03] to-transparent opacity-0 group-hover:opacity-100 transition-opacity" />
            <div class="text-gray-500 text-xs uppercase tracking-wider font-mono mb-2">
              Средний балл
            </div>
            <div class="text-4xl font-medium tracking-tight text-blue-400">
              {metrics.average_score
                ? metrics.average_score.toFixed(2)
                : "0.00"}
            </div>
          </div>
        </div>
      ) : (
        <div class="w-full bg-surface border border-dashed border-border p-8 rounded-2xl text-center mb-12 text-gray-500">
          Данные статистики пока недоступны. Загрузите датасеты и запустите
          синхронизацию.
        </div>
      )}

      {/* --- НИЖНИЙ БЛОК (ДАТАСЕТЫ И НОВОСТИ) --- */}
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-8 pb-10">
        {/* Список датасетов */}
        <div class="col-span-2">
          <div class="flex items-center justify-between mb-6">
            <h2 class="text-xl font-semibold tracking-tight text-white">
              Источники данных
            </h2>
            <Link
              href="/upload"
              class="text-xs text-gray-400 hover:text-white transition border border-border hover:border-gray-500 rounded-full px-3 py-1"
            >
              Добавить +
            </Link>
          </div>

          {datasets.length === 0 ? (
            <div class="text-gray-500 text-sm border border-dashed border-border bg-surface/50 p-10 rounded-2xl text-center">
              Нет загруженных файлов
            </div>
          ) : (
            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              {datasets.map((ds, i) => (
                <Link
                  key={i}
                  href={`/dataset/${ds.ID || ds.id}`}
                  class="group bg-surface border border-border rounded-2xl p-5 hover:border-gray-500 hover:bg-white/[0.02] transition-all duration-200"
                >
                  <div class="flex justify-between items-start mb-4">
                    <div class="text-[10px] text-gray-400 font-mono uppercase tracking-wider bg-black border border-border px-2 py-0.5 rounded truncate max-w-[120px]">
                      {ds.source || "Unknown"}
                    </div>
                    <div class="text-gray-500 group-hover:text-white transition transform group-hover:translate-x-1">
                      →
                    </div>
                  </div>
                  <h3
                    class="text-base font-medium text-gray-200 truncate"
                    title={ds.name}
                  >
                    {ds.name}
                  </h3>
                  <div class="flex items-center justify-between mt-3">
                    <div class="text-xs text-gray-500">
                      {new Date(ds.created_at).toLocaleDateString("ru-RU")}
                    </div>
                    {ds.is_processed && (
                      <div
                        class="w-2 h-2 rounded-full bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.5)]"
                        title="Обработан ИИ"
                      />
                    )}
                  </div>
                </Link>
              ))}
            </div>
          )}
        </div>

        {/* Список новостей */}
        <div class="col-span-1">
          <h2 class="text-xl font-semibold tracking-tight text-white mb-6">
            Новости образования
          </h2>
          <div class="space-y-3">
            {news.length > 0 ? (
              news.map((n, i) => (
                <a
                  key={i}
                  href={n.link || n.Link || "#"}
                  target="_blank"
                  rel="noreferrer"
                  class="block bg-surface border border-border p-4 rounded-2xl hover:bg-white/[0.04] transition-all group"
                >
                  <h4 class="font-medium text-sm text-gray-300 leading-snug mb-3 group-hover:text-white line-clamp-2">
                    {n.title || n.Title || "Без заголовка"}
                  </h4>
                  <div class="flex items-center justify-between">
                    <span class="text-[10px] text-gray-500 uppercase tracking-wider bg-black/50 px-2 py-1 rounded">
                      {n.source || n.Source || "Внешний источник"}
                    </span>
                    <span class="text-xs text-blue-400 opacity-0 group-hover:opacity-100 transition-opacity">
                      Читать ↗
                    </span>
                  </div>
                </a>
              ))
            ) : (
              <div class="text-sm text-gray-500 bg-surface border border-dashed border-border p-6 rounded-2xl text-center">
                Лента пуста или загружается...
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
