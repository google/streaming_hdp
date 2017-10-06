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

// Package chrome manages a Chrome instance.
package chrome

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"streaming_hdp/devtools"
	"streaming_hdp/dom"
)

const (
	pageStableThreshold  = time.Duration(5) * time.Second / time.Millisecond
	userAgentString      = "Mozilla/5.0 (Linux; Android 4.4.4; XT1034 Build/KXB21.14-L1.61) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/59.0.3071.125 Mobile Safari/537.36 PTST/170721.190705"
	viewPortWidth        = 360
	viewPortHeight       = 640
	viewPortPixelDensity = 2
)

var (
	instanceTimeout = time.Duration(25) * time.Second
)

// Instance represents an instance of Chrome.
type Instance struct {
	port              int                  // The port for connecting to DevTools.
	Command           *exec.Cmd            // The Chrome instance command.
	devtoolsConn      *devtools.Connection // The connection to Chrome DevTools.
	userDir           string               // Chrome's user directory. Should be delete upon termination.
	timeoutTimer      *time.Timer          // The timer for detecting timeout. The timer will be reset every time the instance receives a new event.
	mu                sync.Mutex           // Mutex to guard race condition on c.devtoolsConn
	pageLoadCompletes chan bool            // Channel to signal when the page load completes.
	ready             chan bool            // Channel to signal when the connection to DevTools has been established.
}

// New returns a new Chrome instance and also starts a headless
// Chrome running with the specified port in the background.
// Args:
//	- port: the port for connecting to DevTools.
//	- useFullChrome: whether to start Chrome with in headless mode or not.
func New(port int, useFullChrome bool) (*Instance, error) {
	dir, err := ioutil.TempDir("/tmp/", "chrome_data")
	if err != nil {
		fmt.Printf("failed to create a temporary user data directory: %v\n", err)
		return nil, err
	}
	chrome := "google-chrome"
	args := []string{
		"--remote-debugging-port=" + strconv.Itoa(port),
		"--user-data-dir=" + dir,
		"about:blank",
	}
	if !useFullChrome {
		args = append(args, "--headless")
	}
	chromeCmd := exec.Command(chrome, args...)
	err = chromeCmd.Start()
	if err != nil {
		return nil, err
	}
	return &Instance{
		port:    port,
		Command: chromeCmd,
		userDir: dir,
		ready:   make(chan bool, 1),
	}, nil
}

// Started returns whether Chrome has started.
func (c *Instance) started() (bool, error) {
	pid := c.Command.Process.Pid
	out, err := exec.Command("kill", "-s", "0", strconv.Itoa(pid)).CombinedOutput()
	if err != nil {
		fmt.Printf("error finding process: %v\n", err)
		return false, err
	}

	if string(out) == "" {
		return true, nil // pid exist
	}
	return false, nil
}

// Wait waits until Chrome has started.
func (c *Instance) Wait(ctx context.Context) error {
	ok := false
	for !ok {
		select {
		case <-ctx.Done():
			return errors.New("timeout waiting for Chrome to start")
		default:
			ok, err := c.started()
			if ok || err != nil {
				return err
			}
			time.Sleep(5 * time.Second)
		}
	}
	return nil
}

// InitializeTimeout starts the timer for resetting this chrome instance.
func (c *Instance) InitializeTimeout() {
	if c.timeoutTimer != nil {
		log.Fatalf("InitializeTimeout was already called\n")
	}
	timeoutTimer := time.AfterFunc(instanceTimeout, func() {
		c.DisconnectAndTerminate()
	})
	c.timeoutTimer = timeoutTimer
}

// ResetTimeout resets the timeout. Returns true if the timeout hasn't expired, false otherwise.
func (c *Instance) ResetTimeout() bool {
	beforeTimedout := c.timeoutTimer.Stop()
	if beforeTimedout {
		c.timeoutTimer.Reset(instanceTimeout)
	}
	return beforeTimedout
}

// Connect connects to a tab on the Chrome instance.
func (c *Instance) Connect() error {
	tryLimit := 5
	tryCounter := 0
	var err error
	for tryCounter < tryLimit {
		// TODO(vaspol): This assumes that Chrome is always running locally.
		connection, err := devtools.NewConnection("localhost:" + strconv.Itoa(c.port))
		if err == nil {
			c.devtoolsConn = connection
			break
		}
		tryCounter++
		time.Sleep(2 * time.Second) // Sleep until the next try.
	}
	if tryCounter >= tryLimit {
		fmt.Printf("failed to connect to devtools on port: %v\n", c.port)
		c.killInstance()
		return err
	}
	c.pageLoadCompletes = make(chan bool)
	close(c.ready)
	return nil
}

// DisconnectAndTerminate disconnects and terminates from the Chrome instance.
func (c *Instance) DisconnectAndTerminate() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.devtoolsConn == nil {
		return nil
	}
	close(c.pageLoadCompletes) // Send a signal that the page load has complete.
	c.devtoolsConn.Close()
	c.devtoolsConn = nil
	if err := c.killInstance(); err != nil {
		fmt.Printf("failed to kill Chrome instance: %v\n", err)
		return err
	}
	return nil
}

