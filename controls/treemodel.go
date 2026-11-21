package controls

import "slices"

// TreeItem is the bookkeeping a TreeModel keeps in every node of a lazily
// loaded tree. A host's node type embeds it by value, which makes a pointer to
// that type a TreeModelNode:
//
//	type node struct {
//		controls.TreeItem[*node]
//		name string
//	}
//
// The exported fields are the host's to read, and to write where the host
// rearranges loaded nodes itself (regrouping children under synthetic nodes,
// say), followed by TreeModel.Rebuild. Children the host fetched go in through
// TreeModel.SetChildren, nodes it makes itself through TreeModel.Register: a
// node the model hasn't registered has no ID, and its row can't be selected.
type TreeItem[N any] struct {
	Parent   N   // the zero N for a root
	Children []N // in display order; nil until loaded, and again after a Reload
	Expanded bool
	// Loaded reports that Children holds a load's result, possibly empty. An
	// expanded node that isn't Loaded shows a placeholder row instead.
	Loaded bool

	id TreeNodeID
	// placeholderID is the ID of this node's placeholder row, allocated once
	// and reused by every Rebuild, so a selection parked on that row stays on it
	// until the load replaces it. Re-allocated per rebuild, the ID changed under
	// the selection and it jumped to whatever row slid into place.
	placeholderID TreeNodeID
	retired       bool
}

// Item returns it; it is how a TreeModel reaches the TreeItem a host node
// embeds.
func (it *TreeItem[N]) Item() *TreeItem[N] { return it }

// ID is the node's row ID in the TreeView, 0 until registered.
func (it *TreeItem[N]) ID() TreeNodeID { return it.id }

// PlaceholderID is the ID of the node's placeholder row, 0 until one was shown.
func (it *TreeItem[N]) PlaceholderID() TreeNodeID { return it.placeholderID }

// Retired reports that a Reload (DropChildren) or Retire took the node out of
// the tree. A retired node takes no children (SetChildren refuses it) and is
// never loaded; its row may still be on screen until the next Rebuild.
func (it *TreeItem[N]) Retired() bool { return it.retired }

// TreeModelNode is a host node type a TreeModel can hold: a comparable handle,
// in practice a pointer to a struct embedding TreeItem.
type TreeModelNode[N any] interface {
	comparable
	Item() *TreeItem[N]
}

// TreePlaceholder is the TreeNode.Tag of a placeholder row: the node whose
// children it stands in for.
type TreePlaceholder[N any] struct{ Owner N }

// TreeModel is a lazily loaded tree projected into a TreeView. The host owns
// what a node means and how its children are fetched; the model owns what any
// lazy tree host would otherwise re-derive, each rule a shipped bug:
//
//   - a placeholder row under an expanded, unloaded node, with a stable ID
//     (TreeItem.placeholderID);
//   - children installed only into a live node (SetChildren), so nothing is
//     registered under a parent nobody can reach and never released;
//   - a replaced subtree retired at once and released on the next Rebuild,
//     its stale rows still clickable and answerable meanwhile (DropChildren),
//     where dropping nodes without releasing them leaked every Reload's;
//   - the selection put back after every Rebuild on the node it belongs to:
//     the same node, else the one a reload re-created for it, else its parent
//     (Rebuild). TreeView.SetNodes alone can only clamp an index, and keyboard
//     actions then went to a node nobody selected.
//
// It takes over the TreeView's OnExpand, OnCollapse and OnSelect; the host
// keeps OnActivate and OnRightClick and maps their IDs back with ByID. UI
// goroutine only, like the TreeView.
type TreeModel[N TreeModelNode[N]] struct {
	view    *TreeView
	roots   []N
	byID    map[TreeNodeID]N
	nextID  TreeNodeID
	retired []N

	// moving is set while Rebuild moves the selection itself, from the node in
	// from, so the OnSelect the TreeView fires is reported as OnReselect.
	moving bool
	from   N

	// Row describes n's row: Label, Icon, Dim and HasKids. The model fills in
	// ID, Depth, Expanded, Loaded and Tag (n). Required.
	Row func(n N, depth int) TreeNode
	// Placeholder describes the row shown under owner while its children load:
	// Label and Icon. Nil shows Strings.TreeLoading with no icon.
	Placeholder func(owner N) TreeNode
	// Load starts fetching n's children, on an expand of a node not Loaded or a
	// Reload of an expanded one. The host delivers them, on the UI goroutine,
	// through SetChildren and then Rebuild. Never called for a retired node.
	Load func(n N)
	// Same reports whether a and b stand for the same thing, how Rebuild
	// recognises the node a reload re-created for a selected node it replaced.
	// Nil: nothing matches, and such a selection goes to the parent.
	Same func(a, b N) bool
	// Redirect, when set, is asked first where a selection on prev goes after a
	// Rebuild, shown being the nodes now on screen; ok false leaves the decision
	// to the rules above. It is the seam for a host that knows better, such as
	// a rename that changed what Same matches on. A target equal to prev is
	// ignored.
	Redirect func(prev N, shown map[N]bool) (target N, ok bool)
	// OnSelect reports a selection the user made (or Select made). A
	// placeholder row's selection is not reported.
	OnSelect func(n N)
	// OnReselect reports a selection Rebuild moved itself, from a node it no
	// longer shows. Nil reports it through OnSelect.
	OnReselect func(from, to N)
	// OnRetire is called for each node as it is retired: the place to cancel
	// its in-flight load.
	OnRetire func(n N)
	// OnRelease is called by Rebuild with the retired nodes it just released,
	// whose rows are no longer drawn: the place to drop host caches about them.
	OnRelease func(ns []N)
}

