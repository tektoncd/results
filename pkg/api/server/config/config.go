package config

import (
	"github.com/spf13/viper"
	"log"
)

type Config struct {
	DB_USER                  string `mapstructure:"DB_USER"`
	DB_PASSWORD              string `mapstructure:"DB_PASSWORD"`
	DB_HOST                  string `mapstructure:"DB_HOST"`
	DB_PORT                  string `mapstructure:"DB_PORT"`
	DB_NAME                  string `mapstructure:"DB_NAME"`
	DB_SSLMODE               string `mapstructure:"DB_SSLMODE"`
	DB_ENABLE_AUTO_MIGRATION bool   `mapstructure:"DB_ENABLE_AUTO_MIGRATION"`
	GRPC_PORT                string `mapstructure:"GRPC_PORT"`
	REST_PORT                string `mapstructure:"REST_PORT"`
	PROMETHEUS_PORT          string `mapstructure:"PROMETHEUS_PORT"`
	LOG_LEVEL                string `mapstructure:"LOG_LEVEL"`
	TLS_HOSTNAME_OVERRIDE    string `mapstructure:"TLS_HOSTNAME_OVERRIDE"`
	TLS_PATH                 string `mapstructure:"TLS_PATH"`

	NO_AUTH bool `mapstructure:"NO_AUTH"`

	LOGS_API         bool   `mapstructure:"LOGS_API"`
	LOGS_TYPE        string `mapstructure:"LOGS_TYPE"`
	LOGS_BUFFER_SIZE int    `mapstructure:"LOGS_BUFFER_SIZE"`
	LOGS_PATH        string `mapstructure:"LOGS_PATH"`

	S3_BUCKET_NAME        string `mapstructure:"S3_BUCKET_NAME"`
	S3_ENDPOINT           string `mapstructure:"S3_ENDPOINT"`
	S3_HOSTNAME_IMMUTABLE bool   `mapstructure:"S3_HOSTNAME_IMMUTABLE"`
	S3_REGION             string `mapstructure:"S3_REGION"`
	S3_ACCESS_KEY_ID      string `mapstructure:"S3_ACCESS_KEY_ID"`
	S3_SECRET_ACCESS_KEY  string `mapstructure:"S3_SECRET_ACCESS_KEY"`
	S3_MULTI_PART_SIZE    int64  `mapstructure:"S3_MULTI_PART_SIZE"`
}

func Get() *Config {
	viper.SetConfigName("config")
	viper.SetConfigType("env")
	viper.AddConfigPath("/etc/tekton/results")
	viper.AddConfigPath("config/env")
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
