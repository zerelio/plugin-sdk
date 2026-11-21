# Zerelio Plugin SDK

Go SDK for building [Zerelio](https://github.com/zerelio/knowledge) discovery plugins.

## Install

```bash
go get github.com/zerelio/plugin-sdk@latest
```

## Overview

Plugins are standalone binaries launched as subprocesses by the Zerelio platform. Each plugin connects to a provider API, discovers resources, and streams them back as structured events over gRPC via [hashicorp/go-plugin](https://github.com/hashicorp/go-plugin). The SDK handles all transport, serialisation, and lifecycle management — your plugin only implements the discovery logic.

## The Plugin Interface

Every plugin implements `plugin.DiscoveryPlugin`:

```go
type DiscoveryPlugin interface {
    Info(ctx context.Context) (*PluginInfo, error)
    Discover(ctx context.Context, req *DiscoverRequest) (<-chan DiscoveryEvent, error)
    ValidateConfig(ctx context.Context, config map[string]string) (*ValidationResult, error)
}
```

| Method           | Purpose                                                          |
|------------------|------------------------------------------------------------------|
| `Info`           | Return plugin name, version, description, and config schema      |
| `Discover`       | Stream resources, relationships, and progress events             |
| `ValidateConfig` | Validate a raw config map before discovery runs                  |

## Implementing a Plugin

```go
package main

import (
    "context"

    sdk "github.com/zerelio/plugin-sdk"
    "github.com/zerelio/plugin-sdk/plugin"
    "github.com/zerelio/plugin-sdk/types"
)

type MyPlugin struct{ sdk.BasePlugin }

func (p *MyPlugin) Info(_ context.Context) (*plugin.PluginInfo, error) {
    return &plugin.PluginInfo{
        Name:        "my-provider",
        Version:     "0.1.0",
        Description: "Discovers widgets from MyProvider",
    }, nil
}

func (p *MyPlugin) ValidateConfig(_ context.Context, cfg map[string]string) (*plugin.ValidationResult, error) {
    if cfg["api_key"] == "" {
        return &plugin.ValidationResult{Valid: false, Errors: []string{"api_key is required"}}, nil
    }
    return &plugin.ValidationResult{Valid: true}, nil
}

func (p *MyPlugin) Discover(ctx context.Context, req *plugin.DiscoverRequest) (<-chan plugin.DiscoveryEvent, error) {
    ch := make(chan plugin.DiscoveryEvent)
    go func() {
        defer close(ch)
        r := types.NewResource("my-provider", "my.widget", "us-east-1", "acct-123", "example-widget")
        ch <- plugin.DiscoveryEvent{Resource: sdk.ToResourceEvent(r)}
    }()
    return ch, nil
}

func main() {
    sdk.Serve(&MyPlugin{})
}
```

## SDK Utilities

- **`BasePlugin`** — embed for structured logging (`LogInfo`, `LogDebug`, `LogWarn`, `LogError`) and config storage
- **`types.NewResource()`** — build a `Resource` with optional properties and tags via `types.WithProperty` / `types.WithTag`
- **`types.NewRelationship()`** — link two resources with a typed relationship
- **`sdk.ToResourceEvent()` / `sdk.ToRelationshipEvent()`** — convert types to channel events
- **`sdk.MarshalRawData()`** — JSON-marshal any value for the `RawData` field
- **HTTP client** — `httpclient` package with retry, circuit breaker, and rate limiting
- **Crypto utilities** — `crypto` package for token encryption at rest

## Build

```bash
CGO_ENABLED=0 go build -o my-plugin .
```

## Contributing

PRs welcome. For significant changes, open an issue first.

```bash
go mod tidy
go test ./... -race
```

## License

MIT

---

## Part of Zerelio

This plugin is part of the [Zerelio](https://github.com/zerelio/knowledge) infrastructure knowledge graph platform.
