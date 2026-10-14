// Package config reads and validates godebt.yaml, the configuration of spec 003.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/JacobJNilsson/godebt/internal/contract"
)

// File is the content of one godebt.yaml.
type File struct {
	Exclude  []string `yaml:"exclude"`
	Classify Classify `yaml:"classify"`
	Policy   Policy   `yaml:"policy"`
}

// Classify holds the patterns that move files into a source set.
type Classify struct {
	Test []string `yaml:"test"`
}

// Policy gates a run and never changes how the index is computed.
type Policy struct {
	MaxIndex   *int              `yaml:"maxIndex"`
	Regression *Regression       `yaml:"regression"`
	Budgets    map[string]Budget `yaml:"budgets"`
	FailOnNew  []string          `yaml:"failOnNew"`
}

// Regression limits the rise of the index against a baseline.
type Regression struct {
	MaxIncrease        *int     `yaml:"maxIncrease"`
	MaxIncreasePercent *float64 `yaml:"maxIncreasePercent"`
}

// Budget is the highest value that one metric may have.
type Budget struct {
	Max *float64 `yaml:"max"`
}

// Default returns the path of the configuration file that an audit of root reads when the user names none.
func Default(root string) string {
	return filepath.Join(root, "godebt.yaml")
}

// Load reads and validates the configuration file at path.
// A missing file is an error that wraps fs.ErrNotExist.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// LoadDefault reads the configuration file of root, and returns nil and no error when root has none.
func LoadDefault(root string) (*File, error) {
	f, err := Load(Default(root))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return f, err
}

// Contract returns the part of the configuration that changes measurement and that the digest covers.
func (f *File) Contract() contract.Config {
	if f == nil {
		return contract.Config{}
	}
	return contract.Config{Exclude: f.Exclude, TestPatterns: f.Classify.Test}
}

func parse(data []byte) (*File, error) {
	var doc yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return &File{}, nil
		}
		return nil, err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("the file holds more than one YAML document")
	}
	// The decoder names an unknown key only by its line, so a walk of the node tree finds its full path first.
	if err := checkKeys(&doc, schema, ""); err != nil {
		return nil, err
	}
	if err := checkWholeNumbers(&doc); err != nil {
		return nil, err
	}
	f := &File{}
	strict := yaml.NewDecoder(bytes.NewReader(data))
	strict.KnownFields(true)
	if err := strict.Decode(f); err != nil {
		return nil, err
	}
	// The decoder leaves a null regression nil, but the user declared the check.
	if lookup(&doc, "policy", "regression") != nil && f.Policy.Regression == nil {
		f.Policy.Regression = &Regression{}
	}
	if err := f.validate(); err != nil {
		return nil, err
	}
	return f, nil
}
