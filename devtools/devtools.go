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

// Package devtools contains an interface to interact with a Chrome instance using the Chrome Devtools Protocol.
// More infomration on the protocol can be found at https://chromedevtools.github.io/devtools-protocol/
package devtools

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// TabType defines the value of Page.Type for Chrome tabs.
	TabType = "page"

	// The error code of a websocket.CloseError that is expected when the socket has begun the process of closing.
	expectedCloseErrorCode = 1006

	// The size of the buffer for holding temporary unprocessed events assuming 1000 is large enough.
	//
	// TODO: Though buffer size of 1000 should be sufficiently large, this implementation can potentially
	// lead to a deadlock. Ideally, this should be an indefinitely large buffered channel.
	tempBufferSize = 1000
)

// Page is the struct retrieved from /json/ of Chrome in Debug mode. A Page can be a tab, background process, or other.
// Each Page needs a separate connection to control using the Devtools Protocol.
type Page struct {
	Description          string `json:"description"`
	DevtoolsFrontendURL  string `json:"devtoolsFrontendUrl"`
	ID                   string `json:"id"`
	Title                string `json:"title"`
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// EventMessage defines the structure of an event message.
type EventMessage struct {
	MessageID int
	Method    string `json:"method"`
	Params    Params `json:"params"`
}

// Params can hold the parameters of a method, the return value of a method, or the parameters of an event.
type Params map[string]interface{}

// getField returns the field. Returns nil, false if the field does not exist.
// Example field names: "child", "child.grandchild".
func (p Params) getField(field string) (interface{}, bool) {
	var current interface{} = p
	for _, f := range strings.Split(field, ".") {
		m, ok := current.(map[string]interface{})
		if !ok {
			m, ok = current.(Params)
		}
		if !ok {
			return nil, false
		}
		child, ok := m[f]
		if !ok {
			return nil, false
		}
		current = child
	}
	return current, true
}

// Int converts the supplied field to an int, and returns 0 if the underlying value is not numeric.
func (p Params) Int(field string) (int, bool) {
	if fv, ok := p.Float(field); ok {
		return int(fv), true
	}
	return 0, false
}

// Float converts the supplied field to a float64, and returns 0 if the underlying value is not numeric.
func (p Params) Float(field string) (float64, bool) {
	if f, ok := p.getField(field); ok {
		if fv, ok := f.(float64); ok {
			return fv, true
		}
	}
	return 0, false
}

// String converts the supplied field to a string, and returns "" if the underlying value is not a string.
func (p Params) String(field string) (string, bool) {
	if f, ok := p.getField(field); ok {
		if sv, ok := f.(string); ok {
			return sv, true
		}
	}
	return "", false
}

// method holds the information necessary to invoke a method on Chrome. method's are created using the InvokeMethod functions.
type method struct {
	ID     int    `json:"id"`
	Method string `json:"method"`
	Params Params `json:"params"`
}

// ResultType abstracts away the type of the response.
type ResultType string

// Values for ResultType.
const (
	// ResultValid defines when the result of a method invocation is valid.
	ResultValid ResultType = "result"
	// ResultError defines when the result of a method invocation has an error.
	ResultError ResultType = "error"
)

// Result defines a response of an method invocation.
type Result struct {
	ID        int
	MessageID int
	Type      ResultType
	Params    Params
}

// Connection can communicate with a Chrome instance using the Chrome Devtools Protocol.
type Connection struct {
	// Web socket connected to one Chrome's pages
	sock *websocket.Conn

	// Channel used to stop sendMessages subroutine.
	stopSend chan bool

	// Channels used by subroutines to indicate they have stopped.
	recvEnded chan bool
	sendEnded chan bool

	// Channel used to send methods to Chrome.
	toSend chan method

	// Channels used to get the return values from methods.
	resultsMutex sync.Mutex
	results      map[int]chan Result

	// Name and port of Chrome's debugging interface.
	hostport string

	// Used to assign unique IDs to methods.
	methodIDMutex sync.Mutex
	nextMethodID  int

	// Buffer for holding unprocessable events.
	bufferedEvents chan EventMessage

	// The number of message received.
	messageReceived int
}

// NewConnection creates a new Connection, which is connected to the active tab of the Chrome instance specified by hostport.
// If the Chrome instance at hostport just started, the connection may fail. Chrome takes a few seconds to be ready to connect to.
func NewConnection(hostport string) (*Connection, error) {
	// Creates an empty Connection struct.
	c := &Connection{
		sock:            nil,
		stopSend:        make(chan bool),
		recvEnded:       make(chan bool),
		sendEnded:       make(chan bool),
		toSend:          make(chan method),
		resultsMutex:    sync.Mutex{},
		results:         make(map[int]chan Result),
		hostport:        hostport,
		methodIDMutex:   sync.Mutex{},
		nextMethodID:    0,
		bufferedEvents:  make(chan EventMessage, tempBufferSize),
		messageReceived: 0,
	}

	// Finds the active tab.
	activeTab, err := c.ActiveTab()
	if err != nil {
		return nil, err
	}

	// Connects to the active tab's debuging address.
	err = c.ConnectToPage(activeTab)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Pages returns a list of the current Pages in Chrome. These will include tabs and background processes active in Chrome.
func (c *Connection) Pages() ([]*Page, error) {
	resp, err := http.Get("http://" + c.hostport + "/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var pages []*Page
	err = json.NewDecoder(resp.Body).Decode(&pages)
	if err != nil {
		return nil, err
	}

	return pages, nil
}

// ActiveTab returns the currently active tab in Chrome. Chrome stores Pages in order of most recently used, so it returns the first Page with type of "page".
func (c *Connection) ActiveTab() (*Page, error) {
	pages, err := c.Pages()
	if err != nil {
		return nil, err
	}

	for _, page := range pages {
		if page.Type == TabType {
			return page, nil
		}
	}
	return nil, errors.New("no tabs found")
}

// ConnectToPage connects to page's debugger address.
func (c *Connection) ConnectToPage(page *Page) error {
	if c.sock != nil {
		return errors.New("sock is already connected to some Page")
	}

	sock, _, err := websocket.DefaultDialer.Dial(page.WebSocketDebuggerURL, nil)
	if err != nil {
		return err
	}
	c.sock = sock

	// Starts send and receive subroutines.
	go c.receiveMessages()
	go c.sendMessages()

	return nil
}

// Close closes the connection to Chrome.
func (c *Connection) Close() {
	// Tell the subroutines to end.
	c.stopSend <- true

	// Wait for all of them to end.
	<-c.sendEnded

	// Send close only after we are done with sending all messages.
	// Sends a close control message to Chrome, and causes any ReadMessage or WriteMessage to return an "expected" error.
	c.sock.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(10.0*time.Second))

	// Safe to stop all receiving of messages.
	<-c.recvEnded

	// Closes the websocket.
	c.sock.Close()
	c.sock = nil
}

// receiveMessages continually receives messages, and processes received messages.
func (c *Connection) receiveMessages() {
	// Make sure to close all channels when all messages are received.
	defer close(c.bufferedEvents)
	defer close(c.recvEnded)

receiveLoop:
	for {
		// Receives the data as []bytes.
		_, data, err := c.sock.ReadMessage()
		if websocket.IsCloseError(err, expectedCloseErrorCode) {
			break receiveLoop
		}
		if err != nil {
			log.Fatal(err)
		}

		curMessageID := c.messageReceived
		c.messageReceived++

		// Converts the data from []bytes.
		var msg map[string]interface{}
		json.Unmarshal(data, &msg)

		// Checks if the message was a reply to a method.
		if id, ok := msg["id"]; ok {
			idInt := int(id.(float64))

			c.resultsMutex.Lock()
			resultChan, returnResult := c.results[idInt]
			delete(c.results, idInt)
			c.resultsMutex.Unlock()

			if returnResult {
				var params Params
				var resultType ResultType
				if _, ok := msg[string(ResultValid)]; ok {
					params = Params(msg[string(ResultValid)].(map[string]interface{}))
					resultType = ResultValid
				} else if _, ok := msg[string(ResultError)]; ok {
					params = Params(msg[string(ResultError)].(map[string]interface{}))
					resultType = ResultError
				}
				result := Result{
					ID:        idInt,
					MessageID: curMessageID,
					Type:      resultType,
					Params:    params,
				}
				resultChan <- result
			}

		} else {
			// Treats any other message as an event.
			event := EventMessage{
				MessageID: curMessageID,
				Method:    msg["method"].(string),
				Params:    Params(msg["params"].(map[string]interface{})),
			}

			// Add the event to a buffer.
			c.bufferedEvents <- event
		}
	}
}

// sendMessages sends out any methods waiting to be sent.
func (c *Connection) sendMessages() {
	// Make sure to close all channels when done sending messages.
	defer close(c.sendEnded)
sendLoop:
	for {
		select {
		case <-c.stopSend:
			break sendLoop
		case msg := <-c.toSend:
			// Converts the message to JSON.
			data, err := json.Marshal(msg)
			if err != nil {
				log.Fatal(err)
			}

			// Sends the message.
			err = c.sock.WriteMessage(websocket.TextMessage, data)
			if websocket.IsCloseError(err, expectedCloseErrorCode) {
				continue sendLoop
			}
			if err != nil {
				log.Fatal(err)
			}
		}
	}
}

// NextEvent returns the next event.
func (c *Connection) NextEvent() (EventMessage, error) {
	retval, ok := <-c.bufferedEvents
	if !ok {
		return retval, io.EOF
	}
	return retval, nil
}

// newMethodId returns an id for a new method.
func (c *Connection) newMethodID() int {
	c.methodIDMutex.Lock()
	rv := c.nextMethodID
	c.nextMethodID++
	c.methodIDMutex.Unlock()
	return rv
}

// InvokeMethod invokes the specified method in Chrome. Doesn't wait for a response.
func (c *Connection) InvokeMethod(methodName string, params Params) {
	msg := method{
		ID:     c.newMethodID(),
		Method: methodName,
		Params: params,
	}
	c.toSend <- msg
}

// InvokeMethodAndGetReturn invokes the specified method in Chrome and returns Chrome's response.
// If an error occurs, the error response will be returned.
func (c *Connection) InvokeMethodAndGetReturn(methodName string, params Params) Result {
	// TODO: this method doesn't expose the error when something goes bad at the API
	// level. It would be great to expose such error.
	methodID := c.newMethodID()

	msg := method{
		ID:     methodID,
		Method: methodName,
		Params: params,
	}

	c.resultsMutex.Lock()
	resultChan := make(chan Result)
	c.results[methodID] = resultChan
	c.resultsMutex.Unlock()

	c.toSend <- msg
	return <-resultChan
}
