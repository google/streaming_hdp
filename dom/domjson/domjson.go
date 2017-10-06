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

// Package domjson defines the data types used to represent DOM updates. These data types are serializable into JSON.
package domjson

type DOMUpdates struct {
	Updates []*DOMUpdate
}

type DOMUpdate struct {
	Action Action
	Node   Node
}

type Node struct {
	NodeID         string
	ParentNodeID   string // Can be "" if removing or modifying a node.
	PreviousNodeID string // Can be "" if inserting at beginning of level, removing, or modifying a node.
	ElementType    string
	Attributes     map[string]string
	Text           string // The content in the text node, if any.
}

type Action int

const (
	Invalid Action = iota
	Insert
	Remove
	Modify
)
