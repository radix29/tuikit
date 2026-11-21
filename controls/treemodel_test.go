package controls

import (
	"slices"
	"testing"
)

type tmNode struct {
	TreeItem[*tmNode]
	name string
}

// tmTree is a model over tmNodes whose Load only records the request; a test
// delivers children with deliver.
type tmTree struct {
	m         *TreeModel[*tmNode]
	view      *TreeView
	loads     []*tmNode
	selects   []*tmNode
	reselects [][2]*tmNode
	released  []*tmNode
	retired   []*tmNode
}

func newTMTree() *tmTree {
	t := &tmTree{view: NewTreeView()}
	t.view.SetBounds(0, 0, 40, 30)
	t.m = NewTreeModel[*tmNode](t.view)
	t.m.Row = func(n *tmNode, _ int) TreeNode { return TreeNode{Label: n.name, HasKids: true} }
	t.m.Load = func(n *tmNode) { t.loads = append(t.loads, n) }
	t.m.Same = func(a, b *tmNode) bool { return a.name == b.name }
	t.m.OnSelect = func(n *tmNode) { t.selects = append(t.selects, n) }
	t.m.OnReselect = func(from, to *tmNode) { t.reselects = append(t.reselects, [2]*tmNode{from, to}) }
	t.m.OnRetire = func(n *tmNode) { t.retired = append(t.retired, n) }
	t.m.OnRelease = func(ns []*tmNode) { t.released = append(t.released, ns...) }
	return t
}

func tmNodes(names ...string) []*tmNode {
	out := make([]*tmNode, len(names))
	for i, s := range names {
		out[i] = &tmNode{name: s}
	}
	return out
}

func (t *tmTree) deliver(n *tmNode, names ...string) []*tmNode {
	kids := tmNodes(names...)
	if t.m.SetChildren(n, kids) {
		t.m.Rebuild()
	}
	return kids
}

func (t *tmTree) labels() []string {
	var out []string
	for _, n := range t.view.nodes {
		out = append(out, n.Label)
	}
	return out
}

func (t *tmTree) selected(tb *testing.T) *tmNode {
	tb.Helper()
	n, ok := t.m.Selected()
	if !ok {
		tb.Fatalf("no node selected (row %+v)", t.view.SelectedNode())
	}
	return n
}

// root returns a tree with root "srv" expanded and loaded with a, b, c.
func (t *tmTree) root() (*tmNode, []*tmNode) {
	r := &tmNode{name: "srv"}
	t.m.AddRoot(r)
	t.m.Expand(r)
	return r, t.deliver(r, "a", "b", "c")
}

func TestTreeModelExpandShowsPlaceholderThenLoads(t *testing.T) {
	tr := newTMTree()
	r := &tmNode{name: "srv"}
	tr.m.AddRoot(r)
	if got := tr.selects; len(got) != 1 || got[0] != r {
		t.Fatalf("AddRoot selects = %v, want the root reported", got)
	}
	tr.m.Expand(r)
	if got, want := tr.labels(), []string{"srv", Strings.TreeLoading}; !slices.Equal(got, want) {
		t.Fatalf("rows while loading = %q, want %q", got, want)
	}
	if len(tr.loads) != 1 || tr.loads[0] != r {
		t.Fatalf("loads = %v, want one for the root", tr.loads)
	}
	tr.deliver(r, "a", "b")
	if got, want := tr.labels(), []string{"srv", "a", "b"}; !slices.Equal(got, want) {
		t.Fatalf("rows after load = %q, want %q", got, want)
	}
	// Collapse keeps the children; a re-expand shows them with no new load.
	tr.m.Collapse(r)
	tr.m.Expand(r)
	if len(tr.loads) != 1 {
		t.Errorf("re-expanding a loaded node loaded again (%d loads)", len(tr.loads))
	}
	if got, want := tr.labels(), []string{"srv", "a", "b"}; !slices.Equal(got, want) {
		t.Errorf("rows after re-expand = %q, want %q", got, want)
	}
}

// TestTreeModelPlaceholderIDIsStable: a selection on a "Loading..." row stays
// there across a rebuild that adds rows above it.
func TestTreeModelPlaceholderIDIsStable(t *testing.T) {
	tr := newTMTree()
	_, kids := tr.root()
	a, b := kids[0], kids[1]
	tr.m.Expand(b)
	tr.m.Select(b)
	tr.view.sel++ // onto b's placeholder row
	if o, _ := tr.m.SelectedOwner(); o != b {
		t.Fatalf("setup: selection owner = %v, want b's placeholder", o)
	}
	id := b.PlaceholderID()
	tr.m.Expand(a) // a's placeholder appears above
	tr.deliver(a, "a1", "a2")
	if b.PlaceholderID() != id {
		t.Errorf("placeholder ID changed %d → %d", id, b.PlaceholderID())
	}
	tn := tr.view.SelectedNode()
	if ph, ok := tn.Tag.(TreePlaceholder[*tmNode]); !ok || ph.Owner != b {
		t.Errorf("selection moved off b's Loading row to %+v", tn)
	}
	if _, ok := tr.m.Selected(); ok {
		t.Error("Selected answered for a placeholder row")
	}
}

