package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Load reads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	viper.SetConfigType("yaml")

	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		viper.SetConfigName("config")
		viper.AddConfigPath("./configs")
		viper.AddConfigPath(".")
	}

	// Environment variable support
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Set defaults
	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error if desired
		} else {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Expand environment variables in configuration
	expandEnvVars(&config)

	return &config, nil
}

func setDefaults() {
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.readTimeout", "30s")
	viper.SetDefault("server.writeTimeout", "30s")

	viper.SetDefault("mcp.enabled", true)
	viper.SetDefault("mcp.host", "0.0.0.0")
	viper.SetDefault("mcp.port", 8081)

	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "json")

	viper.SetDefault("metrics.enabled", true)
	viper.SetDefault("metrics.path", "/metrics")

	viper.SetDefault("tracing.enabled", false)
	viper.SetDefault("tracing.serviceName", "swagger-mcp-go")

	viper.SetDefault("upstream.timeout", "30s")
	viper.SetDefault("upstream.retryCount", 3)
	viper.SetDefault("upstream.retryDelay", "1s")
	viper.SetDefault("upstream.circuitBreaker.threshold", 5)
	viper.SetDefault("upstream.circuitBreaker.timeout", "60s")

	viper.SetDefault("specs.defaultTTL", "1h")
	viper.SetDefault("specs.maxSize", "10MB")

	viper.SetDefault("policies.rateLimit.enabled", false)
	viper.SetDefault("policies.rateLimit.requestsPerMinute", 100)
	viper.SetDefault("policies.cors.enabled", true)
	viper.SetDefault("policies.cors.allowOrigins", []string{"*"})
	viper.SetDefault("policies.cors.allowMethods", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"})
}

// Config represents the application configuration
type Config struct {
	Server struct {
		Host         string        `yaml:"host"`
		Port         int           `yaml:"port"`
		ReadTimeout  time.Duration `yaml:"readTimeout"`
		WriteTimeout time.Duration `yaml:"writeTimeout"`
	} `yaml:"server"`

	MCP struct {
		Enabled bool   `yaml:"enabled"`
		Host    string `yaml:"host"`
		Port    int    `yaml:"port"`
	} `yaml:"mcp"`

	Logging struct {
		Level  string `yaml:"level"`
		Format string `yaml:"format"`
	} `yaml:"logging"`

	Metrics struct {
		Enabled bool   `yaml:"enabled"`
		Path    string `yaml:"path"`
	} `yaml:"metrics"`

	Tracing struct {
		Enabled     bool   `yaml:"enabled"`
		Endpoint    string `yaml:"endpoint"`
		ServiceName string `yaml:"serviceName"`
	} `yaml:"tracing"`

	Upstream struct {
		Timeout        time.Duration `yaml:"timeout"`
		RetryCount     int           `yaml:"retryCount"`
		RetryDelay     time.Duration `yaml:"retryDelay"`
		CircuitBreaker struct {
			Threshold int           `yaml:"threshold"`
			Timeout   time.Duration `yaml:"timeout"`
		} `yaml:"circuitBreaker"`
	} `yaml:"upstream"`

	Auth struct {
		JWT struct {
			JWKSURL  string `yaml:"jwksURL"`
			Issuer   string `yaml:"issuer"`
			Audience string `yaml:"audience"`
		} `yaml:"jwt"`
		OAuth2 struct {
			TokenURL     string `yaml:"tokenURL"`
			ClientID     string `yaml:"clientID"`
			ClientSecret string `yaml:"clientSecret"`
		} `yaml:"oauth2"`
	} `yaml:"auth"`

	Specs struct {
		DefaultTTL string `yaml:"defaultTTL"`
		MaxSize    string `yaml:"maxSize"`
	} `yaml:"specs"`

	Policies struct {
		RateLimit struct {
			Enabled           bool `yaml:"enabled"`
			RequestsPerMinute int  `yaml:"requestsPerMinute"`
		} `yaml:"rateLimit"`
		CORS struct {
			Enabled      bool     `yaml:"enabled"`
			AllowOrigins []string `yaml:"allowOrigins"`
			AllowMethods []string `yaml:"allowMethods"`
		} `yaml:"cors"`
	} `yaml:"policies"`
}

func expandEnvVars(config *Config) {
	// Expand environment variables in sensitive fields
	config.Auth.OAuth2.ClientID = os.ExpandEnv(config.Auth.OAuth2.ClientID)
	config.Auth.OAuth2.ClientSecret = os.ExpandEnv(config.Auth.OAuth2.ClientSecret)
}
