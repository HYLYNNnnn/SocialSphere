import { BASE_URL, TOKEN_KEY, SEARCH_KEY, MAX_UPLOAD_BYTES, ACCEPTED_MEDIA_TYPES } from "./constants";

export const SESSION_EXPIRED_EVENT = "socialsphere:session-expired";
export const MAX_IMAGE_PROMPT_CHARACTERS = 2000;

export class ApiError extends Error {
  constructor(message, status = 0) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export function decodeToken(token) {
  try {
    const parts = token.split(".");
    if (parts.length !== 3) return null;
    const encoded = parts[1].replace(/-/g, "+").replace(/_/g, "/");
    const payload = decodeURIComponent(Array.from(atob(encoded), (character) =>
      `%${character.charCodeAt(0).toString(16).padStart(2, "0")}`).join(""));
    return JSON.parse(payload);
  } catch {
    return null;
  }
}

function tokenSession(token) {
  if (typeof token !== "string") return null;
  const claims = decodeToken(token);
  const username = claims?.username || claims?.sub;
  if (!claims || typeof username !== "string" || !username.trim()
      || typeof claims.exp !== "number" || !Number.isFinite(claims.exp)
      || claims.exp * 1000 <= Date.now()) return null;
  return { token, username, expiresAt: claims.exp * 1000 };
}

export function getSession() {
  const token = localStorage.getItem(TOKEN_KEY);
  const session = tokenSession(token);
  if (token && !session) localStorage.removeItem(TOKEN_KEY);
  return session;
}

export function clearSession() {
  localStorage.removeItem(TOKEN_KEY);
}

function expireSession() {
  clearSession();
  window.dispatchEvent(new Event(SESSION_EXPIRED_EVENT));
}

export async function apiRequest(path, { authenticated = true, ...options } = {}) {
  const headers = { ...options.headers };
  if (authenticated) {
    const session = getSession();
    if (!session) {
      expireSession();
      throw new ApiError("Your session has ended. Please sign in again.", 401);
    }
    headers.Authorization = `Bearer ${session.token}`;
  }
  let response;
  try {
    response = await fetch(`${BASE_URL}${path}`, { ...options, headers });
  } catch (error) {
    if (error.name === "AbortError") throw error;
    throw new ApiError("Cannot reach the server. Please try again.");
  }
  const text = await response.text();
  let data = text;
  try { data = text ? JSON.parse(text) : null; } catch { /* legacy text errors remain readable */ }
  if (!response.ok) {
    if (response.status === 401 && authenticated) expireSession();
    const message = typeof data?.message === "string" ? data.message
      : typeof data?.error === "string" ? data.error
      : typeof data === "string" && data.trim() ? data.trim() : "The request failed. Please try again.";
    throw new ApiError(message, response.status);
  }
  return data;
}

const jsonOptions = (data) => ({
  method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(data),
});

export async function signIn({ username, password }) {
  const result = await apiRequest("/signin", { ...jsonOptions({ username: username.trim().toLowerCase(), password }), authenticated: false });
  const session = tokenSession(typeof result === "string" ? result : result?.token);
  if (!session || (result?.username && result.username !== session.username)) {
    clearSession();
    throw new ApiError("The server returned an invalid sign-in token.");
  }
  localStorage.setItem(TOKEN_KEY, session.token);
  return session;
}

export function validatePassword(password) {
  if (typeof password !== "string" || !password.trim() || Array.from(password).length < 8 || new Blob([password]).size > 72) {
    throw new ApiError("Use at least 8 characters and no more than 72 UTF-8 bytes for your password.");
  }
}

export async function signUp({ username, password }) {
  validatePassword(password);
  return apiRequest("/signup", {
    ...jsonOptions({ username: username.trim().toLowerCase(), password }), authenticated: false,
  });
}

export async function verifySession() {
  const session = getSession();
  if (!session) return null;
  const account = await apiRequest("/me");
  if (account?.username !== session.username) {
    expireSession();
    throw new ApiError("The server returned an invalid account session.", 401);
  }
  return session;
}

export async function searchPosts({ type = SEARCH_KEY.all, keyword = "" }, options = {}) {
  const query = new URLSearchParams();
  if (type === SEARCH_KEY.mine) query.set("user", getSession()?.username || "");
  else if (type === SEARCH_KEY.user) query.set("user", keyword.trim());
  else if (type === SEARCH_KEY.keyword) query.set("keywords", keyword.trim());
  const parameters = query.toString();
  const result = await apiRequest(`/search${parameters ? `?${parameters}` : ""}`, options);
  if (!Array.isArray(result)) throw new ApiError("The server returned an invalid post list.");
  return result.map((post) => ({ ...post, url: normalizeMediaURL(post.url) }));
}

export function normalizeMediaURL(value) {
  if (typeof value !== "string" || !value.trim()) throw new ApiError("The server returned an invalid media URL.");
  try {
    const base = new URL(`${BASE_URL}/`, window.location.origin);
    const media = new URL(value, base);
    if (!["http:", "https:"].includes(media.protocol) || media.username || media.password) throw new Error("Invalid media protocol");
    return media.href;
  } catch {
    throw new ApiError("The server returned an invalid media URL.");
  }
}

export function validateMediaFile(file) {
  if (!file) throw new ApiError("Please select an image or video.");
  if (!ACCEPTED_MEDIA_TYPES.includes(file.type)) {
    throw new ApiError("Choose a JPEG, PNG, GIF, MP4 or WebM file.");
  }
  if (!file.size || file.size > MAX_UPLOAD_BYTES) throw new ApiError("Choose a nonempty file no larger than 10 MB.");
  return file.type.startsWith("image/") ? "image" : "video";
}

export async function uploadPost(file, message) {
  validateMediaFile(file);
  const description = message.trim();
  if (!description || Array.from(description).length > 2000) throw new ApiError("Enter a message of 1 to 2000 characters.");
  const form = new FormData();
  form.append("message", description);
  form.append("media_file", file);
  return apiRequest("/upload", { method: "POST", body: form });
}

export const deletePost = (postId) => apiRequest(`/post/${encodeURIComponent(postId)}`, { method: "DELETE" });

export async function getImageConfig(options = {}) {
  const config = await apiRequest("/image-config", options);
  if (typeof config?.server_configured !== "boolean" || typeof config?.user_keys_allowed !== "boolean"
      || typeof config?.default_model !== "string" || !Array.isArray(config?.models) || !config.models.length
      || config.models.some((model) => typeof model?.id !== "string" || !model.id.trim()
        || typeof model?.label !== "string" || !model.label.trim())) {
    throw new ApiError("Image settings are unavailable. Please try again.");
  }
  return config;
}

export async function generateImage(prompt, { model, keySource = "server", apiKey } = {}) {
  const description = prompt.trim();
  if (!description || Array.from(description).length > MAX_IMAGE_PROMPT_CHARACTERS) {
    throw new ApiError("Enter an image description of 1 to 2000 characters.");
  }
  if (!["server", "personal"].includes(keySource)) {
    throw new ApiError("Choose how to connect to OpenAI in Image settings.");
  }
  const personalKey = typeof apiKey === "string" ? apiKey.trim() : "";
  if (keySource === "personal" && !personalKey) {
    throw new ApiError("Enter your OpenAI API key in Image settings.");
  }
  const options = jsonOptions({ prompt: description, key_source: keySource, ...(model ? { model } : {}) });
  if (keySource === "personal") options.headers["X-OpenAI-Api-Key"] = personalKey;
  const result = await apiRequest("/generate-image", options);
  if (typeof result?.image_url !== "string" || !result.image_url.startsWith("data:image/")) {
    throw new ApiError("The server returned an invalid generated image.");
  }
  return { imageUrl: result.image_url, prompt: description };
}

export function generatedImageFile(imageUrl) {
  const match = /^data:(image\/(?:png|jpeg|gif));base64,([A-Za-z0-9+/=\s]+)$/.exec(imageUrl || "");
  if (!match) throw new ApiError("The generated image cannot be uploaded. Please generate it again.");
  let contents;
  try { contents = atob(match[2]); } catch { throw new ApiError("The generated image is not valid."); }
  if (!contents.length || contents.length > MAX_UPLOAD_BYTES) throw new ApiError("The generated image exceeds the upload limit.");
  const bytes = Uint8Array.from(contents, (character) => character.charCodeAt(0));
  const extension = match[1] === "image/jpeg" ? "jpg" : match[1].split("/")[1];
  return new File([bytes], `generated.${extension}`, { type: match[1] });
}
