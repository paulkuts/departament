// config/config.go
package config

import (
	"fmt"
	"mitm-departament/pkg/valid"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type AuthCfg struct {
	AccessTokenTTL  time.Duration `mapstructure:"access_token_ttl" validate:"required"`
	RefreshTokenTTL time.Duration `mapstructure:"refresh_token_ttl" validate:"required"`
	JwtSecret       string        `mapstructure:"jwt_secret" validate:"required"`
}

// AppConfig общие настройки приложения
type AppConfig struct {
	Name    string `mapstructure:"name" validate:"required"`
	Version string `mapstructure:"version"`
	Env     string `mapstructure:"env" validate:"oneof=development staging production"`
}

// DBConfig настройки базы данных
type DBConfig struct {
	LocalPath string `mapstructure:"local_path" validate:"required,filepath"`
	DataDir   string `mapstructure:"data_dir" validate:"required"`
	ExportDir string `mapstructure:"export_dir" validate:"required"`
}

type PhotoConfig struct {
	InventoryPhotoDir string `mapstructure:"inventory_photo_dir" validate:"required"`
	AvatarPhotoDir    string `mapstructure:"avatar_photo_dir" validate:"required"`
	MaxPhotoSize      int    `mapstructure:"max_photo_size" validate:"required"`
	MaxPhotos         int    `mapstructure:"max_photos" validate:"required"`
}

// LoggerConfig настройки логгера (совместим с zap.Config)
type LoggerConfig struct {
	Level             string   `mapstructure:"level" validate:"oneof=debug info warn error dpanic panic fatal"`
	Development       bool     `mapstructure:"development"`
	DisableCaller     bool     `mapstructure:"disable_caller"`
	DisableStacktrace bool     `mapstructure:"disable_stacktrace"`
	Encoding          string   `mapstructure:"encoding" validate:"oneof=console json"`
	OutputPaths       []string `mapstructure:"output_paths"`
	ErrorOutputPaths  []string `mapstructure:"error_output_paths"`
}

type ServerConfig struct {
	Host           string        `mapstructure:"host" validate:"required"`
	Port           string        `mapstructure:"port" validate:"required"`
	ReadTimeout    time.Duration `mapstructure:"read_timeout" validate:"required"`
	WriteTimeout   time.Duration `mapstructure:"write_timeout" validate:"required"`
	IdleTimeout    time.Duration `mapstructure:"idle_timeout" validate:"required"`
	MaxHeaderBytes int           `mapstructure:"max_header_bytes" validate:"required"`
}

// Config корневая структура конфигурации
type Config struct {
	App    AppConfig    `mapstructure:"app"`
	Auth   AuthCfg      `mapstructure:"auth"`
	DB     DBConfig     `mapstructure:"db"`
	Photo  PhotoConfig  `mapstructure:"photo"`
	Logger LoggerConfig `mapstructure:"logger"`
	Server ServerConfig `mapstructure:"server"`
}

// NewConfig загружает конфигурацию с приоритетами:
// 1. Переменные окружения
// 2. Файл .env
// 3. YAML-файл в ./configs/
// 4. Hardcoded дефолты
func NewConfig() (*Config, error) {
	// 1. Загружаем .env если есть
	_ = godotenv.Load() // игнорируем ошибку если файла нет

	// 2. Инициализируем viper
	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvPrefix("REG")

	// Пути к конфигам
	v.AddConfigPath("./configs")

	// Имя конфига (можно переопределить через REG_CONFIG_NAME)
	configName := v.GetString("CONFIG_NAME")
	if configName == "" {
		configName = "default"
	}
	v.SetConfigName(configName)
	v.SetConfigType("yaml")

	// Читаем файл (ошибка допустима — будут использованы дефолты)
	_ = v.ReadInConfig()

	// 3. Устанавливаем дефолты
	setDefaults(v)

	// 4. Анмаршалим в структуру
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// 5. Пост-обработка: если путь к БД не задан — используем дефолт
	if cfg.DB.LocalPath == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
		cfg.DB.LocalPath = filepath.Join(wd, "departamentMITM.db")
	}

	// 6. Валидация
	if err := valid.ValidateStruct(cfg); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return &cfg, nil
}

// setDefaults устанавливает значения по умолчанию
func setDefaults(v *viper.Viper) {
	// App
	v.SetDefault("app.name", "departamentMITM-app")
	v.SetDefault("app.version", "0.1.0")
	v.SetDefault("app.env", "development")

	// DB
	// LocalPath намеренно не задаём здесь — вычислим после анмаршалинга

	// Logger (совместимо с zap)
	v.SetDefault("logger.level", "info")
	v.SetDefault("logger.development", true)
	v.SetDefault("logger.encoding", "console")
	v.SetDefault("logger.output_paths", []string{"stdout"})
	v.SetDefault("logger.error_output_paths", []string{"stderr"})
	v.SetDefault("logger.disable_caller", false)
	v.SetDefault("logger.disable_stacktrace", false)
}

// GetDBDir возвращает директорию, где лежит файл БД
func (c *Config) GetDBDir() string {
	return filepath.Dir(c.DB.LocalPath)
}

// IsProduction возвращает true если окружение production
func (c *Config) IsProduction() bool {
	return c.App.Env == "production"
}
