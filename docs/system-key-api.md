# NewAPI 系统 Key 第三方接口文档

## 概述

系统 Key 允许第三方服务安全地调用 NewAPI 的管理接口。通过系统 Key，第三方可以根据 OAuth OpenID 查询用户信息、管理用户的 API Key、查看可用模型等。

## 认证方式

所有接口都需要通过 `Authorization` 请求头携带系统 Key 进行认证：

```
Authorization: Bearer YOUR_SYSTEM_KEY
```

> 系统 Key 可在 NewAPI 后台 **设置 → 系统 Key 管理** 中创建。

---

## 公共参数

以下参数在多个接口中通用：

| 参数 | 类型 | 位置 | 必填 | 说明 |
|:---|:---|:---|:---|:---|
| `open_id` | string | Query / Body | ✅ | 用户在 OAuth Provider 中的唯一标识 |
| `provider_id` | int | Query / Body | ✅ | OAuth Provider 的 ID（对应 `custom_oauth_providers` 表的主键） |

## 公共响应格式

```json
{
  "success": true,
  "message": "",
  "data": { ... }
}
```

错误时 `success` 为 `false`，`message` 包含错误信息。

---

## 接口列表

### 1. 查询用户信息

获取用户的基本信息和账户余额。

- **URL**: `GET /api/external/user`
- **参数**: Query

| 参数 | 类型 | 必填 | 说明 |
|:---|:---|:---|:---|
| `open_id` | string | ✅ | 用户 OAuth OpenID |
| `provider_id` | int | ✅ | OAuth Provider ID |

- **响应示例**:

```json
{
  "success": true,
  "message": "",
  "data": {
    "user_id": 123,
    "username": "testuser",
    "display_name": "测试用户",
    "email": "test@example.com",
    "status": 1,
    "quota": 500000,
    "used_quota": 12000,
    "group": "default"
  }
}
```

| 字段 | 类型 | 说明 |
|:---|:---|:---|
| `user_id` | int | 用户 ID |
| `username` | string | 用户名 |
| `display_name` | string | 显示名称 |
| `email` | string | 邮箱 |
| `status` | int | 用户状态（1=正常，2=禁用） |
| `quota` | int | 总余额（内部单位） |
| `used_quota` | int | 已使用额度 |
| `group` | string | 用户分组 |

- **调用示例**:

```bash
curl -H "Authorization: Bearer sk-sys-xxxx" \
  "https://api.example.com/api/external/user?open_id=12345&provider_id=1"
```

---

### 2. 查询用户 API Key 列表

获取用户的所有 API Key 及使用信息。

- **URL**: `GET /api/external/user/tokens`
- **参数**: Query

| 参数 | 类型 | 必填 | 说明 |
|:---|:---|:---|:---|
| `open_id` | string | ✅ | 用户 OAuth OpenID |
| `provider_id` | int | ✅ | OAuth Provider ID |

- **响应示例**:

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 1,
      "name": "默认令牌",
      "key": "sk-xxxxxxxxxxxxxxxx",
      "status": 1,
      "created_time": 1709640000,
      "expired_time": -1,
      "remain_quota": 0,
      "used_quota": 5000,
      "unlimited_quota": true
    }
  ]
}
```

| 字段 | 类型 | 说明 |
|:---|:---|:---|
| `id` | int | 令牌 ID |
| `name` | string | 令牌名称 |
| `key` | string | 完整 API Key（以 `sk-` 开头） |
| `status` | int | 状态（1=启用，2=禁用） |
| `created_time` | int64 | 创建时间（Unix 时间戳，秒） |
| `expired_time` | int64 | 过期时间（-1=永不过期） |
| `remain_quota` | int | 剩余额度 |
| `used_quota` | int | 已使用额度 |
| `unlimited_quota` | bool | 是否为无限额度 |

- **调用示例**:

```bash
curl -H "Authorization: Bearer sk-sys-xxxx" \
  "https://api.example.com/api/external/user/tokens?open_id=12345&provider_id=1"