// NewTreeModel returns an empty model driving view. It sets view's OnExpand,
// OnCollapse and OnSelect.
func NewTreeModel[N TreeModelNode[N]](view *TreeView) *TreeModel[N] {
	m := &TreeModel[N]{view: view, byID: make(map[TreeNodeID]N)}
	view.OnExpand = func(id TreeNodeID) {
		if n, ok := m.byID[id]; ok {
			m.Expand(n)
		}
	}
	view.OnCollapse = func(id TreeNodeID) {
		if n, ok := m.byID[id]; ok {
			m.Collapse(n)
		}
	}
	view.OnSelect = m.handleSelect
	return m
}

func (m *TreeModel[N]) handleSelect(id TreeNodeID) {
	n, ok := m.byID[id]
	switch {
	case !ok:
	case m.moving && m.OnReselect != nil:
		m.OnReselect(m.from, n)
	case m.OnSelect != nil:
		m.OnSelect(n)
	}
}

func (m *TreeModel[N]) allocID() TreeNodeID {
	m.nextID++
	return m.nextID
}

// Register gives n an ID and makes ByID answer for it: for a node the host
// makes itself rather than delivers through SetChildren (a synthetic grouping
// node, say). AddRoot registers its node.
func (m *TreeModel[N]) Register(n N) {
	it := n.Item()
	it.id = m.allocID()
	m.byID[it.id] = n
}

// Roots returns the root nodes. The slice is the model's; don't modify it.
func (m *TreeModel[N]) Roots() []N { return m.roots }

// ByID returns the node with row ID id. A retired node answers until the
// Rebuild that releases it; a placeholder row's ID never does.
func (m *TreeModel[N]) ByID(id TreeNodeID) (N, bool) {
	n, ok := m.byID[id]
	return n, ok
}

// AddRoot appends n as a root, rebuilds and selects it (reported through
// OnSelect).
func (m *TreeModel[N]) AddRoot(n N) {
	m.Register(n)
	m.roots = append(m.roots, n)
	m.Rebuild()
	m.Select(n)
}

// RemoveRoot removes root n and its subtree, then rebuilds. Every node in it
// is retired (OnRetire, so its in-flight load is cancelled and a late
// SetChildren refused), and the Rebuild releases them (OnRelease). A selection
// inside it goes to whichever row the TreeView's clamp leaves selected.
func (m *TreeModel[N]) RemoveRoot(n N) {
	i := slices.Index(m.roots, n)
	if i < 0 {
		return
	}
	var walk func(N)
	walk = func(c N) {
		m.retire(c)
		for _, gc := range c.Item().Children {
			walk(gc)
		}
	}
	walk(n)
	// A fresh slice: deleting in place (append or slices.Delete alike) shifted
	// the elements of a slice a caller got from Roots() before the call.
	m.roots = slices.Delete(slices.Clone(m.roots), i, i+1)
	m.Rebuild()
}

// Selected returns the node of the selected row; false with no selection or
// on a placeholder row (see SelectedOwner).
func (m *TreeModel[N]) Selected() (N, bool) {
	var zero N
	tn := m.view.SelectedNode()
	if tn == nil {
		return zero, false
	}
	n, ok := m.byID[tn.ID]
	return n, ok
}

