package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"os/exec"
	"syscall"
	"time"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/knowledgetree-dev/knowledge-tree-sdk/pb"
	plugintypes "github.com/knowledgetree-dev/knowledge-tree-sdk/plugin"
	"github.com/knowledgetree-dev/knowledge-tree-types"
)

// PluginHandshake is the shared handshake configuration used by all Knowledge
// Tree discovery plugins. The orchestrator and plugin must agree on this
// configuration to establish the gRPC connection.
var PluginHandshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "KNOWLEDGE_TREE_PLUGIN",
	MagicCookieValue: "kt_discovery_plugin_v1",
}

// BasePlugin provides common functionality shared by all discovery plugins.
// Plugin implementations should embed this struct to get logging, configuration,
// and error handling for free.
type BasePlugin struct {
	// Logger is the structured logger for the plugin process. It is initialised
	// when Serve() is called.
	Logger *slog.Logger

	// Config holds the raw key-value configuration passed to the plugin.
	Config map[string]string

	// name is the plugin name, set during Serve.
	name string
}

// SetName sets the plugin name.
func (b *BasePlugin) SetName(name string) {
	b.name = name
}

// Name returns the plugin name.
func (b *BasePlugin) Name() string {
	return b.name
}

// InitLogger initialises the structured logger. This is called automatically by
// Serve(), but can also be called manually in tests.
func (b *BasePlugin) InitLogger(level string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	b.Logger = slog.New(handler).With("plugin", b.name)
}

// LogInfo logs an informational message.
func (b *BasePlugin) LogInfo(msg string, args ...any) {
	if b.Logger != nil {
		b.Logger.Info(msg, args...)
	}
}

// LogDebug logs a debug message.
func (b *BasePlugin) LogDebug(msg string, args ...any) {
	if b.Logger != nil {
		b.Logger.Debug(msg, args...)
	}
}

// LogWarn logs a warning message.
func (b *BasePlugin) LogWarn(msg string, args ...any) {
	if b.Logger != nil {
		b.Logger.Warn(msg, args...)
	}
}

// LogError logs an error message.
func (b *BasePlugin) LogError(msg string, args ...any) {
	if b.Logger != nil {
		b.Logger.Error(msg, args...)
	}
}

// DiscoveryPluginServer implements pb.DiscoveryPluginServer by wrapping a
// native plugintypes.DiscoveryPlugin. It translates between proto types and
// native Go types for the server side of the gRPC connection.
type DiscoveryPluginServer struct {
	pb.UnimplementedDiscoveryPluginServer
	Impl plugintypes.DiscoveryPlugin
}

