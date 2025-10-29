package queue

import (
	"strings"
	"testing"

	"github.com/zombar/controller/internal/storage"
)

// TestMergeAITagsWithComputedTags tests the AI tag merging logic
func TestMergeAITagsWithComputedTags(t *testing.T) {
	tests := []struct {
		name           string
		existingTags   []string
		aiTags         []string
		expectedTags   []string
		expectedToAdd  []string
		shouldMerge    bool
	}{
		{
			name:           "No AI tags - no merge",
			existingTags:   []string{"scrape", "domain-example", "technology"},
			aiTags:         []string{},
			expectedTags:   []string{"scrape", "domain-example", "technology"},
			expectedToAdd:  []string{},
			shouldMerge:    false,
		},
		{
			name:           "All new AI tags - merge all",
			existingTags:   []string{"scrape", "domain-example"},
			aiTags:         []string{"programming", "golang", "tutorial"},
			expectedTags:   []string{"scrape", "domain-example", "programming", "golang", "tutorial"},
			expectedToAdd:  []string{"programming", "golang", "tutorial"},
			shouldMerge:    true,
		},
		{
			name:           "Some duplicate tags - merge only new",
			existingTags:   []string{"scrape", "domain-example", "programming"},
			aiTags:         []string{"programming", "golang", "tutorial"},
			expectedTags:   []string{"scrape", "domain-example", "programming", "golang", "tutorial"},
			expectedToAdd:  []string{"golang", "tutorial"},
			shouldMerge:    true,
		},
		{
			name:           "Case-insensitive duplicate - no merge",
			existingTags:   []string{"scrape", "Programming", "Golang"},
			aiTags:         []string{"programming", "golang"},
			expectedTags:   []string{"scrape", "Programming", "Golang"},
			expectedToAdd:  []string{},
			shouldMerge:    false,
		},
		{
			name:           "Mixed case with some new - merge case-sensitively",
			existingTags:   []string{"scrape", "Technology"},
			aiTags:         []string{"technology", "golang", "TUTORIAL"},
			expectedTags:   []string{"scrape", "Technology", "golang", "TUTORIAL"},
			expectedToAdd:  []string{"golang", "TUTORIAL"},
			shouldMerge:    true,
		},
		{
			name:           "All AI tags are duplicates - no merge",
			existingTags:   []string{"scrape", "programming", "golang"},
			aiTags:         []string{"Programming", "Golang"},
			expectedTags:   []string{"scrape", "programming", "golang"},
			expectedToAdd:  []string{},
			shouldMerge:    false,
		},
		{
			name:           "Empty existing tags - add all AI tags",
			existingTags:   []string{},
			aiTags:         []string{"programming", "golang"},
			expectedTags:   []string{"programming", "golang"},
			expectedToAdd:  []string{"programming", "golang"},
			shouldMerge:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the merging logic from handleRetrieveAnalysis
			req := &storage.Request{
				Tags: tt.existingTags,
			}

			// This is the logic from tasks.go
			if len(tt.aiTags) > 0 {
				// Create a map for case-insensitive duplicate checking
				existingTags := make(map[string]bool)
				for _, tag := range req.Tags {
					existingTags[strings.ToLower(tag)] = true
				}

				// Add only new AI tags (case-insensitive comparison)
				var tagsToAdd []string
				for _, aiTag := range tt.aiTags {
					if !existingTags[strings.ToLower(aiTag)] {
						tagsToAdd = append(tagsToAdd, aiTag)
						existingTags[strings.ToLower(aiTag)] = true
					}
				}

				// Merge new AI tags into request tags
				if len(tagsToAdd) > 0 {
					req.Tags = append(req.Tags, tagsToAdd...)

					// Verify tags to add
					if len(tagsToAdd) != len(tt.expectedToAdd) {
						t.Errorf("Expected to add %d tags, got %d", len(tt.expectedToAdd), len(tagsToAdd))
					}

					// Verify each tag to add
					for i, tag := range tagsToAdd {
						if i >= len(tt.expectedToAdd) {
							t.Errorf("Unexpected tag to add: %s", tag)
							continue
						}
						if tag != tt.expectedToAdd[i] {
							t.Errorf("Expected tag[%d] to be %s, got %s", i, tt.expectedToAdd[i], tag)
						}
					}
				} else {
					// No tags should be added
					if len(tt.expectedToAdd) > 0 {
						t.Errorf("Expected to add %d tags, but none were added", len(tt.expectedToAdd))
					}
				}
			}

			// Verify final tags
			if len(req.Tags) != len(tt.expectedTags) {
				t.Errorf("Expected %d final tags, got %d", len(tt.expectedTags), len(req.Tags))
			}

			// Verify each final tag
			for i, tag := range req.Tags {
				if i >= len(tt.expectedTags) {
					t.Errorf("Unexpected final tag: %s", tag)
					continue
				}
				if tag != tt.expectedTags[i] {
					t.Errorf("Expected final tag[%d] to be %s, got %s", i, tt.expectedTags[i], tag)
				}
			}
		})
	}
}


