// Package plugin defines the discovery plugin interface and plugin management
// infrastructure. Plugins are the extensible mechanism by which Knowledge Tree
// discovers infrastructure resources. Each plugin implements the DiscoveryPlugin
// interface, typically by wrapping a gRPC client that communicates with an
// external plugin process.
package plugin

import (
	"context"
	"fmt"
	"sync"
)

// PluginInfo contains static metadata about a discovery plugin.
type PluginInfo struct {
	// Name is the unique identifier of the plugin (e.g., "aws", "kubernetes").
	Name string `json:"name"`

	// Version is the semantic version of the plugin.
	Version string `json:"version"`

	// Description is a human-readable summary of what the plugin discovers.
	Description string `json:"description"`

	// Capabilities lists the categories of resources this plugin can discover
	// (e.g., "compute", "networking", "storage").
	Capabilities []string `json:"capabilities"`

	// ConfigSchema is a JSON Schema string that describes the configuration
	// fields the plugin accepts.
	ConfigSchema string `json:"config_schema"`

	// CredentialTypes lists the credential backends the plugin supports
	// (e.g., "aws_access_key", "aws_iam_role", "kubeconfig").
	CredentialTypes []string `json:"credential_types"`
}

// ValidationResult contains the outcome of validating plugin configuration.
type ValidationResult struct {
	// Valid is true if the configuration passes all checks.
	Valid bool `json:"valid"`

	// Errors lists validation errors that prevent the configuration from
	// being used.
	Errors []string `json:"errors"`

	// Warnings lists non-fatal issues with the configuration.
	Warnings []string `json:"warnings"`
}

// DiscoveryEvent represents a single event emitted during a discovery run.
// This is the Go-native equivalent of the proto DiscoveryEvent message.
type DiscoveryEvent struct {
	// Resource is set when a new infrastructure resource is discovered.
	Resource *ResourceEvent `json:"resource,omitempty"`

	// Relationship is set when a relationship between resources is discovered.
	Relationship *RelationshipEvent `json:"relationship,omitempty"`

	// Progress is set to report progress during a long-running discovery.
	Progress *ProgressEvent `json:"progress,omitempty"`

	// Error is set when a non-fatal error occurs during discovery.
	Error *ErrorEvent `json:"error,omitempty"`
}

// ResourceEvent contains data about a discovered infrastructure resource.
type ResourceEvent struct {
	// ID is a globally unique identifier for the resource.
	ID string `json:"id"`

	// Type is the resource type in dotted notation
	// (e.g., "aws.ec2.instance").
	Type string `json:"type"`

	// Name is the human-readable name of the resource.
	Name string `json:"name"`

	// Provider is the infrastructure provider (e.g., "aws", "gcp").
	Provider string `json:"provider"`

	// Region is the geographic region where the resource exists.
	Region string `json:"region"`

	// AccountID is the cloud account or subscription ID.
	AccountID string `json:"account_id"`

	// Properties contains key-value pairs of resource attributes.
	Properties map[string]string `json:"properties"`

	// Tags are user-defined labels on the resource.
	Tags []string `json:"tags"`

	// RawData is the full JSON representation from the provider API.
	RawData []byte `json:"raw_data"`

	// DiscoveredAt is the Unix timestamp when the resource was found.
	DiscoveredAt int64 `json:"discovered_at"`
}

// RelationshipEvent contains data about a discovered relationship.
type RelationshipEvent struct {
	// SourceID is the unique identifier of the source resource.
	SourceID string `json:"source_id"`

	// TargetID is the unique identifier of the target resource.
	TargetID string `json:"target_id"`

	// Type is the relationship type (e.g., "CONTAINS", "DEPENDS_ON").
	Type string `json:"type"`

	// Properties contains additional metadata about the relationship.
	Properties map[string]string `json:"properties"`
}

// ProgressEvent reports progress during a discovery run.
type ProgressEvent struct {
	// Message is a human-readable description of current activity.
	Message string `json:"message"`

	// PercentComplete is the estimated completion percentage (0-100).
	PercentComplete int32 `json:"percent_complete"`

	// ResourcesFound is the count of resources discovered so far.
	ResourcesFound int32 `json:"resources_found"`

	// RelationshipsFound is the count of relationships discovered so far.
	RelationshipsFound int32 `json:"relationships_found"`
}

// ErrorEvent represents a non-fatal error during discovery.
type ErrorEvent struct {
	// Code is a machine-readable error code.
	Code string `json:"code"`

	// Message is a human-readable description of the error.
	Message string `json:"message"`

	// Retryable indicates whether retrying might succeed.
	Retryable bool `json:"retryable"`
}

// DiscoveryPlugin is the primary interface that all discovery plugins must
// implement. It wraps the gRPC proto service into a Go-native interface for
// use within the orchestrator and API layers.
type DiscoveryPlugin interface {
	// Info returns static metadata about the plugin.
	Info(ctx context.Context) (*PluginInfo, error)

	// Discover initiates a discovery run and returns a read-only channel
	// that streams DiscoveryEvent values. The channel is closed when the
	// run completes or encounters a fatal error. Callers should drain the
	// channel to completion.
	Discover(ctx context.Context, req *DiscoverRequest) (<-chan DiscoveryEvent, error)

	// ValidateConfig checks whether the provided configuration is valid
	// for this plugin.
	ValidateConfig(ctx context.Context, config map[string]string) (*ValidationResult, error)
}

// DiscoverRequest contains the parameters for a discovery run, translated
// from the proto DiscoverRequest into a Go-native type.
type DiscoverRequest struct {
	// ScopeID identifies the discovery scope.
	ScopeID string `json:"scope_id"`

	// Config contains plugin-specific configuration.
	Config map[string]string `json:"config"`

	// CredentialSource specifies where to obtain credentials.
	CredentialSource CredentialSource `json:"credential_source"`

	// TimeoutSeconds is the maximum run duration. 0 means no timeout.
	TimeoutSeconds int32 `json:"timeout_seconds"`
}

