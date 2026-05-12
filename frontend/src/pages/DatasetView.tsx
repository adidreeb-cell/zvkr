import { useState, useEffect } from "preact/hooks";
import { apiFetch } from "../api/client";
import { Icons } from "../components/ui/Icons";
import { ChatInterface } from "../components/chat/ChatInterface";
import { DataGrid } from "../components/data/DataGrid";
import type { Dataset } from "../types";
import { route } from "preact-router";

interface DatasetViewProps {
  id?: string;
}

export function DatasetView({ id }: DatasetViewProps) {
  const [dataset, setDataset] = useState<Dataset | null>(null);
  const [activeTab, setActiveTab] = useState<"chat" | "data">("chat");
  const role = localStorage.getItem("role");

  useEffect(() => {
    if (!id) return;
    apiFetch(`/datasets/${id}`)
      .then((r) => r.json())
      .then((data: Dataset) => {
        console.log(data);
        setDataset({
          ...data,
          headers: data.headers,
          data: data.data,
        });
      })
      .catch(console.error);
  }, [id]);

  const removeDataset = async (id: string) => {
    try {
      const res = await apiFetch(`/datasets/${id}/remove`);
      if (!res.ok) throw new Error("Ошибка удаление датасет");

      setTimeout(() => route("/", true), 0);
    } catch (e: any) {
      alert(e.message);
    }
  };

  const downloadFile = async (type: "excel" | "pdf") => {
    try {
      const res = await apiFetch(`/export/${type}/${id}`);
      if (!res.ok) throw new Error("Ошибка генерации файла");

      const blob = await res.blob();
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `Report_${id}.${type === "excel" ? "xlsx" : "pdf"}`;
      a.click();
      window.URL.revokeObjectURL(url);
    } catch (e: any) {
      alert(e.message);
    }
  };

  if (!dataset || !id) {
    return (
      <div class="h-full flex items-center justify-center">
        <Icons.Spinner />
      </div>
    );
  }

  return (
    <div class="flex flex-col h-full bg-background">
      <div class="border-b border-border px-6 py-4 flex items-center justify-between bg-surface/50">
        <div class="flex items-center space-x-6">
          <div>
            <h2 class="font-semibold text-lg tracking-tight text-white max-w-xs truncate">
              {dataset.name}
            </h2>
            <div class="text-[10px] text-gray-500 font-mono mt-0.5">
              {(dataset.data as any[])?.length || 0} ROWS
            </div>
          </div>
          <div class="h-8 w-px bg-border"></div>
          <div class="flex space-x-2">
            <button
              onClick={() => setActiveTab("chat")}
              class={`flex items-center space-x-2 px-3 py-1.5 rounded-lg text-sm font-medium transition ${
                activeTab === "chat"
                  ? "bg-black text-white border border-border shadow-sm"
                  : "text-gray-500 hover:text-gray-300"
              }`}
            >
              <Icons.Chart />
              <span>Аналитика</span>
            </button>
            <button
              onClick={() => setActiveTab("data")}
              class={`flex items-center space-x-2 px-3 py-1.5 rounded-lg text-sm font-medium transition ${
                activeTab === "data"
                  ? "bg-black text-white border border-border shadow-sm"
                  : "text-gray-500 hover:text-gray-300"
              }`}
            >
              <Icons.Table />
              <span>Данные</span>
            </button>
          </div>
        </div>
        <div class="flex space-x-3">
          {role === "admin" && (
            <button
              onClick={() => removeDataset(id)}
              class="flex items-center space-x-1.5 text-xs text-gray-400 bg-black border border-red-400 px-3 py-1.5 rounded-lg hover:text-white transition"
            >
              Удалить датасет
            </button>
          )}

          <button
            onClick={() => downloadFile("excel")}
            class="flex items-center space-x-1.5 text-xs text-gray-400 bg-black border border-border px-3 py-1.5 rounded-lg hover:text-white transition"
          >
            <Icons.Download />
            <span>.XLSX</span>
          </button>
          <button
            onClick={() => downloadFile("pdf")}
            class="flex items-center space-x-1.5 text-xs text-gray-400 bg-black border border-border px-3 py-1.5 rounded-lg hover:text-white transition"
          >
            <Icons.Download />
            <span>.PDF</span>
          </button>
        </div>
      </div>
      <div class="flex-1 overflow-hidden relative">
        {activeTab === "data" ? (
          <DataGrid
            headers={(dataset.headers as string[]) || []}
            data={(dataset.data as any[]) || []}
          />
        ) : (
          <ChatInterface datasetId={id} />
        )}
      </div>
    </div>
  );
}
