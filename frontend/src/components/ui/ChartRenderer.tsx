import { useEffect, useRef } from "preact/hooks";
import Chart from "chart.js/auto";

interface ChartRendererProps {
  chartData: any;
}

const CHART_COLORS = [
  "#3291ff",
  "#f5a623",
  "#50e3c2",
  "#f81ce5",
  "#7928ca",
  "#ff4a92",
];

function normalizeType(type?: string) {
  const raw = String(type || "")
    .trim()
    .toLowerCase();

  const aliases: Record<string, string> = {
    histogram: "bar",
    ascii_bar: "bar",
    ascii: "bar",
    column: "bar",
    columns: "bar",
    area: "line",
    polararea: "polarArea",
  };

  const mapped = aliases[raw] || raw || "bar";

  const allowed = new Set([
    "bar",
    "line",
    "pie",
    "doughnut",
    "polarArea",
    "radar",
    "scatter",
    "bubble",
  ]);

  if (!allowed.has(mapped)) {
    console.warn(`[ChartRenderer] Unknown type "${type}", fallback to "bar"`);
    return "bar";
  }

  return mapped;
}

function toStrArr(arr: any[]): string[] {
  return arr.map((v) => String(v ?? ""));
}

function toNumArr(arr: any[]): number[] {
  return arr.map((v) => {
    const n = Number(v);
    return Number.isFinite(n) ? n : 0;
  });
}

function extractSeries(
  chartData: any,
): { labels: string[]; values: number[] } | null {
  // 1) Chart.js-like format
  if (
    chartData?.data &&
    !Array.isArray(chartData.data) &&
    Array.isArray(chartData.data.labels)
  ) {
    const labels = toStrArr(chartData.data.labels);
    const values = toNumArr(chartData.data.datasets?.[0]?.data || []);
    return { labels, values };
  }

  // 2) ТВОЙ НОВЫЙ ФОРМАТ: x / y
  if (Array.isArray(chartData?.x) && Array.isArray(chartData?.y)) {
    const labels = toStrArr(chartData.x);
    const values = toNumArr(chartData.y);
    return { labels, values };
  }

  // 3) x_axis / y_axis
  if (Array.isArray(chartData?.x_axis) && Array.isArray(chartData?.y_axis)) {
    const labels = toStrArr(chartData.x_axis);
    const values = toNumArr(chartData.y_axis);
    return { labels, values };
  }

  // 4) labels / values
  if (Array.isArray(chartData?.labels) && Array.isArray(chartData?.values)) {
    const labels = toStrArr(chartData.labels);
    const values = toNumArr(chartData.values);
    return { labels, values };
  }

  // 5) data: [{label,value}] | [{x,y}] | etc
  if (Array.isArray(chartData?.data) && chartData.data.length > 0) {
    const first = chartData.data[0];
    const keys = Object.keys(first || {});

    if (!keys.length) return null;

    const lKey =
      keys.find((k) =>
        ["name", "label", "x", "date", "category", "bucket"].includes(
          k.toLowerCase(),
        ),
      ) || keys[0];

    const vKey =
      keys.find((k) =>
        ["value", "y", "count", "total", "amount", "n"].includes(
          k.toLowerCase(),
        ),
      ) ||
      keys[1] ||
      keys[0];

    const labels = toStrArr(chartData.data.map((d: any) => d?.[lKey]));
    const values = toNumArr(chartData.data.map((d: any) => d?.[vKey]));
    return { labels, values };
  }

  return null;
}

export function ChartRenderer({ chartData }: ChartRendererProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const chartRef = useRef<Chart | null>(null);

  useEffect(() => {
    if (!canvasRef.current || !chartData) return;

    const parsed = extractSeries(chartData);
    if (!parsed) {
      console.warn("[ChartRenderer] Unsupported chart payload:", chartData);
      return;
    }

    let { labels, values } = parsed;

    // синхронизация длин
    const n = Math.min(labels.length, values.length);
    labels = labels.slice(0, n);
    values = values.slice(0, n);

    if (!n) {
      console.warn("[ChartRenderer] Empty chart after parsing:", chartData);
      return;
    }

    if (chartRef.current) {
      chartRef.current.destroy();
      chartRef.current = null;
    }

    const ctx = canvasRef.current.getContext("2d");
    if (!ctx) return;

    const type = normalizeType(chartData.type);
    const isPieOrDoughnut = type === "pie" || type === "doughnut";
    const isBar = type === "bar";
    const isLine = type === "line";

    const gradient = ctx.createLinearGradient(0, 0, 0, 400);
    gradient.addColorStop(0, "rgba(50, 145, 255, 0.35)");
    gradient.addColorStop(1, "rgba(50, 145, 255, 0)");

    const bgColors = values.map(
      (_, i) => CHART_COLORS[i % CHART_COLORS.length],
    );

    try {
      chartRef.current = new Chart(ctx, {
        type: type as any,
        data: {
          labels,
          datasets: [
            {
              label: chartData.title || "Metrics",
              data: values,
              backgroundColor:
                isPieOrDoughnut || isBar
                  ? bgColors
                  : isLine
                    ? gradient
                    : "#3291ff",
              borderColor: isPieOrDoughnut
                ? "#000000"
                : isBar
                  ? bgColors
                  : "#3291ff",
              borderWidth: isPieOrDoughnut ? 2 : 1,
              fill: isLine,
              tension: isLine ? 0.25 : 0,
              borderRadius: isBar ? 4 : 0,
            },
          ],
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          animation: false,
          plugins: {
            legend: {
              display: isPieOrDoughnut,
              labels: { color: "#ededed" },
            },
            title: {
              display: !!chartData.title,
              text: chartData.title,
              color: "#ededed",
              font: {
                family: "Inter",
                size: 14,
                weight: 500,
              },
            },
          },
          scales: isPieOrDoughnut
            ? {}
            : {
                y: {
                  grid: { color: "#222" },
                  border: { display: false },
                  ticks: { color: "#666" },
                },
                x: {
                  grid: { display: false },
                  border: { display: false },
                  ticks: { color: "#666" },
                },
              },
        },
      });
    } catch (e) {
      console.error(
        "[ChartRenderer] Chart render error:",
        chartData?.title,
        "type:",
        chartData?.type,
        e,
        chartData,
      );
    }

    return () => {
      if (chartRef.current) {
        chartRef.current.destroy();
        chartRef.current = null;
      }
    };
  }, [chartData]);

  return (
    <div class="p-4 bg-black border border-border rounded-xl mt-4 h-72 shadow-sm">
      <canvas ref={canvasRef}></canvas>
    </div>
  );
}
