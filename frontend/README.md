# SocialSphere frontend

React 18, React Router 6, Material UI and Ant Design client for SocialSphere.

## Run locally

From this directory:

```sh
npm ci
cp .env.example .env.local
npm start
```

The default API base is `/api`. The development proxy sends `/api/*` to the Go server at `http://127.0.0.1:8080`, removing the `/api` prefix. Run the backend using the repository root instructions. For a remote API, set `REACT_APP_API_BASE` to its full API base. Relative media URLs are resolved against that API server rather than the frontend origin.

No OpenAI key belongs in frontend configuration. Image generation calls the authenticated backend; the backend alone reads `OPENAI_API_KEY` and chooses the image model. When AI credentials are absent, the backend error is shown and normal post features remain separate.

## User flow

Register a username and password, sign in, generate an image on Create, or open Collection to upload existing media. Use All, Keyword, User or My posts to filter the feed. Generated images are converted locally from the backend's PNG data URL into a file and posted through the normal multipart upload endpoint. The frontend never downloads an arbitrary external generated-image URL or forwards a provider key.

JPEG, PNG, GIF, MP4 and WebM uploads are limited to 10 MiB. A generated image uses its description as a caption, limited to the post's 2000-character maximum. Only the owner sees a photo's delete action; the backend independently checks ownership.

The client checks token expiry when restoring saved state, verifies the account with `GET /me`, and clears browser state on an authenticated 401. Sign out clears this browser's saved bearer token. Browser decoding is not signature verification; the backend validates every authenticated request.

## Tests and build

```sh
npm test -- --watchAll=false --runInBand
npm run build
```

Frontend regression tests mock the server and image services; they do not make real provider calls. They cover token restoration, error/expiry handling, search parameters, multipart MIME, local generated-image conversion and upload retries. Production hosting should provide the Go API and frontend under one origin or configure API routing and CORS explicitly.
