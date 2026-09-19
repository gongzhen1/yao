package kafkalog

import (
	"encoding/json"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	goukafka "github.com/yaoapp/gou/kafka"
	"github.com/yaoapp/kun/log"
)

// maxQueueSize 日志发送缓冲队列上限，超过即丢弃（防止 Kafka 无消费者时日志积压阻塞主流程）
const maxQueueSize = 65536

// dropWarnInterval 丢弃告警的最小间隔，避免告警本身刷屏
const dropWarnInterval = 30 * time.Second

// maxBatchSize 单次批量发布的最大条数
const maxBatchSize = 500

// batchWait 攒批时最多再等多久。单条发布约 0.75 秒一次往返，远跟不上日志产生速度，
// 攒批后一次往返可发送多条，吞吐提升与批量大小相当
const batchWait = 200 * time.Millisecond

// clientWaitInterval / maxClientWaitTries 等待 Kafka 连接器就绪的间隔与次数（合计最多 10 秒）
const clientWaitInterval = 100 * time.Millisecond
const maxClientWaitTries = 100

// selfLogPrefix 本 Hook 自身告警的前缀，用于识别并拦截回流日志
const selfLogPrefix = "[kafkalog]"

// selfInsertJSONMarker 日志文件（logrus JSON 格式）里「写日志表」那条 SQL 日志的特征片段
const selfInsertJSONMarker = `"msg":"insert into ` + "`yao_log`"

// selfInsertMessagePrefix 「写日志表」SQL 日志的消息前缀
const selfInsertMessagePrefix = "insert into `yao_log`"

// IsSelfInsertLogLine 判断一行 JSON 日志是否为「写日志表自身」产生的 SQL 日志
// （msg 形如 "insert into `yao_log` (...)"）
func IsSelfInsertLogLine(line string) bool {
	return strings.Contains(line, selfInsertJSONMarker)
}

// IsSelfInsertMessage 判断日志消息是否为「写日志表自身」产生的 SQL 日志
func IsSelfInsertMessage(message string) bool {
	return strings.HasPrefix(strings.TrimLeft(message, " \t"), selfInsertMessagePrefix)
}

// Hook 将日志异步发送到 Kafka 的 logrus Hook
type Hook struct {
	clientName string // kafka 连接名称（YAO_LOG_STORE=kafka.<connectName> 中的 connectName）
	topic      string // 日志主题，为空时取客户端配置的第一个 topic
	hostname   string // 容器主机名
	ip         string // 本机 IP，多容器场景下用它区分来源实例
	queue      chan []byte
	stop       chan struct{}
	wg         sync.WaitGroup
	once       sync.Once
	dropped    int64 // 因队列满被丢弃的日志条数
	lastWarn   int64 // 上次丢弃告警时间（UnixNano），用于节流
}

// New 创建一个 Kafka 日志 Hook
func New(clientName, topic string) *Hook {
	hostname, _ := os.Hostname()
	return &Hook{
		clientName: clientName,
		topic:      topic,
		hostname:   hostname,
		ip:         localIP(),
		queue:      make(chan []byte, maxQueueSize),
		stop:       make(chan struct{}),
	}
}

// localIP 返回本机第一个非回环 IPv4 地址。
// hostname 在多容器场景下可能重复（容器主机名可被统一指定），而 IP 能唯一区分实例，
// 这里与 docker-entrypoint.sh 用 `hostname -i` 区分实例日志文件的口径保持一致
func localIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() {
			continue
		}
		if v4 := ipnet.IP.To4(); v4 != nil {
			return v4.String()
		}
	}
	return ""
}

