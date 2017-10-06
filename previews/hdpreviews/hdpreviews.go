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

// Package hdpreviews implements HTTP handler for HD Previews.
//
// This handler intercepts a request from a client and starts a Chrome
// instance that will be used for retrieving the DOM of the page and
// send it back to the client. The handler will wait for the page to
// be stable at Chrome before sending the back the response.
// All <script> tags are removed from the response, as part of the
// HD Previews definition,
package hdpreviews

import (
	"compress/gzip"
	"fmt"
	htmlesc "html"
	"io"
	"net/http"
	"net/http/httputil"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"streaming_hdp/chrome"
	"streaming_hdp/previews/handlerutils"
)

// Handler defines the hdpreview.Handler type.
type Handler struct {
	rendererManager *chrome.InstanceManager // For communicating chrome instances.
	rp              *httputil.ReverseProxy  // The reverse proxy for serving non-shdp content.
}

// New returns a new hdpreview.Handler instance.
func New(chromeInstanceManager *chrome.InstanceManager) (*Handler, error) {
	return &Handler{
		rendererManager: chromeInstanceManager,
		rp:              &httputil.ReverseProxy{Director: func(_ *http.Request) {}},
	}, nil
}

// Close implements cleanup upon closing the handler.
func (h *Handler) Close() error {
	return nil
}

// Implements the handle function for serving a HTTP request.
func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if !req.URL.IsAbs() {
		req.URL.Scheme = "http"
		req.URL.Host = req.Host
	}
	fmt.Printf("[HDP] Handling request for %s\n", req.URL.String())
	queries := req.URL.Query()

	if _, ok := queries["req_for_preview"]; ok {
		// Send a query in parallel to make sure that we have the correct status code.
		statusCodeChan := make(chan int)
		defer close(statusCodeChan)
		go func() {
			// TODO: should we pass along all the request headers and non-Content-Encoding response headers?
			response, err := http.Get(req.URL.String())
			if err != nil {
				statusCodeChan <- http.StatusBadGateway
			}
			statusCodeChan <- response.StatusCode
		}()

		instanceID := h.rendererManager.GetNewInstance(req.URL.String())
		defer h.rendererManager.RemoveInstance(instanceID)

		chromeInstance, err := h.rendererManager.GetInstance(instanceID)
		if err != nil {
			fmt.Printf("failed to get chrome instance: %v\n", err)
			rw.WriteHeader(http.StatusBadGateway)
			return
		}

		err = chromeInstance.WaitUntilChromeReady()
		if err != nil || !chromeInstance.ResetTimeout() { // The timer already expired.
			fmt.Printf("failed after waiting chrome to be ready: %v\n", err)
			rw.WriteHeader(http.StatusBadGateway)
			return
		}
		defer chromeInstance.DisconnectAndTerminate()

		// (3) navigate to the page and the get the response.
		chromeInstance.NavigateToPage(req.URL.String())
		loaded := make(chan struct{})
		go func() {
			pageStabilized := false
			for {
				event, err := chromeInstance.NextEvent()
				if err == io.EOF {
					// no more events to process.
					break
				}
				if pageStabilized {
					// Throw all events away because the page has loaded.
					continue
				}
				switch {
				case event.Method == "Emulation.virtualTimeBudgetExpired":
					// Page stablized.
					pageStabilized = true
					close(loaded)
				}
			}
		}()
		<-loaded // Wait for the page to be loaded
		dom, err := chromeInstance.GetDOM()
		if err != nil {
			rw.WriteHeader(http.StatusBadGateway)
			return
		}

		// (4) strip out all <script> and write the response back to the client.
		resp, err := removeScriptTags(dom)
		if err != nil {
			rw.WriteHeader(http.StatusBadGateway)
			return
		}

		writer, err := gzip.NewWriterLevel(rw, gzip.BestCompression)
		if err != nil {
			rw.WriteHeader(http.StatusBadGateway)
			return
		}
		defer writer.Close()

		rw.Header().Set("Content-Encoding", "gzip")

		// Content length will be different because we are striping <script> tags
		rw.Header()["Content-Length"] = nil

		statusCode := <-statusCodeChan
		rw.WriteHeader(statusCode)

		_, err = io.WriteString(writer, resp)
		if err != nil {
			fmt.Printf("io.WriteString: %v\n", err)
			return
		}
	} else {
		h.rp.ServeHTTP(rw, req)
	}
}

// This function goes through all elements of the HTML passed
// via the response argument and removes all occurences of
// <script> and event handlers in the string.
func removeScriptTags(dom string) (string, error) {
	response := ""
	reader := strings.NewReader(dom)
	z := html.NewTokenizer(reader)
	firstToken := true
	lastTokenWasScript := false
	lastTokenWasStyle := false
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if z.Err() == io.EOF {
				// Probably not HTML.
				break
			}
			fmt.Printf("z.Next: %v\n", z.Err())
			return "", z.Err()
		}

		tk := z.Token()

		// Make sure that we remove all event handlers.
		resultAttrs := []html.Attribute{}
		for _, attr := range tk.Attr {
			if !handlerutils.IsEventHandler(attr.Key) {
				resultAttrs = append(resultAttrs, attr)
			}
		}
		tk.Attr = resultAttrs

		if firstToken && tt == html.TextToken {
			str := htmlesc.UnescapeString(tk.String())
			response += str
			continue
		}
		firstToken = false

		tkString := tk.String()
		// Must unescape style tags.
		if lastTokenWasStyle && tt == html.TextToken {
			tkString = htmlesc.UnescapeString(tkString)
		}
		if lastTokenWasScript && tt == html.TextToken {
			// Skip script tags.
			continue
		}
		lastTokenWasScript = tt == html.StartTagToken && tk.DataAtom == atom.Script
		lastTokenWasStyle = tt == html.StartTagToken && tk.DataAtom == atom.Style
		if tk.DataAtom == atom.Script {
			// Skip script tags.
			continue
		}

		response += tkString
	}
	return response, nil
}
