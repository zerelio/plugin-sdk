package sdk

import (
	"testing"
	"time"

	"google.golang.org/grpc"

	plugintypes "github.com/knowledgetree-dev/knowledge-tree-sdk/plugin"
	"github.com/knowledgetree-dev/knowledge-tree-types"
)

func TestBasePlugin_SetName(t *testing.T) {
	bp := &BasePlugin{}
	bp.SetName("test-plugin")
	if bp.Name() != "test-plugin" {
		t.Errorf("Name() = %q, want %q", bp.Name(), "test-plugin")
	}
}

func TestBasePlugin_InitLogger(t *testing.T) {
	tests := []struct {
		name  string
		level string
	}{
		{"debug level", "debug"},
		{"info level", "info"},
		{"warn level", "warn"},
		{"error level", "error"},
		{"default level", ""},
		{"unknown defaults to info", "trace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp := &BasePlugin{}
			bp.InitLogger(tt.level)
			if bp.Logger == nil {
				t.Error("Logger should be initialized")
			}
		})
	}
}

func TestBasePlugin_LogMethods(t *testing.T) {
	bp := &BasePlugin{}
	// Should not panic when Logger is nil
	bp.LogInfo("test info")
	bp.LogDebug("test debug")
	bp.LogWarn("test warn")
	bp.LogError("test error")

	// Should not panic when Logger is initialized
	bp.InitLogger("info")
	bp.LogInfo("test info with logger")
	bp.LogDebug("test debug with logger")
	bp.LogWarn("test warn with logger")
	bp.LogError("test error with logger")
}

func TestToResourceEvent(t *testing.T) {
	now := time.Now().UTC()
	res := &types.Resource{
		ID:           "aws:ec2:us-east-1:123:my-instance",
		Type:         types.TypeEC2Instance,
		Name:         "my-instance",
		Provider:     "aws",
		Region:       "us-east-1",
		AccountID:    "123",
		Properties:   map[string]string{"state": "running"},
		Tags:         []string{"env:prod"},
		RawData:      []byte(`{"foo":"bar"}`),
		DiscoveredAt: now,
	}

	event := ToResourceEvent(res)
	if event == nil {
		t.Fatal("event should not be nil")
	}
	if event.ID != res.ID {
		t.Errorf("ID = %q, want %q", event.ID, res.ID)
	}
	if event.Type != res.Type {
		t.Errorf("Type = %q, want %q", event.Type, res.Type)
	}
	if event.Name != res.Name {
		t.Errorf("Name = %q, want %q", event.Name, res.Name)
	}
	if event.Provider != res.Provider {
		t.Errorf("Provider = %q, want %q", event.Provider, res.Provider)
	}
	if event.Region != res.Region {
		t.Errorf("Region = %q, want %q", event.Region, res.Region)
	}
	if event.AccountID != res.AccountID {
		t.Errorf("AccountID = %q, want %q", event.AccountID, res.AccountID)
	}
	if event.DiscoveredAt != now.Unix() {
		t.Errorf("DiscoveredAt = %d, want %d", event.DiscoveredAt, now.Unix())
	}
}

func TestToRelationshipEvent(t *testing.T) {
	rel := &types.Relationship{
		SourceID:   "vpc-123",
		TargetID:   "subnet-456",
		Type:       types.RelContains,
		Properties: map[string]string{"cidr": "10.0.0.0/16"},
	}

	event := ToRelationshipEvent(rel)
	if event == nil {
		t.Fatal("event should not be nil")
	}
	if event.SourceID != "vpc-123" {
		t.Errorf("SourceID = %q, want %q", event.SourceID, "vpc-123")
	}
	if event.TargetID != "subnet-456" {
		t.Errorf("TargetID = %q, want %q", event.TargetID, "subnet-456")
	}
	if event.Type != types.RelContains {
		t.Errorf("Type = %q, want %q", event.Type, types.RelContains)
	}
	if event.Properties["cidr"] != "10.0.0.0/16" {
		t.Errorf("Properties[cidr] = %q, want %q", event.Properties["cidr"], "10.0.0.0/16")
	}
}