// Info returns plugin metadata (proto translation).
func (s *DiscoveryPluginServer) Info(ctx context.Context, req *pb.InfoRequest) (*pb.InfoResponse, error) {
	info, err := s.Impl.Info(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.InfoResponse{
		Name:           info.Name,
		Version:        info.Version,
		Description:    info.Description,
		Capabilities:   info.Capabilities,
		ConfigSchema:   info.ConfigSchema,
		CredentialTypes: info.CredentialTypes,
	}, nil
}

// Discover runs a discovery and streams results as proto events.
func (s *DiscoveryPluginServer) Discover(req *pb.DiscoverRequest, stream pb.DiscoveryPlugin_DiscoverServer) error {
	nativeReq := &plugintypes.DiscoverRequest{
		ScopeID: req.ScopeId,
		Config:  req.Config,
		CredentialSource: plugintypes.CredentialSource{
			Type:   req.CredentialSource.GetType(),
			Path:   req.CredentialSource.GetPath(),
			Params: req.CredentialSource.GetParams(),
		},
		TimeoutSeconds: req.TimeoutSeconds,
	}

	eventCh, err := s.Impl.Discover(stream.Context(), nativeReq)
	if err != nil {
		return err
	}

	for event := range eventCh {
		pbEvent := nativeToProtoEvent(&event)
		if err := stream.Send(pbEvent); err != nil {
			return err
		}
	}
	return nil
}

// ValidateConfig validates plugin configuration (proto translation).
func (s *DiscoveryPluginServer) ValidateConfig(ctx context.Context, req *pb.ValidateConfigRequest) (*pb.ValidateConfigResponse, error) {
	result, err := s.Impl.ValidateConfig(ctx, req.Config)
	if err != nil {
		return nil, err
	}
	return &pb.ValidateConfigResponse{
		Valid:    result.Valid,
		Errors:   result.Errors,
		Warnings: result.Warnings,
	}, nil
}

// Serve starts the plugin gRPC server. This is the main entry point that all
// plugin binaries call from their main() function. It sets up logging, registers
// the gRPC service, and handles graceful shutdown.
//
// The plugin implementation must satisfy the plugintypes.DiscoveryPlugin interface.
// Serve blocks until the plugin is terminated by the orchestrator or receives a
// shutdown signal.
//
// Example:
//
//	func main() {
//	    sdk.Serve(&MyPlugin{})
//	}
func Serve(impl plugintypes.DiscoveryPlugin) {
	// Determine the plugin name for logging.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	name := "unknown"
	info, err := impl.Info(ctx)
	if err == nil && info.Name != "" {
		name = info.Name
	}

	// Set up base plugin if the implementation embeds BasePlugin.
	if bp, ok := interface{}(impl).(interface{ SetName(string) }); ok {
		bp.SetName(name)
	}
	if bp, ok := interface{}(impl).(interface{ InitLogger(string) }); ok {
		logLevel := os.Getenv("KT_PLUGIN_LOG_LEVEL")
		if logLevel == "" {
			logLevel = "info"
		}
		bp.InitLogger(logLevel)
	}

	// Create the gRPC server wrapper.
	server := &DiscoveryPluginServer{Impl: impl}

	// Configure the hashicorp/go-plugin serve.
	pluginMap := map[string]plugin.Plugin{
		"discovery": &DiscoveryPluginGRPC{
			DiscoveryPluginServer: server,
		},
	}

	// Set up signal handling for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		if bp, ok := interface{}(impl).(interface{ LogInfo(string, ...any) }); ok {
			bp.LogInfo("received shutdown signal, stopping plugin")
		}
	}()

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: PluginHandshake,
		Plugins:         pluginMap,
		GRPCServer: func(opts []grpc.ServerOption) *grpc.Server {
			return grpc.NewServer(opts...)
		},
	})
}

// DiscoveryPluginGRPC implements the hashicorp/go-plugin Plugin interface
// for serving a DiscoveryPlugin over gRPC.
type DiscoveryPluginGRPC struct {
	plugin.NetRPCUnsupportedPlugin
	DiscoveryPluginServer *DiscoveryPluginServer
}

// GRPCServer registers the gRPC server using the generated proto registration.
func (p *DiscoveryPluginGRPC) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterDiscoveryPluginServer(s, p.DiscoveryPluginServer)
	return nil
}

// GRPCClient returns a plugintypes.DiscoveryPlugin client wrapper.
func (p *DiscoveryPluginGRPC) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &DiscoveryPluginGRPCClient{
		client: pb.NewDiscoveryPluginClient(c),
	}, nil
}

// DiscoveryPluginGRPCClient wraps a pb.DiscoveryPluginClient and implements
// the native plugintypes.DiscoveryPlugin interface by translating between
// proto and native types.
type DiscoveryPluginGRPCClient struct {
	client pb.DiscoveryPluginClient
}

// Info returns plugin metadata by calling the gRPC Info endpoint.
func (c *DiscoveryPluginGRPCClient) Info(ctx context.Context) (*plugintypes.PluginInfo, error) {
	resp, err := c.client.Info(ctx, &pb.InfoRequest{})
	if err != nil {
		return nil, err
	}
	return &plugintypes.PluginInfo{
		Name:            resp.Name,
		Version:         resp.Version,
		Description:     resp.Description,
		Capabilities:    resp.Capabilities,
		ConfigSchema:    resp.ConfigSchema,
		CredentialTypes: resp.CredentialTypes,
	}, nil
}

