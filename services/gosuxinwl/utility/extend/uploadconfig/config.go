// Package uploadconfig edits YAML nodes rather than matching substrings in lines.
package uploadconfig

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

var mu sync.Mutex

func put(m *yaml.Node, key string, value any) error {
	n := &yaml.Node{}
	if e := n.Encode(value); e != nil {
		return e
	}
	for i := 0; i < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			old := m.Content[i+1]
			n.HeadComment = old.HeadComment
			n.LineComment = old.LineComment
			n.FootComment = old.FootComment
			m.Content[i+1] = n
			return nil
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, n)
	return nil
}
func Save(path string, root map[string]interface{}, section string, fields map[string]interface{}) error {
	mu.Lock()
	defer mu.Unlock()
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	var doc yaml.Node
	if e = yaml.Unmarshal(b, &doc); e != nil {
		return e
	}
	if len(doc.Content) == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	m := doc.Content[0]
	if m.Kind != yaml.MappingNode {
		return errors.New("上传配置必须是YAML映射")
	}
	for key, v := range root {
		if e = put(m, key, v); e != nil {
			return e
		}
	}
	var node *yaml.Node
	for i := 0; i < len(m.Content); i += 2 {
		if m.Content[i].Value == section {
			node = m.Content[i+1]
			break
		}
	}
	if node == nil || node.Kind != yaml.MappingNode {
		node = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		if e = put(m, section, map[string]interface{}{}); e != nil {
			return e
		}
		for i := 0; i < len(m.Content); i += 2 {
			if m.Content[i].Value == section {
				m.Content[i+1] = node
				break
			}
		}
	}
	for key, v := range fields {
		if e = put(node, key, v); e != nil {
			return e
		}
	}
	b, e = yaml.Marshal(&doc)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".upload-*.yaml")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(name, path)
}
