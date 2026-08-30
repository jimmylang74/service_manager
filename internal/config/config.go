package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type ServiceConfig struct {
	Name             string            `yaml:"name" json:"name"`
	Executable       string            `yaml:"executable" json:"executable"`
	Arguments        []string          `yaml:"arguments" json:"arguments"`
	WorkingDirectory string            `yaml:"working_directory" json:"working_directory"`
	Environment      map[string]string `yaml:"environment" json:"environment"`
	Restart          RestartPolicy     `yaml:"restart" json:"restart"`
}

type RestartPolicy struct {
	Policy     string `yaml:"policy" json:"policy"`
	DelaySec   int    `yaml:"delay_seconds" json:"delay_seconds"`
	MaxRetries int    `yaml:"max_retries" json:"max_retries"`
}

type ManagerConfig struct {
	WebPort  int             `yaml:"web_port" json:"web_port"`
	LogDir   string          `yaml:"log_dir" json:"log_dir"`
	Services []ServiceConfig `yaml:"services" json:"services"`
}

type Loader struct {
	mu       sync.RWMutex
	path     string
	config   *ManagerConfig
	onChange func(*ManagerConfig)
}

func NewLoader(path string) *Loader {
	return &Loader{path: path}
}

func (l *Loader) OnChange(fn func(*ManagerConfig)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onChange = fn
}

func (l *Loader) Load() (*ManagerConfig, error) {
	fmt.Printf("[DEBUG] Loader.Load: reading from path=%s\n", l.path)
	data, err := os.ReadFile(l.path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", l.path, err)
	}
	fmt.Printf("[DEBUG] Loader.Load: read %d bytes\n", len(data))
	cfg, err := parseYAML(data)
	if err != nil {
		return nil, fmt.Errorf("parse config %s: %w", l.path, err)
	}
	if cfg.WebPort == 0 {
		cfg.WebPort = 7070
	}
	if cfg.LogDir == "" {
		cfg.LogDir = defaultLogDir()
	}
	for i := range cfg.Services {
		if cfg.Services[i].Restart.Policy == "" {
			cfg.Services[i].Restart.Policy = "on-failure"
		}
		if cfg.Services[i].Restart.DelaySec == 0 {
			cfg.Services[i].Restart.DelaySec = 5
		}
		if cfg.Services[i].Restart.MaxRetries == 0 {
			cfg.Services[i].Restart.MaxRetries = 10
		}
	}
	l.mu.Lock()
	l.config = cfg
	l.mu.Unlock()
	if l.onChange != nil {
		l.onChange(cfg)
	}
	return cfg, nil
}

func (l *Loader) Save() error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.config == nil {
		return fmt.Errorf("no config loaded")
	}
	data := marshalYAML(l.config)
	if err := os.MkdirAll(filepath.Dir(l.path), 0755); err != nil {
		return err
	}
	return os.WriteFile(l.path, data, 0644)
}

func (l *Loader) UpdateAndSave(cfg *ManagerConfig) error {
	l.mu.Lock()
	l.config = cfg
	fmt.Printf("[DEBUG] Loader.UpdateAndSave: services=%d, path=%s\n", len(cfg.Services), l.path)
	l.mu.Unlock()
	return l.Save()
}

func (l *Loader) Get() *ManagerConfig {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.config != nil {
		fmt.Printf("[DEBUG] Loader.Get: path=%s, services=%d\n", l.path, len(l.config.Services))
	} else {
		fmt.Printf("[DEBUG] Loader.Get: path=%s, config=nil\n", l.path)
	}
	return l.config
}

func (l *Loader) Path() string { return l.path }

func defaultLogDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "logs"
	}
	return filepath.Join(filepath.Dir(exe), "logs")
}