// SelectedOwner is the node the selection stands for: the selected node, or
// the owner of a selected placeholder row.
func (m *TreeModel[N]) SelectedOwner() (N, bool) {
	var zero N
	tn := m.view.SelectedNode()
	if tn == nil {
		return zero, false
	}
	switch tag := tn.Tag.(type) {
	case N:
		return tag, true
	case TreePlaceholder[N]:
		return tag.Owner, true
	}
	return zero, false
}

// Select selects n's row, if shown, and reports it through OnSelect like a
// click.
func (m *TreeModel[N]) Select(n N) { m.view.SelectID(n.Item().id) }

// NodeAt returns the node drawn at screen position (mx, my); false off any
// node row (a placeholder row, border, scrollbar, blank space).
func (m *TreeModel[N]) NodeAt(mx, my int) (N, bool) {
	var zero N
	id, ok := m.view.NodeIDAt(mx, my)
	if !ok {
		return zero, false
	}
	n, ok := m.byID[id]
	return n, ok
}

// Expand expands n and rebuilds, which shows the placeholder at once when n
// isn't Loaded, then starts its Load. A Loaded node redisplays the children it
// kept: Collapse never drops them.
func (m *TreeModel[N]) Expand(n N) {
	it := n.Item()
	it.Expanded = true
	m.Rebuild()
	if !it.Loaded && !it.retired && m.Load != nil {
		m.Load(n)
	}
}

// Collapse collapses n and rebuilds; a selection below it goes to n.
func (m *TreeModel[N]) Collapse(n N) {
	n.Item().Expanded = false
	m.Rebuild()
}

// SetChildren installs n's loaded children, registering each, and marks n
// Loaded; the caller rebuilds. It replaces, it does not add: any children n
// already has are dropped with their subtrees (DropChildren: OnRetire, then
// OnRelease at the next Rebuild), except nodes also in children, which a host
// may re-deliver. It reports false, installing nothing, for a retired n: its
// children would be registered under a parent no longer in the tree, and never
// released.
func (m *TreeModel[N]) SetChildren(n N, children []N) bool {
	it := n.Item()
	if it.retired {
		return false
	}
	if len(it.Children) > 0 {
		m.dropExcept(n, children)
	}
	for _, c := range children {
		m.Register(c)
		c.Item().Parent = n
	}
	it.Children = children
	it.Loaded = true
	return true
}

// Retire takes n alone out of the tree: it is retired, loses its Children
// (which are not retired: the host re-parented them, or will) and is released
// by the next Rebuild.
func (m *TreeModel[N]) Retire(n N) {
	m.retire(n)
	n.Item().Children = nil
}

func (m *TreeModel[N]) retire(n N) {
	if n.Item().retired {
		return
	}
	n.Item().retired = true
	if m.OnRetire != nil {
		m.OnRetire(n)
	}
	m.retired = append(m.retired, n)
}

// DropChildren detaches n's subtree from n and retires every node in it, each
// released by the next Rebuild, when its row leaves the screen. Loaded is left
// alone. A retired node keeps its Parent: Rebuild walks it to find where a
// selection on a replaced node belongs.
func (m *TreeModel[N]) DropChildren(n N) {
	var walk func(N)
	walk = func(c N) {
		m.retire(c)
		for _, gc := range c.Item().Children {
			walk(gc)
		}
	}
	for _, c := range n.Item().Children {
		walk(c)
	}
	n.Item().Children = nil
}

// dropExcept is DropChildren for SetChildren: the children of n that are not in
// keep are retired with their subtrees; the kept ones stay live.
func (m *TreeModel[N]) dropExcept(n N, keep []N) {
	var walk func(N)
	walk = func(c N) {
		m.retire(c)
		for _, gc := range c.Item().Children {
			walk(gc)
		}
	}
	for _, c := range n.Item().Children {
		if !slices.Contains(keep, c) {
			walk(c)
		}
	}
}

// Reload replaces n's children: they are dropped (DropChildren), n is no
// longer Loaded and, if expanded, its Load starts. The caller rebuilds, or the
// delivery of the load does. A retired n is left alone.
func (m *TreeModel[N]) Reload(n N) {
	it := n.Item()
	if it.retired {
		return
	}
	m.DropChildren(n)
	it.Loaded = false
	if it.Expanded && m.Load != nil {
		m.Load(n)
	}
}

