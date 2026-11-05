const { createProxyMiddleware } = require("http-proxy-middleware");

module.exports = function (app) {
  app.use(createProxyMiddleware({
    pathFilter: "/api",
    target: process.env.API_PROXY_TARGET || "http://127.0.0.1:8080",
    changeOrigin: true,
    pathRewrite: { "^/api": "" },
  }));
};