// Levels 需要处理的日志级别
func (h *Hook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire 将日志条目序列化后放入缓冲队列；队列满时丢弃该条日志
func (h *Hook) Fire(entry *logrus.Entry) error {
	// 自身失败告警不再回流到 Hook：发布失败时会用 log.Error 记录，
	// 而 log.Error 同样经过本 Hook，会形成「发布失败 → 记日志 → 再发布 → 再失败」
	// 的递归放大，实测一秒内可刷出数十万行日志
	if strings.HasPrefix(entry.Message, selfLogPrefix) {
		return nil
	}

	// 「写日志表自身」的 SQL 日志不上报：它只是写日志表这个动作的副作用，
	// 且 bindings 里又带着整行数据（消息体积接近翻倍），上报后仍要在写库环节丢弃
	if IsSelfInsertMessage(entry.Message) {
		return nil
	}

	payload := map[string]interface{}{
		"level":    entry.Level.String(),
		"msg":      entry.Message,
		"time":     entry.Time.Format(time.RFC3339Nano),
		"hostname": h.hostname,
		"ip":       h.ip,
	}
	for k, v := range entry.Data {
		payload[k] = v
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}

	select {
	case h.queue <- data:
	default:
		// 队列已满，丢弃该条日志，避免阻塞业务日志写入。
		// 按 30 秒节流打印一次累计丢弃数，让「数据库日志比文件少」这件事可见、可量化
		h.warnDropped()
	}
	return nil
}

// warnDropped 累加丢弃计数并节流告警
func (h *Hook) warnDropped() {
	n := atomic.AddInt64(&h.dropped, 1)

	now := time.Now().UnixNano()
	last := atomic.LoadInt64(&h.lastWarn)
	if now-last < int64(dropWarnInterval) {
		return
	}
	if !atomic.CompareAndSwapInt64(&h.lastWarn, last, now) {
		return
	}

	// 前缀为 selfLogPrefix，不会再次回流到本 Hook
	log.Warn(selfLogPrefix+" queue is full, dropped logs: %d (total since start)", n)
}

// Start 启动后台发送协程
func (h *Hook) Start() {
	h.once.Do(func() {
		h.wg.Add(1)
		go h.sendLoop()
	})
}

// Close 停止后台发送协程并等待退出
func (h *Hook) Close() {
	close(h.stop)
	h.wg.Wait()
}

func (h *Hook) sendLoop() {
	defer h.wg.Done()
	for {
		select {
		case <-h.stop:
			return
		case data := <-h.queue:
			h.sendBatch(h.collect(data))
			select {
			case <-h.stop:
				// 停止信号已到，把刚发出的这批发完即退出
				return
			default:
			}
		}
	}
}

// collect 以 first 为首条，继续从队列取日志凑成一批，
// 最多 maxBatchSize 条、最多再等 batchWait，避免为攒批引入明显延迟
func (h *Hook) collect(first []byte) [][]byte {
	batch := make([][]byte, 0, maxBatchSize)
	batch = append(batch, first)

	for len(batch) < maxBatchSize {
		select {
		case data := <-h.queue:
			batch = append(batch, data)
		case <-time.After(batchWait):
			return batch
		case <-h.stop:
			return batch
		}
	}
	return batch
}

// sendBatch 批量发布一批日志，一次网络往返发送多条
func (h *Hook) sendBatch(batch [][]byte) {
	if len(batch) == 0 {
		return
	}

	client := h.waitClient()
	if client == nil {
		log.Error(selfLogPrefix+" kafka client %s is not loaded, dropped %d logs", h.clientName, len(batch))
		return
	}

	topic := h.topic
	if topic == "" && len(client.Topics) > 0 {
		topic = client.Topics[0].Topic
	}
	if topic == "" {
		return
	}

	keys := make([]string, len(batch))
	payloads := make([]interface{}, len(batch))
	for i, data := range batch {
		keys[i] = h.hostname
		payloads[i] = data
	}

	if err := client.PublishBatch(topic, keys, payloads); err != nil {
		log.Error(selfLogPrefix+" publish %d logs to kafka failed: %s", len(batch), err.Error())
	}
}

// waitClient 获取 Kafka 客户端。应用启动时 mqs 连接器的加载晚于日志组件，
// 这段窗口内的日志此前会被整批直接丢弃（表现为重启后头几秒的日志文件里有、数据库里没有），
// 这里短暂等待连接器就绪再发送
func (h *Hook) waitClient() *goukafka.Client {
	for i := 0; i < maxClientWaitTries; i++ {
		if client := goukafka.Select(h.clientName); client != nil {
			return client
		}
		select {
		case <-h.stop:
			return nil
		case <-time.After(clientWaitInterval):
		}
	}
	return nil
}
