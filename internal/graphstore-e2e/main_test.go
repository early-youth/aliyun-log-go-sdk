//go:build e2e

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBridgePreservesSuccessfulManagementResponseMetadata(t *testing.T) {
	t.Setenv("GRAPHSTORE_E2E_ACCESS_KEY_ID", "test-id")
	t.Setenv("GRAPHSTORE_E2E_ACCESS_KEY_SECRET", "test-secret")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-log-requestid", "real-response-request")
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"stores":[],"count":0,"total":0}`))
		}
	}))
	defer server.Close()
	for _, action := range []string{"ListGraphStores", "DeleteGraphStore"} {
		t.Run(action, func(t *testing.T) {
			directory := t.TempDir()
			input, err := os.Create(filepath.Join(directory, "input.json"))
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			request := invocation{Action: action, Endpoint: "http://example.com", DialAddress: strings.TrimPrefix(server.URL, "http://"), Project: "project", StoreName: "owned", TimeoutMS: 1000}
			if err := json.NewEncoder(input).Encode(request); err != nil {
				t.Fatal(err)
			}
			if _, err := input.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			output, err := os.Create(filepath.Join(directory, "output.json"))
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			originalInput, originalOutput := os.Stdin, os.Stdout
			os.Stdin, os.Stdout = input, output
			defer func() { os.Stdin, os.Stdout = originalInput, originalOutput }()
			run(false)
			if _, err := output.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			var result map[string]interface{}
			if err := json.NewDecoder(output).Decode(&result); err != nil {
				t.Fatal(err)
			}
			if result["http_status"] != float64(200) || result["request_id"] != "real-response-request" {
				t.Fatalf("response metadata was lost: %v", result)
			}
		})
	}
}

func TestStreamReusesConnectionAndStopsAtEOF(t *testing.T) {
	t.Setenv("GRAPHSTORE_E2E_ACCESS_KEY_ID", "test-id")
	t.Setenv("GRAPHSTORE_E2E_ACCESS_KEY_SECRET", "test-secret")
	addresses := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		addresses = append(addresses, r.RemoteAddr)
		w.Header().Set("x-log-requestid", "stream-request")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stores":[],"count":0,"total":0}`))
	}))
	defer server.Close()
	input, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	p := invocation{Action: "ListGraphStores", Endpoint: "http://example.com", DialAddress: strings.TrimPrefix(server.URL, "http://"), Project: "project", StoreName: "owned", TimeoutMS: 1000}
	encoder := json.NewEncoder(input)
	for i := 0; i < 3; i++ {
		if err := encoder.Encode(p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = input, output
	defer func() { os.Stdin, os.Stdout = oldIn, oldOut }()
	run(true)
	if len(addresses) != 3 || addresses[0] != addresses[1] || addresses[1] != addresses[2] {
		t.Fatalf("connection not reused: %v", addresses)
	}
	if _, err := output.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(output)
	for i := 0; i < 3; i++ {
		var value map[string]interface{}
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		if value["http_status"] != float64(200) || value["http_attempts"] != float64(1) || value["elapsed_us"] == nil {
			t.Fatalf("bad stream result: %v", value)
		}
	}
}
