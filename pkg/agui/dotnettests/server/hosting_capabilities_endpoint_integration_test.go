package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/CapabilitiesEndpointIntegrationTest.cs.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/server"
)

func getCapabilities(t *testing.T, url string) agui.AgentCapabilities {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if err := agui.Validate("AgentCapabilities", raw); err != nil {
		t.Fatalf("served capabilities are not valid 1.0: %v", err)
	}
	var caps agui.AgentCapabilities
	if err := json.Unmarshal(raw, &caps); err != nil {
		t.Fatal(err)
	}
	return caps
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/CapabilitiesEndpointIntegrationTest.cs::CapabilitiesEndpointIntegrationTest.MapAGUICapabilities_ReturnsCapabilitiesJson
func TestCsCapabilitiesEndpointIntegrationTest_MapAGUICapabilities_ReturnsCapabilitiesJson(t *testing.T) {
	srv := httptest.NewServer(server.CapabilitiesHandler(agui.AgentCapabilities{
		Identity:  &agui.IdentityCapabilities{Name: agui.Ptr("TestAgent"), Type: agui.Ptr("ag-ui-dotnet"), Version: agui.Ptr("1.0.0")},
		Transport: &agui.TransportCapabilities{Streaming: agui.Ptr(true)},
		Tools:     &agui.ToolsCapabilities{Supported: agui.Ptr(true), ClientProvided: agui.Ptr(true)},
	}))
	defer srv.Close()
	caps := getCapabilities(t, srv.URL)
	if agui.Deref(caps.Identity.Name) != "TestAgent" || agui.Deref(caps.Identity.Type) != "ag-ui-dotnet" ||
		agui.Deref(caps.Identity.Version) != "1.0.0" || !agui.Deref(caps.Transport.Streaming) ||
		!agui.Deref(caps.Tools.Supported) || !agui.Deref(caps.Tools.ClientProvided) {
		t.Fatalf("capabilities %+v", caps)
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/CapabilitiesEndpointIntegrationTest.cs::CapabilitiesEndpointIntegrationTest.MapAGUICapabilities_WithFactory_ReturnsDynamicCapabilities
func TestCsCapabilitiesEndpointIntegrationTest_MapAGUICapabilities_WithFactory_ReturnsDynamicCapabilities(t *testing.T) {
	// server.CapabilitiesHandler serves a fixed declaration; a per-request
	// factory is a handler building one per request.
	count := 0
	factory := func() agui.AgentCapabilities {
		count++
		return agui.AgentCapabilities{Identity: &agui.IdentityCapabilities{Name: agui.Ptr("Agent-" + string(rune('0'+count)))}}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.CapabilitiesHandler(factory()).ServeHTTP(w, r)
	}))
	defer srv.Close()
	if n := agui.Deref(getCapabilities(t, srv.URL).Identity.Name); n != "Agent-1" {
		t.Fatalf("first %q", n)
	}
	if n := agui.Deref(getCapabilities(t, srv.URL).Identity.Name); n != "Agent-2" {
		t.Fatalf("second %q", n)
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/CapabilitiesEndpointIntegrationTest.cs::CapabilitiesEndpointIntegrationTest.MapAGUICapabilities_PostRequest_Returns405
func TestCsCapabilitiesEndpointIntegrationTest_MapAGUICapabilities_PostRequest_Returns405(t *testing.T) {
	// The specification leaves retrieval of capabilities to the
	// implementation; MapAGUICapabilities maps a GET route, so POST is 405.
	// server.CapabilitiesHandler answers any method: the GET-only route is
	// the mux's (Go 1.22 method patterns), as in ASP.NET routing.
	mux := http.NewServeMux()
	mux.Handle("GET /capabilities", server.CapabilitiesHandler(agui.AgentCapabilities{
		Identity: &agui.IdentityCapabilities{Name: agui.Ptr("TestAgent")},
	}))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/capabilities", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if n := agui.Deref(getCapabilities(t, srv.URL+"/capabilities").Identity.Name); n != "TestAgent" {
		t.Fatalf("GET %q", n)
	}
}
