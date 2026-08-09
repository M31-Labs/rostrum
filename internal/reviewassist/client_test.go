package reviewassist

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/odvcencio/programma/internal/domain"
)

func TestEvaluateUsesOfflinePreviewWithoutAPIKey(t *testing.T) {
	assessment, err := (Client{}).Evaluate(context.Background(), testPlan(), testSubmission())
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Provider != "local-preview" || assessment.Model != "programma-rubric-preview-v1" {
		t.Fatalf("unexpected provenance: %#v", assessment)
	}
	if len(assessment.Scores) != 2 {
		t.Fatalf("got %d scores", len(assessment.Scores))
	}
}

func TestEvaluateCallsResponsesAPIWithStructuredSchema(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/responses" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer secret-test-key" {
			t.Errorf("authorization header missing")
		}
		if err := json.NewDecoder(request.Body).Decode(&captured); err != nil {
			t.Fatal(err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"status":"completed",
			"model":"gpt-5.6-terra-2026-07-01",
			"output":[{"type":"message","content":[{"type":"output_text","text":"{\"scores\":{\"fit\":4.4,\"evidence\":3.9},\"comments\":\"Strong fit; request one measured result.\",\"recommendation\":\"yes\"}"}]}]
		}`))
	}))
	defer server.Close()

	assessment, err := (Client{
		APIKey: "secret-test-key", Model: "gpt-5.6-terra", BaseURL: server.URL, HTTPClient: server.Client(),
	}).Evaluate(context.Background(), testPlan(), testSubmission())
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Provider != "openai" || assessment.Model != "gpt-5.6-terra-2026-07-01" {
		t.Fatalf("unexpected provenance: %#v", assessment)
	}
	if assessment.Scores["fit"] != 4.4 || assessment.Recommendation != "yes" {
		t.Fatalf("unexpected assessment: %#v", assessment)
	}
	text, ok := captured["text"].(map[string]any)
	if !ok {
		t.Fatalf("text config missing: %#v", captured)
	}
	format := text["format"].(map[string]any)
	if format["type"] != "json_schema" || format["strict"] != true {
		t.Fatalf("structured output not strict: %#v", format)
	}
	if captured["store"] != false {
		t.Fatalf("request should set store=false: %#v", captured["store"])
	}
}

func TestEvaluateRejectsRefusal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"completed","model":"gpt-test","output":[{"type":"message","content":[{"type":"refusal","refusal":"cannot review"}]}]}`))
	}))
	defer server.Close()

	_, err := (Client{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()}).Evaluate(context.Background(), testPlan(), testSubmission())
	if err == nil {
		t.Fatal("expected refusal error")
	}
}

func testPlan() domain.ReviewPlan {
	return domain.ReviewPlan{Criteria: []domain.RubricCriterion{
		{ID: "fit", Name: "Fit", Description: "Program fit", MaxScore: 5, Weight: 60},
		{ID: "evidence", Name: "Evidence", Description: "Grounded claims", MaxScore: 5, Weight: 40},
	}}
}

func testSubmission() domain.Submission {
	return domain.Submission{
		ID: "sub_test", Title: "Measured agent operations", Abstract: "A field report with evidence from operators and a concrete failure.",
		Format: "Talk", CategoryID: "agents", Level: "Intermediate", Tags: []string{"operations"},
	}
}
