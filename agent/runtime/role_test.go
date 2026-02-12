package runtime

import "testing"

func TestParseRole(t *testing.T) {
	tests := []struct {
		name  string
		task  string
		label string
		want  string
	}{
		{
			name:  "label frontend has priority",
			task:  "[backend] task from task prefix",
			label: "[frontend] ui task",
			want:  RoleFrontend,
		},
		{
			name:  "custom role from label",
			task:  "[backend] implement api",
			label: "[qa] verify cases",
			want:  "qa",
		},
		{
			name:  "task backend",
			task:  "[backend] implement api",
			label: "",
			want:  RoleBackend,
		},
		{
			name:  "default backend",
			task:  "plain task",
			label: "plain label",
			want:  RoleBackend,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRole(tt.task, tt.label)
			if got != tt.want {
				t.Fatalf("ParseRole() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripRolePrefix(t *testing.T) {
	tests := []struct {
		name string
		task string
		want string
	}{
		{
			name: "strip frontend",
			task: "[frontend] build login page",
			want: "build login page",
		},
		{
			name: "strip backend",
			task: "[backend] build user api",
			want: "build user api",
		},
		{
			name: "strip custom role",
			task: "[devops] configure ci",
			want: "configure ci",
		},
		{
			name: "invalid role format keeps original",
			task: "[qa team] check regression",
			want: "[qa team] check regression",
		},
		{
			name: "keep untouched",
			task: "plain task",
			want: "plain task",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripRolePrefix(tt.task)
			if got != tt.want {
				t.Fatalf("StripRolePrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeRole(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "frontend", want: RoleFrontend},
		{in: "backend", want: RoleBackend},
		{in: " FRONTEND ", want: RoleFrontend},
		{in: "unknown", want: "unknown"},
		{in: "devops-team", want: "devops-team"},
		{in: "qa_team", want: "qa_team"},
		{in: "qa team", want: RoleBackend},
		{in: "", want: RoleBackend},
	}

	for _, tt := range tests {
		got := NormalizeRole(tt.in)
		if got != tt.want {
			t.Fatalf("NormalizeRole(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
