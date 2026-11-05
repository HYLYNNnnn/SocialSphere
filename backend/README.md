# SocialSphere API

Go HTTP API for registration, JWT login, image/video posts, search, owner-only deletion and server-side image generation.

## Start locally

```sh
go run .
```

The default address is `127.0.0.1:8080`. Local mode creates a private persistent `data/` directory. No cloud credentials are needed. The process reads environment variables; it does not automatically read a `.env` file. See the repository root README for exporting a private `.env` and for running the frontend.

## Configuration

| Variable | Default / purpose |
| --- | --- |
| `HOST`, `PORT` | `127.0.0.1`, `8080`; container template explicitly binds `0.0.0.0` |
| `STORAGE_MODE` | `local` or `cloud` |
| `DATA_DIR` | `./data`, for single-process local persistence |
| `FRONTEND_DIR` | `./public`, optional production frontend assets |
| `JWT_SIGNING_KEY` | Private random value, minimum 32 bytes; auto-generated privately only in local mode |
| `OPENAI_API_KEY` | Optional server-only key; absent means generation returns 503 |
| `OPENAI_IMAGE_MODEL` | `gpt-image-2.5-sunburst` by default |
| `ES_URL`, `ES_USERNAME`, `ES_PASSWORD` | Elasticsearch configuration for cloud mode |
| `ES_USER_INDEX`, `ES_POST_INDEX` | New v2 indexes by default |
| `GCS_BUCKET` | Cloud media bucket configured as private, with ADC |
| `CORS_ORIGINS` | Explicit allowed frontend origins; local port 3000 by default |

## Routes

Both `/api/...` and the original root paths are supported.

| Method | Path | Authentication / behavior |
| --- | --- | --- |
| POST | `/signup` | Register username/password; optional age/gender; returns 201 `{username}` |
| POST | `/signin` | Returns `{token, username}` |
| GET | `/me` | Bearer JWT; safe current username |
| POST | `/upload` | Bearer JWT; multipart `message` + `media_file`; returns 201 Post |
| GET | `/search` | Bearer JWT; optional user/keywords/limit; recent-first JSON array, max 100 |
| DELETE | `/post/{id}` | Bearer JWT; author only; returns 204 |
| POST | `/generate-image` | Bearer JWT; `{prompt}`; returns `{image_url}` data URL |
| GET / HEAD | `/media/{id}` | Public social media with byte-range support |
| GET | `/health` | Liveness response |

Usernames are normalized to lowercase and use 3–32 letters, digits or underscores, beginning with a letter or digit. Passwords require at least 8 Unicode characters and at most 72 UTF-8 bytes; only BCrypt hashes are stored. Uploaded images are decoded and bounded in pixel count; video formats are checked by content signatures. Each upload is limited to 10 MiB.

If metadata creation fails after media upload, the service attempts compensating media cleanup with an independent timeout. Deletion removes media before metadata so the author can retry if the index operation fails. This is not a distributed transaction across cloud stores.

Cloud mode fails startup when required settings are missing. The deployment template is API-oriented and requires private configuration. Keep cloud credential files outside source control; the shared JWT key must be consistent across instances. Existing plaintext user records need a separate migration/password reset.

## Verification

`go test -race ./...`, `go vet ./...` and `go build .` passed. HTTP integration and restart checks passed using isolated local persistence. Elasticsearch adapter tests use `httptest`; GCS configuration and reader cleanup use local test doubles. They do not access a live bucket or Elasticsearch cluster.
