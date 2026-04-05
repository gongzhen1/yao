package mqtt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaoapp/gou/application"
	gouMqtt "github.com/yaoapp/gou/mqtt"
	"github.com/yaoapp/yao/config"
	"github.com/yaoapp/gou/process"
)

// TestMain 设置测试环境
func TestMain(m *testing.M) {
	// 设置测试应用根目录
	appRoot, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	os.Setenv("YAO_APP_ROOT", appRoot)
	os.Setenv("GOU_TEST_APP_ROOT", appRoot)

	// 初始化 application
	err = application.Load(appRoot)
	if err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// TestMqttLoad 测试加载 MQTT 配置
func TestMqttLoad(t *testing.T) {
	// 清理之前的客户端
	Stop()
	gouMqtt.Clients = make(map[string]*gouMqtt.Client)

	// 创建 mqtts 目录
	mqttsDir := filepath.Join(os.Getenv("YAO_APP_ROOT"), "mqtts")
	err := os.MkdirAll(mqttsDir, 0755)
	assert.Nil(t, err)
	defer os.RemoveAll(mqttsDir)

	// 准备配置文件
	broker := os.Getenv("TEST_MQTT_BROKER")
	if broker == "" {
		broker = "tcp://localhost:1883"
		t.Logf("Using default broker: %s", broker)
	}

	cfgContent := map[string]interface{}{
		"name":      "test_client",
		"broker":    broker,
		"client_id": "test_client_1",
		"username":  "",
		"password":  "",
		"subscribes": []map[string]interface{}{
			{
				"topic":   "test/topic",
				"qos":     1,
				"process": "scripts.mqtt.handle",
			},
		},
	}
	data, _ := json.MarshalIndent(cfgContent, "", "  ")
	cfgFile := filepath.Join(mqttsDir, "test.mqtt.json")
	err = os.WriteFile(cfgFile, data, 0644)
	assert.Nil(t, err)

	// 注册测试用的 Process
	process.Register("scripts.mqtt.handle", func(proc *process.Process) interface{} {
		topic := proc.Args.String(0)
		payload := proc.Args.Get(1)
		ts := proc.Args.Int(2, 0)
		t.Logf("Process called: topic=%s, payload=%v, ts=%d", topic, payload, ts)
		return map[string]interface{}{"received": true}
	})
	defer process.Unregister("scripts.mqtt.handle")

	// 加载配置
	cfg := config.Config{}
	err = Load(cfg)
	assert.Nil(t, err)

	// 等待订阅建立
	time.Sleep(1 * time.Second)

	// 验证客户端已启动
	client := gouMqtt.Select("test_client")
	assert.NotNil(t, client)

	// 发布测试消息（通过 mqtt.publish Process）
	proc := process.New("mqtt.publish", "test_client", "test/topic", map[string]string{"msg": "hello"}, 1, false)
	_, err = proc.Exec()
	assert.Nil(t, err)

	// 等待回调执行
	time.Sleep(1 * time.Second)

	// 停止所有客户端
	Stop()
}