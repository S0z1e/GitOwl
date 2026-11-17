package desktop

import (
	"sort"
	"strings"

	"gitowl/internal/workflow"
)

// maxTreeDepth is the deepest indentation of the branch trees: a name
// with more folders than that keeps the rest in its leaf's label, as
// "d/e/f".
const maxTreeDepth = 4

// refNode is a folder or a branch of a tree of refs split at "/".
type refNode struct {
	// label is what the row shows; path the full name of the folder or
	// ref.
	label, path string
	folder      bool
	local       *workflow.Branch
	remote      *workflow.RemoteBranch
	children    []*refNode
}

// refTree arranges names into folders at each "/", at most maxTreeDepth
// levels deep: folders first, each level in name order.
func refTree(names []string) []*refNode {
	root := &refNode{}
	for _, name := range names {
		parts := strings.Split(name, "/")
		if len(parts) > maxTreeDepth {
			parts = append(parts[:maxTreeDepth-1:maxTreeDepth-1], strings.Join(parts[maxTreeDepth-1:], "/"))
		}
		n := root
		for i, p := range parts[:len(parts)-1] {
			path := strings.Join(parts[:i+1], "/")
			var next *refNode
			for _, ch := range n.children {
				if ch.path == path && ch.folder {
					next = ch
				}
			}
			if next == nil {
				next = &refNode{label: p, path: path, folder: true}
				n.children = append(n.children, next)
			}
			n = next
		}
		n.children = append(n.children, &refNode{label: parts[len(parts)-1], path: name})
	}
	sortTree(root.children)
	return root.children
}

func sortTree(ns []*refNode) {
	sort.SliceStable(ns, func(i, j int) bool {
		if fi, fj := ns[i].folder, ns[j].folder; fi != fj {
			return fi
		}
		return ns[i].label < ns[j].label
	})
	for _, n := range ns {
		sortTree(n.children)
	}
}

// localTree is the tree of the local branches.
func localTree(bs []workflow.Branch) []*refNode {
	names := make([]string, len(bs))
	for i, b := range bs {
		names[i] = b.Name
	}
	nodes := refTree(names)
	by := map[string]*workflow.Branch{}
	for i := range bs {
		by[bs[i].Name] = &bs[i]
	}
	walk(nodes, func(n *refNode) {
		if !n.folder {
			n.local = by[n.path]
		}
	})
	return nodes
}

// remoteTree is the tree of the remote-tracking branches, under their
// remotes.
func remoteTree(rs []workflow.RemoteBranch) []*refNode {
	names := make([]string, len(rs))
	for i, r := range rs {
		names[i] = r.Name
	}
	nodes := refTree(names)
	by := map[string]*workflow.RemoteBranch{}
	for i := range rs {
		by[rs[i].Name] = &rs[i]
	}
	walk(nodes, func(n *refNode) {
		if !n.folder {
			n.remote = by[n.path]
		}
	})
	return nodes
}

func walk(ns []*refNode, fn func(*refNode)) {
	for _, n := range ns {
		fn(n)
		walk(n.children, fn)
	}
}
