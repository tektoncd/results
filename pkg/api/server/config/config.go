package config

import (
	"log"

	"github.com/spf13/viper"
)

type Config struct {
	DB_USER                  string `mapstructure:"DB_USER"`
	DB_PASSWORD              string `mapstructure:"DB_PASSWORD"`
	DB_HOST                  string `mapstructure:"DB_HOST"`
	DB_PORT                  string `mapstructure:"DB_PORT"`
	DB_NAME                  string `mapstructure:"DB_NAME"`
	DB_SSLMODE               string `mapstructure:"DB_SSLMODE"`
	DB_SSLROOTCERT           string `mapstructure:"DB_SSLROOTCERT"`
	DB_ENABLE_AUTO_MIGRATION bool   `mapstructure:"DB_ENABLE_AUTO_MIGRATION"`
	DB_MAX_IDLE_CONNECTIONS  int    `mapstructure:"DB_MAX_IDLE_CONNECTIONS"`
	DB_MAX_OPEN_CONNECTIONS  int    `mapstructure:"DB_MAX_OPEN_CONNECTIONS"`
	SERVER_PORT              string `mapstructure:"SERVER_PORT"`
	PROMETHEUS_PORT          string `mapstructure:"PROMETHEUS_PORT"`
	PROMETHEUS_HISTOGRAM     bool   `mapstructure:"PROMETHEUS_HISTOGRAM"`
	LOG_LEVEL                string `mapstructure:"LOG_LEVEL"`
	TLS_PATH                 string `mapstructure:"TLS_PATH"`

	GRPC_WORKER_POOL int `mapstructure:"GRPC_WORKER_POOL"`
	K8S_QPS          int `mapstructure:"K8S_QPS"`
	K8S_BURST        int `mapstructure:"K8S_BURST"`

	AUTH_DISABLE     bool `mapstructure:"AUTH_DISABLE"`
	AUTH_IMPERSONATE bool `mapstructure:"AUTH_IMPERSONATE"`

	LOGS_API         bool   `mapstructure:"LOGS_API"`
	LOGS_TYPE        string `mapstructure:"LOGS_TYPE"`
	LOGS_BUFFER_SIZE int    `mapstructure:"LOGS_BUFFER_SIZE"`
	LOGS_PATH        string `mapstructure:"LOGS_PATH"`

	PROFILING      bool   `mapstructure:"PROFILING"`
	PROFILING_PORT string `mapstructure:"PROFILING_PORT"`

	GCS_BUCKET_NAME       string `mapstructure:"GCS_BUCKET_NAME"`
	STORAGE_EMULATOR_HOST string `mapstructure:"STORAGE_EMULATOR_HOST"`

	S3_BUCKET_NAME        string `mapstructure:"S3_BUCKET_NAME"`
	S3_ENDPOINT           string `mapstructure:"S3_ENDPOINT"`
	S3_HOSTNAME_IMMUTABLE bool   `mapstructure:"S3_HOSTNAME_IMMUTABLE"`
	S3_REGION             string `mapstructure:"S3_REGION"`
	S3_ACCESS_KEY_ID      string `mapstructure:"S3_ACCESS_KEY_ID"`
	S3_SECRET_ACCESS_KEY  string `mapstructure:"S3_SECRET_ACCESS_KEY"`
	S3_MULTI_PART_SIZE    int64  `mapstructure:"S3_MULTI_PART_SIZE"`

	CONVERTER_ENABLE   bool `mapstructure:"CONVERTER_ENABLE"`
	CONVERTER_DB_LIMIT int  `mapstructure:"CONVERTER_DB_LIMIT"`

	LOGGING_PLUGIN_API_URL                  string `mapstructure:"LOGGING_PLUGIN_API_URL"`
	LOGGING_PLUGIN_NAMESPACE_KEY            string `mapstructure:"LOGGING_PLUGIN_NAMESPACE_KEY"`
	LOGGING_PLUGIN_STATIC_LABELS            string `mapstructure:"LOGGING_PLUGIN_STATIC_LABELS"`
	LOGGING_PLUGIN_TOKEN_PATH               string `mapstructure:"LOGGING_PLUGIN_TOKEN_PATH"`
	LOGGING_PLUGIN_PROXY_PATH               string `mapstructure:"LOGGING_PLUGIN_PROXY_PATH"`
	LOGGING_PLUGIN_CA_CERT                  string `mapstructure:"LOGGING_PLUGIN_CA_CERT"`
	LOGGING_PLUGIN_QUERY_LIMIT              uint   `mapstructure:"LOGGING_PLUGIN_QUERY_LIMIT"`
	LOGGING_PLUGIN_TLS_VERIFICATION_DISABLE bool   `mapstructure:"LOGGING_PLUGIN_TLS_VERIFICATION_DISABLE"`
	LOGGING_PLUGIN_FORWARDER_DELAY_DURATION uint   `mapstructure:"LOGGING_PLUGIN_FORWARDER_DELAY_DURATION"`
}

func Get() *Config {
	viper.SetConfigName("config")
	viper.SetConfigType("env")
	viper.AddConfigPath("/etc/tekton/results")
	viper.AddConfigPath("config/base/env")
	viper.AddConfigPath("config")
	viper.AddConfigPath(".")

	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		log.Fatalf("Error reading config: %v", err)
	}

	config := Config{}
	if err := viper.Unmarshal(&config); err != nil {
		log.Fatal("Cannot load config:", err)
	}

	return &config
}
