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

// Package dom implements methods to construct initial DOM and further DOM manipulation.
package dom

import (
	"fmt"
	"strconv"
	"strings"

	"streaming_hdp/dom/domjson"
)

// Node represents a node in the DOM tree.
type Node map[string]interface{}

// DOM holds the information for the DOM.
type DOM struct {
	nodeIDMapping   map[string]string // Maps from the node ID to the backend node ID.
	backendNodeIDs  map[string]bool   // A set containing the backend node IDs.
	nodeTypeMapping map[string]string // Maps from backend node ID to the node type.
}

const (
	// NodeID defines the NodeID field.
	NodeID = "nodeId"
	// BackendNodeID defines the BackendNodeID field.
	BackendNodeID = "backendNodeId"
	// NodeName defines the NodeName field.
	NodeName = "nodeName"
	// LocalName defines the LocalName field.
	LocalName = "localName"
	// NodeValue defines the NodeValue field.
	NodeValue = "nodeValue"
	// Children defines the Children field.
	Children = "children"
	// ParentNodeID defines the ParentNodeID field.
	ParentNodeID = "parentNodeId"
	// PreviousNodeID defines the ParentNodeID field.
	PreviousNodeID = "previousNodeId"
	// NodeField defines the Node field.
	NodeField = "node"
	// Attributes defines the Attribute field.
	Attributes = "attributes"
	// Nodes defines the Nodes field.
	Nodes = "nodes"
	// ParentID defines the ParentID field. This field is defined in
	// the Node object but not the insertion object.
	ParentID = "parentId"
	// Name defines the Name field for attribute modification DOM update.
	Name = "name"
	// Value defines the Value field for attribute modification DOM update.
	Value = "value"
)

// NewDOMModel creates an instance of DOM for maintaining states for the model.
func NewDOMModel() *DOM {
	dom := DOM{
		nodeIDMapping:   make(map[string]string),
		backendNodeIDs:  make(map[string]bool),
		nodeTypeMapping: make(map[string]string),
	}
	return &dom
}

// ProcessNodeInsertion turns the node information into a protobuf DOMUpdate with INSERT action.
func (d *DOM) ProcessNodeInsertion(node Node) (*domjson.DOMUpdate, error) {
	nodeDetails := Node(node[NodeField].(map[string]interface{}))

	// nodeID of 0 indicates a non-existing node.
	parentNodeID, err := getNodeIDStr(node, ParentNodeID)
	if err != nil {
		return nil, err
	}
	if parentNodeID != "" {
		var ok bool
		parentNodeID, ok = d.nodeIDMapping[parentNodeID]
		if !ok {
			return nil, fmt.Errorf("parent node does not exists")
		}
	}

	prevNodeID, err := getNodeIDStr(node, PreviousNodeID)
	if err != nil {
		return nil, err
	}
	if prevNodeID != "" {
		var ok bool
		prevNodeID, ok = d.nodeIDMapping[prevNodeID]
		if !ok {
			return nil, fmt.Errorf("the provided previous node does not exists")
		}
	}

	nodeID, err := getNodeIDStr(nodeDetails, NodeID)
	if err != nil {
		return nil, err
	}
	backendNodeID, err := getNodeIDStr(nodeDetails, BackendNodeID)
	if err != nil {
		return nil, err
	}
	insert := d.createNodeInsertUpdate(backendNodeID, parentNodeID, prevNodeID, nodeDetails)
	d.nodeIDMapping[nodeID] = backendNodeID
	return insert, nil
}

// ProcessNodeRemoval turns the node information into a protobuf DOMUpdate with REMOVE action.
func (d *DOM) ProcessNodeRemoval(node Node) (*domjson.DOMUpdate, error) {
	// nodeID of 0 indicates a non-existing node.
	parentNodeID, err := getNodeIDStr(node, ParentNodeID)
	if err != nil {
		return nil, err
	}
	parentBackendID, err := d.getBackendNodeID(parentNodeID)
	if err != nil {
		return nil, err
	}
	nodeID, err := getNodeIDStr(node, NodeID)
	if err != nil {
		return nil, err
	}
	backendNodeID, err := d.getBackendNodeID(nodeID)
	if err != nil {
		return nil, err
	}
	remove := createNodeRemovalUpdate(backendNodeID, parentBackendID)
	delete(d.nodeIDMapping, nodeID)
	delete(d.backendNodeIDs, nodeID)
	delete(d.nodeTypeMapping, nodeID)
	return remove, nil
}

// ProcessNodeAttributeModification turns attribute modification information to DOM update commands.
func (d *DOM) ProcessNodeAttributeModification(node Node) (*domjson.DOMUpdate, error) {
	nodeID, err := getNodeIDStr(node, NodeID)
	if err != nil {
		return nil, err
	}
	backendNodeID, err := d.getBackendNodeID(nodeID)
	if err != nil {
		return nil, err
	}
	name := node[Name].(string)
	value := node[Value].(string)
	attributeModification := createNodeAttributeUpdate(backendNodeID, name, value)
	return attributeModification, nil
}

