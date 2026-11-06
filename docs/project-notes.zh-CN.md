# SocialSphere：简历对应与演示准备

SocialSphere 是原 Social AI 项目的展示名称。历史 demo 中出现的 Social AI、当前页面和仓库中的 SocialSphere，指的是同一个项目。建议仓库、README、简历和新录屏统一使用 **SocialSphere**，历史录屏保留原画面，并注明当时名称。

本说明把 2025 年的原项目经历与 2026 年的维护分开记录。作者已确认：原项目曾成功部署到 Google Cloud，目前网站已下线。简历保留历史部署经历，README 明确当前托管状态，并提供本地运行与演示方式。

## 三条简历内容如何对应代码

简历项目标题为 **SocialSphere: AI-Powered Social Networking Platform · Aug 2025**；原技术栈为 React、Go、JWT、React Router、Elasticsearch、Google Cloud、OpenAI API、DALL·E 3。下表按用户提供的三条原句及原顺序逐条对应。

| 简历内容 | 可对应的功能与代码 | 建议表述与当前差异 |
| --- | --- | --- |
| **1. ReactJS 社交应用、DALL·E 3 辅助创建图片帖、Go 后端、Google Cloud 部署。** 原句将主要产品功能和部署放在同一条。 | 原始项目使用 DALL·E 3；当前 [Landing.js](../frontend/src/components/Landing.js) 负责图片预览和发布，Go [main.go](../backend/main.go) 启动 API；[app.yaml](../backend/app.yaml) 保留 Google App Engine flex 模板，[GCS adapter](../backend/backend/gsc.go) 保留云媒体存储。 | 保留 React、Go、2025 年 DALL·E 3 和作者确认的 Google Cloud 部署经历，注明网站目前已下线。维护版把 AI 调用移到认证后端并配置模型，不能倒填到 2025 年经历。当前演示使用本地运行方式。 |
| **2. 注册、登录、退出流程、React Router v4、服务端 JWT 认证。** 原句为 “Improved the authentication using token-based registration/login/logout flow with React Router v4 and server-side user authentication with JWT.” | React 登录、注册和受保护页面见 [Main.js](../frontend/src/components/Main.js)；Go 的 `/signup`、`/signin`、`/me` 见 [user.go](../backend/handler/user.go)；JWT 验证见 [auth.go](../backend/handler/auth.go)；本地 token 管理见 [api.js](../frontend/src/api.js)。 | 建议把笼统的 “token-based registration/login/logout” 改为“账户注册、JWT 登录、前端退出”。注册创建账户，登录验证凭据并签发 token，退出清除浏览器 token，并不在服务端吊销 JWT。当前依赖和原始代码快照都使用 React Router 6，因此删除 v4，写 **React Router** 即可。“Improved” 没有比较依据时可改为 “Implemented”。 |
| **3. 使用 Elasticsearch 搜索最近帖子、列出个人帖子。** 原句为 “Implemented functions for searching recent posts and listing personal posts using Elasticsearch.” | [Elasticsearch adapter](../backend/backend/elasticsearch.go) 按创建时间排序、支持关键词搜索和用户名过滤；[Collection.js](../frontend/src/components/Collection.js) 与 [api.js](../frontend/src/api.js) 提供最近帖子、关键词、用户名和个人帖子筛选。 | 保留这条原项目经历。当前预览默认使用 [LocalStore](../backend/backend/local.go)，由 `STORAGE_MODE=cloud` 切换至 Elasticsearch/GCS；所以本地录屏里的搜索不要称为正在访问 Elasticsearch。cloud adapter 仍然保留，local 模式并非移除搜索架构。 |

完整当前架构见 [architecture.md](architecture.md)。

