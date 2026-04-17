import { route } from "preact-router";

const API_URL = "/api/v1";

export const apiFetch = async (
  endpoint: string,
  options: RequestInit = {},
): Promise<Response> => {
  const token = localStorage.getItem("token");
  const headers: HeadersInit = { ...options.headers };
  if (token) {
    (headers as Record<string, string>)["Authorization"] = `Bearer ${token}`;
  }

  const res = await fetch(`${API_URL}${endpoint}`, { ...options, headers });

  if (res.status === 401 && endpoint !== "/login") {
    localStorage.removeItem("token");
    localStorage.removeItem("role");
    route("/login", true);
  }

  return res;
};
