// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package readeck

import (
    "encoding/json"
    "io"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

func TestCreateBookmark(t *testing.T) {
    entryURL := "https://example.com/article"
    entryTitle := "Example Title"
    entryContent := "<p>Hello Readeck</p>"
    labels := "alpha, beta, gamma"

    tests := []struct {
        name           string
        baseURL        string
        apiKey         string
        labels         string
        onlyURL        bool
        entryURL       string
        entryTitle     string
        entryContent   string
        serverResponse func(w http.ResponseWriter, r *http.Request)
        wantErr        bool
        errContains    string
    }{
        {
            name:         "successful JSON (URL-only) payload",
            baseURL:      "",
            apiKey:       "",
            labels:       labels,
            onlyURL:      true,
            entryURL:     entryURL,
            entryTitle:   entryTitle,
            entryContent: entryContent,
            serverResponse: func(w http.ResponseWriter, r *http.Request) {
                // Validate headers
                auth := r.Header.Get("Authorization")
                if auth != "Bearer test-token" {
                    t.Errorf("expected Authorization 'Bearer test-token', got %q", auth)
                }
                if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
                    t.Errorf("expected Content-Type application/json, got %q", ct)
                }

                // Validate body
                body, _ := io.ReadAll(r.Body)
                var req map[string]any
                if err := json.Unmarshal(body, &req); err != nil {
                    t.Fatalf("failed to parse JSON body: %v", err)
                }
                if req["url"] != entryURL {
                    t.Errorf("expected url %q, got %v", entryURL, req["url"])
                }
                if req["title"] != entryTitle {
                    t.Errorf("expected title %q, got %v", entryTitle, req["title"])
                }
                // labels should be an array split on commas/spaces
                lbl, ok := req["labels"].([]any)
                if !ok || len(lbl) != 3 {
                    t.Errorf("expected labels array of length 3, got %T len=%d", req["labels"], len(lbl))
                }
                w.WriteHeader(http.StatusOK)
            },
        },
        {
            name:         "successful multipart payload (full content)",
            baseURL:      "",
            apiKey:       "",
            labels:       labels,
            onlyURL:      false,
            entryURL:     entryURL,
            entryTitle:   entryTitle,
            entryContent: entryContent,
            serverResponse: func(w http.ResponseWriter, r *http.Request) {
                auth := r.Header.Get("Authorization")
                if auth != "Bearer test-token" {
                    t.Errorf("expected Authorization 'Bearer test-token', got %q", auth)
                }
                ct := r.Header.Get("Content-Type")
                if !strings.HasPrefix(ct, "multipart/form-data;") {
                    t.Errorf("expected Content-Type multipart/form-data, got %q", ct)
                }
                reader, err := r.MultipartReader()
                if err != nil {
                    t.Fatalf("failed to get multipart reader: %v", err)
                }

                foundURL := false
                foundTitle := false
                foundFeature := false
                foundLabels := 0
                foundResource := false

                for {
                    part, err := reader.NextPart()
                    if err == io.EOF {
                        break
                    }
                    if err != nil {
                        t.Fatalf("error reading multipart: %v", err)
                    }
                    name := part.FormName()
                    switch name {
                    case "url":
                        b, _ := io.ReadAll(part)
                        if string(b) != entryURL {
                            t.Errorf("expected url %q, got %q", entryURL, string(b))
                        }
                        foundURL = true
                    case "title":
                        b, _ := io.ReadAll(part)
                        if string(b) != entryTitle {
                            t.Errorf("expected title %q, got %q", entryTitle, string(b))
                        }
                        foundTitle = true
                    case "feature_find_main":
                        b, _ := io.ReadAll(part)
                        if string(b) != "false" {
                            t.Errorf("expected feature_find_main 'false', got %q", string(b))
                        }
                        foundFeature = true
                    case "labels":
                        _, _ = io.ReadAll(part)
                        foundLabels++
                    case "resource":
                        // First line should be JSON header, then newline, then content
                        headerBytes, _ := io.ReadAll(part)
                        // minimal sanity check for header presence
                        if !strings.Contains(string(headerBytes), entryURL) {
                            t.Errorf("expected resource header to contain entry URL")
                        }
                        foundResource = true
                    default:
                    }
                }
                if !foundURL || !foundTitle || !foundFeature || !foundResource || foundLabels == 0 {
                    t.Errorf("missing expected multipart fields: url=%v title=%v feature=%v resource=%v labels=%d",
                        foundURL, foundTitle, foundFeature, foundResource, foundLabels)
                }
                w.WriteHeader(http.StatusOK)
            },
        },
        {
            name:         "error on missing credentials",
            baseURL:      "",
            apiKey:       "",
            labels:       "",
            onlyURL:      true,
            entryURL:     entryURL,
            entryTitle:   entryTitle,
            entryContent: entryContent,
            serverResponse: func(w http.ResponseWriter, r *http.Request) {
                t.Error("server should not be called for missing creds")
            },
            wantErr:     true,
            errContains: "missing base URL or API key",
        },
        {
            name:         "error on non-2xx response",
            baseURL:      "",
            apiKey:       "",
            labels:       labels,
            onlyURL:      true,
            entryURL:     entryURL,
            entryTitle:   entryTitle,
            entryContent: entryContent,
            serverResponse: func(w http.ResponseWriter, r *http.Request) {
                w.WriteHeader(http.StatusBadRequest)
            },
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Build a test server to capture requests.
            srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                if !strings.HasSuffix(r.URL.Path, "/api/bookmarks/") {
                    t.Errorf("expected path to end with /api/bookmarks/, got %s", r.URL.Path)
                }
                tt.serverResponse(w, r)
            }))
            defer srv.Close()

            baseURL := tt.baseURL
            apiKey := tt.apiKey
            // For the missing-credentials case, do not override defaults to exercise error path.
            if !(tt.wantErr && strings.Contains(tt.errContains, "missing base URL or API key")) {
                if baseURL == "" {
                    baseURL = srv.URL
                }
                if apiKey == "" {
                    apiKey = "test-token"
                }
            }

            client := NewClient(baseURL, apiKey, tt.labels, tt.onlyURL)
            err := client.CreateBookmark(tt.entryURL, tt.entryTitle, tt.entryContent)
            if tt.wantErr && err == nil {
                t.Fatalf("expected error, got nil")
            }
            if !tt.wantErr && err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if tt.errContains != "" && err != nil {
                if !strings.Contains(err.Error(), tt.errContains) {
                    t.Fatalf("expected error to contain %q, got %q", tt.errContains, err.Error())
                }
            }
        })
    }
}
