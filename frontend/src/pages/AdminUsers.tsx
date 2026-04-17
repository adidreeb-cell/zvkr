import { h, Fragment } from "preact";
import { useState, useEffect } from "preact/hooks";
import { apiFetch } from "../api/client";
import { Icons } from "../components/ui/Icons";

interface User {
  id: number;
  username: string;
  role: string;
  created_at: string;
  updated_at: string;
}

export function AdminUsers() {
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Состояния для модального окна создания
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [newUsername, setNewUsername] = useState("");
  const [newRole, setNewRole] = useState("user");
  const [isSubmitting, setIsSubmitting] = useState(false); // Защита от двойного клика

  // Состояние для удаления
  const [deletingId, setDeletingId] = useState<number | null>(null);

  // Состояние для показа сгенерированного пароля
  const [createdUser, setCreatedUser] = useState<{
    username: string;
    role: string;
    password: string;
  } | null>(null);
  const [isCopied, setIsCopied] = useState(false); // Для кнопки копирования пароля

  // Получаем логин текущего админа, чтобы он случайно не удалил сам себя
  const currentUsername = localStorage.getItem("username") || "";

  const fetchUsers = async () => {
    setLoading(true);
    try {
      const res = await apiFetch("/users");
      if (res.ok) {
        const data = await res.json();
        setUsers(data || []);
      } else {
        const err = await res.json();
        setError(err.error || "Ошибка загрузки пользователей");
      }
    } catch (e) {
      setError("Ошибка сети при загрузке данных");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchUsers();
  }, []);

  // --- ЗАЩИТА: Клиентская валидация логина ---
  const validateUsername = (username: string): string | null => {
    if (username.length < 3) return "Логин должен содержать минимум 3 символа";
    if (username.length > 30) return "Логин слишком длинный (максимум 30)";
    if (/\s/.test(username)) return "Логин не должен содержать пробелы";
    if (!/^[a-zA-Z0-9_.-]+$/.test(username)) {
      return "Логин может содержать только латинские буквы, цифры, точки, дефисы и подчеркивания";
    }
    return null;
  };

  const handleCreateUser = async (e: Event) => {
    e.preventDefault();
    setError(null);

    const validationError = validateUsername(newUsername);
    if (validationError) {
      setError(validationError);
      return;
    }

    setIsSubmitting(true);
    try {
      const res = await apiFetch("/users/add", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: newUsername, role: newRole }),
      });

      const data = await res.json();

      if (res.ok) {
        setCreatedUser(data);
        setNewUsername("");
        setNewRole("user");
        setIsCopied(false);
        fetchUsers();
      } else {
        setError(data.error || "Ошибка создания пользователя");
      }
    } catch (e) {
      setError("Ошибка сети. Сервер недоступен.");
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleDeleteUser = async (id: number, username: string) => {
    // ЗАЩИТА: Не даем удалить самого себя
    if (username === currentUsername) {
      alert("Защита: Вы не можете удалить свой собственный аккаунт.");
      return;
    }

    if (
      !confirm(
        `Вы абсолютно уверены, что хотите удалить пользователя "${username}"? Это действие необратимо.`,
      )
    ) {
      return;
    }

    setDeletingId(id);
    try {
      const res = await apiFetch(`/users/remove?id=${id}`, {
        method: "POST",
      });

      if (res.ok) {
        await fetchUsers();
      } else {
        const data = await res.json();
        alert(data.error || "Ошибка удаления");
      }
    } catch (e) {
      alert("Ошибка сети при удалении");
    } finally {
      setDeletingId(null);
    }
  };

  // Копирование пароля в буфер обмена
  const handleCopyPassword = async () => {
    if (createdUser?.password) {
      try {
        await navigator.clipboard.writeText(createdUser.password);
        setIsCopied(true);
        setTimeout(() => setIsCopied(false), 2000);
      } catch (err) {
        alert("Не удалось скопировать пароль. Скопируйте его вручную.");
      }
    }
  };

  const closeModal = () => {
    if (isSubmitting) return; // Не даем закрыть модалку, пока идет запрос
    setIsModalOpen(false);
    setCreatedUser(null);
    setError(null);
    setNewUsername("");
  };

  // Сбрасываем ошибку при наборе текста
  const handleUsernameInput = (e: Event) => {
    setNewUsername((e.target as HTMLInputElement).value);
    if (error) setError(null);
  };

  if (loading && users.length === 0) {
    return (
      <div class="h-full flex flex-col items-center justify-center text-gray-500">
        <Icons.Spinner class="w-8 h-8 animate-spin mb-4" />
        <p class="text-sm">Загрузка пользователей...</p>
      </div>
    );
  }

  return (
    <div class="container mx-auto px-6 py-8 overflow-y-auto h-full custom-scrollbar relative">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between mb-8 gap-4">
        <div>
          <h1 class="text-3xl font-semibold tracking-tight text-white">
            Пользователи
          </h1>
          <p class="text-sm text-gray-400 mt-1">
            Управление доступом и ролями системы
          </p>
        </div>
        <button
          onClick={() => setIsModalOpen(true)}
          class="bg-[#3291ff] hover:bg-[#2075d6] text-white px-5 py-2.5 rounded-xl text-sm font-medium transition shadow-sm whitespace-nowrap"
        >
          + Добавить пользователя
        </button>
      </div>

      {error && !isModalOpen && (
        <div class="bg-red-500/10 border border-red-500/20 text-red-400 p-4 rounded-xl mb-6 text-sm flex items-start gap-3">
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
          {error}
        </div>
      )}

      {/* Таблица пользователей */}
      <div class="bg-surface border border-border rounded-2xl overflow-hidden shadow-sm">
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm whitespace-nowrap">
            <thead class="bg-black/20 text-gray-400 text-xs uppercase tracking-wider border-b border-border">
              <tr>
                <th class="px-6 py-4 font-medium">ID</th>
                <th class="px-6 py-4 font-medium">Пользователь</th>
                <th class="px-6 py-4 font-medium">Роль</th>
                <th class="px-6 py-4 font-medium">Дата создания</th>
                <th class="px-6 py-4 font-medium text-right">Действия</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border">
              {users.map((u) => {
                const isMe = u.username === currentUsername;

                return (
                  <tr
                    key={u.id}
                    class="hover:bg-white/[0.02] transition-colors"
                  >
                    <td class="px-6 py-4 text-gray-500 font-mono">#{u.id}</td>
                    <td class="px-6 py-4 font-medium text-gray-200 flex items-center gap-2">
                      {u.username}
                      {isMe && (
                        <span class="text-[10px] bg-white/10 text-gray-400 px-1.5 py-0.5 rounded uppercase tracking-wide">
                          Вы
                        </span>
                      )}
                    </td>
                    <td class="px-6 py-4">
                      <span
                        class={`px-2 py-1 rounded text-[10px] uppercase font-mono tracking-wider border
                        ${
                          u.role === "admin"
                            ? "bg-red-900/20 text-red-400 border-red-800/30"
                            : u.role === "analyst" || u.role === "аналитик"
                              ? "bg-blue-900/20 text-[#3291ff] border-[#3291ff]/30"
                              : "bg-gray-800 text-gray-400 border-gray-700"
                        }`}
                      >
                        {u.role}
                      </span>
                    </td>
                    <td class="px-6 py-4 text-gray-500 text-xs">
                      {new Date(u.created_at).toLocaleString("ru-RU", {
                        day: "2-digit",
                        month: "2-digit",
                        year: "numeric",
                        hour: "2-digit",
                        minute: "2-digit",
                      })}
                    </td>
                    <td class="px-6 py-4 text-right">
                      {isMe ? (
                        <span
                          class="text-gray-600 text-xs px-2 py-1 select-none"
                          title="Нельзя удалить самого себя"
                        >
                          Заблокировано
                        </span>
                      ) : (
                        <button
                          onClick={() => handleDeleteUser(u.id, u.username)}
                          disabled={deletingId === u.id}
                          class="text-gray-500 hover:text-red-400 transition text-xs border border-transparent hover:border-red-900/50 px-3 py-1.5 rounded disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-end ml-auto w-20"
                        >
                          {deletingId === u.id ? (
                            <Icons.Spinner class="w-3.5 h-3.5 animate-spin" />
                          ) : (
                            "Удалить"
                          )}
                        </button>
                      )}
                    </td>
                  </tr>
                );
              })}
              {users.length === 0 && !loading && (
                <tr>
                  <td
                    colSpan={5}
                    class="px-6 py-12 text-center text-gray-500 border-t border-dashed border-border"
                  >
                    Пользователи не найдены
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Модальное окно создания пользователя */}
      {isModalOpen && (
        <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div
            class="absolute inset-0 bg-background/80 backdrop-blur-sm"
            onClick={closeModal}
          />

          <div class="bg-surface border border-border rounded-2xl p-6 w-full max-w-md shadow-2xl relative z-10 animate-in fade-in zoom-in-95 duration-200">
            <button
              onClick={closeModal}
              disabled={isSubmitting}
              class="absolute top-4 right-4 text-gray-500 hover:text-white transition disabled:opacity-50"
            >
              ✕
            </button>

            <h2 class="text-xl font-semibold mb-6 text-white">
              Новый пользователь
            </h2>

            {error && isModalOpen && (
              <div class="bg-red-500/10 text-red-400 p-3 rounded-xl mb-4 text-sm border border-red-500/20 flex items-start gap-2">
                <svg
                  class="w-4 h-4 flex-shrink-0 mt-0.5"
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
                {error}
              </div>
            )}

            {/* Успешное создание + пароль */}
            {createdUser ? (
              <div class="space-y-4">
                <div class="bg-green-500/10 border border-green-500/20 p-5 rounded-2xl text-center">
                  <div class="text-green-400 text-sm font-medium mb-3 flex items-center justify-center gap-2">
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
                        d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
                      />
                    </svg>
                    Пользователь создан!
                  </div>
                  <p class="text-gray-400 text-xs mb-4 leading-relaxed">
                    Скопируйте и передайте этот пароль пользователю.
                    <br />
                    <span class="text-gray-300 font-medium">
                      Он больше нигде не покажется.
                    </span>
                  </p>

                  <div class="relative group">
                    <div class="bg-black/50 border border-border p-4 rounded-xl font-mono text-lg text-white break-all tracking-wider">
                      {createdUser.password}
                    </div>
                    {/* Кнопка копирования */}
                    <button
                      onClick={handleCopyPassword}
                      class="absolute right-2 top-1/2 -translate-y-1/2 p-2 bg-surface hover:bg-gray-800 border border-border rounded-lg text-gray-400 hover:text-white transition flex items-center gap-2"
                    >
                      {isCopied ? (
                        <span class="text-green-400 text-xs font-medium flex items-center gap-1">
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
                        </span>
                      ) : (
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
                      )}
                    </button>
                  </div>
                </div>
                <button
                  onClick={closeModal}
                  class="w-full bg-surface border border-border hover:bg-white/5 text-white py-2.5 rounded-xl transition text-sm font-medium"
                >
                  Готово, закрыть
                </button>
              </div>
            ) : (
              /* Форма создания */
              <form onSubmit={handleCreateUser} class="space-y-5">
                <div>
                  <label class="block text-[11px] text-gray-400 uppercase tracking-wider mb-2 font-medium">
                    Логин (Username)
                  </label>
                  <input
                    type="text"
                    required
                    disabled={isSubmitting}
                    value={newUsername}
                    onInput={handleUsernameInput}
                    class="w-full bg-black/50 border border-border text-white rounded-xl px-4 py-2.5 focus:outline-none focus:border-[#3291ff] transition text-sm disabled:opacity-50"
                    placeholder="ivanov_i"
                  />
                  <p class="text-[10px] text-gray-500 mt-2">
                    Только латиница и цифры. Без пробелов.
                  </p>
                </div>

                <div>
                  <label class="block text-[11px] text-gray-400 uppercase tracking-wider mb-2 font-medium">
                    Права доступа (Роль)
                  </label>
                  <div class="relative">
                    <select
                      value={newRole}
                      onChange={(e) =>
                        setNewRole((e.target as HTMLSelectElement).value)
                      }
                      disabled={isSubmitting}
                      class="w-full bg-black/50 border border-border text-white rounded-xl pl-4 pr-10 py-2.5 focus:outline-none focus:border-[#3291ff] transition text-sm appearance-none disabled:opacity-50 cursor-pointer"
                    >
                      <option value="user">
                        User — Только просмотр дашбордов
                      </option>
                      <option value="analyst">
                        Analyst — Загрузка данных и отчеты
                      </option>
                      <option value="admin">
                        Admin — Полный доступ (настройки)
                      </option>
                    </select>
                    <div class="absolute inset-y-0 right-0 flex items-center pr-3 pointer-events-none text-gray-500">
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

                <div class="pt-2 flex justify-end gap-3">
                  <button
                    type="button"
                    onClick={closeModal}
                    disabled={isSubmitting}
                    class="px-5 py-2.5 text-sm text-gray-400 hover:text-white transition disabled:opacity-50 font-medium"
                  >
                    Отмена
                  </button>
                  <button
                    type="submit"
                    disabled={isSubmitting}
                    class="bg-[#3291ff] hover:bg-[#2075d6] text-white px-6 py-2.5 rounded-xl text-sm font-medium transition disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2 shadow-sm"
                  >
                    {isSubmitting ? (
                      <>
                        <Icons.Spinner class="w-4 h-4 animate-spin" />
                        Создание...
                      </>
                    ) : (
                      "Создать аккаунт"
                    )}
                  </button>
                </div>
              </form>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
