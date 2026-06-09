<div align="center">
  <h1>Knowledge Tree SDK</h1>
  <p>Go SDK for building infrastructure discovery plugins</p>
</div>

<p align="center">
  <a href="https://github.com/knowledgetree-dev/knowledge-tree-sdk/releases"><img src="https://img.shields.io/github/v/release/knowledgetree-dev/knowledge-tree-sdk?style=flat&label=version" alt="Version"></a>
  <a href="#"><img src="https://img.shields.io/github/actions/workflow/status/knowledgetree-dev/knowledge-tree-sdk/ci.yml?branch=main&style=flat&label=CI" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg?style=flat" alt="MIT License"></a>
  <a href="https://pkg.go.dev/github.com/knowledgetree-dev/knowledge-tree-sdk"><img src="https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat" alt="Go Reference"></a>
</p>

---

Build discovery plugins that feed infrastructure data into [Knowledge Tree](https://github.com/knowledgetree-dev). A plugin connects to any provider API (AWS, GitHub, Kubernetes, PagerDuty, etc.), discovers resources, and streams them back as structured events via gRPC.

Plugins are standalone binaries launched as subprocesses via [hashicorp/go-plugin](https://github.com/hashicorp/go-plugin). No shared libraries, no sidecars — drop a binary in the plugin directory and it works.

## Quick Start

```go
package main

import (
    "context"
    sdk "github.com/knowledgetree-dev/knowledge-tree-sdk"
    "github.com/knowledgetree-dev/knowledge-tree-sdk/plugin"
)

type MyPlugin struct{ sdk.BasePlugin }

func (p *MyPlugin) Info(context.Context, *plugin.Empty) (*plugin.PluginInfo, error) {
    return &plugin.PluginInfo{
        Name:        "my-provider",
        Version:     "0.1.0",
        Description: "Discovers widgets from MyProvider",
    }, nil
}

func (p *MyPlugin) Discover(ctx context.Context, req *plugin.DiscoverRequest) (
    <-chan plugin.DiscoveryEvent, error,
) {
    ch := make(chan plugin.DiscoveryEvent)
    go func() {
        defer close(ch)
        ch <- sdk.ToResourceEvent(sdk.NewResource(
            "my-provider", "my.widget", "us-east-1", "acct-123", "example",
        ))
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

Every plugin implements `plugin.DiscoveryPlugin`:

| Method | Purpose |
|--------|---------|
| `Info()` | Return metadata: name, version, capabilities, config schema |
| `ValidateConfig(*ValidateConfigRequest)` | Check if a config map is valid for this plugin |
| `Discover(*DiscoverRequest, plugin.DiscoveryPlugin_DiscoverServer)` | Stream resources, relationships, and progress |

The SDK provides:

- **Resource helpers** — `sdk.NewResource()`, `sdk.NewRelationship()` with type-safe builders
- **gRPC transport** — hashicorp/go-plugin over stdin/stdout multiplexing — zero port conflicts
- **Config parsing** — structured config validation from raw maps
- **Progress reporting** — stream progress events during long discovery runs

## Architecture

```
┌─────────────────────────────────────────────┐
│ Knowledge Tree Plugin Manager                │
│  ┌──────────┐   gRPC/stdin    ┌──────────┐ │
│  │ Manager  │◄──────────────►│  Plugin  │ │
│  │          │   subprocess    │  Binary  │ │
│  └──────────┘                 └────┬─────┘ │
│                                    │        │
│  Knowledge Tree API ◄──────────────┘        │
└─────────────────────────────────────────────┘
```

The orchestrator spawns the plugin binary, negotiates the go-plugin handshake, and streams discovery events. Plugins never need to know about the transport layer — the SDK handles it.

## Resource Model

```go
// A discovered infrastructure entity
resource := sdk.NewResource(
    provider,          // "aws"
    resourceType,      // "ec2.instance"
    region,            // "us-east-1"
    accountID,         // "acct-123"
    name,              // "web-server-01"
    sdk.WithProperty("instance_type", "t3.large"),
    sdk.WithTag("env", "production"),
)

// A relationship between two resources
rel := sdk.NewRelationship(
    resourceA,         // source
    resourceB,         // target
    sdk.RelDependsOn,  // relationship type
)
```

## Build

```bash
go build -o kt-plugin-my-provider .
```

## Contributing

PRs welcome. For significant changes, open an issue first.

```bash
# Dev setup
go mod tidy
go test ./... -v -count=1 -race
go build ./...
```

## License

MIT
