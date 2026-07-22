package main

import (
	"encoding/json"
	"testing"

	"gitlab.com/pml/network-digital-twin/internal/schema"
)

// newTestRegistry 创建一个带测试数据的 SchemaRegistry。
func newTestRegistry(t *testing.T) schema.SchemaRegistry {
	t.Helper()
	reg := schema.NewSchemaRegistry()
	// 使用项目 ontology 目录
	if err := reg.Load("../../ontology"); err != nil {
		t.Fatalf("load ontology: %v", err)
	}
	return reg
}

func TestCollectAllLabels(t *testing.T) {
	reg := newTestRegistry(t)
	labels := collectAllLabels(reg)

	if len(labels) == 0 {
		t.Fatal("collectAllLabels returned empty slice")
	}

	// 验证去重：labels 中不应有重复项
	seen := make(map[string]bool)
	for _, l := range labels {
		if seen[l] {
			t.Errorf("duplicate label: %s", l)
		}
		seen[l] = true
	}

	// 验证包含已知 Label
	ets := reg.ListEntityTypes()
	if len(ets) == 0 {
		t.Fatal("no entity types loaded")
	}
	// 每个 EntityType 至少有一个 label（自身名称）
	for _, et := range ets {
		found := false
		for _, l := range labels {
			if l == et.Metadata.Name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("label for %s not found in collectAllLabels result", et.Metadata.Name)
		}
	}
}

func TestCollectAllLabels_EmptyRegistry(t *testing.T) {
	reg := schema.NewSchemaRegistry() // 空 registry，未 Load
	labels := collectAllLabels(reg)
	if len(labels) != 0 {
		t.Errorf("expected empty labels, got %d", len(labels))
	}
}

func TestComputeSchemaVersion(t *testing.T) {
	reg := newTestRegistry(t)
	v1 := computeSchemaVersion(reg)
	if v1 == 0 {
		t.Error("computeSchemaVersion returned 0")
	}

	// 同一个 registry 多次调用应返回相同值
	v2 := computeSchemaVersion(reg)
	if v1 != v2 {
		t.Errorf("computeSchemaVersion not deterministic: %d != %d", v1, v2)
	}
}

func TestComputeSchemaVersion_Deterministic(t *testing.T) {
	// 不同 registry 实例加载相同 ontology 应产生相同版本
	reg1 := newTestRegistry(t)
	reg2 := newTestRegistry(t)
	if computeSchemaVersion(reg1) != computeSchemaVersion(reg2) {
		t.Error("same ontology should produce same schema version")
	}
}

func TestMarshalEntityTypes(t *testing.T) {
	reg := newTestRegistry(t)
	data := marshalEntityTypes(reg)
	if len(data) == 0 {
		t.Fatal("marshalEntityTypes returned empty")
	}

	// 验证是合法 JSON
	var items []struct {
		Name   string   `json:"name"`
		Labels []string `json:"labels,omitempty"`
	}
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(items) == 0 {
		t.Error("no entity types marshaled")
	}

	// 验证排序（按名称升序）
	for i := 1; i < len(items); i++ {
		if items[i].Name < items[i-1].Name {
			t.Errorf("items not sorted: %s < %s", items[i].Name, items[i-1].Name)
		}
	}
}

func TestMarshalEntityTypes_EmptyRegistry(t *testing.T) {
	reg := schema.NewSchemaRegistry()
	data := marshalEntityTypes(reg)
	if string(data) != "[]" {
		t.Errorf("expected empty array, got %s", data)
	}
}

func TestMarshalRelationTypes(t *testing.T) {
	reg := newTestRegistry(t)
	data := marshalRelationTypes(reg)
	if len(data) == 0 {
		t.Fatal("marshalRelationTypes returned empty")
	}

	// 验证是合法 JSON
	var items []struct {
		Name   string   `json:"name"`
		Source []string `json:"source"`
		Target []string `json:"target"`
	}
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// ontology 中应有至少一个 RelationType
	rts := reg.ListRelationTypes()
	if len(rts) > 0 && len(items) == 0 {
		t.Error("relation types exist but none marshaled")
	}

	// 验证排序
	for i := 1; i < len(items); i++ {
		if items[i].Name < items[i-1].Name {
			t.Errorf("items not sorted: %s < %s", items[i].Name, items[i-1].Name)
		}
	}
}

func TestMarshalRelationTypes_EmptyRegistry(t *testing.T) {
	reg := schema.NewSchemaRegistry()
	data := marshalRelationTypes(reg)
	if string(data) != "[]" {
		t.Errorf("expected empty array, got %s", data)
	}
}
