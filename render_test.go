package main

import "testing"

func TestRenderENC(t *testing.T) {
	tests := []struct {
		name string
		doc  cobblerDoc
		want string
	}{
		{
			name: "single class with develop environment",
			doc: cobblerDoc{
				classes:     []string{"roles::infra::storage::vault"},
				environment: "develop",
				hasEnv:      true,
				parameters:  map[string]string{},
			},
			want: "classes:\n" +
				"- roles::infra::storage::vault\n" +
				"environment: develop\n" +
				"parameters:\n" +
				"  enc_env: develop\n" +
				"  enc_role:\n" +
				"  - roles::infra::storage::vault\n",
		},
		{
			name: "environment testing is dropped from top level but kept in enc_env",
			doc: cobblerDoc{
				classes:     []string{"roles::base"},
				environment: "testing",
				hasEnv:      true,
				parameters:  map[string]string{},
			},
			want: "classes:\n" +
				"- roles::base\n" +
				"parameters:\n" +
				"  enc_env: testing\n" +
				"  enc_role:\n" +
				"  - roles::base\n",
		},
		{
			name: "multiple classes",
			doc: cobblerDoc{
				classes:     []string{"roles::a", "roles::b"},
				environment: "production",
				hasEnv:      true,
				parameters:  map[string]string{},
			},
			want: "classes:\n" +
				"- roles::a\n" +
				"- roles::b\n" +
				"environment: production\n" +
				"parameters:\n" +
				"  enc_env: production\n" +
				"  enc_role:\n" +
				"  - roles::a\n" +
				"  - roles::b\n",
		},
		{
			name: "no environment key at all",
			doc: cobblerDoc{
				classes:    []string{"roles::base"},
				hasEnv:     false,
				parameters: map[string]string{},
			},
			want: "classes:\n" +
				"- roles::base\n" +
				"parameters:\n" +
				"  enc_role:\n" +
				"  - roles::base\n",
		},
		{
			name: "pre-existing parameters are preserved and sorted with computed ones",
			doc: cobblerDoc{
				classes:     []string{"roles::base"},
				environment: "develop",
				hasEnv:      true,
				parameters:  map[string]string{"zeta": "1", "alpha": "2"},
			},
			want: "classes:\n" +
				"- roles::base\n" +
				"environment: develop\n" +
				"parameters:\n" +
				"  alpha: '2'\n" +
				"  enc_env: develop\n" +
				"  enc_role:\n" +
				"  - roles::base\n" +
				"  zeta: '1'\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderENC(tt.doc)
			if err != nil {
				t.Fatalf("renderENC returned error: %v", err)
			}
			if got != tt.want {
				t.Errorf("renderENC mismatch\n--- want ---\n%s\n--- got ---\n%s", tt.want, got)
			}
		})
	}
}

func TestParseCobbler(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    cobblerDoc
		wantErr bool
	}{
		{
			name: "map form classes with 4-space indent (encapi cobbler wire)",
			body: "classes:\n    roles::infra::storage::vault: {}\nenvironment: develop\nparameters: {}\n",
			want: cobblerDoc{
				classes:     []string{"roles::infra::storage::vault"},
				environment: "develop",
				hasEnv:      true,
			},
		},
		{
			name: "list form classes",
			body: "classes:\n- roles::base\n- roles::extra\nenvironment: develop\nparameters: {}\n",
			want: cobblerDoc{
				classes:     []string{"roles::base", "roles::extra"},
				environment: "develop",
				hasEnv:      true,
			},
		},
		{
			name: "inline flow list classes",
			body: "classes: [roles::a, roles::b]\nenvironment: develop\nparameters: {}\n",
			want: cobblerDoc{
				classes:     []string{"roles::a", "roles::b"},
				environment: "develop",
				hasEnv:      true,
			},
		},
		{
			name: "empty classes map",
			body: "classes: {}\nenvironment: develop\nparameters: {}\n",
			want: cobblerDoc{
				classes:     nil,
				environment: "develop",
				hasEnv:      true,
			},
		},
		{
			name:    "unexpected top-level key",
			body:    "bogus: value\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCobbler([]byte(tt.body))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCobbler returned error: %v", err)
			}
			if !equalStrings(got.classes, tt.want.classes) {
				t.Errorf("classes = %v, want %v", got.classes, tt.want.classes)
			}
			if got.environment != tt.want.environment {
				t.Errorf("environment = %q, want %q", got.environment, tt.want.environment)
			}
			if got.hasEnv != tt.want.hasEnv {
				t.Errorf("hasEnv = %v, want %v", got.hasEnv, tt.want.hasEnv)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