// Rebuild projects the tree into the TreeView, releases the nodes retired
// since the last one, and puts the selection back on the node it belongs to.
//
// TreeView.SetNodes carries a surviving selection across by ID, but a reload
// makes new nodes with new IDs, and a removal or collapse takes the selected
// one off outright; SetNodes can then only clamp an index. So a selection on a
// node no longer shown moves (OnReselect) to Redirect's answer, else the shown
// node Same as it under its resolved parent, else that parent. A parent a
// higher reload re-created is resolved the same way first. With the whole root
// gone, the clamped row is reported.
func (m *TreeModel[N]) Rebuild() {
	var prev N
	has, onPlaceholder := false, false
	if tn := m.view.SelectedNode(); tn != nil {
		switch tag := tn.Tag.(type) {
		case N:
			prev, has = tag, true
		case TreePlaceholder[N]:
			prev, has, onPlaceholder = tag.Owner, true, true
		}
	}
	flat := make([]TreeNode, 0, 32)
	for _, r := range m.roots {
		flat = m.flatten(flat, r, 0)
	}
	m.view.SetNodes(flat)
	m.release()
	if has {
		m.reselect(prev, onPlaceholder, flat)
	}
}

func (m *TreeModel[N]) release() {
	if len(m.retired) == 0 {
		return
	}
	for _, n := range m.retired {
		delete(m.byID, n.Item().id)
	}
	rel := m.retired
	m.retired = nil
	if m.OnRelease != nil {
		m.OnRelease(rel)
	}
}

func (m *TreeModel[N]) flatten(flat []TreeNode, n N, depth int) []TreeNode {
	it := n.Item()
	row := m.Row(n, depth)
	row.ID, row.Depth, row.Expanded, row.Loaded, row.Tag = it.id, depth, it.Expanded, it.Loaded, n
	flat = append(flat, row)
	if !it.Expanded {
		return flat
	}
	if !it.Loaded {
		if it.placeholderID == 0 {
			it.placeholderID = m.allocID()
		}
		ph := TreeNode{Label: Strings.TreeLoading}
		if m.Placeholder != nil {
			ph = m.Placeholder(n)
		}
		flat = append(flat, TreeNode{
			ID: it.placeholderID, Label: ph.Label, Icon: ph.Icon, Dim: ph.Dim,
			Depth: depth + 1, Tag: TreePlaceholder[N]{Owner: n},
		})
		return flat
	}
	for _, c := range it.Children {
		flat = m.flatten(flat, c, depth+1)
	}
	return flat
}

func (m *TreeModel[N]) reselect(prev N, onPlaceholder bool, flat []TreeNode) {
	shown := make(map[N]bool, len(flat))
	placeholderShown := false
	for _, tn := range flat {
		switch tag := tn.Tag.(type) {
		case N:
			shown[tag] = true
		case TreePlaceholder[N]:
			placeholderShown = placeholderShown || tag.Owner == prev
		}
	}
	if m.Redirect != nil {
		if t, ok := m.Redirect(prev, shown); ok && t != prev {
			m.move(prev, t)
			return
		}
	}
	if onPlaceholder && placeholderShown || !onPlaceholder && shown[prev] {
		return // still on screen, and SetNodes kept it selected
	}
	target, ok := m.survivor(prev, shown)
	if !ok {
		// prev's whole root is gone: nothing is related to it, so report whichever
		// node SetNodes' clamp left selected.
		if target, ok = m.Selected(); !ok {
			return
		}
	}
	m.move(prev, target)
}

func (m *TreeModel[N]) move(from, to N) {
	m.moving, m.from = true, from
	defer func() {
		var zero N
		m.moving, m.from = false, zero
	}()
	m.view.SelectID(to.Item().id)
}

// survivor is where a selection on n belongs once n is no longer shown: a
// shown node Same as n under n's parent (a reload re-created it), else the
// parent, itself resolved the same way first, so a parent a higher reload
// re-created is found too (it comes back collapsed, and is then the answer).
// False when n's root is gone.
func (m *TreeModel[N]) survivor(n N, shown map[N]bool) (N, bool) {
	var zero N
	if shown[n] {
		return n, true
	}
	if n.Item().Parent == zero {
		return zero, false
	}
	p, ok := m.survivor(n.Item().Parent, shown)
	if !ok {
		return zero, false
	}
	for _, c := range p.Item().Children {
		if shown[c] && m.Same != nil && m.Same(c, n) {
			return c, true
		}
	}
	return p, true
}

// Len is the number of nodes ByID answers for: every registered node, a
// retired one until the Rebuild that releases it. A host's leak test watches
// it across a Reload and Rebuild.
func (m *TreeModel[N]) Len() int { return len(m.byID) }
