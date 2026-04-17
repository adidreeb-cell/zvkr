export const ROLES = {
  ADMIN: "admin",
  ANALYST: "analyst",
  USER: "user",
  VIEWER: "viewer",
} as const;

export const API_URL = "/api/v1";

export const DATASET_LIMITS = {
  MAX_ROWS_DISPLAY: 100,
  ROWS_PER_PAGE: 50,
} as const;

export const EXPORT_FORMATS = {
  EXCEL: "xlsx",
  PDF: "pdf",
} as const;
