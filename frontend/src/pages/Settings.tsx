import { useState, useEffect } from "preact/hooks";
import { apiFetch } from "../api/client";
import { Icons } from "../components/ui/Icons";

// --- Вспомогательные Иконки Сервисов (SVG) ---
const ServiceIcons = {
  Yandex: () => (
    <svg
      width="24"
      height="24"
      viewBox="0 0 42 42"
      class="w-6 h-6 text-red-500"
      fill="currentColor"
    >
      <path d="m 36.638,41.871 c -2.89,0 -5.234,-2.343 -5.234,-5.234 0,-2.89 2.343,-5.234 5.234,-5.234 2.89,0 5.234,2.343 5.234,5.234 0,2.89 -2.343,5.234 -5.234,5.234 z" />
      <path d="M 0,15.701 C 0,7.029 7.029,0 15.701,0 c 8.671,0 15.701,7.029 15.701,15.701 0,8.672 -7.03,15.701 -15.701,15.701 -8.671,0 -15.701,-7.029 -15.701,-15.701 z" />
    </svg>
  ),
  Sber: () => (
    <svg
      width="24"
      height="24"
      viewBox="0 0 1000 1000"
      class="w-6 h-6 text-green-500"
      fill="currentColor"
    >
      <path d="m 466,999.3 c -1.925,-0.215 -8.675,-0.866 -15,-1.448 C 372.5,990.7 292.5,962.4 225.6,918.2 108.8,841.1 29.1,718.5 7.04,581.9 -0.9,532.8 -1.4,478.5 5.6,428.5 26.8,277.3 119.6,141.3 253.5,65.4 316.7,29.5 385.1,8.5 460.5,1.9 c 28,-2.4 73.8,-1.3 105,2.6 83.7,10.6 165.8,44 234,94.9 86.1,64.3 149.2,154.4 180,257.1 8.2,27.3 14.1,56.6 18.1,88.7 1.4,12.2 1.8,22.3 1.8,55 -0.01,42.7 -0.6,51.5 -6,83.8 -28,110.1 -193.6,275.6 -402.5,310.4 -29.8,4.9 -40.4,5.7 -78.5,6 -19.8,0.1 -37.5,0.1 -39.5,-0.1 z m 86.5,-89.2 c 35.2,-4.8 65.7,-12.6 96.6,-24.6 54.6,-21.2 101.2,-51.7 142.9,-93.3 43.3,-43.3 74.8,-92.4 95.9,-149.6 4.8,-13 12.9,-39.8 12.9,-42.5 -0.008,-0.7 -2.4,3.2 -5.5,8.9 -29.3,55.4 -76.4,109.8 -130.4,150.6 -63.7,48.1 -139.7,80.1 -219.5,92.2 -29.9,4.5 -38.6,5.1 -77.5,5.0 -31.9,-0.04 -39.4,-0.3 -55.4,-2.2 -45.1,-5.4 -88.4,-16.4 -127.7,-32.2 -27.2,-10.9 -65.1,-30.9 -88.7,-46.8 -6.9,-4.7 -7.6,-5 -5.8,-2.4 1.1,1.6 8.5,9.4 16.4,17.4 68.6,68.9 155.7,110.3 253.1,120.4 20.1,2 73.5,1.2 93.1,-1.3 z M 354.6,757.9 C 495.5,746.8 640.5,675 732.2,571 c 53,-60.2 85.4,-132.9 88.4,-198.2 0.3,-7.5 0.9,-13.7 1.4,-13.7 2.4,0 25.2,29.4 36.1,46.7 8.1,12.8 24.1,45 29,58.2 8,21.8 14.4,46.6 17.2,66.8 2.4,17.6 2.3,17.3 4.2,16.9 2.1,-0.4 2.7,-5.1 4,-30.7 1.4,-29.1 -0.5,-62.3 -5.4,-90 -3.3,-18.7 -10.1,-46.1 -13,-52.5 -0.7,-1.6 -3.4,-8.8 -6,-16 -24.8,-68.8 -49.9,-112.2 -81.1,-149.2 -16.9,-20.1 -42.1,-44.1 -56.4,-53.9 -19,-13 -45.3,-22.6 -74.5,-27.2 -15.6,-2.4 -53.9,-2.4 -72.4,-0.01 -79.8,10.6 -158.3,44 -239,101.6 -7.1,5.1 -13.2,9.7 -13.6,10.2 -0.3,0.5 5.7,8.2 13.5,17 22.3,25.3 33.7,39 58.1,69.6 12.4,15.6 23.2,28.8 23.9,29.3 0.9,0.6 5.7,-2.2 16.1,-9.5 35.8,-25.2 79.3,-53.2 112.3,-72 34.1,-19.4 77.7,-41.4 80.3,-40.4 5.2,2 6.1,24.8 1.4,38.2 -14.2,40.4 -48.7,80.6 -102.4,123.5 -71.4,56.9 -130.2,89.4 -186.6,103.1 -66.9,16.2 -116.7,5.7 -153.3,-32.4 -18.6,-19.3 -31,-42.2 -36.2,-66.9 -2.4,-11.2 -2.4,-37.6 -0.02,-50.5 7.3,-39.5 25.7,-77 57.2,-116.5 62,-77.7 152.9,-124.1 264.9,-135.2 16.3,-1.6 86.5,-1.5 109.1,0.1 24.2,1.7 32.4,3.7 47.8,11.4 20.9,10.4 25.5,12 25.5,8.6 0,-2.5 -45,-21.6 -67,-28.4 C 570.6,89.3 515.2,83.7 458.1,89.5 339.4,101.8 231.2,165.4 161.1,264.2 109.5,336.9 83.6,425 87.9,513 c 1.1,23.3 2.4,38.2 4.6,52 6.8,43.7 23.4,84.8 46.5,115.2 35.1,46.2 86.8,73.1 149.8,78 10.5,0.8 53.7,0.6 65.6,-0.3 z" />
    </svg>
  ),
  OpenRouter: () => (
    <svg
      class="w-6 h-6 text-indigo-400"
      fill="currentColor"
      viewBox="0 0 24 24"
    >
      <path d="M16.804 1.957l7.22 4.105v.087L16.73 10.21l.017-2.117-.821-.03c-1.059-.028-1.611.002-2.268.11-1.064.175-2.038.577-3.147 1.352L8.345 11.03c-.284.195-.495.336-.68.455l-.515.322-.397.234.385.23.53.338c.476.314 1.17.796 2.701 1.866 1.11.775 2.083 1.177 3.147 1.352l.3.045c.694.091 1.375.094 2.825.033l.022-2.159 7.22 4.105v.087L16.589 22l.014-1.862-.635.022c-1.386.042-2.137.002-3.138-.162-1.694-.28-3.26-.926-4.881-2.059l-2.158-1.5a21.997 21.997 0 00-.755-.498l-.467-.28a55.927 55.927 0 00-.76-.43C2.908 14.73.563 14.116 0 14.116V9.888l.14.004c.564-.007 2.91-.622 3.809-1.124l1.016-.58.438-.274c.428-.28 1.072-.726 2.686-1.853 1.621-1.133 3.186-1.78 4.881-2.059 1.152-.19 1.974-.213 3.814-.138l.02-1.907z" />
    </svg>
  ),
  Ollama: () => (
    <svg
      viewBox="0 0 24 24"
      class="w-6 h-6 text-gray-300"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
    >
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
      />
    </svg>
  ),
  Moodle: () => (
    <svg
      viewBox="0 0 24 24"
      class="w-6 h-6 text-orange-500"
      fill="currentColor"
    >
      <path d="M12 3L1 9l4 2.18v6L12 21l7-3.82v-6l2-1.09V17h2V9L12 3zm6.82 6L12 12.72 5.18 9 12 5.28 18.82 9zM17 15.99l-5 2.73-5-2.73v-3.72l5 2.73 5-2.73v3.72z" />
    </svg>
  ),
  Database: () => (
    <svg
      viewBox="0 0 24 24"
      class="w-6 h-6 text-blue-400"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
    >
      <path d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4m0 5c0 2.21-3.582 4-8 4s-8-1.79-8-4" />
    </svg>
  ),
  Mail: () => (
    <svg
      viewBox="0 0 24 24"
      class="w-6 h-6 text-purple-400"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
    >
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"
      />
    </svg>
  ),
  RSS: () => (
    <svg viewBox="0 0 24 24" class="w-6 h-6 text-red-400" fill="currentColor">
      <path d="M6.18,15.64A2.18,2.18,0,0,1,8.36,17.82c0,1.21-.97,2.18-2.18,2.18A2.18,2.18,0,0,1,4,17.82,2.18,2.18,0,0,1,6.18,15.64M4,4.44A15.56,15.56,0,0,1,19.56,20h-2.83A12.73,12.73,0,0,0,4,7.27Zm0,5.66a9.9,9.9,0,0,1,9.9,9.9H11.07A7.07,7.07,0,0,0,4,12.93Z" />
    </svg>
  ),
};

