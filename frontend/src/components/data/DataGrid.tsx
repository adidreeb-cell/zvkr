import { h } from "preact";
import { useEffect } from "preact/hooks";

interface DataGridProps {
  headers: string[];
  data: any[];
}

export function DataGrid({ headers, data }: DataGridProps) {
  useEffect(() => {
    console.log(headers, data);
  }, [headers, data]);

  return (
    <div class="overflow-auto h-full w-full custom-scrollbar bg-black">
      <table class="min-w-full divide-y divide-border text-sm text-left">
        <thead class="bg-surface sticky top-0 z-10 shadow-sm">
          <tr>
            {headers.map((h, i) => (
              <th
                key={i}
                class="px-6 py-3 font-medium text-gray-400 text-xs tracking-wider border-b border-border"
              >
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody class="divide-y divide-border">
          {data.slice(0, 100).map((row, idx) => (
            <tr key={idx} class="hover:bg-gray-900/30 transition">
              <td colSpan={headers.length} class="p-0">
                <div class="flex w-full">
                  {headers.map((h, i) => (
                    <div
                      key={i}
                      class="px-6 py-2 text-gray-300 font-mono text-[11px] truncate flex-1 min-w-[150px]"
                    >
                      {row[h]}
                    </div>
                  ))}
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
