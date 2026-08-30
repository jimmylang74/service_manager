package config

import (
	"reflect"
	"testing"
)

func TestParseYAML_TwoServices(t *testing.T) {
	data := []byte(`web_port: 7070
log_dir: ./logs

services:
  - name: svc-one
    executable: /bin/echo
    arguments:
      - "hello from svc-one"
    working_directory: /tmp
    environment:
      ONE_ENV: alpha
    restart:
      policy: on-failure
      delay_seconds: 5
      max_retries: 3

  - name: svc-two
    executable: /usr/local/bin/monitor
    arguments:
      - --config
      - /etc/monitor.yaml
    working_directory: /opt/monitor
    environment:
      TWO_ENV: beta
    restart:
      policy: always
      delay_seconds: 7
      max_retries: 9
`)
	cfg, err := parseYAML(data)
	if err != nil {
		t.Fatalf("parseYAML returned error: %v", err)
	}
	if cfg.WebPort != 7070 {
		t.Errorf("WebPort = %d, want 7070", cfg.WebPort)
	}
	if cfg.LogDir != "./logs" {
		t.Errorf("LogDir = %q, want ./logs", cfg.LogDir)
	}
	if len(cfg.Services) != 2 {
		t.Fatalf("got %d services, want 2: %+v", len(cfg.Services), cfg.Services)
	}

	one := cfg.Services[0]
	if one.Name != "svc-one" || one.Executable != "/bin/echo" {
		t.Errorf("service[0] = %+v, want svc-one /bin/echo", one)
	}
	if !reflect.DeepEqual(one.Arguments, []string{"hello from svc-one"}) {
		t.Errorf("service[0].Arguments = %v, want [hello from svc-one]", one.Arguments)
	}
	if one.WorkingDirectory != "/tmp" {
		t.Errorf("service[0].WorkingDirectory = %q, want /tmp", one.WorkingDirectory)
	}
	if !reflect.DeepEqual(one.Environment, map[string]string{"ONE_ENV": "alpha"}) {
		t.Errorf("service[0].Environment = %v, want {ONE_ENV: alpha}", one.Environment)
	}
	if one.Restart.Policy != "on-failure" || one.Restart.DelaySec != 5 || one.Restart.MaxRetries != 3 {
		t.Errorf("service[0].Restart = %+v, want on-failure/5/3", one.Restart)
	}

	two := cfg.Services[1]
	if two.Name != "svc-two" || two.Executable != "/usr/local/bin/monitor" {
		t.Errorf("service[1] = %+v, want svc-two /usr/local/bin/monitor", two)
	}
	if !reflect.DeepEqual(two.Arguments, []string{"--config", "/etc/monitor.yaml"}) {
		t.Errorf("service[1].Arguments = %v, want [--config /etc/monitor.yaml]", two.Arguments)
	}
	if two.WorkingDirectory != "/opt/monitor" {
		t.Errorf("service[1].WorkingDirectory = %q, want /opt/monitor", two.WorkingDirectory)
	}
	if !reflect.DeepEqual(two.Environment, map[string]string{"TWO_ENV": "beta"}) {
		t.Errorf("service[1].Environment = %v, want {TWO_ENV: beta}", two.Environment)
	}
	if two.Restart.Policy != "always" || two.Restart.DelaySec != 7 || two.Restart.MaxRetries != 9 {
		t.Errorf("service[1].Restart = %+v, want always/7/9", two.Restart)
	}
}

func TestParseYAML_ArgumentsDirectlyBeforeNextService(t *testing.T) {
	// Service[0]'s last sub-section is arguments. The second "- name:" entry
	// must still start a new service instead of being swallowed by the stale
	// "arguments" section state.
	data := []byte(`services:
  - name: svc-one
    arguments:
      - hello
      - world
  - name: svc-two
    arguments:
      - hi
`)
	cfg, err := parseYAML(data)
	if err != nil {
		t.Fatalf("parseYAML returned error: %v", err)
	}
	if len(cfg.Services) != 2 {
		t.Fatalf("got %d services, want 2: %+v", len(cfg.Services), cfg.Services)
	}
	if got := cfg.Services[0].Arguments; !reflect.DeepEqual(got, []string{"hello", "world"}) {
		t.Errorf("service[0].Arguments = %v, want [hello world]", got)
	}
	if got := cfg.Services[1].Arguments; !reflect.DeepEqual(got, []string{"hi"}) {
		t.Errorf("service[1].Arguments = %v, want [hi]", got)
	}
}

func TestParseYAML_NameLikeArgumentNotTreatedAsService(t *testing.T) {
	// An argument that literally looks like "name: x" (nested list item) must
	// stay an argument, not spawn a new service.
	data := []byte(`services:
  - name: svc
    arguments:
      - name: uses-same-key
`)
	cfg, err := parseYAML(data)
	if err != nil {
		t.Fatalf("parseYAML returned error: %v", err)
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("got %d services, want 1: %+v", len(cfg.Services), cfg.Services)
	}
	if got := cfg.Services[0].Arguments; !reflect.DeepEqual(got, []string{"name: uses-same-key"}) {
		t.Errorf("service[0].Arguments = %v, want [name: uses-same-key]", got)
	}
}

func TestParseYAML_SingleService(t *testing.T) {
	// The current in-repo config.yaml format must keep parsing unchanged.
	data := []byte(`web_port: 7070
log_dir: ./logs

services:
  - name: echo-test
    executable: /bin/echo
    arguments:
      - "hello from service-manager"
    working_directory: /tmp
    environment:
      TEST_ENV: development
    restart:
      policy: on-failure
      delay_seconds: 5
      max_retries: 3
`)
	cfg, err := parseYAML(data)
	if err != nil {
		t.Fatalf("parseYAML returned error: %v", err)
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("got %d services, want 1", len(cfg.Services))
	}
	svc := cfg.Services[0]
	if svc.Name != "echo-test" || svc.Executable != "/bin/echo" {
		t.Errorf("service = %+v, want echo-test /bin/echo", svc)
	}
	if !reflect.DeepEqual(svc.Arguments, []string{"hello from service-manager"}) {
		t.Errorf("Arguments = %v, want [hello from service-manager]", svc.Arguments)
	}
	if svc.WorkingDirectory != "/tmp" {
		t.Errorf("WorkingDirectory = %q, want /tmp", svc.WorkingDirectory)
	}
	if !reflect.DeepEqual(svc.Environment, map[string]string{"TEST_ENV": "development"}) {
		t.Errorf("Environment = %v, want {TEST_ENV: development}", svc.Environment)
	}
	if svc.Restart.Policy != "on-failure" || svc.Restart.DelaySec != 5 || svc.Restart.MaxRetries != 3 {
		t.Errorf("Restart = %+v, want on-failure/5/3", svc.Restart)
	}
}

func TestParseYAML_RoundTrip(t *testing.T) {
	// Save() output must parse back to the same config (two services).
	orig, err := parseYAML([]byte(`services:
  - name: a
    executable: /bin/a
    arguments:
      - -x
      - "1"
    working_directory: /tmp
    environment:
      A: "1"
      B: two
    restart:
      policy: always
      delay_seconds: 5
      max_retries: 10
  - name: b
    executable: /bin/b
`))
	if err != nil {
		t.Fatalf("parseYAML returned error: %v", err)
	}
	reparsed, err := parseYAML(marshalYAML(orig))
	if err != nil {
		t.Fatalf("parseYAML(marshalYAML) returned error: %v", err)
	}
	if !reflect.DeepEqual(reparsed, orig) {
		t.Errorf("round trip mismatch:\norig: %+v\nreparsed: %+v", orig, reparsed)
	}
}