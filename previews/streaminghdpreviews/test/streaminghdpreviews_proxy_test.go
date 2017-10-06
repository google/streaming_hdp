// Copyright 2017 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package test

import (
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"streaming_hdp/chrome"
	"streaming_hdp/previews/streaminghdpreviews"
	"streaming_hdp/previews/testutil"
)

const (
	staticDir            = "../../../static"
	htmlTemplateFilename = "streaming_hdp.js"
)

// Returns the HTML template.
func getHTMLTemplate(t *testing.T) string {
	htmlTemplateReader, err := os.Open(filepath.Join(staticDir, htmlTemplateFilename))
	if err != nil {
		t.Fatalf("Failed to get HTML template reader: %v", err)
	}
	htmlTemplate, err := ioutil.ReadAll(htmlTemplateReader)
	if err != nil {
		t.Fatalf("Failed to get HTML template: %v", err)
	}
	return string(htmlTemplate)
}

// Tests that the proxy returns a gzipped version of the response in all resources.
func TestResourceGzipped(t *testing.T) {
	expectedValue := "Foo bar"

	chromeInstanceManager := chrome.NewInstanceManager(true)
	streaminghdpHandler, err := streaminghdpreviews.New("localhost", 8080, chromeInstanceManager, staticDir)
	if err != nil {
		t.Fatalf("Failed to get HDPreviews handler: %v", err)
	}

	env, err := testutil.NewTestEnvironment(streaminghdpHandler, http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			writer, err := gzip.NewWriterLevel(w, gzip.BestCompression)
			if err != nil {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			defer writer.Close()

			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Content-Encoding", "gzip")

			w.WriteHeader(http.StatusOK)
			io.WriteString(writer, expectedValue)
		},
	))

	if err != nil {
		t.Fatalf("Cannot create test environment: %v", err)
	}
	req, err := http.NewRequest("GET", env.OriginServer.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	response, _ := env.Transport.RoundTrip(req)
	if got, want := response.StatusCode, 200; got != want {
		t.Fatalf("Requesting to %v got statusCode: %v, want: %v", env.OriginServer.URL, got, want)
	}

	t.Logf("response: %v", response)
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		t.Errorf("failed to read body from the HTTP response: %v", err)
	}

	if response.StatusCode == http.StatusOK {
		// Check the content type.
		if got, want := response.Header["Content-Type"][0], "text/html"; !strings.Contains(got, want) {
			t.Errorf("Wanted a gzipped response got %v", got)
		}

		// Check for gzipped encoding.
		if got, wanted := string(body), expectedValue; got != wanted {
			t.Errorf("body doesn't match wanted: %v got: %v wasUncompressed: %v\n", wanted, got, response.Uncompressed)
		}
	}
}

// Tests that the proxy returns the correct template response.
func TestTemplateProxyResponse(t *testing.T) {
	tests := []struct {
		label               string
		expectedRetCode     int
		expectedResponse    string
		expectedHost        string
		expectedPort        int
		expectedContentType string
	}{
		{"Valid site", http.StatusOK, getHTMLTemplate(t), "localhost", 8080, "text/html"},
		{"Nonexisting site", http.StatusOK, "", "localhost", 8080, "text/html"},
	}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			chromeInstanceManager := chrome.NewInstanceManager(true)
			streaminghdpHandler, err := streaminghdpreviews.New(test.expectedHost, test.expectedPort, chromeInstanceManager, staticDir)
			if err != nil {
				t.Fatalf("Failed to get HDPreviews handler: %v", err)
			}

			env, err := testutil.NewTestEnvironment(streaminghdpHandler, http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", test.expectedContentType)
					w.WriteHeader(test.expectedRetCode)
				},
			))
			if err != nil {
				t.Fatalf("Cannot create test environment: %v", err)
			}
			req, err := http.NewRequest("GET", env.OriginServer.URL+"?req_for_preview=1", nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			response, _ := env.Transport.RoundTrip(req)
			if got, want := response.StatusCode, test.expectedRetCode; got != want {
				t.Fatalf("Requesting to %v got statusCode: %v, want: %v", env.OriginServer.URL, got, want)
			}

			if response.StatusCode == http.StatusOK {
				if got, want := response.Header["Content-Type"][0], test.expectedContentType; got != want {
					t.Fatalf("Incorrect Content-type wanted: %v got %v", want, got)
				}
				if !response.Uncompressed {
					t.Fatalf("The response should have already been uncompressed.")
				}
				bodyBytes, err := ioutil.ReadAll(response.Body)
				if err != nil {
					t.Errorf("failed to read body from the HTTP response: %v", err)
				}
				if got, wanted := string(bodyBytes), test.expectedResponse; len(got) < len(wanted)-15 {
					// Just a simple check.
					// If the template was executed properly, then it should be slightly smaller.
					t.Errorf("body doesn't match wanted: %v got: %v\n", wanted, got)
				}
			}
		})
	}
}