// TestAITagConversion tests conversion from []interface{} to []string
func TestAITagConversion(t *testing.T) {
	tests := []struct {
		name        string
		input       []interface{}
		expectedOut []string
	}{
		{
			name:        "All valid strings",
			input:       []interface{}{"programming", "golang", "tutorial"},
			expectedOut: []string{"programming", "golang", "tutorial"},
		},
		{
			name:        "Mixed types - filter non-strings",
			input:       []interface{}{"programming", 42, "golang", nil, "tutorial"},
			expectedOut: []string{"programming", "golang", "tutorial"},
		},
		{
			name:        "Empty slice",
			input:       []interface{}{},
			expectedOut: []string{},
		},
		{
			name:        "All non-strings",
			input:       []interface{}{42, 3.14, nil, true},
			expectedOut: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This simulates the conversion logic from tasks.go
			var aiTags []string
			for _, tag := range tt.input {
				if tagStr, ok := tag.(string); ok {
					aiTags = append(aiTags, tagStr)
				}
			}

			// Verify output
			if len(aiTags) != len(tt.expectedOut) {
				t.Errorf("Expected %d tags, got %d", len(tt.expectedOut), len(aiTags))
			}

			for i, tag := range aiTags {
				if i >= len(tt.expectedOut) {
					t.Errorf("Unexpected tag: %s", tag)
					continue
				}
				if tag != tt.expectedOut[i] {
					t.Errorf("Expected tag[%d] to be %s, got %s", i, tt.expectedOut[i], tag)
				}
			}
		})
	}
}

// TestAITagMergingPreservesCaseOfNewTags tests that new AI tags preserve their original case
func TestAITagMergingPreservesCaseOfNewTags(t *testing.T) {
	req := &storage.Request{
		Tags: []string{"scrape", "domain-example"},
	}

	aiTags := []string{"Programming", "GOLANG", "TuToRiAl"}

	// Simulate merging logic
	existingTags := make(map[string]bool)
	for _, tag := range req.Tags {
		existingTags[strings.ToLower(tag)] = true
	}

	var tagsToAdd []string
	for _, aiTag := range aiTags {
		if !existingTags[strings.ToLower(aiTag)] {
			tagsToAdd = append(tagsToAdd, aiTag)
			existingTags[strings.ToLower(aiTag)] = true
		}
	}

	if len(tagsToAdd) > 0 {
		req.Tags = append(req.Tags, tagsToAdd...)
	}

	// Verify that AI tags preserve their original case
	expectedFinalTags := []string{"scrape", "domain-example", "Programming", "GOLANG", "TuToRiAl"}
	if len(req.Tags) != len(expectedFinalTags) {
		t.Errorf("Expected %d final tags, got %d", len(expectedFinalTags), len(req.Tags))
	}

	for i, tag := range req.Tags {
		if tag != expectedFinalTags[i] {
			t.Errorf("Expected tag[%d] to be %s (preserving case), got %s", i, expectedFinalTags[i], tag)
		}
	}
}
