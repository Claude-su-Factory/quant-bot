package cli

import "testing"

func TestStatusEmoji(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{"success", "✅"},
		{"failed", "❌"},
		{"running", "⏳"},
		{"unknown", "❓"},
		{"", "❓"},
	}
	for _, c := range cases {
		got := statusEmoji(c.status)
		if got != c.want {
			t.Errorf("statusEmoji(%q) = %q, 기대 %q", c.status, got, c.want)
		}
	}
}
