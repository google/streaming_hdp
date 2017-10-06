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

// Package stream defines the stream handler for a client to connect to
// the server for getting streaming HDP updates.
package stream

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"streaming_hdp/chrome"
	"streaming_hdp/devtools"
	"streaming_hdp/dom"
	"streaming_hdp/dom/domjson"
)

const (
	// DomDocumentUpdated defines the documentUpdated event.
	DomDocumentUpdated = "DOM.documentUpdated"
	// DomChildNodeCountUpdated defines the childNodeCountUpdated event.
	DomChildNodeCountUpdated = "DOM.childNodeCountUpdated"
	// DomSetChildNodes defines the setChildNodes event.
	DomSetChildNodes = "DOM.setChildNodes"
	// DomChildNodeInserted defines the child node inserted event.
	DomChildNodeInserted = "DOM.childNodeInserted"
	// DomChildNodeRemoved defines the child node removed event.
	DomChildNodeRemoved = "DOM.childNodeRemoved"
	// DomAttributeModified defines the attribute modified event.
	DomAttributeModified = "DOM.attributeModified"
	// EmulationVirtualTimeBudgetExpired defines the event when the time budget has expired.
	EmulationVirtualTimeBudgetExpired = "Emulation.virtualTimeBudgetExpired"

	// The delimeter for the stream.
	delim = "\r"
)

// Handler defines the handler for accepting stream connections.
type Handler struct {
	rendererManager *chrome.InstanceManager // For communicating chrome instances.
	verbose         bool                    // Whether extensive logging should be used.
}

// New returns a new ws.Handler.
func New(chromeInstanceManager *chrome.InstanceManager, verbose bool) (*Handler, error) {
	newHandler := Handler{
		rendererManager: chromeInstanceManager,
		verbose:         verbose,
	}
	return &newHandler, nil
}

// Close implements cleanup upon closing the handler.
func (h *Handler) Close() error {
	return nil
}

