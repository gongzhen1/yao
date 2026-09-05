package connector

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/yaoapp/gou/connector"
	"github.com/yaoapp/gou/process"
	"github.com/yaoapp/kun/exception"
	"github.com/yaoapp/kun/log"
	"github.com/yaoapp/xun/capsule"
)

func init() {
	process.Register("yao.connector.test.redis", ProcessTestRedis)
	process.Register("yao.connector.test.mysql", ProcessTestMySQL)
}

// ProcessTestRedis yao.connector.test.redis
// Args[0] map: {"host": "127.0.0.1", "port": "6379", "user": "", "pass": "", "db": "0", "timeout": 5}
//
//	建立 redis 连接并 ping，返回 PONG。
func ProcessTestRedis(p *process.Process) interface{} {
	p.ValidateArgNums(1)
	m := p.ArgsMap(0)

	options := map[string]interface{}{
		"host":    fmt.Sprintf("%v", m["host"]),
		"port":    fmt.Sprintf("%v", m["port"]),
		"user":    fmt.Sprintf("%v", m["user"]),
		"pass":    fmt.Sprintf("%v", m["pass"]),
		"db":      fmt.Sprintf("%v", m["db"]),
		"timeout": 5,
	}
	if v, ok := m["timeout"].(float64); ok && v > 0 {
		options["timeout"] = int(v)
	}

	source, err := json.Marshal(map[string]interface{}{"type": "redis", "options": options})
	if err != nil {
		exception.Err(err, 400).Throw()
	}

	id := fmt.Sprintf("__test_redis_%d", time.Now().UnixNano())
	c, err := connector.LoadSource(source, id, id+".conn.yao")
	if err != nil {
		log.Error("[connector] redis test fail: %s", err.Error())
		exception.New("%s", 400, err.Error()).Throw()
	}
	defer func() {
		_ = c.Close()
		_ = connector.Remove(id)
	}()

	return "PONG"
}

// ProcessTestMySQL yao.connector.test.mysql
// Args[0] map: {"host": "127.0.0.1", "port": "3306", "user": "root", "pass": "", "db": "test", "charset": "utf8mb4"}
//
//	建立 mysql 连接并执行 SELECT 1，成功返回 OK。
func ProcessTestMySQL(p *process.Process) interface{} {
	p.ValidateArgNums(1)
	m := p.ArgsMap(0)

	user := fmt.Sprintf("%v", m["user"])
	pass := fmt.Sprintf("%v", m["pass"])
	host := fmt.Sprintf("%v", m["host"])
	port := fmt.Sprintf("%v", m["port"])
	db := fmt.Sprintf("%v", m["db"])
	charset := fmt.Sprintf("%v", m["charset"])
	if charset == "" {
		charset = "utf8mb4"
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=%s&parseTime=true&loc=Local&timeout=5s",
		user, pass, host, port, db, charset)

	name := fmt.Sprintf("__test_mysql_%d", time.Now().UnixNano())
	mgr := capsule.New()
	if _, err := mgr.Add(name, "mysql", dsn, false); err != nil {
		log.Error("[connector] mysql add fail: %s", err.Error())
		exception.New("%s", 400, err.Error()).Throw()
	}
	defer func() { _ = mgr.Close() }()

	conn, err := mgr.Primary()
	if err != nil {
		exception.New("%s", 400, err.Error()).Throw()
	}

	var one int
	if err := conn.DB.QueryRow("SELECT 1").Scan(&one); err != nil {
		log.Error("[connector] mysql SELECT 1 fail: %s", err.Error())
		exception.New("%s", 400, err.Error()).Throw()
	}
	if one != 1 {
		exception.New("SELECT 1 unexpected result", 400).Throw()
	}

	return "OK"
}