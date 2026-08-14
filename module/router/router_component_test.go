package router

import "testing"

func TestRouterComponentAddAndLookup(t *testing.T) {
	comp := &RouterComponent{}
	comp.Awake()
	node := &RouterNode{ConnectId: 1, OuterConnID: 1001}
	comp.AddNode(node)
	if got, ok := comp.GetNodeByOuter(1001); !ok || got != node {
		t.Fatalf("outer lookup failed")
	}
	if got, ok := comp.GetNodeByConnect(1); !ok || got != node {
		t.Fatalf("connect lookup failed")
	}
	comp.RemoveNode(node)
	if _, ok := comp.GetNodeByOuter(1001); ok {
		t.Fatalf("node should be removed")
	}
}
