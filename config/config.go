package config

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/caarlos0/env/v6"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"github.com/yaoapp/kun/exception"
	"github.com/yaoapp/kun/log"
	"github.com/yaoapp/yao/utils/kafkalog"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Conf 配置参数
var Conf Config

// LogOutput 日志输出
var LogOutput io.WriteCloser // 日志文件

// kafkaLogHook kafka 日志 hook（YAO_LOG_STORE=kafka.<connectName> 时启用）
var kafkaLogHook *kafkalog.Hook

// DSLExtensions the dsl file Extensions
var DSLExtensions = []string{"*.yao", "*.json", "*.jsonc"}

func init() {
	Init()
}

// Init setting
func Init() {
	// Determine app root: YAO_ROOT env > find app.yao > current directory
	root := os.Getenv("YAO_ROOT")
	if root == "" {
		root = findAppRoot()
	}
	if root == "" {
		root = "."
	}

	filename := filepath.Join(root, ".env")
	if _, err := os.Stat(filename); errors.Is(err, os.ErrNotExist) {
		Conf = LoadWithRoot(root)
		ApplyMode()
		return
	}

	// Load .env then override root if auto-detected
	Conf = LoadFromWithRoot(filename, root)
	ApplyMode()
}

// ApplyMode applies production or development mode based on Conf.Mode
func ApplyMode() {
	switch Conf.Mode {
	case "production":
		Production()
	case "development":
		Development()
	}
}

// findAppRoot finds the Yao application root directory by looking for app.yao
// It traverses up from the current directory until it finds app.yao or reaches the filesystem root
func findAppRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	for {
		// Check for app.yao, app.json, or app.jsonc
		for _, appFile := range []string{"app.yao", "app.json", "app.jsonc"} {
			appFilePath := filepath.Join(dir, appFile)
			if _, err := os.Stat(appFilePath); err == nil {
				return dir
			}
		}

		// Move to parent directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root, no app.yao found
			break
		}
		dir = parent
	}

	return ""
}

// LoadFrom 从配置项中加载
func LoadFrom(envfile string) Config {
	return LoadFromWithRoot(envfile, "")
}

// LoadFromWithRoot loads config from env file with optional root override
func LoadFromWithRoot(envfile string, root string) Config {
	file, err := filepath.Abs(envfile)
	if err != nil {
		cfg := LoadWithRoot(root)
		ReloadLog()
		return cfg
	}

	// load from env
	godotenv.Overload(file)
	cfg := LoadWithRoot(root)
	ReloadLog()
	return cfg
}

// Load the config
func Load() Config {
	return LoadWithRoot("")
}

// LoadWithRoot loads config with an optional root override
// If root is empty, uses YAO_ROOT env or current directory
func LoadWithRoot(root string) Config {
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		exception.New("Can't read config %s", 500, err.Error()).Throw()
	}

	// Root path: use provided root > env YAO_ROOT > default "."
	if root != "" {
		cfg.Root, _ = filepath.Abs(root)
	} else {
		cfg.Root, _ = filepath.Abs(cfg.Root)
	}

	// App Root
	if cfg.AppSource == "" {
		cfg.AppSource = cfg.Root
	}

	// DataRoot
	if cfg.DataRoot == "" {
		cfg.DataRoot = filepath.Join(cfg.Root, "data")
	}
	if !filepath.IsAbs(cfg.DataRoot) {
		cfg.DataRoot = filepath.Join(cfg.Root, cfg.DataRoot)
	}

	// Resolve DB relative paths based on Root
	for i, dsn := range cfg.DB.Primary {
		if !filepath.IsAbs(dsn) && (cfg.DB.Driver == "sqlite3" || cfg.DB.Driver == "") {
			cfg.DB.Primary[i] = filepath.Join(cfg.Root, dsn)
		}
	}
	for i, dsn := range cfg.DB.Secondary {
		if !filepath.IsAbs(dsn) && (cfg.DB.Driver == "sqlite3" || cfg.DB.Driver == "") {
			cfg.DB.Secondary[i] = filepath.Join(cfg.Root, dsn)
		}
	}

	// Trace Driver - default based on mode
	if cfg.Trace.Driver == "" {
		if cfg.Mode == "development" {
			cfg.Trace.Driver = "local"
		} else {
			cfg.Trace.Driver = "store"
		}
	}

	// Trace Path - default to same directory as log file when using local driver
	if cfg.Trace.Driver == "local" {
		if cfg.Trace.Path == "" {
			// Use the log file directory
			logDir := cfg.GetLogDir()
			cfg.Trace.Path = filepath.Join(logDir, "traces")
		}

		if !filepath.IsAbs(cfg.Trace.Path) {
			cfg.Trace.Path = filepath.Join(cfg.Root, cfg.Trace.Path)
		}
	}

	// Trace Prefix - default prefix for store driver
	if cfg.Trace.Driver == "store" && cfg.Trace.Prefix == "" {
		cfg.Trace.Prefix = "trace:"
	}

	return cfg
}

// GetLogDir returns the directory of the log file
func (cfg *Config) GetLogDir() string {
	logPath := cfg.Log
	if logPath == "" {
		logPath = filepath.Join(cfg.Root, "logs", "application.log")
	}

	if !filepath.IsAbs(logPath) {
		logPath = filepath.Join(cfg.Root, logPath)
	}

	return filepath.Dir(logPath)
}

// Production 设定为生产环境
func Production() {
	os.Setenv("YAO_MODE", "production")
	Conf.Mode = "production"
	log.SetLevel(log.InfoLevel)
	log.SetFormatter(log.TEXT)
	if Conf.LogMode == "JSON" {
		log.SetFormatter(log.JSON)
	}
	gin.SetMode(gin.ReleaseMode)
	ReloadLog()
}

