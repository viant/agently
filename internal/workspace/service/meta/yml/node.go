package yml

import "gopkg.in/yaml.v3"

// Node wraps yaml.Node and provides helper methods used by decoders.
type Node yaml.Node

// Pairs iterates over mapping node key/value pairs and calls fn.
func (n *Node) Pairs(fn func(key string, valueNode *Node) error) error {
	yn := (*yaml.Node)(n)
	if yn.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(yn.Content); i += 2 {
		k := yn.Content[i]
		v := yn.Content[i+1]
		if err := fn(k.Value, (*Node)(v)); err != nil {
			return err
		}
	}
	return nil
}

// Interface decodes the node subtree into a generic interface{}.
func (n *Node) Interface() interface{} {
	var v interface{}
	_ = (*yaml.Node)(n).Decode(&v)
	return v
}
