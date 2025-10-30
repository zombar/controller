package queue

import (
	"encoding/json"
	"testing"
)

// TestMetadataExtraction_DifferentTextFields tests that raw_text, cleaned_text, and heuristic_cleaned_text
// are correctly kept separate through JSON marshaling/unmarshaling
func TestMetadataExtraction_DifferentTextFields(t *testing.T) {
	// Three distinct text versions that should NEVER be mixed
	rawText := "This is the raw text from the scraper. It may contain navigation, ads, and other artifacts."
	heuristicText := "This is the heuristic cleaned text. It's shorter and cleaner than raw."
	aiCleanedText := "This is the AI-enhanced cleaned text. It contains more detailed information and additional context that was added by the AI analysis process. This text should be longer than the heuristic version."

	// Create mock analysis metadata (what textanalyzer returns)
	analysisMetadata := map[string]interface{}{
		"heuristic_cleaned_text": heuristicText,
		"cleaned_text":           aiCleanedText, // AI-enhanced (only present when AI enrichment runs)
		"synopsis":               "Test synopsis",
		"tags":                   []interface{}{"tag1", "tag2"},
	}

	t.Logf("INPUT - raw_text: %q (length=%d)", rawText, len(rawText))
	t.Logf("INPUT - heuristic_cleaned_text: %q (length=%d)", heuristicText, len(heuristicText))
	t.Logf("INPUT - cleaned_text: %q (length=%d)", aiCleanedText, len(aiCleanedText))

	// Simulate what the retrieval task does: create scraper_metadata and analyzer_metadata
	requestMetadata := make(map[string]interface{})

	// Scraper metadata contains raw_text
	requestMetadata["scraper_metadata"] = map[string]interface{}{
		"raw_text": rawText,
		"title":    "Test Document",
	}

	// Analyzer metadata contains the cleaned versions
	requestMetadata["analyzer_metadata"] = make(map[string]interface{})
	analyzerMetadata := requestMetadata["analyzer_metadata"].(map[string]interface{})

	// Extract fields (simulating tasks.go lines 962-977)
	if cleanedText, ok := analysisMetadata["cleaned_text"].(string); ok {
		analyzerMetadata["cleaned_text"] = cleanedText
		t.Logf("EXTRACTED - cleaned_text: length=%d", len(cleanedText))
	}
	if heuristicCleanedText, ok := analysisMetadata["heuristic_cleaned_text"].(string); ok {
		analyzerMetadata["heuristic_cleaned_text"] = heuristicCleanedText
		t.Logf("EXTRACTED - heuristic_cleaned_text: length=%d", len(heuristicCleanedText))
	}
	if synopsis, ok := analysisMetadata["synopsis"].(string); ok {
		analyzerMetadata["synopsis"] = synopsis
	}

	// Log state before marshaling
	am := requestMetadata["analyzer_metadata"].(map[string]interface{})
	t.Logf("BEFORE JSON marshal - cleaned_text: %q", am["cleaned_text"])
	t.Logf("BEFORE JSON marshal - heuristic_cleaned_text: %q", am["heuristic_cleaned_text"])

	// Simulate JSON marshaling (what UpdateRequestMetadata does)
	metadataJSON, err := json.Marshal(requestMetadata)
	if err != nil {
		t.Fatalf("Failed to marshal metadata: %v", err)
	}

	t.Logf("JSON size: %d bytes", len(metadataJSON))

	// Simulate JSON unmarshaling (what happens when reading from DB)
	var unmarshaledMetadata map[string]interface{}
	if err := json.Unmarshal(metadataJSON, &unmarshaledMetadata); err != nil {
		t.Fatalf("Failed to unmarshal metadata: %v", err)
	}

	// Extract scraper_metadata from unmarshaled data
	unmarshaledScraperMetadata, ok := unmarshaledMetadata["scraper_metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("scraper_metadata not found in unmarshaled data")
	}

	// Verify raw_text
	unmarshaledRawText, ok := unmarshaledScraperMetadata["raw_text"].(string)
	if !ok {
		t.Fatal("raw_text not found or not a string in scraper_metadata")
	}

	// Extract analyzer_metadata from unmarshaled data
	unmarshaledAnalyzerMetadata, ok := unmarshaledMetadata["analyzer_metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("analyzer_metadata not found in unmarshaled data")
	}

	// Verify cleaned_text
	unmarshaledCleanedText, ok := unmarshaledAnalyzerMetadata["cleaned_text"].(string)
	if !ok {
		t.Fatal("cleaned_text not found or not a string")
	}

	// Verify heuristic_cleaned_text
	unmarshaledHeuristicText, ok := unmarshaledAnalyzerMetadata["heuristic_cleaned_text"].(string)
	if !ok {
		t.Fatal("heuristic_cleaned_text not found or not a string")
	}

	// Log what was unmarshaled
	t.Logf("AFTER JSON unmarshal - raw_text: %q (length=%d)", unmarshaledRawText, len(unmarshaledRawText))
	t.Logf("AFTER JSON unmarshal - heuristic_cleaned_text: %q (length=%d)", unmarshaledHeuristicText, len(unmarshaledHeuristicText))
	t.Logf("AFTER JSON unmarshal - cleaned_text: %q (length=%d)", unmarshaledCleanedText, len(unmarshaledCleanedText))

	// Assertions - verify all three fields are preserved correctly
	if unmarshaledRawText != rawText {
		t.Errorf("raw_text mismatch after JSON round-trip:\nexpected: %q\ngot: %q", rawText, unmarshaledRawText)
	}

	if unmarshaledHeuristicText != heuristicText {
		t.Errorf("heuristic_cleaned_text mismatch after JSON round-trip:\nexpected: %q\ngot: %q", heuristicText, unmarshaledHeuristicText)
	}

	if unmarshaledCleanedText != aiCleanedText {
		t.Errorf("cleaned_text mismatch after JSON round-trip:\nexpected: %q\ngot: %q", aiCleanedText, unmarshaledCleanedText)
	}

	// Verify all three fields are DIFFERENT from each other
	if unmarshaledRawText == unmarshaledHeuristicText {
		t.Error("BUG: raw_text and heuristic_cleaned_text are identical when they should be different")
	}

	if unmarshaledRawText == unmarshaledCleanedText {
		t.Error("BUG: raw_text and cleaned_text are identical when they should be different")
	}

	if unmarshaledHeuristicText == unmarshaledCleanedText {
		t.Error("BUG: heuristic_cleaned_text and cleaned_text are identical when they should be different")
	}

	// Success message
	if unmarshaledRawText != unmarshaledHeuristicText &&
	   unmarshaledHeuristicText != unmarshaledCleanedText &&
	   unmarshaledRawText != unmarshaledCleanedText {
		t.Logf("SUCCESS: All three text fields (raw, heuristic, AI) are correctly preserved as separate values")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
