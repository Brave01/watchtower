package core

import (
	"log"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadConfig() *Config {
	loadEnvFile("configs/.env")
	cfg := DefaultConfig()
	data, err := os.ReadFile("configs/config.yaml")
	if err != nil {
		log.Printf("[Config] 未找到 configs/config.yaml，使用默认配置: %v", err)
		return cfg
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		log.Fatalf("[Config] 配置文件解析失败: %v", err)
	}
	return cfg
}

func GetEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func loadEnvFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}
