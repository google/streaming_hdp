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

package chrome

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/phayes/freeport"
)

// Number of Chrome instances that we should have available.
const numBufferedInstance = 15

// InstanceManager manages Chrome instances.
type InstanceManager struct {
	nextInstanceID int      // The next instance ID for Chrome.
	instanceQueue  chan int // The queue for sending back the instances.

	instancesMutex sync.Mutex        // Protects the following fields.
	instances      map[int]*Instance // Holds a mapping from instance ID to a reference of the Chrome instance.
	urls           map[int]string    // Holds a mapping from instance ID to the URL.
	useFullChrome  bool              // Whether to start Chrome with GUI.
}

// NewInstanceManager creates a new instance manager.
func NewInstanceManager(useFullChrome bool) *InstanceManager {
	newInstanceManager := InstanceManager{
		nextInstanceID: 0,
		instances:      make(map[int]*Instance),
		urls:           make(map[int]string),
		useFullChrome:  useFullChrome,
		instanceQueue:  make(chan int, numBufferedInstance),
	}

	go func() {
		for {
			newInstanceManager.addInstance(useFullChrome)
		}
	}()
	return &newInstanceManager
}

// AddInstance adds an instance to the instance manager. Returns -1, if there is an error.
func (im *InstanceManager) addInstance(useFullChrome bool) {
	im.instancesMutex.Lock()
	id := im.nextInstanceID
	im.nextInstanceID++
	im.instancesMutex.Unlock()

	// Start and connect to an instance of Chrome.
	chromePort, err := freeport.GetFreePort()
	if err != nil {
		fmt.Printf("failed to get an unused port\n")
		return
	}

	// We don't care about the ID of Chrome in this case.
	chromeInstance, err := New(chromePort, useFullChrome)
	if err != nil {
		fmt.Printf("failed to create an instance of chrome\n")
		return
	}

	im.instancesMutex.Lock()
	im.instances[id] = chromeInstance
	im.instancesMutex.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := chromeInstance.Wait(ctx); err != nil {
		fmt.Printf("got an error starting chrome: %v\n", err)
		return
	}
	err = chromeInstance.Connect()
	if err != nil {
		fmt.Printf("chrome instance failed to connect to DevTools: %v\n", err)
		return
	}
	im.instanceQueue <- id
}

// GetURL returns the URL associated to the instanceID.
func (im *InstanceManager) GetURL(instanceID int) (string, error) {
	im.instancesMutex.Lock()
	defer im.instancesMutex.Unlock()
	url, ok := im.urls[instanceID]
	if !ok {
		return "", errors.New("URL with this ID does not exist")
	}
	return url, nil
}

// GetNewInstance returns a Chrome instance and registers the URL to
// the instance. The caller is responsible to call WaitUntilChromeReady()
// to ensure that Chrome is usable. This call also starts the timer
// for the next chrome instance.
func (im *InstanceManager) GetNewInstance(url string) int {
	nextInstanceID := <-im.instanceQueue
	im.instancesMutex.Lock()
	defer im.instancesMutex.Unlock()
	im.urls[nextInstanceID] = url
	im.instances[nextInstanceID].InitializeTimeout()
	return nextInstanceID
}

// GetInstance returns the Chrome instance associated to the instanceID.
func (im *InstanceManager) GetInstance(instanceID int) (*Instance, error) {
	im.instancesMutex.Lock()
	defer im.instancesMutex.Unlock()
	instance, ok := im.instances[instanceID]
	if !ok {
		return nil, errors.New("instance with this ID does not exist")
	}
	return instance, nil
}

// RemoveInstance removes the instance from the manager. It is the responsibility of
// the caller to cleanup the instance before removing the instance.
func (im *InstanceManager) RemoveInstance(instanceID int) error {
	im.instancesMutex.Lock()
	defer im.instancesMutex.Unlock()
	_, ok := im.instances[instanceID]
	if !ok {
		return errors.New("instance with this ID does not exist")
	}
	delete(im.instances, instanceID)
	delete(im.urls, instanceID)
	return nil
}
