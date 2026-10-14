package config

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/JacobJNilsson/godebt/internal/contract"
)

// keys maps each allowed key to the keys allowed under it, and a nil value allows no mapping below.
type keys map[string]keys

// anyKey stands for every key of a mapping whose keys the user chooses, such as metric ids.
const anyKey = "*"

// schema lists every key of File, with the yaml tags of its fields.
var schema = keys{
	"exclude":  nil,
	"classify": {"test": nil},
	"policy": {
		"maxIndex":   nil,
		"regression": {"maxIncrease": nil, "maxIncreasePercent": nil},
		"budgets":    {anyKey: {"max": nil}},
		"failOnNew":  nil,
	},
}

// checkKeys returns a FieldError for the first key under node that allowed does not list.
// A node of the wrong kind passes here, because the decoder reports it with its line.
func checkKeys(node *yaml.Node, allowed keys, at string) error {
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode || allowed == nil {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		below, ok := allowed[key]
		if !ok {
			below, ok = allowed[anyKey]
		}
		if !ok {
			return &contract.FieldError{Field: join(at, key), Reason: "is unknown"}
		}
		if err := checkKeys(node.Content[i+1], below, join(at, key)); err != nil {
			return err
		}
	}
	return nil
}

// wholeNumberKeys are the paths of the policy fields that take an integer.
var wholeNumberKeys = [][]string{{"policy", "maxIndex"}, {"policy", "regression", "maxIncrease"}}

// checkWholeNumbers rejects a float in an integer field, because the decoder would name the field only by its line.
func checkWholeNumbers(doc *yaml.Node) error {
	for _, path := range wholeNumberKeys {
		node := lookup(doc, path...)
		if node == nil {
			continue
		}
		// A tag can claim an integer for a fraction, so the value must also decode as one.
		var n int
		if node.Tag == "!!float" || node.Decode(&n) != nil {
			return &contract.FieldError{Field: strings.Join(path, "."), Reason: "must be a whole number"}
		}
	}
	return nil
}

// lookup returns the value node at the path of mapping keys, or nil when the path is absent.
func lookup(doc *yaml.Node, path ...string) *yaml.Node {
	node := doc
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = node.Content[0]
	}
	for _, key := range path {
		next := (*yaml.Node)(nil)
		for i := 0; node.Kind == yaml.MappingNode && i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == key {
				next = node.Content[i+1]
			}
		}
		if next == nil {
			return nil
		}
		// An alias takes the value of its anchor, so the checks must see that value.
		for next.Kind == yaml.AliasNode {
			next = next.Alias
		}
		node = next
	}
	return node
}

func join(at, key string) string {
	if at == "" {
		return key
	}
	return at + "." + key
}
