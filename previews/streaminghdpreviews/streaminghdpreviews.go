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

// Package streaminghdpreviews implements HTTP handler for HD Previews.
//
// This handler intercepts a request from a client and starts a Chrome
// instance that will be used for retrieving the DOM of the page and
// send it back to the client. The handler will wait for the page to
// be stable at Chrome before sending the back the response.
// All <script> tags are removed from the response, as part of the
// HD Previews definition,
package streaminghdpreviews

import (
	"compress/gzip"
	"fmt"
	htmlesc "html"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"streaming_hdp/chrome"
	"streaming_hdp/previews/handlerutils"
)

const (
	htmlTemplateFilename = "template.html"
	jsStubFilename       = "streaming_hdp.js"
)

// Handler defines the hdpreview.Handler type.
type Handler struct {
	htmlTemplate    *template.Template      // The stub to be sent back with the initial response.
	jsStub          string                  // The string containing the javascript bundle.
	proxyHost       string                  // The host that this proxy is running on.
	port            int                     // The port that the websocket is listening to.
	rendererManager *chrome.InstanceManager // For communicating chrome instances.
	rp              *httputil.ReverseProxy  // The reverse proxy for serving non-shdp content.
}

// New returns a new hdpreview.Handler instance.
// Will return an error if the template cannot be found.
// args:
//	- proxyHost: the host that the proxy is running on.
//	- port: the port that the proxy is listening to on the "host"
func New(proxyHost string, port int, chromeInstanceManager *chrome.InstanceManager, staticFilesPath string) (*Handler, error) {
	htmlTemplate, err := ioutil.ReadFile(filepath.Join(staticFilesPath, htmlTemplateFilename))
	if err != nil {
		return nil, err
	}
	jsStub, err := ioutil.ReadFile(filepath.Join(staticFilesPath, jsStubFilename))
	if err != nil {
		return nil, err
	}

	newHandler := Handler{
		htmlTemplate:    template.Must(template.New("").Parse(string(htmlTemplate))),
		proxyHost:       proxyHost,
		port:            port,
		rendererManager: chromeInstanceManager,
		jsStub:          string(jsStub),
		rp:              &httputil.ReverseProxy{Director: func(_ *http.Request) {}},
	}
	return &newHandler, nil
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
	fmt.Printf("[SHDP] Handling request for %s\n", req.URL.String())
	queries := req.URL.Query()

	// Handle the case where we want to block the onLoad event.
	// We want to delay the onLoad event because when WPT does not
	// capture the screenshots properly when the onLoad event fires right away.
	// The onLoad event fires right away with streaming HD previews because
	// the response to the initial page is a blank page with the javascript
	// snippet, so Chrome will finish parsing the page right away.
	if strings.Contains(req.URL.String(), "slow_script_for_blocking_streaming_hd_previews.js") {
		if _, ok := queries["id"]; !ok {
			fmt.Printf("params \"id\" missing from parameters\n")
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		instanceID, err := strconv.Atoi(queries["id"][0])
		if err != nil {
			fmt.Printf("param \"id\" is not an int\n")
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		err = h.handleSlowScript(instanceID)
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
		rw.WriteHeader(http.StatusOK)
		fmt.Printf("done handling slow_script.js\n")
		return
	}

	// Ideally we want to check for sure that a resource is a
	// document type. We will skip that by adding a custom
	// query parameter that indicates that this is the main
	// SHDP document.
	if _, ok := queries["req_for_preview"]; ok {
		// TODO(vaspol): 2 potential issues:
		// (1) This means that we only do SHDP for only the main frame.
		// May have to revisit how to handle other iframes.
		// (2) Since we are not requesting for the actual resource, we
		// will be missing the correct response headers that are supposed
		// to be part of the main document's response.

		// TODO(vaspol): This will also include the "req_for_preview" query
		// param. Most servers will probably ignore this. Ideally, we want to remove this.
		chromeID := h.rendererManager.GetNewInstance(req.URL.String())

		// Generate the JS stub.
		templateData := struct {
			URL    string
			ID     int
			Bundle template.JS
		}{
			URL:    req.URL.Hostname(),
			ID:     chromeID,
			Bundle: template.JS(h.jsStub),
		}

		// (2) Return with the templated response.
		rw.Header().Set("Content-Encoding", "gzip")
		writer, err := gzip.NewWriterLevel(rw, gzip.BestCompression)
		if err != nil {
			rw.WriteHeader(http.StatusBadGateway)
			return
		}
		defer writer.Close()

		rw.Header().Del("Content-Length")
		rw.Header().Set("Content-Type", "text/html")
		rw.Header().Set("Access-Control-Allow-Origin", "*")
		rw.WriteHeader(http.StatusOK)

		// Start navigating to the page.
		go func() {
			chromeInstance, err := h.rendererManager.GetInstance(chromeID)
			if err != nil {
				fmt.Printf("failed to get chrome instance: %v\n", err)
				return
			}

			fmt.Printf("Waiting for Chrome to be ready: %v\n", chromeID)
			err = chromeInstance.WaitUntilChromeReady()
			if err != nil || !chromeInstance.ResetTimeout() { // The timer already expired.
				fmt.Printf("failed after waiting chrome to be ready: %v\n", err)
				return
			}
			fmt.Printf("Got Chrome: %v\n", chromeID)

			// Subscribe to events.
			chromeInstance.EnableDomains("DOM")
			chromeInstance.NavigateToPage(req.URL.String())
		}()

		err = h.htmlTemplate.Execute(writer, templateData)
		if err != nil {
			fmt.Printf("template.Execute: %v\n", err)
		}
	} else {
		h.rp.ServeHTTP(rw, req)
	}
}

// This blocks until Chrome instance with instanceID finishes loading the page.
func (h *Handler) handleSlowScript(instanceID int) error {
	chromeInstance, err := h.rendererManager.GetInstance(instanceID)
	if err != nil {
		fmt.Printf("failed to get chrome instance: %v\n", err)
		return err
	}

	err = chromeInstance.WaitUntilChromeReady()
	if err != nil || !chromeInstance.ResetTimeout() {
		fmt.Printf("failed after waiting chrome to be ready: %v\n", err)
		return err
	}

	chromeInstance.WaitUntilPageLoadCompletes()
	return nil
}

// This function goes through all elements of the HTML passed
// via the response argument and removes all occurences of
// <script> and event handlers in the string.
// It also inserts "toAdd" string right after the body tag.
//
// TODO(vaspol): This is temporary. Based on an offline discussion,
// sending back HTML as a string is not going to work. Should parse
// DOM and send each element individually.
func removeScriptTagsAndAddSnippet(dom, toAdd string) (string, error) {
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
		// Add the "toAdd" string right after the body tag.
		if tt == html.StartTagToken && tk.DataAtom == atom.Body {
			str := htmlesc.UnescapeString(tk.String())
			response += str + toAdd
			continue
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
