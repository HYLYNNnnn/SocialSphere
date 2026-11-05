# SocialSphere

A social networking application with React, Go, JWT authentication, image/video posts, text and user search, and server-side AI image generation.

The original 2025 image-generation implementation used DALL·E 3. The maintenance update moves AI requests to the backend and makes the image model configurable because [DALL·E 3 has been removed from the API](https://developers.openai.com/api/docs/models/dall-e-3).

## Features

- Register with a BCrypt-hashed password, sign in with a JWT, restore an existing login and sign out locally.
- Upload JPEG, PNG, GIF, MP4 or WebM media up to 10 MiB, with backend content checks.
- Browse recent posts, search text or usernames, and list your own posts.
- Delete only your own posts, including stored media.
- Generate an image through the authenticated backend, preview it, then publish it through the normal upload flow.
- Run locally with persistent files, or configure Elasticsearch and Google Cloud Storage for cloud mode.

## Repository layout

- `frontend/`: React 18, React Router 6, Material UI and Ant Design, plus frontend tests.
- `backend/`: Go API, storage adapters, authentication and integration tests.
- `Dockerfile` and `compose.yaml`: combined frontend/API image and a persistent local data volume.
- `.env.example`: server-side environment variable names with no private credentials.

## Local development

Requirements: Go 1.24.6 or newer and Node.js with npm. The verification environment used Go 1.27.3 and Node.js 24.

Start the API in one terminal:

```sh
cd backend
go run .
```

Start the frontend from a second terminal:

```sh
cd frontend
npm ci
npm start
```

Open http://localhost:3000. The development proxy forwards `/api` to `http://127.0.0.1:8080`; `API_PROXY_TARGET` can override that development target. The API also exposes its original root endpoint paths for compatibility.

Local mode is the default and does not require Elasticsearch, GCS or an AI key. Accounts, password hashes, posts and media persist under `backend/data/`. A private JWT signing key is generated there on first startup and reused after restart. That entire directory and its private files are ignored by Git.

## Run the combined image

With Docker Desktop running, from the repository root:

```sh
docker compose up --build
```

Open http://localhost:8080. Compose uses a named volume for local data; `docker compose down` stops the application while retaining that volume. This setup runs one local storage instance.

To build and serve the frontend manually instead, run `npm run build` in `frontend/`, then run `FRONTEND_DIR=../frontend/build go run .` in `backend/`.

## Optional AI configuration

Copy `.env.example` to a private `.env` in the repository root, then provide `OPENAI_API_KEY`. The default model is `gpt-image-2.5-sunburst`, configurable through `OPENAI_IMAGE_MODEL`. The server uses the [current Image API](https://developers.openai.com/api/reference/resources/images/methods/generate), requests PNG output, and returns validated image bytes as a data URL. Prompts accept 1–2000 Unicode characters.

Compose reads the root `.env` for interpolation. When launching Go directly, export the settings before starting the backend:

```sh
set -a
source .env
set +a
cd backend
go run .
```

A browser-side `REACT_APP_OPENAI_KEY` is no longer used. Real generation requires a valid server-side key and access to the configured model. Without a key the API returns an explicit 503 configuration message; uploads and other social features still work. The image endpoint has a per-instance budget of two initial requests and one additional request every 30 seconds. Verification uses a local simulated provider and does not make paid AI requests.

## Storage and deployment

Set `STORAGE_MODE=cloud` to use Elasticsearch and GCS. Configure `ES_URL`, optional `ES_USERNAME` / `ES_PASSWORD`, `GCS_BUCKET` and a private random `JWT_SIGNING_KEY` of at least 32 bytes. GCS uses Application Default Credentials. Configure allowed browser origins through `CORS_ORIGINS` when using a separately hosted frontend.

The default cloud indexes are `socialsphere_users_v2` and `socialsphere_posts_v2`. Old indexes containing plaintext passwords need a separate reviewed migration/password reset; this version never falls back to plaintext password comparison.

Configure a private GCS bucket; the application does not set public object ACLs. Public social media is streamed through opaque application media URLs; search and mutations require authentication. Actual bucket visibility depends on its cloud IAM and ACL configuration. Local file storage is single-process; cloud mode shares users, posts and media across instances. All instances must use the same private JWT signing key.

`backend/app.yaml` is a deployment template with cloud mode and the public bind address explicit. Supply private cloud values in an ignored `backend/app.private.yaml` copy before deploying. No cloud resources are created by this repository, and the template has not been deployed here. An API-only deployment also needs separately hosted frontend assets or the combined image.

JWTs expire after 24 hours. Signing out clears the browser's stored token; it does not revoke an already copied token on the stateless server.

## Tests and verification

```sh
cd backend
go test -race ./...
go vet ./...
go build .
```

```sh
cd frontend
npm ci
npm test -- --watchAll=false --runInBand
CI=true npm run build
```

Verified on 2026-11-04:

- 36 frontend tests passed across 4 suites; strict production build succeeded.
- 34 Go test functions and their parameterized subtests passed (60 pass events), including the race detector; vet and binary build succeeded.
- Live local HTTP checks passed for the combined UI/API, registration, JWT login, upload, recent search, media byte ranges, author-only deletion and actual development proxy forwarding.
- Restart preserved accounts, posts, media and the signing key. Owner deletion removed stored media. The application was stopped after testing.

Elasticsearch adapter tests use a local simulated HTTP service; GCS configuration and reader cleanup are tested locally. Neither adapter was connected to live cloud services. Docker and actual cloud deployment were not run; real AI generation was not called. See [修复说明](修复说明.zh-CN.md) for the original issues and implementation changes.
