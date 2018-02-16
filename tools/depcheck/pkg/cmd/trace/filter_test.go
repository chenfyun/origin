package trace

import (
	"strings"
	"testing"

	"github.com/gonum/graph/concrete"

	depgraph "github.com/openshift/origin/tools/depcheck/pkg/graph"
)

type testNode struct {
	name          string
	outboundEdges []string
}

func getVendorNodes() []*testNode {
	return []*testNode{
		{
			name: "github.com/test/repo/vendor/github.com/testvendor/prefix1",
			outboundEdges: []string{
				"github.com/test/repo/vendor/github.com/testvendor/prefix1/one",
			},
		},
		{
			name: "github.com/test/repo/vendor/github.com/testvendor/prefix1/one",
			outboundEdges: []string{
				"github.com/test/repo/vendor/github.com/testvendor/prefix2/one",
			},
		},
		{
			name: "github.com/test/repo/vendor/github.com/testvendor/prefix2",
			outboundEdges: []string{
				"github.com/test/repo/vendor/github.com/testvendor/prefix2/one",
			},
		},
		{
			name:          "github.com/test/repo/vendor/github.com/testvendor/prefix2/one",
			outboundEdges: []string{},
		},
		{
			name: "github.com/test/repo/vendor/github.com/docker/docker-test-util",
			outboundEdges: []string{
				"github.com/test/repo/vendor/github.com/docker/docker-test-util/api",
			},
		},
		{
			name: "github.com/test/repo/vendor/github.com/docker/docker-test-util/api",
			outboundEdges: []string{
				"github.com/test/repo/vendor/github.com/google/glog",
			},
		},
		{
			name:          "github.com/test/repo/vendor/github.com/google/glog",
			outboundEdges: []string{},
		},
	}
}

func getNonVendorNodes() []*testNode {
	return []*testNode{
		{
			name: "github.com/test/repo/pkg/prefix1",
			outboundEdges: []string{
				"github.com/test/repo/pkg/prefix1/one",
			},
		},
		{
			name: "github.com/test/repo/pkg/prefix1/one",
			outboundEdges: []string{
				"github.com/test/repo/vendor/github.com/testvendor/prefix1",
			},
		},
		{
			name:          "github.com/test/repo/pkg/prefix2",
			outboundEdges: []string{},
		},
	}
}

func buildTestGraph(nodes []*testNode) (*depgraph.MutableDirectedGraph, error) {
	g := depgraph.NewMutableDirectedGraph(concrete.NewDirectedGraph())

	for _, n := range nodes {
		err := g.AddNode(&depgraph.Node{
			UniqueName: n.name,
			Id:         g.NewNodeID(),
		})
		if err != nil {
			return nil, err
		}
	}

	for _, n := range nodes {
		from, exists := g.NodeByName(n.name)
		if !exists {
			continue
		}

		for _, dep := range n.outboundEdges {
			to, exists := g.NodeByName(dep)
			if !exists {
				continue
			}

			g.SetEdge(concrete.Edge{
				F: from,
				T: to,
			}, 0)
		}
	}

	return g, nil
}

