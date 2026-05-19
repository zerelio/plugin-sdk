package plugin

import (
	"context"
	"testing"
)

// mockPlugin implements DiscoveryPlugin for testing.
type mockPlugin struct {
	info   *PluginInfo
	events []DiscoveryEvent
	err    error
}

func (m *mockPlugin) Info(_ context.Context) (*PluginInfo, error) {
	return m.info, nil
}

func (m *mockPlugin) Discover(_ context.Context, _ *DiscoverRequest) (<-chan DiscoveryEvent, error) {
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan DiscoveryEvent, len(m.events))
	for _, e := range m.events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func (m *mockPlugin) ValidateConfig(_ context.Context, _ map[string]string) (*ValidationResult, error) {
	return &ValidationResult{Valid: true}, nil
}

func TestNewPluginManager(t *testing.T) {
	pm := NewPluginManager(nil)
	if pm == nil {
		t.Fatal("NewPluginManager returned nil")
	}
	if len(pm.plugins) != 0 {
		t.Error("expected empty plugins map")
	}
}

func TestPluginManager_LoadAndGet(t *testing.T) {
	pm := NewPluginManager(func(path string) (DiscoveryPlugin, func(), error) {
		return &mockPlugin{info: &PluginInfo{Name: "test", Version: "1.0"}}, nil, nil
	})

	if err := pm.LoadPlugin("test", "/bin/test"); err != nil {
		t.Fatalf("LoadPlugin failed: %v", err)
	}

	p, err := pm.GetPlugin("test")
	if err != nil {
		t.Fatalf("GetPlugin failed: %v", err)
	}

	info, _ := p.Info(context.Background())
	if info.Name != "test" {
		t.Errorf("expected plugin name 'test', got %q", info.Name)
	}
}

func TestPluginManager_LoadDuplicate(t *testing.T) {
	pm := NewPluginManager(func(path string) (DiscoveryPlugin, func(), error) {
		return &mockPlugin{}, nil, nil
	})

	_ = pm.LoadPlugin("test", "/bin/test")
	err := pm.LoadPlugin("test", "/bin/test2")
	if err == nil {
		t.Error("expected error loading duplicate plugin")
	}
}

func TestPluginManager_GetNotFound(t *testing.T) {
	pm := NewPluginManager(nil)
	_, err := pm.GetPlugin("nonexistent")
	if err == nil {
		t.Error("expected error for missing plugin")
	}
}

func TestPluginManager_ListPlugins(t *testing.T) {
	pm := NewPluginManager(func(path string) (DiscoveryPlugin, func(), error) {
		return &mockPlugin{}, nil, nil
	})

	_ = pm.LoadPlugin("aws", "/bin/aws")
	_ = pm.LoadPlugin("gcp", "/bin/gcp")

	names := pm.ListPlugins()
	if len(names) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(names))
	}
}

func TestPluginManager_Unload(t *testing.T) {
	pm := NewPluginManager(func(path string) (DiscoveryPlugin, func(), error) {
		return &mockPlugin{}, nil, nil
	})

	_ = pm.LoadPlugin("test", "/bin/test")
	if err := pm.UnloadPlugin("test"); err != nil {
		t.Fatalf("UnloadPlugin failed: %v", err)
	}

	if _, err := pm.GetPlugin("test"); err == nil {
		t.Error("expected error after unloading")
	}
}

func TestPluginManager_UnloadNotFound(t *testing.T) {
	pm := NewPluginManager(nil)
	err := pm.UnloadPlugin("nonexistent")
	if err == nil {
		t.Error("expected error unloading missing plugin")
	}
}

func TestPluginManager_ConcurrentAccess(t *testing.T) {
	pm := NewPluginManager(func(path string) (DiscoveryPlugin, func(), error) {
		return &mockPlugin{info: &PluginInfo{Name: path}}, nil, nil
	})

	// Load plugins concurrently.
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			name := string(rune('a' + n))
			_ = pm.LoadPlugin(name, "/bin/"+name)
			done <- true
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	names := pm.ListPlugins()
	if len(names) != 10 {
		t.Errorf("expected 10 plugins, got %d", len(names))
	}
}

func TestDefaultLoader(t *testing.T) {
	pm := NewPluginManager(nil)
	err := pm.LoadPlugin("test", "/bin/test")
	if err == nil {
		t.Error("expected error from default loader")
	}
}
