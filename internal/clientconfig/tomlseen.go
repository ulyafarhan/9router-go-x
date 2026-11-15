package clientconfig

import (
	"fmt"

	"github.com/pelletier/go-toml/v2/unstable"
)

type tomlKeyKind uint8

const (
	tomlKeyTable tomlKeyKind = iota
	tomlKeyArrayTable
	tomlKeyValueKind
)

type tomlSeenEntry struct {
	kind     tomlKeyKind
	explicit bool
	kv       bool
	children map[string]*tomlSeenEntry
}

// tomlSeenTracker validates TOML table, array-table, dotted-key, and inline-table
// uniqueness in O(n) total time during unstable.Parser traversal, replacing the
// upstream O(n²) slice-based SeenTracker in go-toml/v2.
type tomlSeenTracker struct {
	root      *tomlSeenEntry
	current   *tomlSeenEntry
	pendingKV []*tomlSeenEntry
}

func newTOMLSeenTracker() *tomlSeenTracker {
	root := &tomlSeenEntry{kind: tomlKeyTable}
	return &tomlSeenTracker{
		root:    root,
		current: root,
	}
}

func (e *tomlSeenEntry) child(name string) *tomlSeenEntry {
	if e.children == nil {
		return nil
	}
	return e.children[name]
}

func (e *tomlSeenEntry) addChild(name string, kind tomlKeyKind, explicit, kv bool) *tomlSeenEntry {
	if e.children == nil {
		e.children = make(map[string]*tomlSeenEntry, 4)
	}
	c := &tomlSeenEntry{
		kind:     kind,
		explicit: explicit,
		kv:       kv,
	}
	e.children[name] = c
	return c
}

func (s *tomlSeenTracker) sealPendingKV() {
	for _, e := range s.pendingKV {
		if e.kv {
			e.explicit = true
			e.kv = false
		}
	}
	s.pendingKV = s.pendingKV[:0]
}

func (s *tomlSeenTracker) checkExpression(node *unstable.Node) error {
	switch node.Kind {
	case unstable.Comment:
		return nil
	case unstable.KeyValue:
		return s.checkKeyValue(node)
	case unstable.Table:
		return s.checkTable(node)
	case unstable.ArrayTable:
		return s.checkArrayTable(node)
	default:
		return fmt.Errorf("unexpected top-level node kind: %s", node.Kind)
	}
}

func (s *tomlSeenTracker) descendTablePrefix(node *unstable.Node) (*tomlSeenEntry, string, error) {
	parent := s.root
	it := node.Key()
	var last string
	for it.Next() {
		name := string(it.Node().Data)
		if it.IsLast() {
			last = name
			break
		}
		entry := parent.child(name)
		if entry == nil {
			entry = parent.addChild(name, tomlKeyTable, false, false)
		} else if entry.kind == tomlKeyValueKind {
			return nil, "", fmt.Errorf("toml: expected %s to be a table, not a value", name)
		}
		parent = entry
	}
	return parent, last, nil
}

func (s *tomlSeenTracker) checkTable(node *unstable.Node) error {
	s.sealPendingKV()
	parent, k, err := s.descendTablePrefix(node)
	if err != nil {
		return err
	}
	entry := parent.child(k)
	if entry != nil {
		if entry.kind != tomlKeyTable {
			return fmt.Errorf("toml: key %s should be a table", k)
		}
		if entry.explicit {
			return fmt.Errorf("toml: table %s already exists", k)
		}
		entry.explicit = true
	} else {
		entry = parent.addChild(k, tomlKeyTable, true, false)
	}
	s.current = entry
	return nil
}

func (s *tomlSeenTracker) checkArrayTable(node *unstable.Node) error {
	s.sealPendingKV()
	parent, k, err := s.descendTablePrefix(node)
	if err != nil {
		return err
	}
	entry := parent.child(k)
	if entry == nil {
		entry = parent.addChild(k, tomlKeyArrayTable, true, false)
	} else {
		if entry.kind != tomlKeyArrayTable {
			return fmt.Errorf("toml: key %s already exists as non-array table", k)
		}
		entry.children = nil
	}
	s.current = entry
	return nil
}

func (s *tomlSeenTracker) checkKeyValue(node *unstable.Node) error {
	parent := s.current
	it := node.Key()
	for it.Next() {
		k := string(it.Node().Data)
		entry := parent.child(k)
		if entry == nil {
			entry = parent.addChild(k, tomlKeyTable, false, true)
			if !it.IsLast() {
				s.pendingKV = append(s.pendingKV, entry)
			}
		} else {
			if it.IsLast() {
				return fmt.Errorf("toml: key %s is already defined", k)
			}
			if entry.kind != tomlKeyTable {
				return fmt.Errorf("toml: expected %s to be a table", k)
			}
			if entry.explicit {
				return fmt.Errorf("toml: cannot redefine explicit table %s", k)
			}
		}
		parent = entry
	}
	parent.kind = tomlKeyValueKind
	parent.kv = false

	v := node.Value()
	switch v.Kind {
	case unstable.InlineTable:
		return checkTOMLInlineTable(v)
	case unstable.Array:
		return checkTOMLArray(v)
	}
	return nil
}

func checkTOMLInlineTable(node *unstable.Node) error {
	sub := newTOMLSeenTracker()
	it := node.Children()
	for it.Next() {
		if err := sub.checkKeyValue(it.Node()); err != nil {
			return err
		}
	}
	return nil
}

func checkTOMLArray(node *unstable.Node) error {
	it := node.Children()
	for it.Next() {
		n := it.Node()
		switch n.Kind {
		case unstable.InlineTable:
			if err := checkTOMLInlineTable(n); err != nil {
				return err
			}
		case unstable.Array:
			if err := checkTOMLArray(n); err != nil {
				return err
			}
		}
	}
	return nil
}
