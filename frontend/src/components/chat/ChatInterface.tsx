import { useState, useEffect, useRef } from "preact/hooks";
import { marked } from "marked";
import { apiFetch } from "../../api/client";
import { Icons } from "../ui/Icons";
import { ChartRenderer } from "../ui/ChartRenderer";
import type { ChatMessage } from "../../types";

interface ChatInterfaceProps {
  datasetId: string;
}

type UiMessage = ChatMessage & {
  _id: string;
};

function makeId() {
  try {
    if (typeof crypto !== "undefined" && crypto.randomUUID) {
      return crypto.randomUUID();
    }
  } catch {
    // ignore
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

function parseChartsFromCodeOutput(codeOutput: unknown): any[] {
  if (!codeOutput) return [];

  try {
    const parsed =
      typeof codeOutput === "string" ? JSON.parse(codeOutput) : codeOutput;

    if (
      parsed &&
      typeof parsed === "object" &&
      Array.isArray((parsed as any).charts)
    ) {
      return (parsed as any).charts;
    }
  } catch (e) {
    console.error("Failed to parse code_output:", e, codeOutput);
  }

  return [];
}

export function ChatInterface({ datasetId }: ChatInterfaceProps) {
  const [messages, setMessages] = useState<UiMessage[]>([]);
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(false);

  // Настройки AI Агента
  const [useCode, setUseCode] = useState(true);
  const [useNews, setUseNews] = useState(false);

  const bottomRef = useRef<HTMLDivElement>(null);

  // Загрузка истории чата при открытии
  useEffect(() => {
    let cancelled = false;

    const loadHistory = async () => {
      try {
        const res = await apiFetch(`/datasets/${datasetId}/chat`);
        if (!res.ok) return;

        const history = await res.json();

        if (cancelled) return;

        if (Array.isArray(history) && history.length > 0) {
          const prepared: UiMessage[] = history.map((m: ChatMessage) => {
            // 1) Пытаемся достать графики из code_output для сообщений из истории
            let charts = parseChartsFromCodeOutput(m.code_output);

            // 2) Fallback: если бэкенд отдал их как массив напрямую
            if (!charts.length && Array.isArray((m as any).charts)) {
              charts = (m as any).charts;
            }

            return {
              ...m,
              _id: makeId(),
              charts, // <-- Добавляем распарсенные графики сюда
            };
          });

          setMessages(prepared);
        } else {
          setMessages([
            {
              _id: makeId(),
              role: "system",
              content:
                "Готов к анализу данных. Задайте вопрос или попросите построить график.",
              code_output: "",
              source_code: "",
            },
          ]);
        }
      } catch (err) {
        console.error("Failed to load chat history", err);
      }
    };

    loadHistory();

    return () => {
      cancelled = true;
    };
  }, [datasetId]);

  // Скролл вниз при новых сообщениях
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, loading]);

  const sendMessage = async (e: Event) => {
    e.preventDefault();

    const userText = input.trim();
    if (!userText || loading) return;

    // Сразу показываем сообщение пользователя в UI
    setMessages((prev) => [
      ...prev,
      { _id: makeId(), role: "user", content: userText, source_code: "" },
    ]);

    setInput("");
    setLoading(true);

    try {
      const res = await apiFetch(`/datasets/${datasetId}/chat`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          message: userText,
          use_code: useCode,
          use_news: useNews,
        }),
      });

      const data = await res.json();

      // 1) основной вариант: charts внутри code_output
      let charts = parseChartsFromCodeOutput(data?.code_output);

      // 2) fallback: если backend отдаёт charts напрямую
      if (!charts.length && Array.isArray(data?.charts)) {
        charts = data.charts;
      }

      setMessages((prev) => [
        ...prev,
        {
          _id: makeId(),
          role: "bot",
          content: data?.reply || data?.error || "Пустой ответ от сервера",
          charts,
          source_code: data?.source_code || "",
          isError: !!data?.error || !res.ok,
        },
      ]);
    } catch (err: any) {
      setMessages((prev) => [
        ...prev,
        {
          _id: makeId(),
          role: "bot",
          content: `Ошибка сети: ${err?.message || "unknown error"}`,
          source_code: "",
          isError: true,
        },
      ]);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div class="flex flex-col h-full bg-background relative">
      {/* --- ОБЛАСТЬ СООБЩЕНИЙ --- */}
      <div class="flex-1 overflow-y-auto p-6 space-y-8 custom-scrollbar pb-40">
        {messages.map((msg) => (
          <div
            key={msg._id}
            class={`flex ${msg.role === "user" ? "justify-end" : "justify-start"}`}
          >
            <div
              class={`max-w-[85%] ${
                msg.role === "user"
                  ? "bg-surface border border-border rounded-2xl px-5 py-4 shadow-sm"
                  : ""
              }`}
            >
              {msg.role !== "user" && (
                <div class="flex items-center space-x-2 mb-3">
                  <div class="w-6 h-6 rounded bg-white text-black flex items-center justify-center text-[10px] font-bold tracking-tighter shadow-sm">
                    {msg.role === "system" ? "SYS" : "AI"}
                  </div>
                  <span class="text-xs text-gray-500 font-medium">
                    {msg.role === "system" ? "Система" : "Аналитик"}
                  </span>
                </div>
              )}

              <div
                class={`prose prose-invert prose-sm max-w-none ${msg.isError ? "text-red-400" : "text-gray-200"}`}
                dangerouslySetInnerHTML={{
                  __html: marked.parse(msg.content || "") as string,
                }}
              />

              {/* Рендер графиков */}
              {Array.isArray(msg.charts) &&
                msg.charts.map((c: any, idx: number) => {
                  const chartKey = `${msg._id}-${c?.title ?? "chart"}-${c?.type ?? "bar"}-${idx}`;
                  return <ChartRenderer key={chartKey} chartData={c} />;
                })}

              {/* Рендер исходного кода (Python) */}
              {msg.source_code && (
                <details class="mt-4 group border border-border rounded-lg bg-black overflow-hidden">
                  <summary class="text-xs text-gray-500 cursor-pointer p-3 bg-surface hover:text-white transition flex items-center space-x-2 select-none">
                    <Icons.Code class="w-4 h-4" />
                    <span class="font-mono">source.py</span>
                  </summary>
                  <pre class="text-[11px] p-4 text-gray-300 font-mono overflow-x-auto m-0 leading-relaxed border-t border-border">
                    {msg.source_code}
                  </pre>
                </details>
              )}
            </div>
          </div>
        ))}

        {/* Индикатор загрузки */}
        {loading && (
          <div class="flex justify-start items-center space-x-3 text-gray-500">
            <div class="w-6 h-6 rounded border border-border flex items-center justify-center bg-surface animate-pulse">
              <Icons.Spinner class="w-4 h-4 animate-spin" />
            </div>
            <span class="text-sm font-medium animate-pulse">
              Анализ данных...
            </span>
          </div>
        )}

        <div ref={bottomRef}></div>
      </div>

      {/* --- ЗОНА ВВОДА --- */}
      <div class="absolute bottom-0 left-0 right-0 p-6 bg-gradient-to-t from-background via-background/95 to-transparent">
        <form
          onSubmit={sendMessage}
          class="max-w-4xl mx-auto relative group flex flex-col"
        >
          {/* НАСТРОЙКИ */}
          <div class="flex items-center space-x-4 mb-3 ml-2">
            <label class="flex items-center space-x-2 cursor-pointer group/label">
              <input
                type="checkbox"
                checked={useCode}
                onChange={(e) =>
                  setUseCode((e.target as HTMLInputElement).checked)
                }
                class="rounded border-border bg-surface text-blue-500 focus:ring-blue-500/50 w-3.5 h-3.5 transition"
              />
              <span class="text-xs text-gray-400 group-hover/label:text-gray-200 transition font-medium">
                Python Agent (Код)
              </span>
            </label>

            <label class="flex items-center space-x-2 cursor-pointer group/label">
              <input
                type="checkbox"
                checked={useNews}
                onChange={(e) =>
                  setUseNews((e.target as HTMLInputElement).checked)
                }
                class="rounded border-border bg-surface text-green-500 focus:ring-green-500/50 w-3.5 h-3.5 transition"
              />
              <span class="text-xs text-gray-400 group-hover/label:text-green-400 transition font-medium flex items-center">
                Учитывать новости и законы
              </span>
            </label>
          </div>

          <div class="relative">
            <input
              value={input}
              onInput={(e) => setInput((e.target as HTMLInputElement).value)}
              placeholder="Спроси что-нибудь о данных..."
              class="w-full bg-surface border border-border text-white rounded-2xl pl-5 pr-14 py-4 focus:outline-none focus:border-gray-500 transition shadow-lg text-sm"
              disabled={loading}
            />
            <button
              type="submit"
              disabled={loading || !input.trim()}
              class="absolute right-2 top-2 bottom-2 aspect-square flex items-center justify-center bg-white text-black rounded-xl hover:bg-gray-200 transition disabled:opacity-50 disabled:bg-surface disabled:text-gray-500"
            >
              <Icons.Send class="w-4 h-4" />
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
