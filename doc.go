// Package sdk provides the plugin SDK for Knowledge Tree discovery plugins.
//
// The SDK simplifies writing new discovery plugins by providing:
//   - BasePlugin: common functionality for logging, config, and error handling
//   - Serve(): a helper to start the gRPC plugin server (hashicorp/go-plugin pattern)
//   - Helper functions for creating resources, relationships, and sending events
//
// # Writing a Plugin
//
// A minimal plugin consists of three parts:
//
//  1. Implement the DiscoveryPlugin interface:
//
//     type MyPlugin struct {
//         sdk.BasePlugin
//     }
//
//     func (p *MyPlugin) Info(ctx context.Context) (*plugin.PluginInfo, error) {
//         return &plugin.PluginInfo{
//             Name:    "my-plugin",
//             Version: "1.0.0",
//         }, nil
//     }
//
//     func (p *MyPlugin) Discover(ctx context.Context, req *plugin.DiscoverRequest) (<-chan plugin.DiscoveryEvent, error) {
//         ch := make(chan plugin.DiscoveryEvent)
//         go func() {
//             defer close(ch)
//             // Discover resources and send events on ch
//             res := sdk.NewResource("my-plugin", "my_type", "us-east-1", "123456", "my-resource")
//             res.Properties = map[string]string{"key": "value"}
//             ch <- plugin.DiscoveryEvent{Resource: &plugin.ResourceEvent{
//                 ID:         res.ID,
//                 Type:       res.Type,
//                 Name:       res.Name,
//                 Provider:   res.Provider,
//                 Region:     res.Region,
//                 AccountID:  res.AccountID,
//                 Properties: res.Properties,
//                 Tags:       res.Tags,
//                 RawData:    res.RawData,
//             }}
//         }()
//         return ch, nil
//     }
//
//     func (p *MyPlugin) ValidateConfig(ctx context.Context, config map[string]string) (*plugin.ValidationResult, error) {
//         return &plugin.ValidationResult{Valid: true}, nil
//     }
//
//  2. Create a main() that calls sdk.Serve():
//
//     func main() {
//         sdk.Serve(&MyPlugin{})
//     }
//
//  3. Build the plugin as a standalone binary:
//
//     go build -o my-plugin ./plugins/my-plugin
//
// The plugin binary communicates with the Knowledge Tree orchestrator over gRPC
// using the hashicorp/go-plugin framework for process management and handshake.
package sdk
