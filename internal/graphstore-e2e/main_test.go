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
			main()
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
