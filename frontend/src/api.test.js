import {
  apiRequest, getSession, clearSession, signIn, signUp, verifySession, searchPosts,
  validatePassword, validateMediaFile, uploadPost, getImageConfig, generateImage, generatedImageFile,
  normalizeMediaURL, SESSION_EXPIRED_EVENT,
} from "./api";
import { SEARCH_KEY, TOKEN_KEY, MAX_UPLOAD_BYTES } from "./constants";

const originalFetch = global.fetch;
const makeToken = (claims = {}) => `header.${btoa(JSON.stringify({ username: "alice", exp: Math.floor(Date.now() / 1000) + 3600, ...claims }))}.signature`;
const respond = (status, data) => ({ ok: status >= 200 && status < 300, status,
  text: async () => typeof data === "string" ? data : data == null ? "" : JSON.stringify(data) });

beforeEach(() => {
  localStorage.clear();
  global.fetch = jest.fn();
});
afterAll(() => { global.fetch = originalFetch; });
const signedIn = () => { const token = makeToken(); localStorage.setItem(TOKEN_KEY, token); return token; };

test("restores a valid JWT and removes expired or malformed saved tokens", () => {
  const token = signedIn();
  expect(getSession()).toMatchObject({ username: "alice", token });
  localStorage.setItem(TOKEN_KEY, makeToken({ exp: 1 }));
  expect(getSession()).toBeNull();
  expect(localStorage.getItem(TOKEN_KEY)).toBeNull();
  localStorage.setItem(TOKEN_KEY, "not-a-jwt");
  expect(getSession()).toBeNull();
});

test("sign-in sends credentials in a JSON body and stores a validated token", async () => {
  const token = makeToken();
  global.fetch.mockResolvedValue(respond(200, { token, username: "alice" }));
  await expect(signIn({ username: " Alice ", password: "password-example" })).resolves.toMatchObject({ username: "alice" });
  const [url, options] = global.fetch.mock.calls[0];
  expect(url).toBe("/api/signin");
  expect(JSON.parse(options.body)).toEqual({ username: "alice", password: "password-example" });
  expect(localStorage.getItem(TOKEN_KEY)).toBe(token);
});

test.each(["not-a-jwt", makeToken({ exp: 1 }), makeToken({ username: null, sub: null })])("rejects an invalid sign-in token", async (token) => {
  global.fetch.mockResolvedValue(respond(200, { token }));
  await expect(signIn({ username: "alice", password: "password-example" })).rejects.toThrow("invalid sign-in token");
  expect(localStorage.getItem(TOKEN_KEY)).toBeNull();
});

test("shows a structured API error instead of logging credentials", async () => {
  global.fetch.mockResolvedValue(respond(401, { message: "Invalid username or password." }));
  const log = jest.spyOn(console, "log").mockImplementation(() => {});
  const error = jest.spyOn(console, "error").mockImplementation(() => {});
  try {
    await expect(signIn({ username: "alice", password: "password-example" })).rejects.toMatchObject({ status: 401, message: "Invalid username or password." });
    expect(log).not.toHaveBeenCalled(); expect(error).not.toHaveBeenCalled();
  } finally { log.mockRestore(); error.mockRestore(); }
});

test("signup accepts HTTP 201 and sends only supported fields", async () => {
  global.fetch.mockResolvedValue(respond(201, { username: "alice" }));
  await signUp({ username: " ALICE ", password: "password-example", confirm: "not-sent" });
  expect(JSON.parse(global.fetch.mock.calls[0][1].body)).toEqual({ username: "alice", password: "password-example" });
});

test("password validation counts Unicode characters and UTF-8 bytes", () => {
  expect(() => validatePassword("密".repeat(24))).not.toThrow();
  expect(() => validatePassword("密".repeat(25))).toThrow("72 UTF-8 bytes");
  expect(() => validatePassword("🦉".repeat(4))).toThrow("8 characters");
  expect(() => validatePassword("        ")).toThrow();
});

test("verifies the cached account through the backend", async () => {
  signedIn(); global.fetch.mockResolvedValue(respond(200, { username: "alice" }));
  await expect(verifySession()).resolves.toMatchObject({ username: "alice" });
  expect(global.fetch.mock.calls[0][0]).toBe("/api/me");
});

test("does not accept an account mismatch from the backend", async () => {
  signedIn(); global.fetch.mockResolvedValue(respond(200, { username: "bob" }));
  await expect(verifySession()).rejects.toMatchObject({ status: 401 });
  expect(getSession()).toBeNull();
});

test("a protected 401 clears the token and notifies the UI", async () => {
  signedIn(); global.fetch.mockResolvedValue(respond(401, { message: "Session expired" }));
  const expired = jest.fn(); window.addEventListener(SESSION_EXPIRED_EVENT, expired);
  try {
    await expect(apiRequest("/search")).rejects.toMatchObject({ status: 401 });
    expect(localStorage.getItem(TOKEN_KEY)).toBeNull();
    expect(expired).toHaveBeenCalledTimes(1);
  } finally { window.removeEventListener(SESSION_EXPIRED_EVENT, expired); }
});

