import { h, Fragment } from "preact";
import { useState, useEffect, useRef } from "preact/hooks";
import { apiFetch } from "../api/client";
import { Icons } from "../components/ui/Icons";
import {
  LineChart,
  Line,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import * as XLSX from "xlsx";
import html2pdf from "html2pdf.js";

// --- ТИПИЗАЦИЯ ---
interface TimeSeriesItem {
  period: string;
  total_students: number;
  active_students: number;
  average_score: number;
  status_breakdown: Record<string, number>;
}

interface AnalyticsData {
  summary: string;
  metrics: {
    total_students: number;
    active_students: number;
    average_score: number;
    status_breakdown: Record<string, number>;
  };
  time_series: TimeSeriesItem[];
  trends: string[];
  anomalies: string[];
}

export function AdvancedAnalytics() {
  const [data, setData] = useState<AnalyticsData | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isRefetching, setIsRefetching] = useState(false);

  // Стейты для фильтров по времени
  const [availablePeriods, setAvailablePeriods] = useState<string[]>([]);
  const [fromPeriod, setFromPeriod] = useState("");
  const [toPeriod, setToPeriod] = useState("");

  // ============================================================================
  // ЗАГРУЗКА ДАННЫХ И ИНИЦИАЛИЗАЦИЯ
  // ============================================================================

  // Первичная инициализация с "умным" выбором диапазона (последний год - 4)
  useEffect(() => {
    let isMounted = true;

    const initData = async () => {
      setIsLoading(true);
      try {
        // 1. Получаем общие данные без фильтров, чтобы узнать доступные периоды
        const res = await apiFetch(`/analytics/advanced`);
        const json = await res.json();

        if (!json.success || !json.data) {
          if (isMounted) setIsLoading(false);
          return;
        }

        const allData = json.data as AnalyticsData;
        const periods = allData.time_series.map((ts) => ts.period).sort();

        if (isMounted) setAvailablePeriods(periods);

        if (periods.length > 0) {
          // 2. Определяем последний год и высчитываем -4 года
          const latestPeriod = periods[periods.length - 1];
          let startPeriod = periods[0]; // По умолчанию самый ранний доступный

          const latestYear = parseInt(latestPeriod, 10);
          if (!isNaN(latestYear)) {
            const targetStart = (latestYear - 4).toString(); // Диапазон 5 лет
            // Ищем точный год или ближайший к нему из доступных
            const found = periods.find((p) => p >= targetStart);
            if (found) startPeriod = found;
          } else {
            // Если формат периодов не числовой, берем последние 5 записей
            startPeriod = periods[Math.max(0, periods.length - 5)];
          }

          if (isMounted) {
            setFromPeriod(startPeriod);
            setToPeriod(latestPeriod);
          }

          // 3. Запрашиваем пересчитанные данные для полученного диапазона
          const filteredRes = await apiFetch(
            `/analytics/advanced?from=${startPeriod}&to=${latestPeriod}`,
          );
          const filteredJson = await filteredRes.json();
          if (filteredJson.success && filteredJson.data && isMounted) {
            setData(filteredJson.data);
          }
        } else {
          if (isMounted) setData(allData);
        }
      } catch (error) {
        console.error("Ошибка инициализации аналитики:", error);
      } finally {
        if (isMounted) setIsLoading(false);
      }
    };

    initData();
    return () => {
      isMounted = false;
    };
  }, []);

  // Ручной вызов загрузки (при применении фильтров)
  const fetchAnalytics = async (from = "", to = "") => {
    setIsRefetching(true);
    try {
      const query = new URLSearchParams();
      if (from) query.append("from", from);
      if (to) query.append("to", to);
      const qs = query.toString() ? `?${query.toString()}` : "";

      const res = await apiFetch(`/analytics/advanced${qs}`);
      const json = await res.json();
      if (json.success && json.data) {
        setData(json.data);
      }
    } catch (error) {
      console.error("Ошибка загрузки аналитики:", error);
    } finally {
      setIsRefetching(false);
    }
  };

  // ============================================================================
  // ЗАЩИТА ОТ ДУРАКА ПРИ ВЫБОРЕ
  // ============================================================================

  const handleFromChange = (e: any) => {
    const val = e.currentTarget.value;
    setFromPeriod(val);
    // Если выбрали начальный период больше, чем конечный -> подтягиваем конечный
    if (val !== "" && toPeriod !== "" && val > toPeriod) {
      setToPeriod(val);
    }
  };

  const handleToChange = (e: any) => {
    const val = e.currentTarget.value;
    setToPeriod(val);
    // Если выбрали конечный период меньше, чем начальный -> отодвигаем начальный
    if (val !== "" && fromPeriod !== "" && val < fromPeriod) {
      setFromPeriod(val);
    }
  };

  const handleApplyFilters = () => {
    fetchAnalytics(fromPeriod, toPeriod);
  };

  const handleClearFilters = () => {
    setFromPeriod("");
    setToPeriod("");
    fetchAnalytics("", "");
  };

  // ============================================================================
  // ЛОГИКА ЭКСПОРТА EXCEL & PDF
  // ============================================================================

  const handleExportFullExcel = () => {
    if (!data) return;
    const workbook = XLSX.utils.book_new();

    const tsData = data.time_series.map((item) => ({
      Период: item.period,
      "Всего студентов": item.total_students,
      "Активные студенты": item.active_students,
      "Средний балл": item.average_score,
    }));
    XLSX.utils.book_append_sheet(
      workbook,
      XLSX.utils.json_to_sheet(tsData),
      "Временные ряды",
    );

    const insightsData = [
      ...data.trends.map((t) => ({ Тип: "Тренд", Описание: t })),
      ...data.anomalies.map((a) => ({ Тип: "Аномалия", Описание: a })),
    ];
    XLSX.utils.book_append_sheet(
      workbook,
      XLSX.utils.json_to_sheet(insightsData),
      "AI Инсайты",
    );

    XLSX.writeFile(
      workbook,
      `Analytics_${fromPeriod || "all"}_${toPeriod || "all"}.xlsx`,
    );
  };

  const handleExportWidgetExcel = (
    type: "time_series" | "trends" | "anomalies",
    filename: string,
  ) => {
    if (!data) return;
    const workbook = XLSX.utils.book_new();
    let exportData: any[] = [];

    if (type === "time_series") {
      exportData = data.time_series.map((item) => ({
        Период: item.period,
        "Всего студентов": item.total_students,
        "Активные студенты": item.active_students,
        "Средний балл": item.average_score,
      }));
    } else if (type === "trends") {
      exportData = data.trends.map((t) => ({ "Выявленные тренды": t }));
    } else if (type === "anomalies") {
      exportData = data.anomalies.map((a) => ({ "Выявленные аномалии": a }));
    }

    const worksheet = XLSX.utils.json_to_sheet(exportData);
    XLSX.utils.book_append_sheet(workbook, worksheet, "Данные");
    XLSX.writeFile(
      workbook,
      `${filename}_${new Date().toISOString().split("T")[0]}.xlsx`,
    );
  };

  const handleExportPDF = async (elementId: string, filename: string) => {
    const element = document.getElementById(elementId);
    if (!element) return;

    const opt = {
      margin: 0.3,
      filename: `${filename}_${new Date().toISOString().split("T")[0]}.pdf`,
      image: { type: "jpeg", quality: 1 },
      html2canvas: {
        scale: 2,
        useCORS: true,
        logging: false,
        backgroundColor: "#111111",
        onclone: (clonedDoc: Document) => {
          const elements = clonedDoc.querySelectorAll("*");
          for (let i = 0; i < elements.length; i++) {
            const el = elements[i] as HTMLElement;
            const style = window.getComputedStyle(el);
            if (style.color && style.color.includes("oklch"))
              el.style.color = "#ffffff";
            if (
              style.backgroundColor &&
              style.backgroundColor.includes("oklch")
            )
              el.style.backgroundColor = "transparent";
            if (style.borderColor && style.borderColor.includes("oklch"))
              el.style.borderColor = "#333333";
          }
        },
      },
      jsPDF: { unit: "in", format: "a4", orientation: "landscape" },
    };

    try {
      await html2pdf().set(opt).from(element).save();
    } catch (err) {
      console.error("Ошибка генерации PDF:", err);
      alert("Не удалось сгенерировать PDF. Проверьте консоль.");
    }
  };

  const WidgetActions = ({
    targetId,
    excelType,
    filename,
  }: {
    targetId: string;
    excelType: any;
    filename: string;
  }) => (
    <div class="flex items-center space-x-2 opacity-0 group-hover:opacity-100 transition-opacity">
      <button
        onClick={() => handleExportWidgetExcel(excelType, filename)}
        title="Скачать данные (Excel)"
        class="p-1.5 text-gray-400 hover:text-green-400 hover:bg-green-400/10 rounded-md transition-colors"
      >
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
            d="M12 10v6m0 0l-3-3m3 3l3-3m2 8H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
          />
        </svg>
      </button>
      <button
        onClick={() => handleExportPDF(targetId, filename)}
        title="Скачать график (PDF)"
        class="p-1.5 text-gray-400 hover:text-red-400 hover:bg-red-400/10 rounded-md transition-colors"
      >
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
            d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"
          />
        </svg>
      </button>
    </div>
  );

  // ============================================================================
  // РЕНДЕР
  // ============================================================================

  if (isLoading) {
    return (
      <div class="h-full flex items-center justify-center">
        <div class="flex flex-col items-center text-gray-400">
          <Icons.Spinner class="w-8 h-8 animate-spin mb-4 text-accent" />
          <p class="text-sm font-medium animate-pulse">Сбор AI-аналитики...</p>
        </div>
      </div>
    );
  }

  if (!data) {
    return (
      <div class="h-full flex items-center justify-center text-gray-500">
        <p>Нет данных. Загрузите датасет и запустите синхронизацию метрик.</p>
      </div>
    );
  }

  return (
    <div class="container mx-auto px-6 py-8 overflow-y-auto h-full custom-scrollbar relative">
      {isRefetching && (
        <div class="absolute inset-0 z-50 flex items-center justify-center bg-background/50 backdrop-blur-sm rounded-xl">
          <Icons.Spinner class="w-10 h-10 animate-spin text-accent" />
        </div>
      )}

      {/* --- HEADER --- */}
      <header class="flex flex-col md:flex-row justify-between items-start md:items-center mb-6 gap-4">
        <div>
          <h1 class="text-3xl font-semibold tracking-tight text-white">
            Глубокая аналитика
          </h1>
          <p class="text-sm text-gray-400 mt-1">
            Отчеты, сгенерированные ИИ на основе исторических данных
          </p>
        </div>
        <div class="flex items-center space-x-3">
          <button
            onClick={handleExportFullExcel}
            class="flex items-center px-4 py-2 bg-green-600/10 hover:bg-green-600/20 text-green-500 border border-green-600/20 rounded-xl text-sm font-medium transition-colors"
          >
            <svg
              class="w-4 h-4 mr-2"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M12 10v6m0 0l-3-3m3 3l3-3m2 8H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
              />
            </svg>
            Скачать всё (Excel)
          </button>
          <button
            onClick={() => handleExportPDF("full-dashboard", "Full_Dashboard")}
            class="flex items-center px-4 py-2 bg-white text-black hover:bg-gray-200 rounded-xl text-sm font-medium transition-colors shadow-sm"
          >
            <svg
              class="w-4 h-4 mr-2"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M17 17h2a2 2 0 002-2v-4a2 2 0 00-2-2H5a2 2 0 00-2 2v4a2 2 0 002 2h2m2 4h6a2 2 0 002-2v-4a2 2 0 00-2-2H9a2 2 0 00-2 2v4a2 2 0 002 2zm8-12V5a2 2 0 00-2-2H9a2 2 0 00-2 2v4h10z"
              />
            </svg>
            Отчет (PDF)
          </button>
        </div>
      </header>

      {/* --- БЛОК ФИЛЬТРОВ ПО ВРЕМЕНИ С ЗАЩИТОЙ --- */}
      <div class="flex flex-wrap items-center gap-4 mb-8 bg-surface p-4 rounded-xl border border-border">
        <div class="flex items-center space-x-2">
          <label class="text-sm text-gray-400">С периода:</label>
          <div class="relative">
            <select
              value={fromPeriod}
              onChange={handleFromChange}
              disabled={isRefetching}
              class="bg-black/20 border border-border text-white text-sm rounded-lg pl-3 pr-8 py-1.5 appearance-none focus:outline-none focus:border-accent min-w-[110px] disabled:opacity-50 cursor-pointer"
            >
              <option value="">Все время</option>
              {availablePeriods.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
            <div class="absolute inset-y-0 right-0 flex items-center pr-2 pointer-events-none text-gray-400">
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
                  d="M19 9l-7 7-7-7"
                />
              </svg>
            </div>
          </div>
        </div>

        <div class="flex items-center space-x-2">
          <label class="text-sm text-gray-400">По период:</label>
          <div class="relative">
            <select
              value={toPeriod}
              onChange={handleToChange}
              disabled={isRefetching}
              class="bg-black/20 border border-border text-white text-sm rounded-lg pl-3 pr-8 py-1.5 appearance-none focus:outline-none focus:border-accent min-w-[110px] disabled:opacity-50 cursor-pointer"
            >
              <option value="">Все время</option>
              {availablePeriods.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
            <div class="absolute inset-y-0 right-0 flex items-center pr-2 pointer-events-none text-gray-400">
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
                  d="M19 9l-7 7-7-7"
                />
              </svg>
            </div>
          </div>
        </div>

        <button
          onClick={handleApplyFilters}
          disabled={isRefetching}
          class="px-4 py-1.5 bg-accent/10 text-accent hover:bg-accent/20 border border-accent/20 rounded-lg text-sm font-medium transition-colors disabled:opacity-50"
        >
          Применить
        </button>

        {(fromPeriod || toPeriod) && (
          <button
            onClick={handleClearFilters}
            disabled={isRefetching}
            class="px-4 py-1.5 text-gray-400 hover:text-white rounded-lg text-sm font-medium transition-colors disabled:opacity-50"
          >
            Сбросить (За все время)
          </button>
        )}

        <div class="ml-auto text-sm text-gray-400">
          Студентов в выборке:{" "}
          <span class="text-white font-mono">
            {data.metrics.total_students}
          </span>
        </div>
      </div>

      {/* --- CONTENT AREA --- */}
      <div
        id="full-dashboard"
        class="space-y-6 pb-10 bg-background rounded-xl p-1"
      >
        {/* INIGHTS GRID */}
        <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
          <section
            id="widget-trends"
            class="bg-surface border border-border p-6 rounded-2xl relative overflow-hidden group"
          >
            <div class="absolute top-0 left-0 w-1 h-full bg-blue-500 rounded-l-2xl" />
            <div class="flex justify-between items-center mb-4">
              <h3 class="text-blue-400 font-semibold flex items-center text-lg">
                📈 Ключевые тренды
              </h3>
              <WidgetActions
                targetId="widget-trends"
                excelType="trends"
                filename="Trends"
              />
            </div>
            <ul class="space-y-3">
              {data.trends?.length > 0 ? (
                data.trends.map((t, i) => (
                  <li key={i} class="text-sm text-gray-300 flex items-start">
                    <span class="text-blue-500 mr-2 mt-0.5">•</span>
                    {t}
                  </li>
                ))
              ) : (
                <li class="text-sm text-gray-500">Тренды не обнаружены</li>
              )}
            </ul>
          </section>

          <section
            id="widget-anomalies"
            class="bg-surface border border-border p-6 rounded-2xl relative overflow-hidden group"
          >
            <div class="absolute top-0 left-0 w-1 h-full bg-red-500 rounded-l-2xl" />
            <div class="flex justify-between items-center mb-4">
              <h3 class="text-red-400 font-semibold flex items-center text-lg">
                ⚠️ Выявленные аномалии
              </h3>
              <WidgetActions
                targetId="widget-anomalies"
                excelType="anomalies"
                filename="Anomalies"
              />
            </div>
            <ul class="space-y-3">
              {data.anomalies?.length > 0 ? (
                data.anomalies.map((a, i) => (
                  <li key={i} class="text-sm text-gray-300 flex items-start">
                    <span class="text-red-500 mr-2 mt-0.5">•</span>
                    {a}
                  </li>
                ))
              ) : (
                <li class="text-sm text-gray-500">Аномалий не обнаружено</li>
              )}
            </ul>
          </section>
        </div>

        {/* CHARTS GRID */}
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Line Chart */}
          <div
            id="widget-line-chart"
            class="bg-surface border border-border p-6 rounded-2xl group"
          >
            <div class="flex justify-between items-center mb-6">
              <h3 class="font-medium text-gray-200 tracking-tight">
                Динамика среднего балла
              </h3>
              <WidgetActions
                targetId="widget-line-chart"
                excelType="time_series"
                filename="AvgScore_Chart"
              />
            </div>
            {data.time_series?.length === 0 ? (
              <div class="h-[250px] flex items-center justify-center text-gray-500 text-sm">
                Нет данных за этот период
              </div>
            ) : (
              <div class="h-[250px] w-full bg-surface">
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart
                    data={data.time_series}
                    margin={{ top: 5, right: 20, left: 0, bottom: 5 }}
                  >
                    <CartesianGrid
                      strokeDasharray="3 3"
                      stroke="#2a2a2a"
                      vertical={false}
                    />
                    <XAxis
                      dataKey="period"
                      stroke="#666"
                      fontSize={12}
                      tickLine={false}
                      axisLine={false}
                    />
                    <YAxis
                      domain={["auto", "auto"]}
                      stroke="#666"
                      fontSize={12}
                      tickLine={false}
                      axisLine={false}
                    />
                    <Tooltip
                      contentStyle={{
                        backgroundColor: "#1a1a1a",
                        border: "1px solid #333",
                        borderRadius: "8px",
                      }}
                      itemStyle={{ color: "#3b82f6" }}
                    />
                    <Line
                      isAnimationActive={false}
                      type="monotone"
                      dataKey="average_score"
                      name="Средний балл"
                      stroke="#3b82f6"
                      strokeWidth={3}
                      dot={{ r: 4, fill: "#1a1a1a", strokeWidth: 2 }}
                      activeDot={{ r: 6 }}
                    />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            )}
          </div>

          {/* Bar Chart */}
          <div
            id="widget-bar-chart"
            class="bg-surface border border-border p-6 rounded-2xl group"
          >
            <div class="flex justify-between items-center mb-6">
              <h3 class="font-medium text-gray-200 tracking-tight">
                Изменение контингента (Активные)
              </h3>
              <WidgetActions
                targetId="widget-bar-chart"
                excelType="time_series"
                filename="Students_Chart"
              />
            </div>
            {data.time_series?.length === 0 ? (
              <div class="h-[250px] flex items-center justify-center text-gray-500 text-sm">
                Нет данных за этот период
              </div>
            ) : (
              <div class="h-[250px] w-full bg-surface">
                <ResponsiveContainer width="100%" height="100%">
                  <BarChart
                    data={data.time_series}
                    margin={{ top: 5, right: 20, left: 0, bottom: 5 }}
                  >
                    <CartesianGrid
                      strokeDasharray="3 3"
                      stroke="#2a2a2a"
                      vertical={false}
                    />
                    <XAxis
                      dataKey="period"
                      stroke="#666"
                      fontSize={12}
                      tickLine={false}
                      axisLine={false}
                    />
                    <YAxis
                      stroke="#666"
                      fontSize={12}
                      tickLine={false}
                      axisLine={false}
                    />
                    <Tooltip
                      contentStyle={{
                        backgroundColor: "#1a1a1a",
                        border: "1px solid #333",
                        borderRadius: "8px",
                      }}
                      itemStyle={{ color: "#10b981" }}
                      cursor={{ fill: "#2a2a2a" }}
                    />
                    <Bar
                      isAnimationActive={false}
                      dataKey="active_students"
                      name="Активных студентов"
                      fill="#10b981"
                      radius={[4, 4, 0, 0]}
                      maxBarSize={50}
                    />
                  </BarChart>
                </ResponsiveContainer>
              </div>
            )}
          </div>
        </div>

        {/* DATA TABLE */}
        <div
          id="widget-table"
          class="bg-surface border border-border rounded-2xl overflow-hidden group"
        >
          <div class="px-6 py-4 border-b border-border bg-black/20 flex justify-between items-center">
            <h3 class="font-medium text-gray-200 tracking-tight">
              Сводная таблица по периодам
            </h3>
            <WidgetActions
              targetId="widget-table"
              excelType="time_series"
              filename="Data_Table"
            />
          </div>
          <div class="overflow-x-auto bg-surface">
            <table class="w-full text-left text-sm">
              <thead class="bg-surface text-gray-400 font-medium">
                <tr>
                  <th class="px-6 py-4 font-normal">Период</th>
                  <th class="px-6 py-4 font-normal">Всего студентов</th>
                  <th class="px-6 py-4 font-normal">Активные</th>
                  <th class="px-6 py-4 font-normal">Средний балл</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-border">
                {data.time_series?.length === 0 && (
                  <tr>
                    <td colSpan={4} class="px-6 py-4 text-center text-gray-500">
                      Нет данных
                    </td>
                  </tr>
                )}
                {data.time_series?.map((row, i) => (
                  <tr key={i} class="hover:bg-white/[0.02] transition-colors">
                    <td class="px-6 py-4 font-mono text-gray-300">
                      {row.period}
                    </td>
                    <td class="px-6 py-4 text-gray-300 font-medium">
                      {row.total_students}
                    </td>
                    <td class="px-6 py-4 text-emerald-400 font-medium">
                      {row.active_students}
                    </td>
                    <td class="px-6 py-4 text-blue-400 font-medium">
                      {row.average_score?.toFixed(2) || "0.00"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  );
}
