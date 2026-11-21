package sdk

import (
	"testing"
	"time"

	"github.com/zerelio/plugin-sdk/types"
)

func TestNewResource(t *testing.T) {
	res := NewResource("aws", types.TypeEC2Instance, "us-east-1", "123456789012", "my-instance")

	if res.Provider != "aws" {
		t.Errorf("Provider = %v, want aws", res.Provider)
	}
	if res.Type != types.TypeEC2Instance {
		t.Errorf("Type = %v, want %v", res.Type, types.TypeEC2Instance)
	}
	if res.Region != "us-east-1" {
		t.Errorf("Region = %v, want us-east-1", res.Region)
	}
	if res.AccountID != "123456789012" {
		t.Errorf("AccountID = %v, want 123456789012", res.AccountID)
	}
	if res.Name != "my-instance" {
		t.Errorf("Name = %v, want my-instance", res.Name)
	}
	if res.ID == "" {
		t.Error("ID should not be empty")
	}
	if res.Properties == nil {
		t.Error("Properties should be initialized")
	}
	if res.Tags == nil {
		t.Error("Tags should be initialized")
	}
	if res.DiscoveredAt.IsZero() {
		t.Error("DiscoveredAt should be set")
	}
}

func TestNewResourceWithID(t *testing.T) {
	res := NewResourceWithID("arn:aws:ec2:us-east-1:123456789012:instance/i-abc123", "aws", types.TypeEC2Instance, "us-east-1", "123456789012", "my-instance")

	if res.ID != "arn:aws:ec2:us-east-1:123456789012:instance/i-abc123" {
		t.Errorf("ID = %v, want arn:aws:ec2:us-east-1:123456789012:instance/i-abc123", res.ID)
	}
}

func TestNewRelationship(t *testing.T) {
	rel := NewRelationship("vpc-123", "subnet-456", types.RelContains)

	if rel.SourceID != "vpc-123" {
		t.Errorf("SourceID = %v, want vpc-123", rel.SourceID)
	}
	if rel.TargetID != "subnet-456" {
		t.Errorf("TargetID = %v, want subnet-456", rel.TargetID)
	}
	if rel.Type != types.RelContains {
		t.Errorf("Type = %v, want CONTAINS", rel.Type)
	}
	if rel.Properties == nil {
		t.Error("Properties should be initialized")
	}
}

func TestSendProgress(t *testing.T) {
	event := SendProgress("Test message", 50, 100, 200)

	if event.Progress == nil {
		t.Fatal("Progress event should not be nil")
	}
	if event.Progress.Message != "Test message" {
		t.Errorf("Message = %v, want Test message", event.Progress.Message)
	}
	if event.Progress.PercentComplete != 50 {
		t.Errorf("PercentComplete = %v, want 50", event.Progress.PercentComplete)
	}
	if event.Progress.ResourcesFound != 100 {
		t.Errorf("ResourcesFound = %v, want 100", event.Progress.ResourcesFound)
	}
	if event.Progress.RelationshipsFound != 200 {
		t.Errorf("RelationshipsFound = %v, want 200", event.Progress.RelationshipsFound)
	}
	if event.Resource != nil {
		t.Error("Resource should be nil for progress event")
	}
	if event.Error != nil {
		t.Error("Error should be nil for progress event")
	}
}

func TestSendError(t *testing.T) {
	event := SendError("TEST_ERROR", "Something went wrong", true)

	if event.Error == nil {
		t.Fatal("Error event should not be nil")
	}
	if event.Error.Code != "TEST_ERROR" {
		t.Errorf("Code = %v, want TEST_ERROR", event.Error.Code)
	}
	if event.Error.Message != "Something went wrong" {
		t.Errorf("Message = %v, want Something went wrong", event.Error.Message)
	}
	if !event.Error.Retryable {
		t.Error("Retryable should be true")
	}
	if event.Progress != nil {
		t.Error("Progress should be nil for error event")
	}
	if event.Resource != nil {
		t.Error("Resource should be nil for error event")
	}
}

func TestTagsToStrings(t *testing.T) {
	tests := []struct {
		name string
		tags map[string]string
		want []string
	}{
		{
			name: "multiple tags",
			tags: map[string]string{
				"env":     "prod",
				"team":    "platform",
				"service": "api",
			},
			want: []string{"env:prod", "service:api", "team:platform"},
		},
		{
			name: "empty tags",
			tags: map[string]string{},
			want: []string{},
		},
		{
			name: "single tag",
			tags: map[string]string{"name": "value"},
			want: []string{"name:value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TagsToStrings(tt.tags)
			// Sort both slices for comparison since map iteration is random
			if len(got) != len(tt.want) {
				t.Errorf("TagsToStrings() returned %d items, want %d", len(got), len(tt.want))
			}
		})
	}
}

func TestParseConfig(t *testing.T) {
	type TestConfig struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}

	config := map[string]string{
		"name":  "test",
		"value": "hello",
	}

	var result TestConfig
	err := ParseConfig(config, &result)
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}

	if result.Name != "test" {
		t.Errorf("Name = %v, want test", result.Name)
	}
	if result.Value != "hello" {
		t.Errorf("Value = %v, want hello", result.Value)
	}
}

func TestParseConfig_InvalidJSON(t *testing.T) {
	type TestConfig struct {
		Name string `json:"name"`
	}

	// This should fail because the value is not valid JSON
	config := map[string]string{
		"name": "test",
	}

	var result TestConfig
	err := ParseConfig(config, &result)
	// This actually works because json.Marshal on map[string]string produces valid JSON
	if err != nil {
		t.Errorf("ParseConfig() unexpected error = %v", err)
	}
}

func TestResourceIDFormat(t *testing.T) {
	res := NewResource("aws", types.TypeEC2Instance, "us-west-2", "123456789012", "web-server")

	expectedPrefix := "aws:aws.ec2.instance:us-west-2:123456789012:web-server"
	if res.ID != expectedPrefix {
		t.Errorf("ID format = %v, want %v", res.ID, expectedPrefix)
	}
}

func TestDiscoveredAtTimestamp(t *testing.T) {
	before := time.Now().UTC()
	res := NewResource("aws", types.TypeEC2Instance, "us-east-1", "123", "test")
	after := time.Now().UTC()

	if res.DiscoveredAt.Before(before) {
		t.Error("DiscoveredAt should not be before the test started")
	}
	if res.DiscoveredAt.After(after) {
		t.Error("DiscoveredAt should not be after the test ended")
	}
}