func TestVendorPackagesCollapsedIntoRepo(t *testing.T) {
	g, err := buildTestGraph(getVendorNodes())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedRepoNodeCount := 0
	for _, n := range g.Nodes() {
		node, ok := n.(*depgraph.Node)
		if !ok {
			t.Fatalf("expected node to be of type *depgraph.Node")
		}

		if strings.Contains(node.UniqueName, "/vendor/") {
			continue
		}

		expectedRepoNodeCount++
	}

	vendorRoots := []string{
		"github.com/test/repo/vendor/github.com/testvendor/prefix1",
		"github.com/test/repo/vendor/github.com/testvendor/prefix2",
		"github.com/test/repo/vendor/github.com/google/glog",
		"github.com/test/repo/vendor/github.com/docker/docker-test-util",
	}

	filteredGraph, err := FilterPackages(g, vendorRoots)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	actualRepoNodeCount := 0
	actualVendorNodeCount := 0
	for _, n := range filteredGraph.Nodes() {
		node, ok := n.(*depgraph.Node)
		if !ok {
			t.Fatalf("expected node to be of type *depgraph.Node")
		}

		if strings.Contains(node.UniqueName, "/vendor/") {
			actualVendorNodeCount++
			continue
		}

		actualRepoNodeCount++
	}

	if actualVendorNodeCount != len(vendorRoots) {
		t.Fatalf("expected filtered graph to have been reduced to %v vendor nodes, but saw %v", len(vendorRoots), actualVendorNodeCount)
	}
	if expectedRepoNodeCount != actualRepoNodeCount {
		t.Fatalf("expected reduced graph to have original amount of non-vendor nodes (%v), but saw %v", expectedRepoNodeCount, actualRepoNodeCount)
	}

	// ensure all vendor roots are in the new graph
	for _, n := range filteredGraph.Nodes() {
		node, ok := n.(*depgraph.Node)
		if !ok {
			t.Fatal("expected node to be of type *depgraph.Node")
		}

		seen := false
		for _, root := range vendorRoots {
			if node.UniqueName == root {
				seen = true
				break
			}
		}

		if !seen {
			t.Fatalf("expected node with name %q to exist in the known vendor roots set %v", node.UniqueName, vendorRoots)
		}
	}

	expectedOutgoingEdges := map[string][]string{
		"github.com/test/repo/vendor/github.com/docker/docker-test-util": {
			"github.com/test/repo/vendor/github.com/google/glog",
		},
		"github.com/test/repo/vendor/github.com/testvendor/prefix1": {
			"github.com/test/repo/vendor/github.com/testvendor/prefix2",
		},
	}

	// ensure expected edges exist between nodes
	for _, n := range filteredGraph.Nodes() {
		node, ok := n.(*depgraph.Node)
		if !ok {
			continue
		}

		expectedNodes, exists := expectedOutgoingEdges[node.UniqueName]
		if !exists {
			continue
		}

		actualNodes := filteredGraph.From(n)
		if len(expectedNodes) != len(actualNodes) {
			t.Fatalf("expected node with name %q to have %v outward edges, but saw %v\n", node.UniqueName, len(expectedNodes), len(actualNodes))
		}

		for idx := range expectedNodes {
			actual, ok := actualNodes[idx].(*depgraph.Node)
			if !ok {
				t.Fatal("expected node to be of type *depgraph.Node")
			}

			if expectedNodes[idx] != actual.UniqueName {
				t.Fatalf("expected outgoing edge for node with name %q to point towards node with name %q, saw instead a node with name %q", node.UniqueName, expectedNodes[idx], actual.UniqueName)
			}
		}
	}

}

func TestCollapsedGraphPreservesNonVendorNodes(t *testing.T) {

	// build full list of vendored / repo packages
	g, err := buildTestGraph(append(getVendorNodes(), getNonVendorNodes()...))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedRepoNodeCount := 0
	for _, n := range g.Nodes() {
		node, ok := n.(*depgraph.Node)
		if !ok {
			t.Fatalf("expected node to be of type *depgraph.Node")
		}

		if strings.Contains(node.UniqueName, "/vendor/") {
			continue
		}

		expectedRepoNodeCount++
	}

	vendorRoots := []string{
		"github.com/test/repo/vendor/github.com/testvendor/prefix1",
		"github.com/test/repo/vendor/github.com/testvendor/prefix2",
		"github.com/test/repo/vendor/github.com/google/glog",
		"github.com/test/repo/vendor/github.com/docker/docker-test-util",
	}

	filteredGraph, err := FilterPackages(g, vendorRoots)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	actualRepoNodeCount := 0
	for _, n := range g.Nodes() {
		node, ok := n.(*depgraph.Node)
		if !ok {
			t.Fatalf("expected node to be of type *depgraph.Node")
		}

		if strings.Contains(node.UniqueName, "/vendor/") {
			continue
		}

		actualRepoNodeCount++
	}

	// reduced graph should still have same amount of non-vendor nodes
	if expectedRepoNodeCount != actualRepoNodeCount {
		t.Fatalf("expected reduced graph to contain %v nodes, but saw %v", expectedRepoNodeCount, actualRepoNodeCount)
	}

	expectedOutgoingEdges := map[string][]string{
		"github.com/test/repo/pkg/prefix1": {
			"github.com/test/repo/pkg/prefix1/one",
		},
		"github.com/test/repo/pkg/prefix1/one": {
			"github.com/test/repo/vendor/github.com/testvendor/prefix1",
		},
	}

	// ensure edges between non-vendor nodes remain intact
	for _, n := range filteredGraph.Nodes() {
		node, ok := n.(*depgraph.Node)
		if !ok {
			t.Fatalf("expected node to be of type *depgraph.Node")
		}

		expectedEdges, exists := expectedOutgoingEdges[node.UniqueName]
		if !exists {
			continue
		}

		actualEdges := filteredGraph.From(n)
		if len(expectedEdges) != len(actualEdges) {
			t.Fatalf("expeced node with name %q to contain %v outgoing edges, but saw %v", node.UniqueName, len(expectedEdges), len(actualEdges))
		}

		for _, expected := range expectedEdges {
			seen := false
			for _, n := range actualEdges {
				actual, ok := n.(*depgraph.Node)
				if !ok {
					t.Fatalf("expected node to be of type *depgraph.Node")
				}

				if expected == actual.UniqueName {
					seen = true
				}
			}

			if !seen {
				t.Fatalf("expected edge from %q to %q to exist in reduced graph", node.UniqueName, expected)
			}
		}
	}
}