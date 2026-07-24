package config

// Config 为进程级配置（可由 main 从 flag / 环境变量填充）。
type Config struct {
	ListenAddr string
	GinMode    string // debug | release | test
	Model      string // 指定 Agent 使用的模型（对应 ACP session/set_config_option id=model）
}
