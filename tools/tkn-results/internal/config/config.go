package config

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	// EnvSSLRootFilePath is the environment variable name for the path to
	// local SSL cert to use for requests.
	EnvSSLRootFilePath = "TKN_RESULTS_SSL_ROOTS_FILE_PATH"
	// EnvSSLRootFilePath is the environment variable name for the SSL server
	// name override.
	EnvSSLServerNameOverride = "TKN_RESULTS_SSL_SERVER_NAME_OVERRIDE"
)

var (
	addr      = pflag.StringP("addr", "a", "", "Result API server address")
	authToken = pflag.StringP("authtoken", "t", "", "authorization bearer token to use for authenticated requests")

	env = map[string]string{
		EnvSSLRootFilePath:       "Path to local SSL cert to use.",
		EnvSSLServerNameOverride: "SSL server name override (useful if using with a proxy such as kubectl port-forward).",
	}
)

type Config struct {
	Address string
	Token   string

	SSL SSLConfig
}

type SSLConfig struct {
	RootsFilePath      string `mapstructure:"roots_file_path"`
	ServerNameOverride string `mapstructure:"server_name_override"`
}

func init() {
	viper.SetConfigName("results")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("$HOME/.config/tkn")
}

func GetConfig() (*Config, error) {
	pflag.Parse()
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		return nil, err
	}

	for k := range env {
		if err := viper.BindEnv(k); err != nil {
			return nil, err
		}
	}

	// Config should be evaluated in the following order (last wins):
	// 1. Environment variables
	// 2. Config File
	// 3. Flags

	// Initial config is contains the env variables,
	// so that the unmarshal can take priority if those values are set.
	cfg := &Config{
		SSL: SSLConfig{
			RootsFilePath:      viper.GetString(EnvSSLRootFilePath),
			ServerNameOverride: viper.GetString(EnvSSLServerNameOverride),
		},
	}

	if err := viper.ReadInConfig(); err == nil {
		if err := viper.Unmarshal(cfg); err != nil {
			return nil, err
		}
	} else {
		fmt.Println(err)
	}

	// Flags should override other values.
	if s := viper.GetString("addr"); s != "" {
		cfg.Address = s
	}
	if s := viper.GetString("authtoken"); s != "" {
		cfg.Token = viper.GetString("authtoken")
	}

	return cfg, nil
}

func EnvVarHelp() string {
	b := new(strings.Builder)
	fmt.Println("Environment Variables:")
	for k, v := range env {
		fmt.Fprintf(b, "\t%s: %s\n", k, v)
	}
	return b.String()
}
