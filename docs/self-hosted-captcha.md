# 自建人机验证（Cloudflare Turnstile 兼容）

NewAPI 的 Turnstile 接线同时支持 Cloudflare 官方服务和自建的兼容服务。

## 88API 当前配置

- 在后台开启 Turnstile 校验。
- “Turnstile 站点密钥”填写自建验证服务地址，例如 `https://verify.88api.ai`。前端会加载 `<地址>/widget.js`，并通过 `Captcha88.render()` 渲染勾选、行为验证、点击或滑块挑战。
- “Turnstile 私钥”填写与验证服务共享的核销密钥。
- 部署环境设置：

  ```env
  TURNSTILE_VERIFY_URL=https://verify.88api.ai/turnstile/v0/siteverify
  ```

后端会把前端提交的 `turnstile` 一次性 token 以 Cloudflare 兼容表单发送到该地址，并读取响应中的 `success`。

## 官方 Cloudflare 回退

如果站点密钥不是 HTTP(S) URL，前端按标准 Cloudflare site key 处理并加载官方 Turnstile 脚本。不设置 `TURNSTILE_VERIFY_URL` 时，后端默认使用 Cloudflare 官方 siteverify 地址。

## 受保护入口

注册、邮箱验证码、密码登录、找回密码、标准 OAuth 登录、微信和 Telegram 登录均复用这套校验。OAuth 账号绑定依赖已有登录会话，不额外要求挑战。