维护版的 [ImageGenerator](../backend/service/image.go) 使用可配置的服务端模型，默认 `gpt-image-2.5-sunburst` 已按 [OpenAI Image API 官方参考](https://developers.openai.com/api/reference/resources/images/methods/generate)核对支持；[DALL·E 3 官方页面](https://developers.openai.com/api/docs/models/dall-e-3)将旧模型标记为 removed。页面现增加模型选择及临时个人密钥输入，支持 GPT Image 2、2.5 和 2.6 系列。个人密钥功能由服务端 `ALLOW_USER_IMAGE_KEYS=true` 显式开启，密钥只保留在页面内存和单次后端请求中，不持久化。它不会提供免费的密钥或额度；真实生成仍需有效的 OpenAI API 密钥、模型权限和额度。上述现状与 2025 年简历中保留 DALL·E 3 的历史并不冲突。

### JWT 的准确解释

1. **Register**：创建账户；当前维护版先验证输入，再保存 BCrypt 密码哈希。注册接口本身不签发 JWT。
2. **Login**：验证用户名和密码，成功后签发 24 小时有效的 HS256 JWT。
3. **Authenticated requests**：浏览器通过 `Authorization: Bearer ...` 发送 token；后端验证签名及身份、时间等 claims。
4. **Restore login**：浏览器读到已保存的 token 后，通过 `/me` 向后端复核。
5. **Logout**：清除浏览器本地 token 和登录状态；服务端没有 token 黑名单或撤销机制，已经复制到其他地方的有效 token 不会因此立即失效。

前三类项目经历能与现有代码对应，但下面这些具体实现应作为 **2026 年维护** 记录，除非有独立证据说明 2025 年已经实现：BCrypt 替代原明文存储、严格 JWT 校验、作者删除权限与媒体清理、上传大小和内容验证、本地持久化适配器、服务端 AI 调用及限流、回归测试和组合构建。不要把维护新增内容都回填到 2025 年的简历。

## 建议的英文简历 bullet

以下版本保留原项目的核心技术和历史，不添加性能百分比、用户量、吞吐或当前云端在线状态。项目日期可继续按实际经历写 **Aug 2025**。

- Built a social networking web application with React and Go, integrated OpenAI’s DALL·E 3 to help users create image posts, and deployed the application on Google Cloud.
- Implemented account registration and JWT-based login with protected navigation using React Router, and client-side logout that clears the stored token.
- Implemented recent-post search and personal-post listings with Elasticsearch.

如果简历篇幅有限，技术栈行可写：`React · Go · JWT · React Router · Elasticsearch · Google Cloud · OpenAI API · DALL·E 3`。不要写 React Router v4，也不要把当前维护版的默认模型替换进 2025 年技术栈。

## 建议的 GitHub Description

> A React and Go social app with JWT authentication, image/video sharing, Elasticsearch search, Google Cloud Storage, and AI image generation.

这个描述概括项目的完整实现；本地/云模式、历史 demo 和当前 AI 配置在 README 正文里解释。Description 不需要列模型版本，也不暗示存在当前可访问的云端地址。

## 新 demo：约 105 秒

当前本地预览地址：[http://localhost:8080](http://localhost:8080)。新录屏展示当前页面与真实本地社交流程；在尚未提供有效密钥时，不包含真实 AI 生成请求。可以先展示 Image settings 中的模型选择和个人密钥入口，避免把真实密钥录入视频。录制前准备一个演示账户、一个小于 10 MiB 的图片、一段支持的短视频，以及容易辨认的帖子文字；如果已有演示内容，可直接展示，减少等待。

| 时间 | 操作 | 可直接使用的英文旁白 |
| --- | --- | --- |
| 0–8 秒 | 展示 SocialSphere 标题和登录页。 | “SocialSphere is a social application built with React and Go for sharing images, videos, and AI-generated artwork.” |
| 8–23 秒 | 展示注册表单，然后用演示账户登录；进入 Create 页面。 | “Users can create an account and sign in. The Go backend issues a JWT, which protects account actions and API requests.” |
| 23–43 秒 | 打开 Collection → Create New Post，选择图片、输入简短描述并提交；展示新帖。 | “In the collection, I can upload an image with a caption and see the new post immediately.” |
| 43–58 秒 | 搜索刚才描述中的关键词，再切换用户名或自己的帖子筛选。 | “Posts can be filtered by text, username, or my own account. The cloud configuration uses Elasticsearch for search.” |
| 58–73 秒 | 切换 Videos，播放一段演示短视频；必要时提前准备好视频帖。 | “The same publishing workflow also supports video posts, with playback directly in the collection.” |
| 73–83 秒 | 返回 Images，打开自己的图片并删除一条专门用于删除演示的帖子。 | “Authors can delete their own posts. The backend checks ownership and removes the stored media.” |
| 83–95 秒 | 回到 Create，展示 prompt 输入和图像预览入口；不展示虚构的生成结果。 | “The original 2025 project used DALL·E 3. This maintained version sends image requests through the authenticated backend; this local recording does not make a live AI request.” |
| 95–105 秒 | 展示当前用户名，点击 Log out，回到登录页。 | “This preview uses persistent local storage. The project also includes Elasticsearch and Google Cloud Storage adapters. Signing out clears the browser's token.” |

说明：视频里的搜索操作在当前预览中实际使用 local store，旁白只说明 cloud 配置使用 Elasticsearch，不把本地搜索演示说成实时访问云端。

### 如需展示历史 AI 生成效果

可以把 83–95 秒替换为历史录屏的约 12 秒片段，明确标注：

> Original 2025 demo · DALL·E 3 · Former project name: Social AI

转入历史片段前说：“Here is the image-generation workflow from the original 2025 demo.” 片段结束后再回到当前 SocialSphere 页面。历史片段展示当时流程；它不构成当前模型、当前 key 或当前云资源仍可用的证明。

新录屏已通过 [Google Drive 分享链接](https://drive.google.com/file/d/1H6R5kVk7SKsjU4kpb23TPljHFBYTDQaB/view?usp=sharing)放入 README 的 Demo 区域。原始录屏文件保留作参考。当前没有 key 时，如需验证配置错误处理，可单独演示一次生成请求收到的配置提示；不要把这个提示剪成一次成功生成。
