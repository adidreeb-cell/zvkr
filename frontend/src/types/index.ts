export interface Dataset {
  ID?: string;
  id?: string;
  source: string;
  name: string;
  created_at: string;
  headers?: string[] | string;
  data?: any[] | string;
}

export interface Metrics {
  total_students: number;
  active_students: number;
  average_score: number;
}

export interface NewsItem {
  link?: string;
  Link?: string;
  title?: string;
  Title?: string;
}

export type Role = "system" | "user" | "bot";

export interface ChatMessage {
  role: Role;
  content: string;
  charts?: any[];
  source_code: string;
  code_output: string;
  isError?: boolean;
}

export interface User {
  id: number;
  username: string;
  role: string;
  created_at: string;
}
