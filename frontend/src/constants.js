export const TOKEN_KEY = "token";
export const BASE_URL = (process.env.REACT_APP_API_BASE || "/api").replace(/\/+$/, "");
export const SEARCH_KEY = { all: 0, keyword: 1, user: 2, mine: 3 };
export const MAX_UPLOAD_BYTES = 10 * 1024 * 1024;
export const ACCEPTED_MEDIA_TYPES = [
  "image/jpeg", "image/png", "image/gif", "video/mp4", "video/webm",
];