// TestTreeModelReloadReselectsRecreatedNode: a reload replaces the children;
// the selection follows to the new node for the same thing and the old ones
// are released by the rebuild.
func TestTreeModelReloadReselectsRecreatedNode(t *testing.T) {
	tr := newTMTree()
	r, kids := tr.root()
	tr.m.Select(kids[1])
	base := tr.m.Len()
	tr.m.Reload(r)
	if !kids[1].Retired() || len(tr.retired) != 3 {
		t.Fatalf("Reload retired %v, want a, b, c", tr.retired)
	}
	if len(tr.loads) != 2 || tr.loads[1] != r {
		t.Fatalf("Reload of an expanded node didn't load it: %v", tr.loads)
	}
	if n, ok := tr.m.ByID(kids[1].ID()); !ok || n != kids[1] {
		t.Error("a retired node stopped answering before the rebuild released it")
	}
	if tr.m.SetChildren(kids[1], tmNodes("x")) {
		t.Error("SetChildren accepted children for a retired node")
	}
	fresh := tr.deliver(r, "a", "b", "c")
	if got := tr.selected(t); got != fresh[1] {
		t.Errorf("selection after reload = %q (%p), want the new b", got.name, got)
	}
	if got := tr.reselects; len(got) != 1 || got[0][0] != kids[1] || got[0][1] != fresh[1] {
		t.Errorf("reselects = %v, want old b → new b", got)
	}
	if tr.m.Len() != base {
		t.Errorf("Len after reload = %d, want %d: replaced nodes still registered", tr.m.Len(), base)
	}
	if len(tr.released) != 3 {
		t.Errorf("released %d nodes, want 3", len(tr.released))
	}
}

// TestTreeModelReselectFallsToParent: a selected node gone with nothing Same
// in its place moves to its parent; a parent the reload re-created is resolved
// first.
func TestTreeModelReselectFallsToParent(t *testing.T) {
	tr := newTMTree()
	r, kids := tr.root()
	b := kids[1]
	tr.m.Expand(b)
	b1 := tr.deliver(b, "b1")[0]
	tr.m.Select(b1)
	tr.m.Reload(r) // b and b1 retired
	fresh := tr.deliver(r, "a", "b", "c")
	if got := tr.selected(t); got != fresh[1] {
		t.Errorf("selection after reload above = %q, want the re-created parent b", got.name)
	}
	tr.m.Select(fresh[2])
	tr.m.Reload(r)
	tr.deliver(r, "a")
	if got := tr.selected(t); got != r {
		t.Errorf("selection on a removed node = %q, want the parent", got.name)
	}
	tr.m.Expand(fresh[0]) // retired: no load
	if tr.loads[len(tr.loads)-1] == fresh[0] {
		t.Error("Expand loaded a retired node")
	}
}

func TestTreeModelCollapseMovesSelectionToNode(t *testing.T) {
	tr := newTMTree()
	r, kids := tr.root()
	tr.m.Select(kids[2])
	tr.m.Collapse(r)
	if got := tr.selected(t); got != r {
		t.Errorf("selection after collapse = %q, want srv", got.name)
	}
}

func TestTreeModelRedirect(t *testing.T) {
	tr := newTMTree()
	r, kids := tr.root()
	tr.m.Select(kids[0])
	tr.m.Redirect = func(prev *tmNode, shown map[*tmNode]bool) (*tmNode, bool) {
		for n := range shown {
			if prev.name == "a" && n.name == "z" {
				return n, true
			}
		}
		return nil, false
	}
	tr.m.Reload(r)
	fresh := tr.deliver(r, "b", "c", "z")
	if got := tr.selected(t); got != fresh[2] {
		t.Errorf("selection = %q, want the redirect target z", got.name)
	}
}