func TestMarshalRawData(t *testing.T) {
	tests := []struct {
		name string
		in   interface{}
		want string
	}{
		{
			name: "simple map",
			in:   map[string]string{"key": "value"},
			want: `{"key":"value"}`,
		},
		{
			name: "struct",
			in:   struct{ Name string }{Name: "test"},
			want: `{"Name":"test"}`,
		},
		{
			name: "nil",
			in:   nil,
			want: "null",
		},
		{
			name: "slice",
			in:   []string{"a", "b"},
			want: `["a","b"]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MarshalRawData(tt.in)
			if string(got) != tt.want {
				t.Errorf("MarshalRawData() = %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestMarshalRawData_Unmarshalable(t *testing.T) {
	// Channels can't be marshalled to JSON
	got := MarshalRawData(make(chan int))
	if len(got) == 0 {
		t.Error("should return error JSON for unmarshalable types")
	}
	if got[0] != '{' {
		t.Errorf("expected error JSON, got %q", string(got))
	}
}

func TestEmitResource(t *testing.T) {
	ch := make(chan plugintypes.DiscoveryEvent, 1)
	res := NewResource("aws", types.TypeEC2Instance, "us-east-1", "123", "test")

	EmitResource(ch, res)

	select {
	case event := <-ch:
		if event.Resource == nil {
			t.Fatal("Resource should not be nil")
		}
		if event.Resource.ID != res.ID {
			t.Errorf("ID = %q, want %q", event.Resource.ID, res.ID)
		}
		if event.Resource.Name != "test" {
			t.Errorf("Name = %q, want %q", event.Resource.Name, "test")
		}
	default:
		t.Fatal("expected event on channel")
	}
}

func TestEmitRelationship(t *testing.T) {
	ch := make(chan plugintypes.DiscoveryEvent, 1)
	rel := NewRelationship("src", "tgt", types.RelDependsOn)

	EmitRelationship(ch, rel)

	select {
	case event := <-ch:
		if event.Relationship == nil {
			t.Fatal("Relationship should not be nil")
		}
		if event.Relationship.SourceID != "src" {
			t.Errorf("SourceID = %q, want %q", event.Relationship.SourceID, "src")
		}
		if event.Relationship.TargetID != "tgt" {
			t.Errorf("TargetID = %q, want %q", event.Relationship.TargetID, "tgt")
		}
	default:
		t.Fatal("expected event on channel")
	}
}

func TestParseConfigYAML(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]string
		key     string
		wantErr bool
	}{
		{
			name:    "missing key",
			config:  map[string]string{"other": "val"},
			key:     "config_yaml",
			wantErr: true,
		},
		{
			name:    "valid json value",
			config:  map[string]string{"config_yaml": `{"name":"test"}`},
			key:     "config_yaml",
			wantErr: false,
		},
		{
			name:    "string value",
			config:  map[string]string{"config_yaml": `"hello"`},
			key:     "config_yaml",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var target map[string]interface{}
			err := ParseConfigYAML(tt.config, tt.key, &target)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseConfigYAML() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPluginHandshake(t *testing.T) {
	if PluginHandshake.ProtocolVersion != 1 {
		t.Errorf("ProtocolVersion = %d, want 1", PluginHandshake.ProtocolVersion)
	}
	if PluginHandshake.MagicCookieKey != "KNOWLEDGE_TREE_PLUGIN" {
		t.Errorf("MagicCookieKey = %q, want %q", PluginHandshake.MagicCookieKey, "KNOWLEDGE_TREE_PLUGIN")
	}
	if PluginHandshake.MagicCookieValue != "kt_discovery_plugin_v1" {
		t.Errorf("MagicCookieValue = %q", PluginHandshake.MagicCookieValue)
	}
}

func TestDiscoveryPluginGRPC_GRPCServer(t *testing.T) {
	server := &DiscoveryPluginServer{}
	p := &DiscoveryPluginGRPC{DiscoveryPluginServer: server}
	gs := grpc.NewServer()
	defer gs.Stop()
	err := p.GRPCServer(nil, gs)
	if err != nil {
		t.Errorf("GRPCServer() error = %v", err)
	}
}
