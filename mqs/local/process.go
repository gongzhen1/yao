package localevent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaoapp/gou/application"
	"github.com/yaoapp/gou/process"
	"github.com/yaoapp/kun/exception"
	"github.com/yaoapp/kun/log"
	yaoevent "github.com/yaoapp/yao/event"
	"github.com/yaoapp/yao/event/types"
)

var ProcessHandlers = map[string]process.Handler{
	"publish":   processEventPublish,
	"subscribe": processEventSubscribe,
}

func init() {
	process.RegisterGroup("event", ProcessHandlers)
}

// Load 加载所有本地事件监听配置
func Load() error {
	// 检查 mqs 目录是否存在
	exists, err := application.App.Exists("mqs")
	if err != nil {
		return err
	}
	if !exists {
		log.Trace("[Event] mqs directory not found, skip")
		return nil
	}

	messages := []string{}
	exts := []string{"*.event.yao", "*.event.json", "*.event.jsonc"}
	err = application.App.Walk("mqs", func(root, file string, isdir bool) error {
		if isdir {
			return nil
		}

		// 读取配置文件
		content, err := application.App.Read(file)
		if err != nil {
			messages = append(messages, fmt.Sprintf("read %s failed: %v", file, err))
			return nil
		}

		// 解析配置（每个文件一个对象）
		var config EventConfig
		if err := json.Unmarshal(content, &config); err != nil {
			messages = append(messages, fmt.Sprintf("parse %s failed: %v", file, err))
			return nil
		}

		// 注册处理器
		if config.Event == "" || config.Process == "" {
			messages = append(messages, fmt.Sprintf("invalid config in %s: event and process required", file))
			return nil
		}

		// 提取前缀
		prefix := config.Event
		if i := strings.Index(config.Event, "."); i > 0 {
			prefix = config.Event[:i]
		}

		// 创建处理器
		handler := &eventHandler{processName: config.Process}

		// 构建注册选项
		var opts []types.HandlerOption
		if config.MaxWorkers != nil {
			opts = append(opts, yaoevent.MaxWorkers(*config.MaxWorkers))
		}
		if config.ReservedWorkers != nil {
			opts = append(opts, yaoevent.ReservedWorkers(*config.ReservedWorkers))
		}
		if config.QueueSize != nil {
			opts = append(opts, yaoevent.QueueSize(*config.QueueSize))
		}

		// 注册处理器
		yaoevent.Register(prefix, handler, opts...)
		log.Info("[Event] registered prefix %s -> %s", prefix, config.Process)

		return nil
	}, exts...)

	if len(messages) > 0 {
		return fmt.Errorf("%s", strings.Join(messages, ";\n"))
	}

	// 如果事件服务已启动，需要重新加载以初始化新注册的处理器的工作池
	// 如果未启动，Start 会在引擎启动时被调用
	if yaoevent.IsStarted() {
		if err := yaoevent.Reload(); err != nil {
			return err
		}
	}

	return nil
}

// EventConfig 事件监听配置
type EventConfig struct {
	Event           string `json:"event"`
	Process         string `json:"process"`
	MaxWorkers      *int   `json:"max_workers,omitempty"`
	ReservedWorkers *int   `json:"reserved_workers,omitempty"`
	QueueSize       *int   `json:"queue_size,omitempty"`
}

// processEventPublish 本地事件发布
// 参数: eventType, data
func processEventPublish(proc *process.Process) interface{} {
	args := proc.Args
	if len(args) < 2 {
		exception.New("event.publish requires 2 arguments: eventType, data", 400).Throw()
	}

	eventType, ok := args[0].(string)
	if !ok {
		exception.New("eventType must be string", 400).Throw()
	}

	data := args[1]

	ctx := proc.Context
	if ctx == nil {
		ctx = context.Background()
	}

	if _, err := yaoevent.Push(ctx, eventType, data); err != nil {
		exception.New("event.publish failed: %v", 500, err).Throw()
	}

	return true
}

// processEventSubscribe 本地事件订阅
// 参数: pattern, process
func processEventSubscribe(proc *process.Process) interface{} {
	args := proc.Args
	if len(args) < 2 {
		exception.New("event.subscribe requires 2 arguments: pattern, process", 400).Throw()
	}

	pattern, ok := args[0].(string)
	if !ok {
		exception.New("pattern must be string", 400).Throw()
	}

	processName, ok := args[1].(string)
	if !ok {
		exception.New("process must be string", 400).Throw()
	}

	listener := &eventListener{processName: processName}
	yaoevent.Listen(pattern, listener)
	return true
}

// eventListener 用于 Listen 监听
type eventListener struct {
	processName string
}

func (l *eventListener) OnEvent(ev *types.Event) {
	p, err := process.Of(l.processName, ev.Type, ev.Payload)
	if err != nil {
		log.Error("event listen process %s create failed: %v", l.processName, err)
		return
	}

	if err := p.Execute(); err != nil {
		log.Error("event listen process %s execute failed: %v", l.processName, err)
	}
	p.Release()
}

func (l *eventListener) Shutdown(ctx context.Context) error {
	return nil
}

type eventHandler struct {
	processName string
}

func (h *eventHandler) Handle(ctx context.Context, ev *types.Event, resp chan<- types.Result) {
	p, err := process.Of(h.processName, ev.Type, ev.Payload)
	if err != nil {
		log.Error("event process %s create failed: %v", h.processName, err)
		resp <- types.Result{}
		return
	}

	if err := p.Execute(); err != nil {
		log.Error("event process %s execute failed: %v", h.processName, err)
	}
	p.Release()
	resp <- types.Result{}
}

func (h *eventHandler) Shutdown(ctx context.Context) error {
	return nil
}