test("does not send a protected request without a usable token", async () => {
  await expect(apiRequest("/search")).rejects.toMatchObject({ status: 401 });
  expect(global.fetch).not.toHaveBeenCalled();
});

test("preserves AbortError so obsolete searches can be ignored", async () => {
  signedIn(); const aborted = Object.assign(new Error("Aborted"), { name: "AbortError" });
  global.fetch.mockRejectedValue(aborted);
  await expect(apiRequest("/search")).rejects.toBe(aborted);
});

test("encodes search text and sends the saved token", async () => {
  const token = signedIn(); global.fetch.mockResolvedValue(respond(200, []));
  await searchPosts({ type: SEARCH_KEY.keyword, keyword: " sun & sea + 海 " });
  const [url, options] = global.fetch.mock.calls[0];
  expect(new URL(url, "http://localhost").searchParams.get("keywords")).toBe("sun & sea + 海");
  expect(options.headers.Authorization).toBe(`Bearer ${token}`);
});

test("My posts uses the authenticated username, not arbitrary search text", async () => {
  signedIn(); global.fetch.mockResolvedValue(respond(200, []));
  await searchPosts({ type: SEARCH_KEY.mine, keyword: "bob" });
  expect(new URL(global.fetch.mock.calls[0][0], "http://localhost").searchParams.get("user")).toBe("alice");
});

test("rejects an invalid post list and propagates read errors", async () => {
  signedIn(); global.fetch.mockResolvedValueOnce(respond(200, null)).mockResolvedValueOnce(respond(500, { message: "Posts unavailable" }));
  await expect(searchPosts({})).rejects.toThrow("invalid post list");
  await expect(searchPosts({})).rejects.toThrow("Posts unavailable");
});

test("upload preserves the real file MIME and lets the browser encode multipart", async () => {
  signedIn(); global.fetch.mockResolvedValue(respond(201, { id: "post-1" }));
  const file = new File(["png-bytes"], "photo.png", { type: "image/png" });
  await uploadPost(file, " A caption ");
  const [url, options] = global.fetch.mock.calls[0];
  expect(url).toBe("/api/upload");
  expect(options.headers["Content-Type"]).toBeUndefined();
  expect(options.body.get("message")).toBe("A caption");
  expect(options.body.get("media_file").type).toBe("image/png");
  expect(options.body.get("media_file").name).toBe("photo.png");
});

test("rejects unsupported, empty or oversized media before network upload", async () => {
  expect(() => validateMediaFile(new File(["svg"], "script.svg", { type: "image/svg+xml" }))).toThrow("JPEG");
  expect(() => validateMediaFile(new File([], "empty.png", { type: "image/png" }))).toThrow("nonempty");
  expect(() => validateMediaFile({ type: "image/png", size: MAX_UPLOAD_BYTES + 1 })).toThrow("10 MB");
  await expect(uploadPost(null, "caption")).rejects.toThrow("select");
  expect(global.fetch).not.toHaveBeenCalled();
});

test("image generation calls only the authenticated backend endpoint", async () => {
  signedIn(); const image = `data:image/png;base64,${btoa("png-bytes")}`;
  global.fetch.mockResolvedValue(respond(200, { image_url: image }));
  await expect(generateImage(" A blue bird ")).resolves.toEqual({ imageUrl: image, prompt: "A blue bird" });
  expect(global.fetch.mock.calls[0][0]).toBe("/api/generate-image");
  expect(JSON.parse(global.fetch.mock.calls[0][1].body)).toEqual({ prompt: "A blue bird", key_source: "server" });
});

test("image options use website authentication and support cancelling a stale load", async () => {
  const token = signedIn(); const controller = new AbortController();
  const config = { server_configured: false, user_keys_allowed: true, default_model: "gpt-image-2",
    models: [{ id: "gpt-image-2", label: "GPT Image 2" }] };
  global.fetch.mockResolvedValue(respond(200, config));
  await expect(getImageConfig({ signal: controller.signal })).resolves.toEqual(config);
  const [url, options] = global.fetch.mock.calls[0];
  expect(url).toBe("/api/image-config");
  expect(options.headers).toEqual({ Authorization: `Bearer ${token}` });
  expect(options.signal).toBe(controller.signal);
});

test("unusable image options are rejected with a safe setup message", async () => {
  signedIn(); global.fetch.mockResolvedValue(respond(200, { models: [] }));
  await expect(getImageConfig()).rejects.toThrow("Image settings are unavailable");
});

