# YAO 事件和 MQTT 配置指南

本指南介绍如何在 YAO 中配置和使用 MQTT 消息队列和本地事件系统。

## 目录

- [MQTT 配置](#mqtt-配置)
- [本地事件配置](#本地事件配置)
- [脚本示例](#脚本示例)
- [最佳实践](#最佳实践)

---

## MQTT 配置

### 配置文件位置

将 MQTT 配置文件放在 `mqs/` 目录中，文件名格式为 `*.mqtt.yao` 或 `*.mqtt.json`。

### 配置示例

`mqs/test.mqtt.yao`:

```json
{
  "name": "test",
  "broker": "tcp://127.0.0.1:1883",
  "client_id": "yao_subscriber",
  "username": "admin",
  "password": "emqx_DQzGkH",
  "subscribes": [
    {
      "topic": "test/topic",
      "qos": 1,
      "process": "scripts.custom.mqtt.receive"
    },
    {
      "topic": "device",
      "qos": 0,
      "process": "scripts.custom.mqtt.receive"
    }
  ]
}
```

### 配置参数说明

| 参数 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `name` | string | ✓ | MQTT 连接别名 |
| `broker` | string | ✓ | Broker 地址（格式: `tcp://host:port`） |
| `client_id` | string | ✓ | MQTT 客户端 ID |
| `username` | string | ✓ | 认证用户名 |
| `password` | string | ✓ | 认证密码 |
| `subscribes` | array | ✓ | 订阅主题配置列表 |

#### 订阅配置参数

| 参数 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `topic` | string | ✓ | 订阅的主题名 |
| `qos` | number | ✓ | 服务质量等级（0、1、2） |
| `process` | string | ✓ | 消息处理 process 名称 |

### MQTT 脚本处理

创建脚本文件 `scripts/custom/mqtt.yao`:

```javascript
/**
 * MQTT 消息接收处理
 * @param {string} topic - 消息主题
 * @param {object} params - 消息负载
 * @param {number} ts - 时间戳
 * @returns {string} 处理结果（可选，返回 base64 编码字符串）
 */
function receive(topic, params, ts) {
    log.Info("mqtt params", JSON.stringify(params));
    // log.Info(JSON.parse(params).payload);
    return "eyJtc2ciOiJzdWNjZXNzIn0="
}

/**
 * 启动 MQTT 发送示例
 */
function start() {
    let res = Process("mqtt.publish", "test", "device", {
        "msg": "来自yao客户端"
    });
    return res;
}

/**
 * 直接调用插件发送 MQTT 消息
 * @param {object} params - 发送参数
 */
function send(params) {
    let res = Process("plugins.mqtt.publish", {
        "broker": "tcp://192.168.80.130:1883",
        "topic": "mqtt",
        "username": "admin",
        "password": "emqx_DQzGkH",
        "payload": {
            "set_points": {
                "00001": 1,
                "00002": 1,
                "00003": 1,
                "00004": 1,
            }
        },
        "clientId": "mqtt",
        "qos": 1
    });
    return res;
}
```

---

## 本地事件配置

### 配置文件位置

将事件配置文件放在 `mqs/` 目录中，文件名格式为 `*.event.yao` 或 `*.event.json`。

### 配置示例

`mqs/local.event.yao`:

```json
{
  "event": "LOCAL_EV",
  "process": "scripts.custom.event.onEvent",
  "max_workers": 100,
  "reserved_workers": 10,
  "queue_size": 2000
}
```

### 配置参数说明

| 参数 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `event` | string | ✓ | 事件名称（格式: `前缀.事件名`，本例为 `LOCAL_EV`） |
| `process` | string | ✓ | 事件处理 process 名称 |
| `max_workers` | number | ✗ | 最大工作线程数（默认: 256） |
| `reserved_workers` | number | ✗ | 预留工作线程数（默认: 32） |
| `queue_size` | number | ✗ | 事件队列大小（默认: 4096） |

### 本地事件脚本处理

创建脚本文件 `scripts/custom/event.ts`:

```javascript
/**
 * 事件处理函数
 * 当本地事件被发送时触发
 */
function onEvent(...args) {
    log.Info("onEvent: ", args);
}

/**
 * 发送本地事件示例
 * 触发 LOCAL_EV 事件
 */
function send() {
    Process("event.publish", "LOCAL_EV", { "msg": "Hello Event: LOCAL_EV" })
    return ""
}

/**
 * 发送 SUB_LOCAL_EV 事件示例
 */
function send2() {
    Process("event.publish", "SUB_LOCAL_EV", { "msg": "Hello Event: SUB_LOCAL_EV" })
    return ""
}

/**
 * 动态订阅事件示例
 * 订阅 SUB_LOCAL_EV 事件，指定处理函数为 onEvent
 * @returns {string} 订阅ID，用于后续取消订阅
 */
function subscribe() {
    let uid = Process("event.subscribe", "SUB_LOCAL_EV", "scripts.custom.event.onEvent");
    return uid
}

/**
 * 取消订阅事件示例
 * 根据订阅ID取消事件订阅
 * @param {object} params - 参数对象，包含订阅ID
 * @param {string} params.id - 订阅ID
 * @returns {boolean} 取消订阅是否成功
 */
function unSubscribe(params) {
    let res = Process("event.unSubscribe", params.id);
    return res
}

/**
 * 发送本地事件示例（旧版本）
 * 触发 LOCAL_EV 事件 10 次
 */
function sendOld() {
    for (let i = 1; i < 10; i++) {
        Process("event.publish", "LOCAL_EV", { "msg": "Hello Event " + i })
    }
    return ""
}
```

---

## 脚本示例

### 1. MQTT 消息发送

在任何脚本中发送 MQTT 消息：

```javascript
// 使用已配置的 MQTT 连接发送
let result = Process("mqtt.publish", "test", "device", {
    "msg": "来自yao客户端",
    "timestamp": Date.now()
});

log.Info("MQTT publish result", result);
```

### 2. 本地事件发送

在任何脚本中发送本地事件：

```javascript
// 发送单个事件
Process("event.publish", "LOCAL_EV", { 
    "msg": "Hello Event",
    "data": {...}
});

// 批量发送事件
for (let i = 1; i <= 10; i++) {
    Process("event.publish", "LOCAL_EV", { 
        "msg": `Event ${i}`,
        "index": i
    });
}
```

### 3. 动态事件订阅与取消订阅

```javascript
// 动态订阅事件
let subscriptionId = Process("event.subscribe", "USER_LOGIN_EVENT", "scripts.user.onLogin");

// 发送事件触发订阅的处理函数
Process("event.publish", "USER_LOGIN_EVENT", { 
    "user_id": 123, 
    "username": "john_doe" 
});

// 取消订阅事件
let unsubscribeResult = Process("event.unSubscribe", subscriptionId);
log.Info("Unsubscribe result", unsubscribeResult);
```

### 4. MQTT 消息接收与处理

配置的 process 会自动被调用：

```javascript
/**
 * 处理 MQTT 消息
 * 框架会自动调用此函数
 */
function receive(topic, params, ts) {
    log.Info("Topic", topic);
    log.Info("Params", JSON.stringify(params));
    log.Info("Timestamp", ts);
    
    // 解析消息负载
    try {
        let payload = JSON.parse(params);
        // 处理业务逻辑
        log.Info("Processed payload", payload);
    } catch (e) {
        log.Error("Parse error", e);
    }
    
    // 返回结果（可选，base64 编码）
    return "eyJzdGF0dXMiOiJvayJ9";
}
```

### 5. 事件处理函数

```javascript
/**
 * 处理本地事件
 * 框架会自动调用此函数
 */
function onEvent(...args) {
    log.Info("Event data", args);
    
    // 执行业务逻辑
    // 与 MQTT 集成、数据库操作等
    
    // 可选：发送 MQTT 消息作为响应
    Process("mqtt.publish", "test", "response", {
        "original_event": args[0],
        "status": "processed"
    });
}
```

---

### 1. 性能优化

- **资源配置**：根据消息吞吐量调整 `max_workers` 和 `queue_size`
- **高频处理**：设置较高的 `max_workers` 和 `queue_size`
- **低频处理**：使用较小的资源配置以节省内存

```json
{
  "event": "LOCAL_HIGH_FREQ",
  "process": "scripts.custom.event.onHighFreq",
  "max_workers": 200,
  "queue_size": 5000
}
```

### 2. 错误处理

```javascript
function receive(topic, params, ts) {
    try {
        // 处理消息
        log.Info("Processing", topic);
    } catch (e) {
        log.Error("Error processing message", e);
        // 返回错误状态
        return "eyJlcnJvciI6IlByb2Nlc3MgZmFpbGVkIn0=";
    }
}
```

### 3. 日志记录

```javascript
function onEvent(...args) {
    log.Debug("Event received", args);  // 调试信息
    log.Info("Processing event");       // 一般信息
    log.Warn("Potential issue");        // 警告
    log.Error("Critical error", err);   // 错误
}
```

### 4. 配置安全

- **不在配置文件中暴露密码**（考虑使用环境变量）
- **限制 MQTT Broker 访问权限**
- **使用 TLS 加密连接**（将 `tcp://` 改为 `tls://`）

```json
{
  "name": "secure",
  "broker": "tls://broker.example.com:8883",
  "client_id": "yao_subscriber",
  "username": "${MQTT_USERNAME}",
  "password": "${MQTT_PASSWORD}"
}
```

---

## 常见问题

### Q: 如何测试 MQTT 连接？

A: 可以在脚本中调用发送函数进行测试：

```javascript
function test() {
    let res = Process("mqtt.publish", "test", "test/topic", {
        "test": "message"
    });
    return res;
}
```

### Q: 本地事件和 MQTT 可以结合使用吗？

A: 可以。在事件处理函数中发送 MQTT 消息：

```javascript
function onEvent(...args) {
    // 接收事件
    log.Info("Event received", args);
    
    // 通过 MQTT 转发到其他系统
    Process("mqtt.publish", "test", "events", {
        "event_data": args[0],
        "timestamp": Date.now()
    });
}
```

### Q: 如何处理消息丢失？

A: 使用 QoS 1 或 2 确保消息可靠性：

```json
{
  "topic": "important/topic",
  "qos": 1,
  "process": "scripts.handlers.important"
}
```

### Q: 动态订阅和配置文件订阅有什么区别？

A: 
- **配置文件订阅**：在 `*.event.yao` 配置文件中定义，是持久化的订阅，应用启动时自动生效
- **动态订阅**：通过 `event.subscribe()` 在运行时动态创建，可以随时订阅和取消订阅，适合临时性的事件处理需求

### Q: 动态订阅的事件处理函数有什么要求？

A: 动态订阅的事件处理函数需要满足以下要求：
1. 必须是有效的 process 路径（如 `scripts.custom.event.onEvent`）
2. 函数签名应为 `function onEvent(...args)`，可以接收任意参数
3. 函数需要在对应的脚本文件中定义

---

## 参考资源

- [YAO 官方文档](https://yao.run)
- [MQTT 协议规范](https://mqtt.org)
- [JavaScript 脚本指南](https://yao.run)

---

**最后更新**: 2026-04-06