# 自建人机验证（Cloudflare Turnstile 兼容）

本项目已内置 Cloudflare Turnstile 支持。若你使用**自建的、Cloudflare 兼容**的
人机验证服务，可零改造复用这套接线，无需引入并行机制。

## 工作原理

- 后端：`middleware/turnstile-check.go` 的 `TurnstileCheck()` 会把前端提交的
  `?turnstile=<token>` 用表单方式 POST 到 siteverify 端点，读取返回的 `success`。
  siteverify 端点由 `TURNSTILE_VERIFY_URL` 决定，**默认 Cloudflare 官方地址**，
  留空即与上游完全一致，可随时回退。
- 前端：`web/src/components/turnstile.tsx` 根据“Turnstile 站点密钥”加载对应的
  `widget.js` 并渲染验证组件；用户通过后回传一次性 token，随注册/登录等请求带上。

## 启用步骤

1. 在后台「系统设置 → 通用/安全」开启 Turnstile 校验，并填写：
   - **站点密钥（site key）**：你的验证服务地址（例如 `https://verify.example.com`）。
     前端据此加载 `<站点密钥>/widget.js`。
   - **私钥（secret key）**：与验证服务共享的密钥（用于 siteverify 校验签名）。
2. 部署时设置环境变量，指向你的 siteverify 端点（保持 Cloudflare 响应结构 `{success}`）：

   ```env
   TURNSTILE_VERIFY_URL=https://verify.example.com/turnstile/v0/siteverify
   ```

   不设置该变量时，siteverify 仍走 Cloudflare 官方端点（默认行为）。

## 生效范围

沿用现有 `TurnstileCheck()` 中间件的挂载点（见 `router/api-router.go`）：
注册、发送邮箱验证码、登录、找回密码、签到。

## 回退

删除 `TURNSTILE_VERIFY_URL` 并在站点密钥/私钥处填回 Cloudflare 的值即可恢复官方 Turnstile。