test("personal generation sends the key only in its header and keeps the website JWT", async () => {
  const token = signedIn(); const key = "sk-example-personal-key";
  const cookieBefore = document.cookie;
  sessionStorage.clear();
  global.fetch.mockResolvedValue(respond(200, { image_url: "data:image/png;base64,cG5n" }));
  await generateImage(" A bird ", { model: "gpt-image-2", keySource: "personal", apiKey: ` ${key} ` });
  const [url, options] = global.fetch.mock.calls[0];
  expect(url).toBe("/api/generate-image");
  expect(options.headers).toEqual({ "Content-Type": "application/json", "X-OpenAI-Api-Key": key, Authorization: `Bearer ${token}` });
  expect(JSON.parse(options.body)).toEqual({ prompt: "A bird", model: "gpt-image-2", key_source: "personal" });
  expect(options.body).not.toContain(key); expect(url).not.toContain(key);
  expect(localStorage.length).toBe(1); expect(localStorage.getItem(TOKEN_KEY)).toBe(token);
  expect(sessionStorage.length).toBe(0); expect(document.cookie).toBe(cookieBefore);
});

test("personal generation never sends an empty key or falls back to the server key", async () => {
  signedIn();
  await expect(generateImage("A bird", { keySource: "personal", apiKey: "   " })).rejects.toThrow("Enter your OpenAI API key");
  await expect(generateImage("A bird", { keySource: "personal" })).rejects.toThrow("Enter your OpenAI API key");
  expect(global.fetch).not.toHaveBeenCalled();
});

test("server generation omits a personal key and an upstream key rejection preserves sign-in", async () => {
  const token = signedIn(); const expired = jest.fn();
  window.addEventListener(SESSION_EXPIRED_EVENT, expired);
  global.fetch.mockResolvedValue(respond(502, { message: "OpenAI could not accept this API key." }));
  try {
    await expect(generateImage("A bird", { model: "gpt-image-2", keySource: "server", apiKey: "sk-unused" }))
      .rejects.toMatchObject({ status: 502, message: "OpenAI could not accept this API key." });
    expect(global.fetch.mock.calls[0][1].headers["X-OpenAI-Api-Key"]).toBeUndefined();
    expect(localStorage.getItem(TOKEN_KEY)).toBe(token); expect(expired).not.toHaveBeenCalled();
  } finally { window.removeEventListener(SESSION_EXPIRED_EVENT, expired); }
});

test("generated data converts locally to a PNG without requesting an external URL", () => {
  const file = generatedImageFile(`data:image/png;base64,${btoa("png-bytes")}`);
  expect(file.name).toBe("generated.png"); expect(file.type).toBe("image/png"); expect(file.size).toBe(9);
  expect(() => generatedImageFile("https://external.example/image.png")).toThrow("cannot be uploaded");
  expect(() => generatedImageFile("data:image/svg+xml;base64,PHN2Zz4=")).toThrow();
  expect(global.fetch).not.toHaveBeenCalled();
});

test("normalizes media against the configured API origin and rejects unsafe schemes", () => {
  expect(normalizeMediaURL("/api/media/post-1")).toBe("http://localhost/api/media/post-1");
  expect(normalizeMediaURL("https://storage.example/photo.png")).toBe("https://storage.example/photo.png");
  expect(() => normalizeMediaURL("javascript:alert(1)")).toThrow("invalid media URL");
  expect(() => normalizeMediaURL("data:image/svg+xml,test")).toThrow();
});

test("absolute API configuration resolves relative media at that API server", () => {
  const original = process.env.REACT_APP_API_BASE;
  try {
    process.env.REACT_APP_API_BASE = "https://api.example.test/api/";
    jest.resetModules();
    const { normalizeMediaURL: normalize } = require("./api");
    expect(normalize("/api/media/post-1")).toBe("https://api.example.test/api/media/post-1");
  } finally {
    if (original === undefined) delete process.env.REACT_APP_API_BASE;
    else process.env.REACT_APP_API_BASE = original;
  }
});

test("clearSession removes the saved bearer token", () => {
  signedIn(); clearSession(); expect(getSession()).toBeNull();
});


test("generation enforces the server's 2000-character Unicode limit before sending", async () => {
  signedIn(); const image = `data:image/png;base64,${btoa("png-bytes")}`;
  global.fetch.mockResolvedValue(respond(200, { image_url: image }));
  const unicodePrompt = "🦉".repeat(2000);
  await expect(generateImage(unicodePrompt)).resolves.toMatchObject({ prompt: unicodePrompt });
  expect(global.fetch).toHaveBeenCalledTimes(1);
  await expect(generateImage("a".repeat(2001))).rejects.toThrow("1 to 2000 characters");
  await expect(generateImage("🦉".repeat(2001))).rejects.toThrow("1 to 2000 characters");
  expect(global.fetch).toHaveBeenCalledTimes(1);
});