// CredentialSource describes where to obtain credentials for a plugin.
type CredentialSource struct {
	// Type is the credential backend (e.g., "vault", "env", "file").
	Type string `json:"type"`

	// Path is the location within the credential backend.
	Path string `json:"path"`

	// Params contains additional parameters for the credential source.
	Params map[string]string `json:"params"`
}

// PluginConfig holds the configuration for loading and managing a plugin.
type PluginConfig struct {
	// Name is the unique identifier for the plugin.
	Name string `json:"name"`

	// Path is the filesystem path to the plugin binary.
	Path string `json:"path"`

	// Args are command-line arguments passed to the plugin binary on startup.
	Args []string `json:"args"`

	// Env are environment variables set for the plugin process.
	Env map[string]string `json:"env"`
}

// PluginLoader is a function that loads a DiscoveryPlugin from a binary path.
// It returns the plugin, a closer function that must be called to release
// resources (kill the plugin process), and an error if loading fails.
type PluginLoader func(path string) (plugin DiscoveryPlugin, closer func(), err error)

// PluginManager manages the lifecycle of discovery plugins. It handles
// loading plugins from disk, caching them by name, and providing safe
// concurrent access.
type PluginManager struct {
	// mu protects the plugins map for concurrent access.
	mu sync.RWMutex

	// plugins maps plugin names to their loaded DiscoveryPlugin instances.
	plugins map[string]DiscoveryPlugin

	// closers maps plugin names to their cleanup functions.
	closers map[string]func()

	// loader is the function used to load a plugin from a binary path.
	// This is configurable to allow testing with mock plugins.
	loader PluginLoader
}

// NewPluginManager creates a new PluginManager with the given loader function.
// If loader is nil, a default loader is used that returns a not-yet-implemented
// error. Callers should provide a real loader via SetLoader or always pass one.
func NewPluginManager(loader PluginLoader) *PluginManager {
	pm := &PluginManager{
		plugins: make(map[string]DiscoveryPlugin),
		closers: make(map[string]func()),
	}
	if loader != nil {
		pm.loader = loader
	} else {
		pm.loader = defaultLoader
	}
	return pm
}

// SetLoader replaces the plugin loader function. This is useful when the
// default loader is not available (e.g., from a different package) and the
// caller wants to wire in a real implementation after construction.
func (pm *PluginManager) SetLoader(loader PluginLoader) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.loader = loader
}

// LoadPlugin loads a discovery plugin from the given binary path and
// registers it under the given name. Returns an error if a plugin with
// the same name is already loaded or if loading fails.
//
// The actual plugin loading (gRPC handshake, etc.) happens without holding
// the internal lock, allowing concurrent loading of multiple plugins.
func (pm *PluginManager) LoadPlugin(name string, path string) error {
	pm.mu.Lock()
	if _, exists := pm.plugins[name]; exists {
		pm.mu.Unlock()
		return fmt.Errorf("plugin %q is already loaded", name)
	}
	pm.mu.Unlock()

	plugin, closer, err := pm.loader(path)
	if err != nil {
		return fmt.Errorf("failed to load plugin %q from %q: %w", name, path, err)
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Double-check after re-acquiring lock.
	if _, exists := pm.plugins[name]; exists {
		if closer != nil {
			closer()
		}
		return fmt.Errorf("plugin %q is already loaded", name)
	}

	pm.plugins[name] = plugin
	if closer != nil {
		pm.closers[name] = closer
	}
	return nil
}

// GetPlugin returns the discovery plugin registered under the given name.
// Returns an error if no plugin with that name is loaded.
func (pm *PluginManager) GetPlugin(name string) (DiscoveryPlugin, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	p, ok := pm.plugins[name]
	if !ok {
		return nil, fmt.Errorf("plugin %q not found", name)
	}
	return p, nil
}

// ListPlugins returns the names of all currently loaded plugins.
func (pm *PluginManager) ListPlugins() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	names := make([]string, 0, len(pm.plugins))
	for name := range pm.plugins {
		names = append(names, name)
	}
	return names
}

// UnloadPlugin removes the plugin registered under the given name. Returns
// an error if no plugin with that name is loaded. Calls the closer to
// release plugin process resources.
func (pm *PluginManager) UnloadPlugin(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, ok := pm.plugins[name]; !ok {
		return fmt.Errorf("plugin %q not found", name)
	}

	if closer, ok := pm.closers[name]; ok {
		closer()
		delete(pm.closers, name)
	}
	delete(pm.plugins, name)
	return nil
}

// CloseAll unloads all plugins, calling each closer in sequence.
func (pm *PluginManager) CloseAll() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for name, closer := range pm.closers {
		closer()
		delete(pm.closers, name)
		delete(pm.plugins, name)
	}
}

// RegisterPlugin directly registers a DiscoveryPlugin instance under the
// given name. Unlike LoadPlugin, this does not invoke the loader function
// and is intended for built-in plugins that are compiled into the binary.
// If a plugin with the same name already exists, it is replaced.
func (pm *PluginManager) RegisterPlugin(name string, p DiscoveryPlugin) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Close existing plugin if replacing.
	if closer, ok := pm.closers[name]; ok {
		closer()
		delete(pm.closers, name)
	}

	pm.plugins[name] = p
}

// defaultLoader is the default plugin loading function. It will be
// replaced with a real gRPC-based loader when SetLoader is called.
func defaultLoader(path string) (DiscoveryPlugin, func(), error) {
	return nil, nil, fmt.Errorf("default plugin loader not yet implemented (path: %s)", path)
}
