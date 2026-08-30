package kafka

import (
	"fmt"
	"strings"

	"github.com/yaoapp/gou/application"
	goukafka "github.com/yaoapp/gou/kafka"
	"github.com/yaoapp/kun/log"
	"github.com/yaoapp/yao/config"
	"github.com/yaoapp/yao/share"
)

// Load 加载所有 Kafka 配置
func Load(cfg config.Config) error {
	// 检查 mqs 目录是否存在
	exists, err := application.App.Exists("mqs")
	if err != nil {
		return err
	}
	if !exists {
		log.Trace("[Kafka] mqs directory not found, skip")
		return nil
	}

	messages := []string{}
	exts := []string{"*.kafka.yao", "*.kafka.json", "*.kafka.jsonc"}
	err = application.App.Walk("mqs", func(root, file string, isdir bool) error {
		if isdir {
			return nil
		}
		_, err := goukafka.Load(file, share.ID(root, file))
		if err != nil {
			messages = append(messages, err.Error())
		}
		return nil // 继续加载其他文件
	}, exts...)

	if len(messages) > 0 {
		return fmt.Errorf("%s", strings.Join(messages, ";\n"))
	}
	return nil
}

// Start 启动所有客户端（已在 Load 时启动，此函数仅为统一接口）
func Start() {
	log.Info("[Kafka] all clients are running")
}

// Stop 停止所有客户端
func Stop() {
	goukafka.StopAll()
}