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
	"strings"
	"testing"

	"streaming_hdp/chrome"
	"streaming_hdp/previews/hdpreviews"
	"streaming_hdp/previews/testutil"
)

// Tests that the response from HD Preview must not contain a script tag.
func TestScriptTagRemoved(t *testing.T) {
	chromeInstanceManager := chrome.NewInstanceManager(true)
	hdpreviewsHandler, err := hdpreviews.New(chromeInstanceManager)
	if err != nil {
		t.Fatalf("Failed to get HDPreviews handler: %v", err)
	}

	env, err := testutil.NewTestEnvironment(hdpreviewsHandler, http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header()["Content-Type"] = append([]string(nil), "text/html")
			w.WriteHeader(http.StatusOK)
		},
	))
	if err != nil {
		t.Fatalf("Cannot create test environment: %v", err)
	}
	req, err := http.NewRequest("GET", env.OriginServer.URL+"?req_for_preview=1", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, _ := env.Transport.RoundTrip(req)
	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Fatalf("StatusCode got: %v, want: %v", got, want)
	}
	defer resp.Body.Close()
	respBody, _ := ioutil.ReadAll(resp.Body)
	respBodyStr := string(respBody)

	// TODO(vaspol): this can be more robust by having a HTML parsing.
	if strings.Contains("</script>", respBodyStr) {
		t.Fatalf("Script tag exists in respBody: %v", respBodyStr)
	}
}

// Tests that the response status code from HD Preview is correct.
func TestResponseStatusCode(t *testing.T) {
	tests := []struct {
		label           string
		expectedRetCode int
	}{
		{"Valid site", http.StatusOK},
		{"Nonexisting site", http.StatusBadGateway},
	}
	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			chromeInstanceManager := chrome.NewInstanceManager(true)
			hdpreviewsHandler, err := hdpreviews.New(chromeInstanceManager)
			if err != nil {
				t.Fatalf("Failed to get HDPreviews handler: %v", err)
			}

			env, err := testutil.NewTestEnvironment(
				hdpreviewsHandler,
				http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(test.expectedRetCode)
					},
				),
			)
			if err != nil {
				t.Fatalf("Cannot create test environment: %v", err)
			}
			req, err := http.NewRequest("GET", env.OriginServer.URL+"?req_for_preview=1", nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			resp, _ := env.Transport.RoundTrip(req)
			if got, want := resp.StatusCode, test.expectedRetCode; got != want {
				t.Fatalf("Requesting to %v got statusCode: %v, want: %v", env.OriginServer.URL, got, want)
			}
			defer resp.Body.Close()
		})
	}
}

// Tests that the proxy returns a gzipped version of the response in all resources.
func TestResourceGzipped(t *testing.T) {
	expectedValue := "Foo bar"

	chromeInstanceManager := chrome.NewInstanceManager(true)
	hdpreviewsHandler, err := hdpreviews.New(chromeInstanceManager)
	if err != nil {
		t.Fatalf("Failed to get HDPreviews handler: %v", err)
	}

	env, err := testutil.NewTestEnvironment(hdpreviewsHandler, http.HandlerFunc(
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