// Discover runs a discovery and streams events by calling the gRPC Discover endpoint.
func (c *DiscoveryPluginGRPCClient) Discover(ctx context.Context, req *plugintypes.DiscoverRequest) (<-chan plugintypes.DiscoveryEvent, error) {
	pbReq := &pb.DiscoverRequest{
		ScopeId:     req.ScopeID,
		Config:      req.Config,
		TimeoutSeconds: req.TimeoutSeconds,
	}
	if req.CredentialSource.Type != "" {
		pbReq.CredentialSource = &pb.CredentialSource{
			Type:   req.CredentialSource.Type,
			Path:   req.CredentialSource.Path,
			Params: req.CredentialSource.Params,
		}
	}

	stream, err := c.client.Discover(ctx, pbReq)
	if err != nil {
		return nil, err
	}

	eventCh := make(chan plugintypes.DiscoveryEvent, 100)
	go func() {
		defer close(eventCh)
		for {
			pbEvent, err := stream.Recv()
			if err != nil {
				return
			}
			native := protoToNativeEvent(pbEvent)
			eventCh <- native
		}
	}()
	return eventCh, nil
}

// ValidateConfig validates plugin configuration by calling the gRPC ValidateConfig endpoint.
func (c *DiscoveryPluginGRPCClient) ValidateConfig(ctx context.Context, config map[string]string) (*plugintypes.ValidationResult, error) {
	resp, err := c.client.ValidateConfig(ctx, &pb.ValidateConfigRequest{Config: config})
	if err != nil {
		return nil, err
	}
	return &plugintypes.ValidationResult{
		Valid:    resp.Valid,
		Errors:   resp.Errors,
		Warnings: resp.Warnings,
	}, nil
}

// DefaultPluginLoader creates a hashicorp/go-plugin client for the given
// binary path and returns a DiscoveryPlugin that communicates over gRPC.
// The returned closer should be called to kill the plugin process when done.
func DefaultPluginLoader(path string) (plugintypes.DiscoveryPlugin, func(), error) {
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: PluginHandshake,
		Plugins: map[string]plugin.Plugin{
			"discovery": &DiscoveryPluginGRPC{},
		},
		Cmd:             exec.Command(path),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		StartTimeout:    10 * time.Second,
		GRPCDialOptions: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		},
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, nil, fmt.Errorf("failed to connect to plugin: %w", err)
	}

	raw, err := rpcClient.Dispense("discovery")
	if err != nil {
		client.Kill()
		return nil, nil, fmt.Errorf("failed to dispense plugin: %w", err)
	}

	dp, ok := raw.(plugintypes.DiscoveryPlugin)
	if !ok {
		client.Kill()
		return nil, nil, fmt.Errorf("unexpected plugin type %T", raw)
	}

	return dp, func() { client.Kill() }, nil
}

// nativeToProtoEvent converts a native DiscoveryEvent to a proto DiscoveryEvent.
func nativeToProtoEvent(e *plugintypes.DiscoveryEvent) *pb.DiscoveryEvent {
	pbEvent := &pb.DiscoveryEvent{}
	switch {
	case e.Resource != nil:
		pbEvent.Event = &pb.DiscoveryEvent_Resource{
			Resource: &pb.ResourceDiscovered{
				Id:           e.Resource.ID,
				Type:         e.Resource.Type,
				Name:         e.Resource.Name,
				Provider:     e.Resource.Provider,
				Region:       e.Resource.Region,
				AccountId:    e.Resource.AccountID,
				Properties:   e.Resource.Properties,
				Tags:         e.Resource.Tags,
				RawData:      e.Resource.RawData,
				DiscoveredAt: e.Resource.DiscoveredAt,
			},
		}
	case e.Relationship != nil:
		pbEvent.Event = &pb.DiscoveryEvent_Relationship{
			Relationship: &pb.RelationshipDiscovered{
				SourceId:         e.Relationship.SourceID,
				TargetId:         e.Relationship.TargetID,
				RelationshipType: e.Relationship.Type,
				Properties:       e.Relationship.Properties,
			},
		}
	case e.Progress != nil:
		pbEvent.Event = &pb.DiscoveryEvent_Progress{
			Progress: &pb.ProgressUpdate{
				Message:            e.Progress.Message,
				PercentComplete:    e.Progress.PercentComplete,
				ResourcesFound:     e.Progress.ResourcesFound,
				RelationshipsFound: e.Progress.RelationshipsFound,
			},
		}
	case e.Error != nil:
		pbEvent.Event = &pb.DiscoveryEvent_Error{
			Error: &pb.ErrorOccurred{
				Code:      e.Error.Code,
				Message:   e.Error.Message,
				Retryable: e.Error.Retryable,
			},
		}
	}
	return pbEvent
}

