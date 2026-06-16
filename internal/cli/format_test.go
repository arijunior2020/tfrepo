package cli

import "testing"

func TestCountLabel(t *testing.T) {
	tests := []struct {
		count            int
		singular, plural string
		want             string
	}{
		{0, "repositório", "repositórios", "0 repositórios"},
		{1, "repositório", "repositórios", "1 repositório"},
		{2, "repositório", "repositórios", "2 repositórios"},
		{1, "branch", "branches", "1 branch"},
		{3, "branch", "branches", "3 branches"},
	}

	for _, tt := range tests {
		if got := countLabel(tt.count, tt.singular, tt.plural); got != tt.want {
			t.Errorf("countLabel(%d, %q, %q) = %q, want %q", tt.count, tt.singular, tt.plural, got, tt.want)
		}
	}
}
