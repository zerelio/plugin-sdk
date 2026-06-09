# Knowledge Tree SDK

Go SDK for writing Knowledge Tree discovery plugins.

A discovery plugin discovers infrastructure resources (repositories, servers, databases, etc.) from a provider API and streams them back to Knowledge Tree for graph construction, documentation, and AI-powered querying.

## Quick Start

```go
package main

import (
    "context"
    sdk "github.com/knowledgetree-dev/knowledge-tree-sdk"
    "github.com/knowledgetree-dev/knowledge-tree-sdk/plugin"
)

type MyPlugin struct{ sdk.BasePlugin }

func (p *MyPlugin) Info(ctx context.Context) (*plugin.PluginInfo, error) {
    return &plugin.PluginInfo{
        Name:        "my-provider",
        Version:     "0.0.1",
        Description: "Discovers widgets from MyProvider",
    }, nil
}

func (p *MyPlugin) ValidateConfig(ctx context.Context, cfg map[string]string) (*plugin.ValidationResult, error) {
    if cfg["api_key"] == "" {
        return &plugin.ValidationResult{Valid: false, Errors: []string{"api_key is required"}}, nil
    }
    return &plugin.ValidationResult{Valid: true}, nil
}

func (p *MyPlugin) Discover(ctx context.Context, req *plugin.DiscoverRequest) (<-chan plugin.DiscoveryEvent, error) {
    ch := make(chan plugin.DiscoveryEvent)
    go func() {
        defer close(ch)
        ch <- plugin.ToResourceEvent(plugin.ResourceEvent{
            ID: "my:widget:abc", Type: "my.widget", Name: "Example",
            Provider: "my-provider", Properties: map[string]string{"env": "prod"},
        })
    }()
    return ch, nil
}

func main() {
    sdk.Serve(&MyPlugin{})
}
```

## Install

```bash
go get github.com/knowledgetree-dev/knowledge-tree-sdk@latest
```

## Interface

A plugin implements `plugin.DiscoveryPlugin`:

| Method | Purpose |
|--------|---------|
| `Info()` | Return metadata: name, version, description, config schema |
| `ValidateConfig()` | Check if a config map is valid for this plugin |
| `Discover()` | Stream resources and relationships via a channel |

## Building

Build as a standalone binary. Knowledge Tree loads it as a subprocess via hashicorp/go-plugin.

```bash
go build -o kt-plugin-my-provider .
```

## Plugin Architecture

```
Knowledge Tree API ←→ Plugin Manager ←→ plugin binary (gRPC subprocess)
                                              └── calls provider API
                                                  → emits resources + relationships
```

Communication uses hashicorp/go-plugin over gRPC stdin/stdout multiplexing.

## License

MIT
