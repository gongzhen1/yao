package websocket

import (
	"github.com/yaoapp/gou/application"
	"github.com/yaoapp/gou/websocket"
	"github.com/yaoapp/kun/log"
	"github.com/yaoapp/yao/config"
	"github.com/yaoapp/yao/share"
)

// Load 加载websocket
func Load(cfg config.Config) error {

	// 加载服务端
	exists, err := application.App.Exists("apis")
	if err != nil {
		return err
	}
	if !exists {
		log.Trace("[websockets] websockets directory not found, skip")
		return nil
	}
	exts := []string{"*.ws.json", "*.ws.yao", "*.ws.jsonc"}
	err = application.App.Walk("apis", func(root, filename string, isdir bool) error {
		if isdir {
			return nil
		}
		content := share.ReadFile(filename)
		_, err := websocket.LoadWebSocketServer(string(content), share.ID(root, filename))
		if err != nil {
			log.With(log.F{"root": root, "file": filename}).Error(err.Error())
		}
		return nil // 继续加载其他文件
	}, exts...)

	// 加载客户端
	exists, err = application.App.Exists("websockets")
	if err != nil {
		return err
	}
	if !exists {
		log.Trace("[websockets] websockets directory not found, skip")
		return nil
	}
	exts = []string{"*.ws.json", "*.ws.yao", "*.ws.jsonc"}
	err = application.App.Walk("websockets", func(root, file string, isdir bool) error {
		if isdir {
			return nil
		}
		content := share.ReadFile(file)
		ws, err := websocket.LoadWebSocket(string(content), share.ID(root, file))
		if err != nil {
			log.With(log.F{"root": root, "file": file}).Error(err.Error())
			return nil
		}
		// 自动打开客户端连接（非阻塞）
		go func() {
			name := share.ID(root, file)
			log.Info("[websocket] client %s connecting...", name)
			if err := ws.Open(); err != nil {
				log.Error("[websocket] client %s error: %v", name, err)
			}
		}()
		return nil // 继续加载其他文件
	}, exts...)
	return err
}