// protoToNativeEvent converts a proto DiscoveryEvent to a native DiscoveryEvent.
func protoToNativeEvent(e *pb.DiscoveryEvent) plugintypes.DiscoveryEvent {
	var native plugintypes.DiscoveryEvent
	switch ev := e.Event.(type) {
	case *pb.DiscoveryEvent_Resource:
		native.Resource = &plugintypes.ResourceEvent{
			ID:           ev.Resource.Id,
			Type:         ev.Resource.Type,
			Name:         ev.Resource.Name,
			Provider:     ev.Resource.Provider,
			Region:       ev.Resource.Region,
			AccountID:    ev.Resource.AccountId,
			Properties:   ev.Resource.Properties,
			Tags:         ev.Resource.Tags,
			RawData:      ev.Resource.RawData,
			DiscoveredAt: ev.Resource.DiscoveredAt,
		}
	case *pb.DiscoveryEvent_Relationship:
		native.Relationship = &plugintypes.RelationshipEvent{
			SourceID:   ev.Relationship.SourceId,
			TargetID:   ev.Relationship.TargetId,
			Type:       ev.Relationship.RelationshipType,
			Properties: ev.Relationship.Properties,
		}
	case *pb.DiscoveryEvent_Progress:
		native.Progress = &plugintypes.ProgressEvent{
			Message:            ev.Progress.Message,
			PercentComplete:    ev.Progress.PercentComplete,
			ResourcesFound:     ev.Progress.ResourcesFound,
			RelationshipsFound: ev.Progress.RelationshipsFound,
		}
	case *pb.DiscoveryEvent_Error:
		native.Error = &plugintypes.ErrorEvent{
			Code:      ev.Error.Code,
			Message:   ev.Error.Message,
			Retryable: ev.Error.Retryable,
		}
	}
	return native
}

// Ensure DiscoveryPluginGRPCClient implements plugintypes.DiscoveryPlugin.
var _ plugintypes.DiscoveryPlugin = (*DiscoveryPluginGRPCClient)(nil)

// Ensure DiscoveryPluginServer implements pb.DiscoveryPluginServer.
var _ pb.DiscoveryPluginServer = (*DiscoveryPluginServer)(nil)

// ToResourceEvent converts a types.Resource to a plugin.ResourceEvent.
func ToResourceEvent(r *types.Resource) *plugintypes.ResourceEvent {
	return &plugintypes.ResourceEvent{
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
	}
}

// ToRelationshipEvent converts a types.Relationship to a plugin.RelationshipEvent.
func ToRelationshipEvent(r *types.Relationship) *plugintypes.RelationshipEvent {
	return &plugintypes.RelationshipEvent{
		SourceID:   r.SourceID,
		TargetID:   r.TargetID,
		Type:       r.Type,
		Properties: r.Properties,
	}
}

// MarshalRawData marshals a value to JSON bytes for the RawData field.
func MarshalRawData(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte(fmt.Sprintf(`{"error":"marshal failed: %s"}`, err.Error()))
	}
	return data
}
