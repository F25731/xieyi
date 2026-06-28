# 视频解析接口调用文档

本文档面向调用方。你只需要使用本站提供的 API Key 调用 OpenAI 兼容接口即可。

## 接口信息

```text
Base URL: https://api.zmoapi.cn/v1
Endpoint: /chat/completions
Method: POST
模型: video-parse
```

完整请求地址：

```text
https://api.zmoapi.cn/v1/chat/completions
```

## 鉴权

在请求头中携带你的 API Key：

```http
Authorization: Bearer sk-你的密钥
Content-Type: application/json
```

## 支持平台

接口会自动识别分享链接所属平台，目前支持：

- 抖音
- 快手
- 小红书
- 哔哩哔哩
- 微博
- TikTok
- YouTube
- 虎牙
- 今日头条
- 微信视频号
- 最右
- 皮皮搞笑
- 豆包/即梦/千问等媒体链接

## 请求参数

使用 OpenAI 兼容的 `chat/completions` 格式。

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 固定填 `video-parse` |
| `messages` | array | 是 | OpenAI 兼容消息数组 |
| `messages[].role` | string | 是 | 用户消息填 `user` |
| `messages[].content` | string | 是 | 分享链接或完整分享文本 |

## curl 示例

```bash
curl https://api.zmoapi.cn/v1/chat/completions \
  -H "Authorization: Bearer sk-你的密钥" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "video-parse",
    "messages": [
      {
        "role": "user",
        "content": "复制打开抖音 https://v.douyin.com/xxxxxx/"
      }
    ]
  }'
```

## JavaScript 示例

```javascript
const response = await fetch("https://api.zmoapi.cn/v1/chat/completions", {
  method: "POST",
  headers: {
    "Authorization": "Bearer sk-你的密钥",
    "Content-Type": "application/json"
  },
  body: JSON.stringify({
    model: "video-parse",
    messages: [
      {
        role: "user",
        content: "复制出来的平台分享文本或链接"
      }
    ]
  })
});

const data = await response.json();
const result = JSON.parse(data.choices[0].message.content);
console.log(result.normalized);
```

## Python 示例

```python
import json
import requests

resp = requests.post(
    "https://api.zmoapi.cn/v1/chat/completions",
    headers={
        "Authorization": "Bearer sk-你的密钥",
        "Content-Type": "application/json",
    },
    json={
        "model": "video-parse",
        "messages": [
            {
                "role": "user",
                "content": "复制出来的平台分享文本或链接",
            }
        ],
    },
    timeout=60,
)

data = resp.json()
result = json.loads(data["choices"][0]["message"]["content"])
print(result["normalized"])
```

## 返回格式

接口返回 OpenAI 兼容结构：

```json
{
  "id": "chatcmpl-xxx",
  "object": "chat.completion",
  "created": 1782657107,
  "model": "video-parse",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "{...解析结果 JSON 字符串...}"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 1,
    "completion_tokens": 1,
    "total_tokens": 2
  }
}
```

真正的视频解析结果在：

```text
choices[0].message.content
```

注意：`content` 是一个 JSON 字符串，使用时需要再次 `JSON.parse`。

## 解析结果结构

`choices[0].message.content` 解析后结构如下：

```json
{
  "ok": true,
  "status": 200,
  "api": {
    "id": "dy",
    "name": "抖音视频图集解析",
    "group": "国内短视频",
    "method": "GET"
  },
  "input": {
    "originalUrl": "用户提交的原始文本",
    "normalizedUrl": "https://v.douyin.com/xxxxxx/",
    "extracted": true,
    "candidates": [
      "https://v.douyin.com/xxxxxx/"
    ],
    "autoDetected": true
  },
  "upstream": {
    "code": 200,
    "msg": "解析成功",
    "data": {}
  },
  "normalized": {
    "title": "作品标题",
    "author": "作者",
    "avatar": "作者头像",
    "cover": "封面图",
    "videos": [],
    "images": [],
    "audios": [],
    "links": []
  },
  "durationMs": 2865
}
```

## 常用字段说明

| 字段 | 说明 |
| --- | --- |
| `ok` | 是否解析成功 |
| `status` | 上游请求 HTTP 状态码 |
| `api.id` | 自动识别到的平台 ID |
| `input.normalizedUrl` | 从分享文本中提取出的真实分享链接 |
| `upstream` | 上游返回的原始数据 |
| `normalized` | 统一整理后的结果 |
| `normalized.title` | 标题 |
| `normalized.cover` | 封面 |
| `normalized.videos` | 视频链接列表 |
| `normalized.images` | 图片链接列表 |
| `normalized.audios` | 音频链接列表 |

## 视频链接字段

`normalized.videos` 中每一项结构：

```json
{
  "label": "live_photo 1",
  "url": "https://example.com/video.mp4",
  "type": "video",
  "filename": "video_1_live_photo_1"
}
```

## 图片链接字段

`normalized.images` 中每一项结构：

```json
{
  "label": "images 1",
  "url": "https://example.com/image.webp",
  "type": "image",
  "filename": "image_1_images_1"
}
```

## 固定平台模型

一般直接使用自动识别模型即可：

```text
video-parse
```

如需固定平台，也可以使用：

| 模型 | 平台 |
| --- | --- |
| `video-parse-dy` | 抖音 |
| `video-parse-ks` | 快手 |
| `video-parse-xhs` | 小红书 |
| `video-parse-bilibili` | 哔哩哔哩 |
| `video-parse-weibo` | 微博 |
| `video-parse-tiktok` | TikTok |
| `video-parse-youtube` | YouTube |

## 错误示例

解析失败时，`choices[0].message.content` 中的 `ok` 会是 `false`：

```json
{
  "ok": false,
  "status": 400,
  "error": "cannot detect platform from input"
}
```

常见原因：

- 分享文本里没有链接
- 链接平台暂不支持
- API Key 无效或余额不足
- 原平台链接失效
- 上游解析服务暂时不可用

## 注意事项

- 请传入完整分享文本或完整 URL。
- 部分平台返回的是图文，视频数组可能为空，图片在 `normalized.images`。
- 部分抖音 Live Photo 会返回到 `normalized.videos`。
- 直链通常带有效期，请及时使用。
- 不要把 API Key 暴露在前端公开代码中。