// ProcessSetChildNodes turns the node change information into insert updates.
func (d *DOM) ProcessSetChildNodes(node Node) ([]*domjson.DOMUpdate, error) {
	parentNodeID, err := getNodeIDStr(node, ParentID)
	if err != nil {
		return nil, err
	}
	parentBackendID, err := d.getBackendNodeID(parentNodeID)
	if err != nil {
		return nil, err
	}
	nodes := node["nodes"].([]interface{})
	result := []*domjson.DOMUpdate{}
	prevNodeID := ""
	for _, nodeInterface := range nodes {
		curNode := Node(nodeInterface.(map[string]interface{}))
		nodeSubTreeUpdates := []*domjson.DOMUpdate{}
		d.generateInitialDOMHelper(curNode, parentBackendID, prevNodeID, &nodeSubTreeUpdates)
		prevNodeID, err = getNodeIDStr(curNode, BackendNodeID)
		result = append(result, nodeSubTreeUpdates...)
	}
	return result, nil
}

// GenerateInitialDOM takes in a root node and generates a slice of DOM Updates.
func (d *DOM) GenerateInitialDOM(rootNode Node) ([]*domjson.DOMUpdate, error) {
	result := []*domjson.DOMUpdate{}
	if _, err := d.generateInitialDOMHelper(rootNode, "", "", &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Helper for generating the DOM and keep appending the results to the result parameter.
// Returns the processed node id.
func (d *DOM) generateInitialDOMHelper(curNode Node, parentNodeID, prevNodeID string, result *[]*domjson.DOMUpdate) (string, error) {
	backendNodeID, err := getNodeIDStr(curNode, BackendNodeID)
	if err != nil {
		return "", err
	}
	nodeID, err := getNodeIDStr(curNode, NodeID)
	if err != nil {
		return "", err
	}
	if _, ok := d.backendNodeIDs[backendNodeID]; !ok {
		d.backendNodeIDs[backendNodeID] = true
		if strings.ToLower(d.nodeTypeMapping[parentNodeID]) != "script" {
			// Skip the root document node and scripts.
			insert := d.createNodeInsertUpdate(backendNodeID, parentNodeID, prevNodeID, curNode)
			*result = append(*result, insert)
		}
	}

	if curNode[Children] != nil {
		children := curNode[Children].([]interface{})
		prevNodeID := ""
		for _, c := range children {
			childNode := Node(c.(map[string]interface{}))
			prevNodeID, err = d.generateInitialDOMHelper(childNode, backendNodeID, prevNodeID, result)
			if err != nil {
				return "", err
			}
		}
	}
	d.nodeIDMapping[nodeID] = backendNodeID
	return backendNodeID, nil
}

// Helper for creating a node insertion update protobuf object.
func (d *DOM) createNodeInsertUpdate(nodeID, parentNodeID, prevNodeID string, node Node) *domjson.DOMUpdate {
	attributes := make(map[string]string)
	elementType := node[NodeName].(string)
	if strings.ToLower(elementType) != "script" {
		attributes = getAttributes(node)
	}
	jsonNode := domjson.Node{
		NodeID:         nodeID,
		ParentNodeID:   parentNodeID,
		PreviousNodeID: prevNodeID,
		Attributes:     attributes,
		ElementType:    elementType,
		Text:           node[NodeValue].(string),
	}
	insert := domjson.DOMUpdate{
		Action: domjson.Insert,
		Node:   jsonNode,
	}
	d.nodeTypeMapping[nodeID] = elementType
	return &insert
}

// Helper for creating a node removal update protobuf object.
func createNodeRemovalUpdate(nodeID, parentNodeID string) *domjson.DOMUpdate {
	jsonNode := domjson.Node{
		NodeID:       nodeID,
		ParentNodeID: parentNodeID,
	}
	remove := domjson.DOMUpdate{
		Action: domjson.Remove,
		Node:   jsonNode,
	}
	return &remove
}

// Helper for creating a node attribute update.
func createNodeAttributeUpdate(nodeID, attrName, attrValue string) *domjson.DOMUpdate {
	jsonNode := domjson.Node{
		NodeID: nodeID,
		Attributes: map[string]string{
			attrName: attrValue,
		},
	}
	attributeUpdate := domjson.DOMUpdate{
		Action: domjson.Modify,
		Node:   jsonNode,
	}
	return &attributeUpdate
}

// Helper function to get the node ID string.
// Returns the id in string. If the ID is 0, the returned string will be empty indicating
// that the node ID is for a null node. For example, this function will return empty string
// for the parent of a root node.
func getNodeIDStr(node Node, field string) (string, error /* ok could be bool */) {
	idInterface, ok := node[field]
	if !ok {
		return "", fmt.Errorf("node %v missing field %s", node, field)
	}
	idFloat, ok := idInterface.(float64)
	if !ok {
		return "", fmt.Errorf("field %s in node %v is not a number", field, node)
	}
	if idFloat == 0 {
		// This is the case where a node is added without a previous child.
		// 0 means a null node.
		return "", nil
	}
	return strconv.Itoa(int(idFloat)), nil
}

// getAttributes transforms a list of key-value attribute pairs to a map of
// attribute key and value.
func getAttributes(node Node) map[string]string {
	attributesMap := make(map[string]string)
	if attributesPairs, ok := node[Attributes].([]interface{}); ok {
		for i := 0; i < len(attributesPairs); i += 2 {
			attributesMap[attributesPairs[i].(string)] = attributesPairs[i+1].(string)
		}
	}
	return attributesMap
}

// Helper for retrieving the backend node ID of a node.
func (d *DOM) getBackendNodeID(nodeID string) (string, error) {
	backendNodeID, ok := d.nodeIDMapping[nodeID]
	if !ok {
		return "", fmt.Errorf("node %v does not have a backend node", nodeID)
	}
	return backendNodeID, nil
}