```

---

### 3. 查询用户可用模型列表

获取用户分组可用的所有模型。

- **URL**: `GET /api/external/user/models`
- **参数**: Query

| 参数 | 类型 | 必填 | 说明 |
|:---|:---|:---|:---|
| `open_id` | string | ✅ | 用户 OAuth OpenID |
| `provider_id` | int | ✅ | OAuth Provider ID |

- **响应示例**:

```json
{
  "success": true,
  "message": "",
  "data": [
    "gpt-4o",
    "gpt-4o-mini",
    "claude-3-5-sonnet",
    "deepseek-chat"
  ]
}
```

- **调用示例**:

```bash
curl -H "Authorization: Bearer sk-sys-xxxx" \
  "https://api.example.com/api/external/user/models?open_id=12345&provider_id=1"
```

---

### 4. 为用户创建 API Key

为指定用户创建一个新的 API Key。

- **URL**: `POST /api/external/user/tokens`
- **参数**: JSON Body

| 参数 | 类型 | 必填 | 说明 |
|:---|:---|:---|:---|
| `open_id` | string | ✅ | 用户 OAuth OpenID |
| `provider_id` | int | ✅ | OAuth Provider ID |
| `name` | string | ✅ | 令牌名称（最长50字符） |
| `expired_time` | int64 | ❌ | 过期时间戳（秒）。`-1`=永不过期，`0`=默认（永不过期） |
| `unlimited_quota` | bool | ❌ | 是否无限额度，默认 `false` |
| `remain_quota` | int | ❌ | 剩余额度（仅 `unlimited_quota=false` 时生效） |

- **请求示例**:

```json
{
  "open_id": "12345",
  "provider_id": 1,
  "name": "我的API Key",
  "unlimited_quota": true,
  "expired_time": -1
}
```

- **响应示例**:

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 42,
    "key": "sk-xxxxxxxxxxxxxxxx"
  }
}
```

> ⚠️ 创建成功后返回的 `key` 是完整的 API Key，请妥善保管。

- **调用示例**:

```bash
curl -X POST -H "Authorization: Bearer sk-sys-xxxx" \
  -H "Content-Type: application/json" \
  -d '{"open_id":"12345","provider_id":1,"name":"测试Key","unlimited_quota":true}' \
  "https://api.example.com/api/external/user/tokens"
```

---

### 5. 删除用户的 API Key

删除指定用户的一个 API Key。

- **URL**: `DELETE /api/external/user/tokens/:token_id`
- **参数**: Path + Query

| 参数 | 类型 | 位置 | 必填 | 说明 |
|:---|:---|:---|:---|:---|
| `token_id` | int | Path | ✅ | 要删除的令牌 ID |
| `open_id` | string | Query | ✅ | 用户 OAuth OpenID |
| `provider_id` | int | Query | ✅ | OAuth Provider ID |

- **响应示例**:

```json
{
  "success": true,
  "message": ""
}
```

> 只能删除属于该用户的令牌，不能跨用户删除。

- **调用示例**:

```bash
curl -X DELETE -H "Authorization: Bearer sk-sys-xxxx" \
  "https://api.example.com/api/external/user/tokens/42?open_id=12345&provider_id=1"
```

---

## 错误码说明

| HTTP 状态码 | 说明 |
|:---|:---|
| `200` | 成功（检查 `success` 字段确认业务结果） |
| `400` | 请求参数错误（缺少必填参数、格式不正确等） |
| `401` | 系统 Key 无效或已过期 |
| `404` | 未找到对应用户（OpenID + ProviderId 无匹配） |
| `500` | 服务器内部错误 |

## 注意事项

1. **系统 Key 有效期**：系统 Key 可设置有效期，过期后将无法调用任何接口
2. **调用日志**：所有接口调用均会记录日志，可在后台 **系统 Key 管理 → 调用日志** 中查看
3. **用户定位**：用户通过 `open_id` + `provider_id` 组合唯一定位
4. **额度单位**：`quota` 和 `used_quota` 为系统内部单位，具体含义取决于系统配置