func marshalYAMLValue(v interface{}, ind int) string {
	switch val := v.(type) {
	case int:
		return strconv.Itoa(val)
	case string:
		if strings.ContainsAny(val, ":#{}[],>|*!%@`\"'\\") || val == "" {
			return "\"" + strings.ReplaceAll(val, "\"", "\\\"") + "\""
		}
		return val
	case []string:
		if len(val) == 0 {
			return "[]"
		}
		var sb strings.Builder
		for _, item := range val {
			sb.WriteString(strings.Repeat(" ", ind) + "  - " + marshalYAMLValue(item, ind+2) + "\n")
		}
		return strings.TrimRight(sb.String(), "\n")
	case map[string]string:
		if len(val) == 0 {
			return "{}"
		}
		var sb strings.Builder
		for k, v2 := range val {
			sb.WriteString(strings.Repeat(" ", ind) + "  " + k + ": " + marshalYAMLValue(v2, ind+2) + "\n")
		}
		return strings.TrimRight(sb.String(), "\n")
	case RestartPolicy:
		var sb strings.Builder
		sb.WriteString("\n")
		sb.WriteString(strings.Repeat(" ", ind) + "  policy: " + marshalYAMLValue(val.Policy, ind+2) + "\n")
		sb.WriteString(strings.Repeat(" ", ind) + "  delay_seconds: " + marshalYAMLValue(val.DelaySec, ind+2) + "\n")
		sb.WriteString(strings.Repeat(" ", ind) + "  max_retries: " + marshalYAMLValue(val.MaxRetries, ind+2) + "\n")
		return strings.TrimRight(sb.String(), "\n")
	case ServiceConfig:
		var sb strings.Builder
		sb.WriteString("\n")
		sb.WriteString(strings.Repeat(" ", ind) + "  - name: " + marshalYAMLValue(val.Name, ind+4) + "\n")
		sb.WriteString(strings.Repeat(" ", ind) + "    executable: " + marshalYAMLValue(val.Executable, ind+4) + "\n")
		if len(val.Arguments) > 0 {
			sb.WriteString(strings.Repeat(" ", ind) + "    arguments:\n")
			for _, arg := range val.Arguments {
				sb.WriteString(strings.Repeat(" ", ind) + "      - " + marshalYAMLValue(arg, ind+6) + "\n")
			}
		}
		if val.WorkingDirectory != "" {
			sb.WriteString(strings.Repeat(" ", ind) + "    working_directory: " + marshalYAMLValue(val.WorkingDirectory, ind+4) + "\n")
		}
		if len(val.Environment) > 0 {
			sb.WriteString(strings.Repeat(" ", ind) + "    environment:\n")
			for k, v2 := range val.Environment {
				sb.WriteString(strings.Repeat(" ", ind) + "      " + k + ": " + marshalYAMLValue(v2, ind+6) + "\n")
			}
		}
		sb.WriteString(strings.Repeat(" ", ind) + "    restart:" + marshalYAMLValue(val.Restart, ind+4))
		return strings.TrimRight(sb.String(), "\n")
	default:
		return fmt.Sprintf("%v", v)
	}
}

func marshalYAML(cfg *ManagerConfig) []byte {
	var sb strings.Builder
	sb.WriteString("web_port: " + marshalYAMLValue(cfg.WebPort, 0) + "\n")
	sb.WriteString("log_dir: " + marshalYAMLValue(cfg.LogDir, 0) + "\n")
	sb.WriteString("\nservices:\n")
	for _, svc := range cfg.Services {
		sb.WriteString(marshalYAMLValue(svc, 0) + "\n")
	}
	return []byte(sb.String())
}

var (
	reScalar = regexp.MustCompile(`^(\w[\w_]*)\s*:\s*(.*)$`)
)

func parseYAML(data []byte) (*ManagerConfig, error) {
	lines := strings.Split(string(data), "\n")
	cfg := &ManagerConfig{Services: []ServiceConfig{}}

	var currentService *ServiceConfig
	section := ""

	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		depth := len(line) - len(strings.TrimLeft(line, " "))

		if strings.HasPrefix(trimmed, "- ") {
			itemVal := strings.TrimSpace(trimmed[2:])
			if section == "arguments" && currentService != nil && depth > 2 {
				currentService.Arguments = append(currentService.Arguments, unquote(itemVal))
			} else if kv := reScalar.FindStringSubmatch(itemVal); kv != nil && kv[1] == "name" {
				svc := ServiceConfig{Name: unquote(kv[2])}
				cfg.Services = append(cfg.Services, svc)
				idx := len(cfg.Services) - 1
				currentService = &cfg.Services[idx]
				section = "service_body"
			}
			continue
		}

		kv := reScalar.FindStringSubmatch(trimmed)
		if kv == nil {
			continue
		}
		key := kv[1]
		val := strings.TrimSpace(kv[2])

		if depth == 0 {
			switch key {
			case "web_port":
				cfg.WebPort = atoi(val)
			case "log_dir":
				cfg.LogDir = unquote(val)
			case "services":
				section = "services"
			}
			continue
		}

		if section == "services" && depth == 2 && currentService == nil {
			if key == "name" {
				svc := ServiceConfig{Name: unquote(val)}
				cfg.Services = append(cfg.Services, svc)
				idx := len(cfg.Services) - 1
				currentService = &cfg.Services[idx]
				section = "service_body"
			}
			continue
		}

		if currentService != nil && depth >= 4 {
			switch key {
			case "name":
				currentService.Name = unquote(val)
			case "executable":
				currentService.Executable = unquote(val)
			case "working_directory":
				currentService.WorkingDirectory = unquote(val)
			case "arguments":
				section = "arguments"
			case "environment":
				section = "environment"
				if currentService.Environment == nil {
					currentService.Environment = make(map[string]string)
				}
			case "restart":
				section = "restart"
			case "policy":
				currentService.Restart.Policy = unquote(val)
			case "delay_seconds":
				currentService.Restart.DelaySec = atoi(val)
			case "max_retries":
				currentService.Restart.MaxRetries = atoi(val)
			default:
				if section == "environment" && currentService.Environment != nil {
					currentService.Environment[key] = unquote(val)
				}
			}
			continue
		}

		if currentService != nil && depth == 2 {
			section = ""
		}
	}

	return cfg, nil
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