// Development 设定为开发环境
func Development() {
	os.Setenv("YAO_MODE", "development")
	Conf.Mode = "development"
	log.SetLevel(log.TraceLevel)
	log.SetFormatter(log.TEXT)
	if Conf.LogMode == "JSON" {
		log.SetFormatter(log.JSON)
	}
	gin.SetMode(gin.DebugMode)
	ReloadLog()
}

// ReloadLog 重新打开日志
func ReloadLog() {
	CloseLog()
	OpenLog()
}

// filterWriter 包裹日志输出，丢弃「写日志表自身」产生的 SQL 日志行。
// 这类日志是写入日志表这个动作的副作用，bindings 里又重复携带了整行数据，
// 落到日志文件里只是纯噪声（上报链路已在 hook 里同样剔除）
type filterWriter struct{ w io.Writer }

func (f *filterWriter) Write(p []byte) (int, error) {
	// 绝大多数日志不含该特征，走快路径直接透传
	if !kafkalog.IsSelfInsertLogLine(string(p)) {
		if _, err := f.w.Write(p); err != nil {
			return 0, err
		}
		return len(p), nil
	}

	out := make([]byte, 0, len(p))
	for _, line := range strings.SplitAfter(string(p), "\n") {
		if line != "" && line != "\n" && kafkalog.IsSelfInsertLogLine(line) {
			continue
		}
		out = append(out, line...)
	}
	if len(out) > 0 {
		if _, err := f.w.Write(out); err != nil {
			return 0, err
		}
	}
	// 丢弃的行也按已写入原始长度返回，避免上层把过滤当成写失败
	return len(p), nil
}

func (f *filterWriter) Close() error {
	if c, ok := f.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// logrusWriter 把写入的内容按行转发给 logrus。
// GIN 的访问日志由 gin.DefaultWriter 直接写文件、不经过 logrus，因此这些日志只会出现在
// 本地日志文件里，不会经由 logrus hook 上报 Kafka、也就写不进 yao_log 表，
// 表现为「数据库日志比文件日志少一批 [GIN] 访问日志」。
// 转发给 logrus 后，访问日志与其它日志走同一条链路（同时进文件与上报）。
type logrusWriter struct{}

func (logrusWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line == "" {
			continue
		}
		// 直接调用 logrus 而不是 log.Info：log.Info 内部走 fmt.Sprintf，
		// 会把访问日志 URL 里的 %xx 当成格式化占位符吃掉
		logrus.Info(line)
	}
	return len(p), nil
}

// OpenLog 打开日志
func OpenLog() {

	if Conf.Log == "" {
		Conf.Log = filepath.Join(Conf.Root, "logs", "application.log")
	}

	if !filepath.IsAbs(Conf.Log) {
		Conf.Log = filepath.Join(Conf.Root, Conf.Log)
	}

	logfile, err := filepath.Abs(Conf.Log)
	if err != nil {
		return
	}

	logpath := filepath.Dir(logfile)

	// Check if the log path exists
	if _, err := os.Stat(logpath); errors.Is(err, os.ErrNotExist) {
		LogOutput, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0666)
		log.SetOutput(LogOutput)
		gin.DefaultWriter = logrusWriter{}
		return
	}

	LogOutput = &filterWriter{w: &lumberjack.Logger{
		Filename:   logfile,
		MaxSize:    Conf.LogMaxSize, // megabytes
		MaxBackups: Conf.LogMaxBackups,
		MaxAge:     Conf.LogMaxAage, //days
		LocalTime:  Conf.LogLocalTime,
	}}

	log.SetOutput(LogOutput)
	gin.DefaultWriter = logrusWriter{}

	// 配置了 kafka 日志存储时，注册 logrus hook 将日志异步发送到 Kafka。
	// hook 只创建一次：启动过程会多次 OpenLog（ReloadLog），重复创建会让同一条日志被多个 hook
	// 重复序列化，且旧 hook 无法从 logrus 注销
	if strings.HasPrefix(Conf.LogStore, "kafka.") {
		clientName := strings.TrimPrefix(Conf.LogStore, "kafka.")
		if kafkaLogHook == nil {
			kafkaLogHook = kafkalog.New(clientName, Conf.LogKafkaTopic)
			logrus.StandardLogger().AddHook(kafkaLogHook)
			kafkaLogHook.Start()
		}
	}
}

// CloseLog 关闭日志
func CloseLog() {
	if LogOutput != nil {
		err := LogOutput.Close()
		if err != nil {
			log.Error("Failed to close log output: %v", err)
			return
		}
	}

	// 这里不再关闭 Kafka hook：CloseLog 只被 ReloadLog（配置加载、Production/Development 切换）调用，
	// 紧接着就会 OpenLog，而启动过程会 ReloadLog 多次。若在此关闭并置空 hook，会带来两个问题：
	//   ① 旧 hook 已经通过 AddHook 注册进 logrus 且无法移除，成为只进不出的僵尸 hook：
	//      每条日志被重复序列化，其队列最终写满并持续刷「queue is full」告警；
	//   ② 旧 hook 队列里尚未发出的日志，会在「连接器还没加载完 + stop 已关闭」时被整批丢弃
	//      （实测重启瞬间丢 17 条：文件里有、数据库里没有）。
	// hook 改为随进程生命周期存续，由 OpenLog 按「已存在即复用」的方式幂等处理。
}

// IsDevelopment returns true if the current mode is development
func IsDevelopment() bool {
	return Conf.Mode == "development"
}