// killInstance kills the Chrome process by sending the Kill signal to the process.
func (c *Instance) killInstance() error {
	if err := c.Command.Process.Kill(); err != nil {
		fmt.Printf("failed to kill Chrome instance: %v\n", err)
		return err
	}
	os.RemoveAll(c.userDir)
	if _, err := c.Command.Process.Wait(); err != nil {
		fmt.Printf("failed on waiting Chrome instance: %v\n", err)
		return err
	}
	return nil
}

// EnableDomains enables subscription of DevTools domains.
// Args:
//	- domains: contains the name of the domain to be enabled.
func (c *Instance) EnableDomains(domains ...string) {
	// Ensure that we already have connected to Chrome DevTools.
	if c.devtoolsConn == nil {
		fmt.Printf("%p trying to enable domains but not connected to Devtools", c)
	}
	dc := c.devtoolsConn
	for _, domain := range domains {
		dc.InvokeMethod(domain+".enable", devtools.Params{})
	}
}

// NextEvent returns the next event received by this Chrome instance.
// This also resets the timer set for detecting the instance timeout.
func (c *Instance) NextEvent() (devtools.EventMessage, error) {
	c.ResetTimeout()
	return c.devtoolsConn.NextEvent()
}

// NavigateToPage navigates to the specified page.
// Args:
//	- page:	    the URL of the page to navigate to.
func (c *Instance) NavigateToPage(page string) error {
	fmt.Printf("Navigating to: %v\n", page)
	// Ensure that we already have connected to Chrome DevTools.
	if c.devtoolsConn == nil {
		log.Fatalf("%v navigating to %v, but is not connected to Chrome on port %v\n", c, page, c.port)
	}

	// Setup handler for when the page load stablizes.
	// Use emulation domain to monitor when the page stablizes.
	dc := c.devtoolsConn
	dc.InvokeMethod("Network.setUserAgentOverride", devtools.Params{
		"userAgent": userAgentString,
	})

	dc.InvokeMethod("Emulation.setDeviceMetricsOverride", devtools.Params{
		"width":             viewPortWidth,
		"height":            viewPortHeight,
		"deviceScaleFactor": viewPortPixelDensity,
		"mobile":            true,
	})

	result := dc.InvokeMethodAndGetReturn("Emulation.setVirtualTimePolicy",
		devtools.Params{
			"policy": "pauseIfNetworkFetchesPending",
			"budget": int(pageStableThreshold),
		})
	if result.Type == devtools.ResultError {
		fmt.Printf("method invocation error: %v\n", result.Params)
	}

	// Navigate to the target site.
	dc.InvokeMethod("Page.navigate", devtools.Params{
		"url": page})
	return nil
}

// GetDOMInstance returns an instance to the root node of the DOM tree.
func (c *Instance) GetDOMInstance() (dom.Node, error) {
	dc := c.devtoolsConn
	if dc == nil {
		log.Fatalf("%v getting DOM, but is not connected to Chrome on port %v\n", c, c.port)
	}
	resp := dc.InvokeMethodAndGetReturn("DOM.getDocument", devtools.Params{"depth": -1})
	if resp.Type == devtools.ResultError {
		fmt.Printf("unable to get DOM: %v\n", resp.Params)
		return nil, errors.New("unable to get the root document from DevTools")
	}
	// Check if the response is valid.
	if _, ok := resp.Params["root"]; !ok {
		return nil, errors.New("malformed response. Missing \"root\" attribute")
	}
	return dom.Node(resp.Params["root"].(map[string]interface{})), nil
}

// GetDOM retrieves the DOM from Chrome.
func (c *Instance) GetDOM() (string, error) {
	dc := c.devtoolsConn
	if dc == nil {
		log.Fatalf("Getting DOM, but is not connected to Chrome on port %v\n", c.port)
	}
	root, err := c.GetDOMInstance()
	if err != nil {
		return "", err
	}
	output := dc.InvokeMethodAndGetReturn("DOM.getOuterHTML", devtools.Params{"nodeId": int(root["nodeId"].(float64))})
	if output.Type == devtools.ResultError {
		return "", errors.New("unable to get the DOM from DevTools")
	}
	if _, ok := output.Params["outerHTML"]; !ok {
		return "", errors.New("outerHTML attribute missing from the response")
	}
	return output.Params["outerHTML"].(string), nil
}

// RequestChildNodes tells Chrome to monitor the given node for subsequent children changes to the node.
func (c *Instance) RequestChildNodes(nodeID float64) {
	dc := c.devtoolsConn
	if dc == nil {
		log.Fatalf("%p requesting dom, but is not connected to Chrome on port %v\n", c, c.port)
	}
	dc.InvokeMethod("DOM.requestChildNodes", devtools.Params{
		"nodeId": nodeID,
		"depth":  -1,
	})
}

// WaitUntilPageLoadCompletes will block until the page load on this Chrome instance completes.
func (c *Instance) WaitUntilPageLoadCompletes() {
	<-c.pageLoadCompletes
}

// WaitUntilChromeReady will block until we are connected to DevTools.
func (c *Instance) WaitUntilChromeReady() error {
	<-c.ready
	if c.devtoolsConn == nil {
		return errors.New("not connected to a Chrome instance")
	}
	return nil
}
