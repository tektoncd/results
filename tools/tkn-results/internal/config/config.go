package config

import (
	"log"

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
	env = map[string]string{
		EnvSSLRootFilePath:       "Path to local SSL cert to use.",
		EnvSSLServerNameOverride: "SSL server name override (useful if using with a proxy such as kubectl port-forward).",
	}
	cfg *Config
)

type Config struct {
	// Address is the server address to connect to.
	Address string

	// Token is the bearer token to use for authentication. Takes priority over ServiceAccount.
	Token string
	// ServiceAccount is the Kubernetes Service Account to use to authenticate with the Results API.
	// When specified, the client will fetch a bearer token from the Kubernetes API and use that token
	// for all Results API requests.
	ServiceAccount *ServiceAccount `mapstructure:"service_account"`

	// SSL contains SSL configuration information.
	SSL SSLConfig
	// Portforward enable auto portforwarding to tekton-results-api-service
	// When Address is set and Portforward is true, tkn-results will portforward tekton-results-api-service automatically
	Portforward bool
	// Insecure determines whether to use insecure GRPC tls communication
	Insecure bool
}

type SSLConfig struct {
	RootsFilePath      string `mapstructure:"roots_file_path"`
	ServerNameOverride string `mapstructure:"server_name_override"`
}

type ServiceAccount struct {
	Namespace string
	Name      string
}

func Init() {
	viper.SetConfigName("results")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("$HOME/.config/tkn")
	err := setConfig()
	if err != nil {
		log.Fatal("error setting up flags and config", err)
	}
}

func setConfig() error {
	for k := range env {
		if err := viper.BindEnv(k); err != nil {
			return err
		}
	}

	// Config should be evaluated in the following order (last wins):
	// 1. Environment variables
	// 2. Config File
	// 3. Flags

	// Initial config is contains the env variables,
	// so that the unmarshal can take priority if those values are set.
	cfg = &Config{
		SSL: SSLConfig{
			RootsFilePath:      viper.GetString(EnvSSLRootFilePath),
			ServerNameOverride: viper.GetString(EnvSSLServerNameOverride),
		},
	}

	if err := viper.ReadInConfig(); err == nil {
		if err := viper.Unmarshal(cfg); err != nil {
			return err
		}
	} else {
		return err
	}

	// Flags should override other values.
	if s := viper.GetString("addr"); s != "" {
		cfg.Address = s
	}
	if s := viper.GetString("authtoken"); s != "" {
		cfg.Token = viper.GetString("authtoken")
	}
	cfg.Portforward = viper.GetBool("portforward")
	cfg.Insecure = viper.GetBool("insecure")
	return nil
}

func GetConfig() *Config {
	return cfg
}
