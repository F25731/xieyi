# NewAPI Video Wrapper

给 NewAPI 用的视频解析上游适配器。客户仍然调用 NewAPI 的 OpenAI 兼容接口，NewAPI 把请求转发到本服务，本服务自动识别分享链接平台并调用上游解析接口。

## 运行

```bash
go run .
```

默认地址：

- 后台：`http://127.0.0.1:18788/admin`
- NewAPI Base URL：`http://127.0.0.1:18788/v1`
- 默认后台密码：`Fyb2530+`

## NewAPI 配置

在 NewAPI 里新增 OpenAI 兼容渠道：

- Base URL：`http://127.0.0.1:18788/v1`
- 模型：`video-parse`
- 密钥：后台里的 `Wrapper Secret`

客户调用时，message 放分享链接或复制出来的分享文本即可。

请求示例：

```bash
curl http://127.0.0.1:18788/v1/chat/completions \
  -H "Authorization: Bearer 后台里的WrapperSecret" \
  -H "Content-Type: application/json" \
  -d '{"model":"video-parse","messages":[{"role":"user","content":"复制出来的抖音/快手/小红书分享链接"}]}'
```

固定平台模型也支持：

- `video-parse-dy`
- `video-parse-ks`
- `video-parse-xhs`
- `video-parse-bilibili`
- `video-parse-youtube`

## 默认并发

默认 `128` 个 worker，队列 `10000`，适合大服务器和网络 I/O 型上游请求。后台可修改，worker 数和队列长度修改后需要重启生效。

## Docker

```bash
docker compose up -d --build
```

## 返回

`/v1/chat/completions` 返回标准 OpenAI 兼容结构，解析结果在：

```text
choices[0].message.content
```

内容是 JSON 字符串，包含 `api`、`input`、`upstream`、`normalized`。
