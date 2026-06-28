package shared

import (
	"os"

	"github.com/spf13/viper"
)

type Config struct {
	ProxyPort    int
	LogLevel     string
	AuditLogPath string
	DryRun       bool
}

func LoadConfig() (*Config, error) {
	viper.SetConfigName(".env")
	viper.SetConfigType("env")
	viper.AddConfigPath(".")
	viper.AddConfigPath("../")
	viper.AddConfigPath(os.Getenv("ALCATRAZ_CONFIG_DIR"))

	viper.AutomaticEnv()

	viper.SetDefault("PROXY_PORT", 8080)
	viper.SetDefault("LOG_LEVEL", "info")
	viper.SetDefault("AUDIT_LOG_PATH", "/var/log/alcatraz/audit.log")
	viper.SetDefault("DRY_RUN", false)

	_ = viper.ReadInConfig()

	return &Config{
		ProxyPort:    viper.GetInt("PROXY_PORT"),
		LogLevel:     viper.GetString("LOG_LEVEL"),
		AuditLogPath: viper.GetString("AUDIT_LOG_PATH"),
		DryRun:       viper.GetBool("DRY_RUN"),
	}, nil
}