// Implements the handle function for serving a HTTP request.
func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// Received a HTTP request. Upgrade this to a stream connection.
	queries := req.URL.Query()
	if _, ok := queries["id"]; !ok {
		fmt.Println(`params "id" missing from parameters`)
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	instanceID, err := strconv.Atoi(queries["id"][0])
	if err != nil {
		fmt.Println(`param "id" is not an int`)
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	defer h.rendererManager.RemoveInstance(instanceID)
	fmt.Printf("Serving stream request with instance id: %v\n", instanceID)

	chromeInstance, err := h.rendererManager.GetInstance(instanceID)
	if err != nil {
		fmt.Printf("failed to get chrome instance: %v\n", err)
		rw.WriteHeader(http.StatusBadGateway)
		return
	}

	fmt.Printf("Waiting for Chrome to be ready: %v\n", instanceID)
	err = chromeInstance.WaitUntilChromeReady()
	if err != nil || !chromeInstance.ResetTimeout() { // The timer already expired.
		fmt.Printf("failed after waiting chrome to be ready: %v\n", err)
		rw.WriteHeader(http.StatusBadGateway)
		return
	}
	fmt.Printf("Got Chrome: %v\n", instanceID)
	defer chromeInstance.DisconnectAndTerminate()

	rw.Header().Set("Content-Encoding", "gzip")
	writer, err := gzip.NewWriterLevel(rw, gzip.BestCompression)
	if err != nil {
		rw.WriteHeader(http.StatusBadGateway)
		return
	}
	defer writer.Close()

	rw.Header().Set("Content-Type", "application/octet-stream")
	rw.Header().Set("Access-Control-Allow-Origin", "*")
	rw.WriteHeader(http.StatusOK)
	domModel := dom.NewDOMModel()

	// TODO(vaspol): We perform blocking actions in the event loop (wsConnection.WriteMessage and
	// chromeInstance.GetDOMInstance). This is problematic because DevTools events will
	// get buffered while we're not processing them. It is possible that more events will
	// be buffered than can fit in the buffered channel, thus creating a deadlock.
	// For now we ignore this problem.
	for {
		event, err := chromeInstance.NextEvent()
		if err == io.EOF {
			// no more events to process.
			return
		}
		switch event.Method {
		case DomDocumentUpdated:
			rootNode, err := chromeInstance.GetDOMInstance()
			if err != nil {
				fmt.Printf("error retrieving DOM instance on getting DOM.documentUpdated event: %v\n", err)
				return
			}
			// Send back a stream message.
			domUpdates, err := domModel.GenerateInitialDOM(rootNode)
			if err != nil {
				fmt.Printf("error generating initial DOM: %v\n", err)
				return
			}
			jsonDOMUpdates := domjson.DOMUpdates{Updates: domUpdates}
			fmt.Printf("document updated\n")
			err = h.sendMessage(writer, jsonDOMUpdates)
			if err != nil {
				fmt.Printf("error sending initial dom: %v\n", err)
				return
			}
		case DomChildNodeCountUpdated:
			chromeInstance.RequestChildNodes(event.Params["nodeId"].(float64))

		case DomSetChildNodes:
			domUpdates, err := domModel.ProcessSetChildNodes(dom.Node(event.Params))
			if err != nil {
				fmt.Printf("error generating updates from setChildNodes: %v\n", err)
				continue
			}
			jsonDOMUpdates := domjson.DOMUpdates{Updates: domUpdates}
			err = h.sendMessage(writer, jsonDOMUpdates)
			if err != nil {
				fmt.Printf("error sending setChildNodes updates: %v\n", err)
				continue
			}
		case DomChildNodeInserted:
			node := event.Params["node"].(map[string]interface{})
			chromeInstance.RequestChildNodes(node["nodeId"].(float64))
			fallthrough
		case DomAttributeModified:
			fallthrough
		case DomChildNodeRemoved:
			err := h.handleNodeUpdate(event, domModel, chromeInstance, writer)
			if err != nil {
				continue
			}
		case EmulationVirtualTimeBudgetExpired:
			// Page has stablized.
			return
		}
	}
}

// Handles the node updates.
func (h *Handler) handleNodeUpdate(
	event devtools.EventMessage, domModel *dom.DOM, chromeInstance *chrome.Instance, w *gzip.Writer) error {
	var nodeUpdate *domjson.DOMUpdate
	var err error
	switch event.Method {
	case DomChildNodeInserted:
		nodeUpdate, err = domModel.ProcessNodeInsertion(dom.Node(event.Params))
	case DomChildNodeRemoved:
		nodeUpdate, err = domModel.ProcessNodeRemoval(dom.Node(event.Params))
	case DomAttributeModified:
		nodeUpdate, err = domModel.ProcessNodeAttributeModification(dom.Node(event.Params))
	}
	if err != nil {
		fmt.Printf("error generating node update: %v\n", err)
		// TODO(vaspol): let the proxy continue on error for now. deal with this later.
		return err
	} else if nodeUpdate == nil {
		return nil
	}
	domUpdates := []*domjson.DOMUpdate{nodeUpdate}
	jsonDOMUpdates := domjson.DOMUpdates{Updates: domUpdates}
	fmt.Printf("in handle node update: %v\n", event.Params)
	err = h.sendMessage(w, jsonDOMUpdates)
	if err != nil {
		fmt.Printf("error sending node updates: %v\n", err)
		return err
	}
	return nil
}

// Sends the message in the protobuf format through the wire.
func (h *Handler) sendMessage(w *gzip.Writer, jsonDOMUpdates domjson.DOMUpdates) error {
	wireFormat, err := json.Marshal(jsonDOMUpdates)
	if h.verbose {
		io.Copy(os.Stdout, strings.NewReader(string(wireFormat)))
	}
	if err != nil {
		fmt.Printf("error marshaling to JSON: :%v\n", wireFormat)
		return err
	}
	_, err = io.WriteString(w, string(wireFormat)+delim)
	return err
}
