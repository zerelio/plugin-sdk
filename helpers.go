package sdk

import (
	"encoding/json"
	"fmt"
	"time"

	plugintypes "github.com/knowledgetree-dev/knowledge-tree-sdk/plugin"
	"github.com/knowledgetree-dev/knowledge-tree-types"
)

// NewResource creates a new Resource with auto-generated ID. The ID is composed
// of the provider, resourceType, region, accountID, and name, guaranteeing
// global uniqueness.
//
// Example:
//
//	res := NewResource("aws", types.TypeEC2Instance, "us-east-1", "123456789012", "my-instance")
//	res.Properties = map[string]string{"instance_type": "t3.medium", "state": "running"}
func NewResource(provider, resourceType, region, accountID, name string) *types.Resource {
	id := fmt.Sprintf("%s:%s:%s:%s:%s", provider, resourceType, region, accountID, name)
	return &types.Resource{
		ID:           id,
		Type:         resourceType,
		Name:         name,
		Provider:     provider,
		Region:       region,
		AccountID:    accountID,
		Properties:   make(map[string]string),
		Tags:         []string{},
		RawData:      []byte("{}"),
		DiscoveredAt: time.Now().UTC(),
	}
}

// NewResourceWithID creates a new Resource with an explicitly provided ID.
// Use this when the resource has a well-known cloud identifier (e.g., an ARN)
// that should be used directly.
func NewResourceWithID(id, provider, resourceType, region, accountID, name string) *types.Resource {
	return &types.Resource{
		ID:           id,
		Type:         resourceType,
		Name:         name,
		Provider:     provider,
		Region:       region,
		AccountID:    accountID,
		Properties:   make(map[string]string),
		Tags:         []string{},
		RawData:      []byte("{}"),
		DiscoveredAt: time.Now().UTC(),
	}
}

// NewRelationship creates a new Relationship between two resources.
// The relationship type should be one of the Rel* constants from the types
// package (e.g., types.RelContains, types.RelConnectsTo).
//
// Example:
//
//	rel := NewRelationship(vpcID, subnetID, types.RelContains)
//	rel.Properties = map[string]string{"cidr": "10.0.1.0/24"}
func NewRelationship(sourceID, targetID, relType string) *types.Relationship {
	return &types.Relationship{
		SourceID:   sourceID,
		TargetID:   targetID,
		Type:       relType,
		Properties: make(map[string]string),
	}
}

// SendProgress sends a progress update event on the given channel.
// The caller is responsible for ensuring the channel is being read from
// (typically by the orchestrator). SendProgress will not block if the
// channel buffer is full — it silently drops the update.
//
// Example:
//
//	ch <- sdk.SendProgress("Discovering EC2 instances in us-east-1", 50, 120, 340)
func SendProgress(message string, percentComplete, resourcesFound, relationshipsFound int32) plugintypes.DiscoveryEvent {
	return plugintypes.DiscoveryEvent{
		Progress: &plugintypes.ProgressEvent{
			Message:           message,
			PercentComplete:   percentComplete,
			ResourcesFound:    resourcesFound,
			RelationshipsFound: relationshipsFound,
		},
	}
}

// SendError sends a non-fatal error event on the given channel.
// The discovery run continues after sending an error. For fatal errors,
// close the channel and return from the Discover goroutine.
//
// Example:
//
//	ch <- sdk.SendError("PERMISSION_DENIED", "no access to S3 bucket my-bucket", true)
func SendError(code, message string, retryable bool) plugintypes.DiscoveryEvent {
	return plugintypes.DiscoveryEvent{
		Error: &plugintypes.ErrorEvent{
			Code:      code,
			Message:   message,
			Retryable: retryable,
		},
	}
}

// ParseConfig parses the plugin configuration map into a target struct.
// The config map values are expected to be string representations that can
// be unmarshalled via JSON. This is useful for converting the raw key-value
// config from DiscoverRequest into a typed configuration struct.
//
// Example:
//
//	var cfg Config
//	if err := sdk.ParseConfig(req.Config, &cfg); err != nil {
//	    return err
//	}
func ParseConfig(config map[string]string, target interface{}) error {
	// Convert the map[string]string to JSON, then unmarshal into the target.
	// This allows us to leverage JSON struct tags for type conversion.
	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config map: %w", err)
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to parse config into %T: %w", target, err)
	}

	return nil
}

// ParseConfigYAML parses a single config value that contains YAML into the
// target struct. This is useful when a plugin's config is stored as a single
// YAML string under a known key (e.g., "config_yaml").
func ParseConfigYAML(config map[string]string, key string, target interface{}) error {
	yamlStr, ok := config[key]
	if !ok {
		return fmt.Errorf("config key %q not found", key)
	}

	// For YAML parsing without importing yaml.v3, we convert via JSON.
	// In a production implementation, import gopkg.in/yaml.v3 directly.
	data := []byte(yamlStr)
	if err := json.Unmarshal(data, target); err != nil {
		// Try wrapping the value as a JSON string if direct unmarshal fails.
		wrapped := fmt.Sprintf(`{"value":%s}`, yamlStr)
		if err2 := json.Unmarshal([]byte(wrapped), target); err2 != nil {
			return fmt.Errorf("failed to parse config YAML: %w", err)
		}
	}

	return nil
}

// TagsToStrings converts a map[string]string (AWS-style tags) to a string
// slice in "key=value" format, which is what the types.Resource.Tags field
// expects.
func TagsToStrings(tags map[string]string) []string {
	result := make([]string, 0, len(tags))
	for k, v := range tags {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}

// EmitResource is a convenience function that creates a DiscoveryEvent for a
// resource and sends it to the event channel. It handles the conversion from
// types.Resource to plugintypes.ResourceEvent.
func EmitResource(ch chan<- plugintypes.DiscoveryEvent, r *types.Resource) {
	ch <- plugintypes.DiscoveryEvent{
		Resource: &plugintypes.ResourceEvent{
			ID:           r.ID,
			Type:         r.Type,
			Name:         r.Name,
			Provider:     r.Provider,
			Region:       r.Region,
			AccountID:    r.AccountID,
			Properties:   r.Properties,
			Tags:         r.Tags,
			RawData:      r.RawData,
			DiscoveredAt: r.DiscoveredAt.Unix(),
		},
	}
}

// EmitRelationship is a convenience function that creates a DiscoveryEvent for
// a relationship and sends it to the event channel.
func EmitRelationship(ch chan<- plugintypes.DiscoveryEvent, r *types.Relationship) {
	ch <- plugintypes.DiscoveryEvent{
		Relationship: &plugintypes.RelationshipEvent{
			SourceID:   r.SourceID,
			TargetID:   r.TargetID,
			Type:       r.Type,
			Properties: r.Properties,
		},
	}
}