// --- Интерфейсы ---
interface RSSSource {
  name: string;
  url: string;
}

interface AppSettings {
  llm_provider: "openrouter" | "ollama" | "yandex" | "gigachat";
  openrouter_api_key: string;
  openrouter_model: string;
  ollama_url: string;
  ollama_model: string;
  yandex_folder_id: string;
  yandex_api_key: string;
  yandex_model: string;
  gigachat_client_id: string;
  gigachat_client_secret: string;

  enable_imap: boolean;
  imap_host: string;
  imap_port: number;
  imap_username: string;
  imap_password?: string;

  enable_moodle: boolean;
  moodle_url: string;
  moodle_token: string;

  enable_univ_db: boolean;
  univ_db_type: "postgres" | "mysql" | "sqlserver";
  univ_db_dsn: string;

  // Кроны
  news_cron: string;
  imap_cron: string;
  analytics_cron: string;
  moodle_cron: string;
  erp_cron: string;

  // RSS Источники (JSON строка)
  rss_sources: string;
}

export function Settings() {
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [isDirty, setIsDirty] = useState(false);
  const [errors, setErrors] = useState<string[]>([]);
  const [settings, setSettings] = useState<AppSettings | null>(null);

  useEffect(() => {
    if (localStorage.getItem("role") !== "admin")
      return (window.location.href = "/");
    apiFetch("/settings")
      .then((r) => r.json())
      .then(setSettings)
      .catch(() => alert("Ошибка загрузки"))
      .finally(() => setLoading(false));
  }, []);

  const handleChange = (e: any) => {
    const { name, value, type, checked } = e.target;
    setIsDirty(true);
    setErrors([]);
    setSettings((p) =>
      p
        ? {
            ...p,
            [name]:
              type === "checkbox"
                ? checked
                : type === "number"
                  ? Number(value)
                  : value,
          }
        : p,
    );
  };

  const setProvider = (name: string, val: string) => {
    setIsDirty(true);
    setErrors([]);
    setSettings((p) => (p ? { ...p, [name]: val } : p));
  };

  const updateRSS = (newList: RSSSource[]) => {
    setIsDirty(true);
    setErrors([]);
    setSettings((p) =>
      p ? { ...p, rss_sources: JSON.stringify(newList) } : p,
    );
  };

  const getRSSList = (): RSSSource[] => {
    try {
      return JSON.parse(settings?.rss_sources || "[]");
    } catch {
      return [];
    }
  };

  const validate = (): boolean => {
    if (!settings) return false;
    const errs: string[] = [];
    if (
      settings.enable_imap &&
      (!settings.imap_host || !settings.imap_username)
    )
      errs.push("Заполните хост и логин IMAP");
    if (
      settings.enable_moodle &&
      (!settings.moodle_url || !settings.moodle_token)
    )
      errs.push("Заполните URL и Токен Moodle");
    if (settings.enable_univ_db && !settings.univ_db_dsn)
      errs.push("Укажите DSN базы данных ВУЗа");
    if (settings.llm_provider === "openrouter" && !settings.openrouter_api_key)
      errs.push("Укажите API ключ OpenRouter");
    if (
      settings.llm_provider === "gigachat" &&
      (!settings.gigachat_client_id || !settings.gigachat_client_secret)
    )
      errs.push("Укажите Client ID и Secret для GigaChat");
    if (
      settings.llm_provider === "yandex" &&
      (!settings.yandex_api_key ||
        !settings.yandex_folder_id ||
        !settings.yandex_model)
    )
      errs.push("Укажите ключи YandexGPT");
    setErrors(errs);
    return errs.length === 0;
  };

  const saveSettings = async () => {
    if (!validate()) return window.scrollTo({ top: 0, behavior: "smooth" });
    setSaving(true);
    try {
      const res = await apiFetch("/settings", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(settings),
      });
      const data = await res.json();
      if (res.ok) {
        alert("✅ Успешно сохранено");
        setIsDirty(false);
      } else setErrors([data.error || "Ошибка сохранения"]);
    } catch (err) {
      setErrors(["Ошибка сети"]);
    } finally {
      setSaving(false);
    }
  };

  if (loading || !settings)
    return (
      <div class="h-full flex items-center justify-center">
        <Icons.Spinner class="w-8 h-8 animate-spin text-indigo-500" />
      </div>
    );

  return (
    <div class="container mx-auto px-6 py-8 h-full overflow-y-auto custom-scrollbar pb-32">
      <header class="mb-10 flex justify-between items-end border-b border-white/5 pb-6">
        <div>
          <h1 class="text-3xl font-bold tracking-tight text-white mb-2">
            Настройки системы
          </h1>
          <p class="text-sm text-gray-400">
            Управление AI, источниками данных и расписанием
          </p>
        </div>
        {isDirty && (
          <span class="bg-yellow-500/10 text-yellow-500 border border-yellow-500/20 px-3 py-1 rounded-full text-xs font-medium animate-pulse flex items-center">
            <span class="w-2 h-2 rounded-full bg-yellow-500 mr-2"></span>{" "}
            Несохраненные изменения
          </span>
        )}
      </header>

      {errors.length > 0 && (
        <div class="mb-8 bg-red-500/10 border border-red-500/30 text-red-400 p-5 rounded-2xl flex items-start shadow-lg shadow-red-500/5">
          <Icons.AlertCircle class="w-6 h-6 mr-3 flex-shrink-0 mt-0.5" />
          <div>
            <h3 class="font-semibold mb-1">Ошибка валидации</h3>
            <ul class="list-disc pl-5 text-sm space-y-1">
              {errors.map((e, i) => (
                <li key={i}>{e}</li>
              ))}
            </ul>
          </div>
        </div>
      )}

      <div class="grid grid-cols-1 xl:grid-cols-2 gap-8 max-w-7xl">
        {/* --- 1. AI ДВИЖОК --- */}
        <div class="bg-[#0A0A0B] border border-white/10 rounded-3xl p-7 relative overflow-hidden group">
          <div class="absolute inset-0 bg-gradient-to-br from-indigo-500/5 to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-500" />
          <h2 class="text-xl font-semibold text-white flex items-center mb-6 relative z-10">
            <span class="bg-white/10 p-2 rounded-xl mr-3">🧠</span> Нейросеть
            (LLM)
          </h2>

          <div class="grid grid-cols-2 gap-3 mb-6 relative z-10">
            {[
              {
                id: "openrouter",
                name: "OpenRouter",
                icon: <ServiceIcons.OpenRouter />,
                desc: "100+ моделей",
              },
              {
                id: "yandex",
                name: "YandexGPT",
                icon: <ServiceIcons.Yandex />,
                desc: "Облако Яндекс",
              },
              {
                id: "gigachat",
                name: "GigaChat",
                icon: <ServiceIcons.Sber />,
                desc: "API Сбера",
              },
              {
                id: "ollama",
                name: "Ollama",
                icon: <ServiceIcons.Ollama />,
                desc: "Локально",
              },
            ].map((prov) => (
              <button
                key={prov.id}
                onClick={() => setProvider("llm_provider", prov.id)}
                class={`flex flex-col items-start p-4 rounded-2xl border text-left transition-all duration-200
                  ${
                    settings.llm_provider === prov.id
                      ? "bg-indigo-500/10 border-indigo-500/50 ring-1 ring-indigo-500/50 shadow-lg shadow-indigo-500/10"
                      : "bg-black/40 border-white/5 hover:bg-white/5 hover:border-white/20"
                  }`}
              >
                <div class="flex items-center justify-between w-full mb-2">
                  {prov.icon}
                  <div
                    class={`w-4 h-4 rounded-full border-2 flex items-center justify-center ${settings.llm_provider === prov.id ? "border-indigo-500" : "border-gray-600"}`}
                  >
                    {settings.llm_provider === prov.id && (
                      <div class="w-2 h-2 rounded-full bg-indigo-500" />
                    )}
                  </div>
                </div>
                <h3 class="font-medium text-white text-sm">{prov.name}</h3>
                <p class="text-[10px] text-gray-500 mt-0.5">{prov.desc}</p>
              </button>
            ))}
          </div>

          <div class="space-y-4 bg-black/40 p-5 rounded-2xl border border-white/5 relative z-10">
            {settings.llm_provider === "openrouter" && (
              <div class="space-y-3 animate-in fade-in zoom-in-95 duration-200">
                <div>
                  <label class="block text-xs text-gray-400 mb-1 ml-1">
                    API Ключ
                  </label>
                  <input
                    type="password"
                    name="openrouter_api_key"
                    value={settings.openrouter_api_key}
                    onChange={handleChange}
                    placeholder="sk-or-v1-..."
                    class="w-full bg-[#111] border border-white/10 text-white rounded-xl px-4 py-2.5 text-sm focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500 outline-none transition-all"
                  />
                </div>
                <div>
                  <label class="block text-xs text-gray-400 mb-1 ml-1">
                    Модель
                  </label>
                  <input
                    type="text"
                    name="openrouter_model"
                    value={settings.openrouter_model}
                    onChange={handleChange}
                    placeholder="google/gemma-3-27b-it"
                    class="w-full bg-[#111] border border-white/10 text-white rounded-xl px-4 py-2.5 text-sm focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500 outline-none transition-all"
                  />
                </div>
              </div>
            )}
            {settings.llm_provider === "yandex" && (
              <div class="space-y-3 animate-in fade-in zoom-in-95 duration-200">
                <div>
                  <label class="block text-xs text-gray-400 mb-1 ml-1">
                    Folder ID
                  </label>
                  <input
                    type="text"
                    name="yandex_folder_id"
                    value={settings.yandex_folder_id}
                    onChange={handleChange}
                    class="w-full bg-[#111] border border-white/10 text-white rounded-xl px-4 py-2.5 text-sm focus:border-red-500 outline-none"
                  />
                </div>
                <div>
                  <label class="block text-xs text-gray-400 mb-1 ml-1">
                    API Key
                  </label>
                  <input
                    type="password"
                    name="yandex_api_key"
                    value={settings.yandex_api_key}
                    onChange={handleChange}
                    class="w-full bg-[#111] border border-white/10 text-white rounded-xl px-4 py-2.5 text-sm focus:border-red-500 outline-none"
                  />
                </div>
                <div>
                  <label class="block text-xs text-gray-400 mb-1 ml-1">
                    Model
                  </label>
                  <input
                    type="text"
                    name="yandex_model"
                    value={settings.yandex_model}
                    onChange={handleChange}
                    class="w-full bg-[#111] border border-white/10 text-white rounded-xl px-4 py-2.5 text-sm focus:border-red-500 outline-none"
                  />
                </div>
              </div>
            )}
            {settings.llm_provider === "gigachat" && (
              <div class="space-y-3 animate-in fade-in zoom-in-95 duration-200">
                <div>
                  <label class="block text-xs text-gray-400 mb-1 ml-1">
                    Client ID
                  </label>
                  <input
                    type="text"
                    name="gigachat_client_id"
                    value={settings.gigachat_client_id}
                    onChange={handleChange}
                    placeholder="6f7b..."
                    class="w-full bg-[#111] border border-white/10 text-white rounded-xl px-4 py-2.5 text-sm focus:border-green-500 outline-none"
                  />
                </div>
                <div>
                  <label class="block text-xs text-gray-400 mb-1 ml-1">
                    Client Secret
                  </label>
                  <input
                    type="password"
                    name="gigachat_client_secret"
                    value={settings.gigachat_client_secret}
                    onChange={handleChange}
                    placeholder="••••••••"
                    class="w-full bg-[#111] border border-white/10 text-white rounded-xl px-4 py-2.5 text-sm focus:border-green-500 outline-none"
                  />
                </div>
              </div>
            )}
            {settings.llm_provider === "ollama" && (
              <div class="space-y-3 animate-in fade-in zoom-in-95 duration-200">
                <div>
                  <label class="block text-xs text-gray-400 mb-1 ml-1">
                    Ollama URL
                  </label>
                  <input
                    type="text"
                    name="ollama_url"
                    value={settings.ollama_url}
                    onChange={handleChange}
                    placeholder="http://localhost:11434"
                    class="w-full bg-[#111] border border-white/10 text-white rounded-xl px-4 py-2.5 text-sm focus:border-gray-500 outline-none"
                  />
                </div>
                <div>
                  <label class="block text-xs text-gray-400 mb-1 ml-1">
                    Модель
                  </label>
                  <input
                    type="text"
                    name="ollama_model"
                    value={settings.ollama_model}
                    onChange={handleChange}
                    placeholder="llama3"
                    class="w-full bg-[#111] border border-white/10 text-white rounded-xl px-4 py-2.5 text-sm focus:border-gray-500 outline-none"
                  />
                </div>
              </div>
            )}
          </div>
        </div>

        {/* --- 2. РАСПИСАНИЕ ЗАДАЧ (CRON) --- */}
        <div class="bg-[#0A0A0B] border border-white/10 rounded-3xl p-7 relative overflow-hidden group">
          <div class="absolute inset-0 bg-gradient-to-br from-teal-500/5 to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-500" />
          <h2 class="text-xl font-semibold text-white flex items-center mb-6 relative z-10">
            <span class="bg-teal-500/10 text-teal-400 p-2 rounded-xl mr-3">
              ⏰
            </span>{" "}
            Расписание (Cron)
          </h2>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 relative z-10">
            {[
              { id: "news_cron", name: "Новости (RSS)", def: "@every 3h" },
              { id: "imap_cron", name: "Почта (IMAP)", def: "@every 30m" },
              { id: "analytics_cron", name: "Анализ данных", def: "@every 1h" },
              {
                id: "moodle_cron",
                name: "Синхронизация Moodle",
                def: "@every 6h",
              },
              { id: "erp_cron", name: "Выгрузка ERP", def: "@every 24h" },
            ].map((item) => (
              <div key={item.id}>
                <label class="block text-[10px] text-gray-500 mb-1 ml-1 uppercase font-medium">
                  {item.name}
                </label>
                <input
                  name={item.id}
                  value={settings[item.id as keyof AppSettings] as string}
                  onChange={handleChange}
                  placeholder={item.def}
                  class="w-full bg-[#111] border border-white/10 text-white rounded-xl px-4 py-2 text-sm focus:border-teal-500 focus:ring-1 focus:ring-teal-500 outline-none transition-all"
                />
              </div>
            ))}
          </div>
        </div>

        {/* --- 3. ИСТОЧНИКИ RSS --- */}
        <div class="bg-[#0A0A0B] border border-white/10 rounded-3xl p-7 relative overflow-hidden group">
          <div class="absolute inset-0 bg-gradient-to-br from-red-500/5 to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-500" />
          <h2 class="text-xl font-semibold text-white flex items-center mb-6 relative z-10">
            <span class="bg-red-500/10 text-red-400 p-2 rounded-xl mr-3">
              <ServiceIcons.RSS />
            </span>{" "}
            RSS Источники
          </h2>

          <div class="space-y-3 max-h-[300px] overflow-y-auto pr-2 custom-scrollbar relative z-10">
            {getRSSList().map((src, idx) => (
              <div
                key={idx}
                class="flex gap-2 bg-[#111] p-2 rounded-xl border border-white/10 focus-within:border-red-500/50 transition-colors"
              >
                <input
                  class="bg-transparent text-sm flex-1 outline-none text-white px-2 placeholder-gray-600"
                  placeholder="Название (напр. Хабр)"
                  value={src.name}
                  onChange={(e) => {
                    const l = getRSSList();
                    l[idx].name = (e.target as any).value;
                    updateRSS(l);
                  }}
                />
                <input
                  class="bg-transparent text-xs flex-[2] outline-none text-gray-400 border-l border-white/10 px-2 focus:text-white placeholder-gray-600"
                  placeholder="https://..."
                  value={src.url}
                  onChange={(e) => {
                    const l = getRSSList();
                    l[idx].url = (e.target as any).value;
                    updateRSS(l);
                  }}
                />
                {/* Заменили Icons.Trash на встроенный SVG чтобы избежать ошибки Undefined Component */}
                <button
                  onClick={() =>
                    updateRSS(getRSSList().filter((_, i) => i !== idx))
                  }
                  class="text-red-500/70 hover:text-red-500 hover:bg-red-500/10 p-2 rounded-lg transition-colors"
                  title="Удалить"
                >
                  <svg
                    class="w-4 h-4"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                    xmlns="http://www.w3.org/2000/svg"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                    ></path>
                  </svg>
                </button>
              </div>
            ))}
          </div>
          <button
            onClick={() => updateRSS([...getRSSList(), { name: "", url: "" }])}
            class="relative z-10 w-full mt-4 py-3 border border-dashed border-white/20 rounded-xl text-sm font-medium text-gray-400 hover:text-white hover:border-white/40 hover:bg-white/5 transition-all"
          >
            + Добавить ленту RSS
          </button>
        </div>

        {/* --- 4. ERP DATABASE --- */}
        <div class="bg-[#0A0A0B] border border-white/10 rounded-3xl p-7 relative overflow-hidden group">
          <div
            class={`absolute inset-0 bg-gradient-to-br from-blue-500/5 to-transparent transition-opacity duration-500 ${settings.enable_univ_db ? "opacity-100" : "opacity-0"}`}
          />
          <div class="flex items-center justify-between mb-6 relative z-10">
            <h2 class="text-xl font-semibold text-white flex items-center">
              <span class="bg-blue-500/10 text-blue-400 p-2 rounded-xl mr-3">
                <ServiceIcons.Database />
              </span>{" "}
              ERP База
            </h2>
            <label class="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                name="enable_univ_db"
                checked={settings.enable_univ_db}
                onChange={handleChange}
                class="sr-only peer"
              />
              <div class="w-11 h-6 bg-gray-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-blue-500"></div>
            </label>
          </div>
          <div
            class={`space-y-5 transition-all duration-300 relative z-10 ${!settings.enable_univ_db ? "opacity-30 grayscale pointer-events-none" : ""}`}
          >
            <div class="flex gap-2 p-1 bg-black/40 rounded-xl border border-white/5">
              {["postgres", "mysql", "sqlserver"].map((db) => (
                <button
                  key={db}
                  onClick={() => setProvider("univ_db_type", db)}
                  disabled={!settings.enable_univ_db}
                  class={`flex-1 text-xs py-2 rounded-lg font-medium capitalize transition-colors ${settings.univ_db_type === db ? "bg-white/10 text-white shadow-sm" : "text-gray-500 hover:text-gray-300"}`}
                >
                  {db === "sqlserver" ? "MS SQL" : db}
                </button>
              ))}
            </div>
            <div>
              <label class="block text-xs text-gray-400 mb-1 ml-1">
                DSN (Строка подключения)
              </label>
              <input
                type="text"
                name="univ_db_dsn"
                value={settings.univ_db_dsn}
                onChange={handleChange}
                disabled={!settings.enable_univ_db}
                placeholder="postgres://user:pass@localhost:5432/db"
                class="w-full bg-[#111] border border-white/10 text-white rounded-xl px-4 py-2.5 text-sm focus:border-blue-500 outline-none disabled:bg-black/20 transition-all"
              />
            </div>
          </div>
        </div>

        {/* --- 5. MOODLE --- */}
        <div class="bg-[#0A0A0B] border border-white/10 rounded-3xl p-7 relative overflow-hidden group">
          <div
            class={`absolute inset-0 bg-gradient-to-br from-orange-500/5 to-transparent transition-opacity duration-500 ${settings.enable_moodle ? "opacity-100" : "opacity-0"}`}
          />
          <div class="flex items-center justify-between mb-6 relative z-10">
            <h2 class="text-xl font-semibold text-white flex items-center">
              <span class="bg-orange-500/10 text-orange-400 p-2 rounded-xl mr-3">
                <ServiceIcons.Moodle />
              </span>{" "}
              Moodle
            </h2>
            <label class="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                name="enable_moodle"
                checked={settings.enable_moodle}
                onChange={handleChange}
                class="sr-only peer"
              />
              <div class="w-11 h-6 bg-gray-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-orange-500"></div>
            </label>
          </div>
          <div
            class={`space-y-4 transition-all duration-300 relative z-10 ${!settings.enable_moodle ? "opacity-30 grayscale pointer-events-none" : ""}`}
          >
            <div>
              <label class="block text-xs text-gray-400 mb-1 ml-1">
                URL сервера
              </label>
              <input
                type="text"
                name="moodle_url"
                value={settings.moodle_url}
                onChange={handleChange}
                disabled={!settings.enable_moodle}
                placeholder="https://moodle.university.ru"
                class="w-full bg-[#111] border border-white/10 text-white rounded-xl px-4 py-2.5 text-sm focus:border-orange-500 outline-none transition-all"
              />
            </div>
            <div>
              <label class="block text-xs text-gray-400 mb-1 ml-1">
                Токен API
              </label>
              <input
                type="password"
                name="moodle_token"
                value={settings.moodle_token}
                onChange={handleChange}
                disabled={!settings.enable_moodle}
                class="w-full bg-[#111] border border-white/10 text-white rounded-xl px-4 py-2.5 text-sm focus:border-orange-500 outline-none transition-all"
              />
            </div>
          </div>
        </div>

        {/* --- 6. IMAP ПОЧТА --- */}
        <div class="bg-[#0A0A0B] border border-white/10 rounded-3xl p-7 relative overflow-hidden group">
          <div
            class={`absolute inset-0 bg-gradient-to-br from-purple-500/5 to-transparent transition-opacity duration-500 ${settings.enable_imap ? "opacity-100" : "opacity-0"}`}
          />
          <div class="flex items-center justify-between mb-6 relative z-10">
            <h2 class="text-xl font-semibold text-white flex items-center">
              <span class="bg-purple-500/10 text-purple-400 p-2 rounded-xl mr-3">
                <ServiceIcons.Mail />
              </span>{" "}
              IMAP Почта
            </h2>
            <label class="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                name="enable_imap"
                checked={settings.enable_imap}
                onChange={handleChange}
                class="sr-only peer"
              />
              <div class="w-11 h-6 bg-gray-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-purple-500"></div>
            </label>
          </div>
          <div
            class={`grid grid-cols-2 gap-4 transition-all duration-300 relative z-10 ${!settings.enable_imap ? "opacity-30 grayscale pointer-events-none" : ""}`}
          >
            <div class="col-span-2 sm:col-span-1">
              <label class="block text-xs text-gray-400 mb-1 ml-1">Host</label>
              <input
                type="text"
                name="imap_host"
                value={settings.imap_host}
                onChange={handleChange}
                class="w-full bg-[#111] border border-white/10 text-white rounded-xl px-4 py-2.5 text-sm focus:border-purple-500 outline-none transition-all"
              />
            </div>
            <div class="col-span-2 sm:col-span-1">
              <label class="block text-xs text-gray-400 mb-1 ml-1">Port</label>
              <input
                type="number"
                name="imap_port"
                value={settings.imap_port}
                onChange={handleChange}
                class="w-full bg-[#111] border border-white/10 text-white rounded-xl px-4 py-2.5 text-sm focus:border-purple-500 outline-none transition-all"
              />
            </div>
            <div class="col-span-2">
              <label class="block text-xs text-gray-400 mb-1 ml-1">
                Email / Login
              </label>
              <input
                type="text"
                name="imap_username"
                value={settings.imap_username}
                onChange={handleChange}
                class="w-full bg-[#111] border border-white/10 text-white rounded-xl px-4 py-2.5 text-sm focus:border-purple-500 outline-none transition-all"
              />
            </div>
            <div class="col-span-2">
              <label class="block text-xs text-gray-400 mb-1 ml-1">
                Пароль
              </label>
              <input
                type="password"
                name="imap_password"
                value={settings.imap_password}
                onChange={handleChange}
                placeholder="********"
                class="w-full bg-[#111] border border-white/10 text-white rounded-xl px-4 py-2.5 text-sm focus:border-purple-500 outline-none transition-all"
              />
            </div>
          </div>
        </div>
      </div>

      {/* --- FLOATING SAVE BAR --- */}
      <div class="fixed bottom-0 left-0 right-0 md:left-64 z-50 p-4 pointer-events-none">
        <div class="max-w-7xl mx-auto flex justify-end">
          <div class="pointer-events-auto bg-[#0A0A0B]/80 backdrop-blur-xl border border-white/10 p-4 rounded-3xl shadow-2xl flex items-center justify-between w-full md:w-auto md:min-w-[400px]">
            <div class="text-sm font-medium mr-6 hidden md:block">
              {isDirty ? (
                <span class="text-white">Есть изменения</span>
              ) : (
                <span class="text-gray-500">Настройки актуальны</span>
              )}
            </div>
            <button
              onClick={saveSettings}
              disabled={saving || !isDirty}
              class={`flex items-center justify-center font-medium px-8 py-3 rounded-2xl transition-all w-full md:w-auto
                ${saving ? "bg-white/10 text-gray-400" : !isDirty ? "bg-white/5 text-gray-600" : "bg-white text-black hover:bg-gray-200 shadow-[0_0_20px_rgba(255,255,255,0.2)]"}`}
            >
              {saving ? (
                <Icons.Spinner class="w-5 h-5 mr-2 animate-spin" />
              ) : (
                "Сохранить"
              )}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
