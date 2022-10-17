package conf

type ConfigFile struct {
	DB_USER               string `mapstructure:"DB_USER"`
	DB_PASSWORD           string `mapstructure:"DB_PASSWORD"`
	DB_HOST               string `mapstructure:"DB_HOST"`
	DB_PORT               string `mapstructure:"DB_PORT"`
	DB_NAME               string `mapstructure:"DB_NAME"`
	DB_SSLMODE            string `mapstructure:"DB_SSLMODE"`
	GRPC_PORT             string `mapstructure:"GRPC_PORT"`
	REST_PORT             string `mapstructure:"REST_PORT"`
	PROMETHEUS_PORT       string `mapstructure:"PROMETHEUS_PORT"`
	TLS_HOSTNAME_OVERRIDE string `mapstructure:"TLS_HOSTNAME_OVERRIDE"`
	TLS_PATH              string `mapstructure:"TLS_PATH"`

	LOG_TYPE       string `mapstructure:"LOG_TYPE"`
	LOG_CHUNK_SIZE int    `mapstructure:"LOG_CHUNK_SIZE"`
	LOGS_DATA      string `mapstructure:"LOGS_DATA"`

	S3_BUCKET_NAME        string `mapstructure:"S3_BUCKET_NAME"`
	S3_ENDPOINT           string `mapstructure:"S3_ENDPOINT"`
	S3_HOSTNAME_IMMUTABLE bool   `mapstructure:"S3_HOSTNAME_IMMUTABLE"`
	S3_REGION             string `mapstructure:"S3_REGION"`
	S3_ACCESS_KEY_ID      string `mapstructure:"S3_ACCESS_KEY_ID"`
	S3_SECRET_ACCESS_KEY  string `mapstructure:"S3_SECRET_ACCESS_KEY"`
	S3_MULTI_PART_SIZE    int64  `mapstructure:"S3_MULTI_PART_SIZE"`
}
