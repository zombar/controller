package clients

import "testing"

func TestNormalizeTag(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Single word tags (no hyphens)
		{"Single word", "technology", "technology"},
		{"Single word caps", "TECHNOLOGY", "TECHNOLOGY"},
		
		// Double-barrelled tags (one hyphen) - should remain unchanged
		{"Double-barrelled", "machine-learning", "machine-learning"},
		{"Double-barrelled caps", "Machine-Learning", "Machine-Learning"},
		{"Double-barrelled numbers", "web-3", "web-3"},
		
		// Triple+ barrelled tags (multiple hyphens) - should be truncated to double
		{"Triple-barrelled", "machine-learning-model", "machine-learning"},
		{"Quad-barrelled", "deep-neural-network-architecture", "deep-neural"},
		{"Five parts", "one-two-three-four-five", "one-two"},
		
		// Edge cases
		{"Empty string", "", ""},
		{"Just hyphen", "-", "-"},
		{"Start with hyphen", "-test", "-test"},
		{"End with hyphen", "test-", "test-"},
		{"Multiple consecutive hyphens", "test--tag", "test-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeTag(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeTag(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