func TestTreeModelRemoveRoot(t *testing.T) {
	tr := newTMTree()
	r, _ := tr.root()
	s := &tmNode{name: "srv2"}
	tr.m.AddRoot(s)
	tr.m.RemoveRoot(r)
	if got := tr.m.Roots(); len(got) != 1 || got[0] != s {
		t.Fatalf("roots = %v, want srv2 alone", got)
	}
	if tr.m.Len() != 1 {
		t.Errorf("Len = %d, want 1: the removed subtree is still registered", tr.m.Len())
	}
	if got, want := tr.labels(), []string{"srv2"}; !slices.Equal(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
}

// TestTreeModelRemoveRootRetiresSubtree: a removed root's nodes are retired
// (OnRetire) and released (OnRelease), and a load in flight under it that
// lands afterwards is refused rather than registered under an unreachable
// parent.
func TestTreeModelRemoveRootRetiresSubtree(t *testing.T) {
	tr := newTMTree()
	r, kids := tr.root()
	tr.m.Expand(kids[0]) // its load is now in flight
	if tr.m.Len() != 4 {
		t.Fatalf("Len before remove = %d, want 4", tr.m.Len())
	}
	tr.m.RemoveRoot(r)
	all := []*tmNode{r, kids[0], kids[1], kids[2]}
	for _, n := range all {
		if !n.Retired() {
			t.Errorf("%s not retired", n.name)
		}
		if !slices.Contains(tr.retired, n) {
			t.Errorf("OnRetire missed %s", n.name)
		}
		if !slices.Contains(tr.released, n) {
			t.Errorf("OnRelease missed %s", n.name)
		}
	}
	if tr.m.Len() != 0 {
		t.Errorf("Len after remove = %d, want 0", tr.m.Len())
	}
	if tr.m.SetChildren(kids[0], tmNodes("x", "y")) {
		t.Error("late SetChildren under the removed root was accepted")
	}
	tr.m.Rebuild()
	if tr.m.Len() != 0 {
		t.Errorf("Len after the late delivery = %d, want 0", tr.m.Len())
	}
	if len(tr.labels()) != 0 {
		t.Errorf("rows = %q, want none", tr.labels())
	}
}

// TestTreeModelRemoveRootKeepsOthers: removing a root that isn't the last
// leaves the others in order and doesn't shift a Roots() slice taken before
// the call.
func TestTreeModelRemoveRootKeepsOthers(t *testing.T) {
	tr := newTMTree()
	rs := tmNodes("r1", "r2", "r3")
	for _, r := range rs {
		tr.m.AddRoot(r)
	}
	held := tr.m.Roots()
	tr.m.RemoveRoot(rs[0])
	if got := tr.m.Roots(); !slices.Equal(got, rs[1:]) {
		t.Errorf("roots = %v, want r2 r3", got)
	}
	if !slices.Equal(held, rs) {
		t.Errorf("held Roots() slice changed to %v", held)
	}
	if got, want := tr.labels(), []string{"r2", "r3"}; !slices.Equal(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
	if rs[1].Retired() || rs[2].Retired() {
		t.Error("a surviving root was retired")
	}
}

// TestTreeModelRemoveSelectedRootClamps: a selection inside the removed root
// goes to the row the TreeView's clamp leaves selected, reported as a
// reselect.
func TestTreeModelRemoveSelectedRootClamps(t *testing.T) {
	tr := newTMTree()
	r, kids := tr.root()
	s := &tmNode{name: "srv2"}
	tr.m.AddRoot(s)
	tr.m.Select(kids[1])
	tr.m.RemoveRoot(r)
	if got := tr.selected(t); got != s {
		t.Errorf("selection = %q, want srv2", got.name)
	}
}

// TestTreeModelRetireKeepsChildren: Retire takes one node out and leaves the
// nodes below it, which the host re-parented, live.
func TestTreeModelRetireKeepsChildren(t *testing.T) {
	tr := newTMTree()
	r, kids := tr.root()
	g := &tmNode{TreeItem: TreeItem[*tmNode]{Parent: r, Loaded: true, Children: kids[:2]}, name: "group"}
	tr.m.Register(g)
	tr.m.Retire(g)
	tr.m.Rebuild()
	if kids[0].Retired() || kids[1].Retired() || !g.Retired() || g.Children != nil {
		t.Errorf("Retire: group retired=%v children=%v; kids retired %v %v", g.Retired(), g.Children, kids[0].Retired(), kids[1].Retired())
	}
	if _, ok := tr.m.ByID(g.ID()); ok {
		t.Error("the retired group still answers after the rebuild")
	}
}

func TestTreeModelSetChildrenReplacesAndRetiresOld(t *testing.T) {
	tt := newTMTree()
	root := tmNodes("r")[0]
	tt.m.AddRoot(root)
	old := tt.deliver(root, "a", "b")
	tt.deliver(root, "c", "d")
	if len(tt.retired) != 2 || !slices.Equal(tt.retired, old) {
		t.Fatalf("retired = %v, want the two replaced children", tt.retired)
	}
	if len(tt.released) != 2 {
		t.Fatalf("released = %d, want 2 after Rebuild", len(tt.released))
	}
	if got := tt.m.Len(); got != 3 {
		t.Fatalf("Len = %d, want 3", got)
	}
}

func TestTreeModelSetChildrenKeepsRedelivered(t *testing.T) {
	tt := newTMTree()
	root := tmNodes("r")[0]
	tt.m.AddRoot(root)
	kids := tt.deliver(root, "a", "b")
	tt.m.SetChildren(root, kids[:1])
	tt.m.Rebuild()
	if len(tt.retired) != 1 || tt.retired[0] != kids[1] {
		t.Fatalf("retired = %v, want only b", tt.retired)
	}
	if kids[0].Retired() {
		t.Fatal("re-delivered node was retired")
	}
}

func TestTreeModelRetireThenDropChildrenReportsOnce(t *testing.T) {
	tt := newTMTree()
	root := tmNodes("r")[0]
	tt.m.AddRoot(root)
	kids := tt.deliver(root, "a")
	tt.m.Retire(kids[0])
	tt.m.DropChildren(root)
	tt.m.Rebuild()
	if len(tt.retired) != 1 || len(tt.released) != 1 {
		t.Fatalf("retired=%d released=%d, want 1 each", len(tt.retired), len(tt.released))
	}
}
