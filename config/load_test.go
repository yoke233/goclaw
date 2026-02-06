package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Load without config file (should use defaults)
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg == nil {
		t.Fatal("Expected non-nil config")
	}

	// Check defaults
	if cfg.Agents.Defaults.Model == "" {
		t.Error("Expected default model to be set")
	}

	if cfg.Agents.Defaults.MaxIterations == 0 {
		t.Error("Expected default max_iterations to be set")
	}

	if cfg.Gateway.Port == 0 {
		t.Error("Expected default gateway port to be set")
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test config file
	configContent := `{
		"agents": {
			"defaults": {
				"model": "gpt-4",
				"max_iterations": 20,
				"temperature": 0.8,
				"max_tokens": 8192
			}
		},
		"providers": {
			"openai": {
				"api_key": "test-key-12345",
				"base_url": "https://api.openai.com/v1",
				"timeout": 60
			}
		},
		"gateway": {
			"host": "0.0.0.0",
			"port": 9000,
			"read_timeout": 60,
			"write_timeout": 60
		}
	}`

	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load config
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Check values
	if cfg.Agents.Defaults.Model != "gpt-4" {
		t.Errorf("Expected model 'gpt-4', got '%s'", cfg.Agents.Defaults.Model)
	}

	if cfg.Agents.Defaults.MaxIterations != 20 {
		t.Errorf("Expected max_iterations 20, got %d", cfg.Agents.Defaults.MaxIterations)
	}

	if cfg.Agents.Defaults.Temperature != 0.8 {
		t.Errorf("Expected temperature 0.8, got %f", cfg.Agents.Defaults.Temperature)
	}

	if cfg.Providers.OpenAI.APIKey != "test-key-12345" {
		t.Errorf("Expected API key 'test-key-12345', got '%s'", cfg.Providers.OpenAI.APIKey)
	}

	if cfg.Gateway.Port != 9000 {
		t.Errorf("Expected port 9000, got %d", cfg.Gateway.Port)
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: &Config{
				Agents: AgentsConfig{
					Defaults: AgentDefaults{
						Model:         "gpt-4",
						MaxIterations: 10,
						Temperature:   0.7,
						MaxTokens:     4096,
					},
				},
				Providers: ProvidersConfig{
					OpenAI: OpenAIProviderConfig{
						APIKey: "sk-test-key-123456789",
					},
				},
				Gateway: GatewayConfig{
					Port:         8080,
					ReadTimeout:  30,
					WriteTimeout: 30,
				},
				Channels: ChannelsConfig{
					Telegram: TelegramChannelConfig{
						Enabled: false,
					},
					WhatsApp: WhatsAppChannelConfig{
						Enabled: false,
					},
					Feishu: FeishuChannelConfig{
						Enabled: false,
					},
				},
				Tools: ToolsConfig{
					Shell: ShellToolConfig{
						Enabled:    true,
						DeniedCmds: []string{"rm -rf", "dd", "mkfs"},
						Timeout:    30,
					},
					Web: WebToolConfig{
						Timeout: 10,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing model",
			cfg: &Config{
				Agents: AgentsConfig{
					Defaults: AgentDefaults{
						Model:         "",
						MaxIterations: 10,
						Temperature:   0.7,
					},
				},
				Providers: ProvidersConfig{
					OpenAI: OpenAIProviderConfig{
						APIKey: "sk-test-key-123456789",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no provider API key",
			cfg: &Config{
				Agents: AgentsConfig{
					Defaults: AgentDefaults{
						Model:         "gpt-4",
						MaxIterations: 10,
						Temperature:   0.7,
					},
				},
				Providers: ProvidersConfig{},
			},
			wantErr: true,
		},
		{
			name: "invalid temperature",
			cfg: &Config{
				Agents: AgentsConfig{
					Defaults: AgentDefaults{
						Model:         "gpt-4",
						MaxIterations: 10,
						Temperature:   3.0, // Invalid: > 2
					},
				},
				Providers: ProvidersConfig{
					OpenAI: OpenAIProviderConfig{
						APIKey: "sk-test-key-123456789",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "telegram enabled but no token",
			cfg: &Config{
				Agents: AgentsConfig{
					Defaults: AgentDefaults{
						Model:         "gpt-4",
						MaxIterations: 10,
					},
				},
				Providers: ProvidersConfig{
					OpenAI: OpenAIProviderConfig{
						APIKey: "sk-test-key-123456789",
					},
				},
				Channels: ChannelsConfig{
					Telegram: TelegramChannelConfig{
						Enabled: true,
						Token:   "",
					},
				},
				Gateway: GatewayConfig{
					Port: 8080,
				},
				Tools: ToolsConfig{
					Shell: ShellToolConfig{
						Enabled:    true,
						DeniedCmds: []string{"rm -rf", "dd", "mkfs"},
						Timeout:    30,
					},
					Web: WebToolConfig{
						Timeout: 10,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid gateway port",
			cfg: &Config{
				Agents: AgentsConfig{
					Defaults: AgentDefaults{
						Model:         "gpt-4",
						MaxIterations: 10,
					},
				},
				Providers: ProvidersConfig{
					OpenAI: OpenAIProviderConfig{
						APIKey: "sk-test-key-123456789",
					},
				},
				Gateway: GatewayConfig{
					Port: 70000, // Invalid: > 65535
				},
				Tools: ToolsConfig{
					Shell: ShellToolConfig{
						Enabled:    true,
						DeniedCmds: []string{"rm -rf", "dd", "mkfs"},
						Timeout:    30,
					},
					Web: WebToolConfig{
						Timeout: 10,
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSaveConfig(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{
		Agents: AgentsConfig{
			Defaults: AgentDefaults{
				Model:         "gpt-4",
				MaxIterations: 20,
			},
		},
		Providers: ProvidersConfig{
			OpenAI: OpenAIProviderConfig{
				APIKey: "sk-test-key-123456789",
			},
		},
	}

	configPath := filepath.Join(tmpDir, "config.json")

	// Save config
	if err := Save(cfg, configPath); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Check file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}

	// Load and verify
	loadedCfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}

	if loadedCfg.Agents.Defaults.Model != cfg.Agents.Defaults.Model {
		t.Errorf("Expected model %s, got %s", cfg.Agents.Defaults.Model, loadedCfg.Agents.Defaults.Model)
	}
}

func TestGetDefaultConfigPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get home directory")
	}

	path, err := GetDefaultConfigPath()
	if err != nil {
		t.Fatalf("Failed to get default config path: %v", err)
	}

	expectedPath := filepath.Join(homeDir, ".goclaw", "config.json")
	if path != expectedPath {
		t.Errorf("Expected path %s, got %s", expectedPath, path)
	}
}
