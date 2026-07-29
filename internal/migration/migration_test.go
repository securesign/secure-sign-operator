package migration

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newObj(annotations map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test",
			Annotations: annotations,
		},
	}
}

func TestKey(t *testing.T) {
	got := Key("v1alpha1", "rekorSearchUI")
	want := "migration.rhtas.redhat.com/v1alpha1.rekorSearchUI"
	if got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
}

func TestSetAndRead(t *testing.T) {
	obj := newObj(nil)
	key := Key("v1alpha1", "field")

	type payload struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	if err := Set(obj, key, payload{Name: "test", Count: 42}); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	var got payload
	ok, err := Read(obj, key, &got)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if !ok {
		t.Fatal("Read() returned false, want true")
	}
	if got.Name != "test" || got.Count != 42 {
		t.Errorf("Read() = %+v, want {Name:test Count:42}", got)
	}
}

func TestReadMissing(t *testing.T) {
	obj := newObj(nil)
	var target string
	ok, err := Read(obj, Key("v1alpha1", "missing"), &target)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if ok {
		t.Error("Read() returned true for missing key")
	}
}

func TestReadEmpty(t *testing.T) {
	key := Key("v1alpha1", "empty")
	obj := newObj(map[string]string{key: ""})
	var target string
	ok, err := Read(obj, key, &target)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if ok {
		t.Error("Read() returned true for empty value")
	}
}

func TestPop(t *testing.T) {
	obj := newObj(nil)
	key := Key("v1alpha1", "field")
	if err := Set(obj, key, "hello"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	var got string
	ok, err := Pop(obj, key, &got)
	if err != nil {
		t.Fatalf("Pop() error: %v", err)
	}
	if !ok {
		t.Fatal("Pop() returned false, want true")
	}
	if got != "hello" {
		t.Errorf("Pop() = %q, want %q", got, "hello")
	}
	if Has(obj, key) {
		t.Error("Pop() did not remove the annotation")
	}
}

func TestPopMissing(t *testing.T) {
	obj := newObj(nil)
	var target string
	ok, err := Pop(obj, Key("v1alpha1", "missing"), &target)
	if err != nil {
		t.Fatalf("Pop() error: %v", err)
	}
	if ok {
		t.Error("Pop() returned true for missing key")
	}
}

func TestHas(t *testing.T) {
	key := Key("v1alpha1", "field")
	obj := newObj(map[string]string{key: "data"})

	if !Has(obj, key) {
		t.Error("Has() returned false, want true")
	}
	if Has(obj, Key("v1alpha1", "other")) {
		t.Error("Has() returned true for absent key")
	}
}

func TestRemove(t *testing.T) {
	key := Key("v1alpha1", "field")
	obj := newObj(map[string]string{key: "data", "other": "keep"})

	Remove(obj, key)
	if Has(obj, key) {
		t.Error("Remove() did not delete the key")
	}
	if obj.GetAnnotations()["other"] != "keep" {
		t.Error("Remove() deleted unrelated annotation")
	}
}

func TestStripAll(t *testing.T) {
	obj := newObj(map[string]string{
		Key("v1alpha1", "field1"): "a",
		Key("v1alpha1", "field2"): "b",
		"unrelated":               "keep",
	})

	StripAll(obj)

	if Has(obj, Key("v1alpha1", "field1")) || Has(obj, Key("v1alpha1", "field2")) {
		t.Error("StripAll() did not remove all migration annotations")
	}
	if obj.GetAnnotations()["unrelated"] != "keep" {
		t.Error("StripAll() removed non-migration annotation")
	}
}

func TestStripAllNilsEmptyMap(t *testing.T) {
	obj := newObj(map[string]string{
		Key("v1alpha1", "only"): "data",
	})

	StripAll(obj)

	if obj.GetAnnotations() != nil {
		t.Errorf("StripAll() should nil out annotations when empty, got %v", obj.GetAnnotations())
	}
}

func TestSetNilAnnotations(t *testing.T) {
	obj := newObj(nil)
	key := Key("v1alpha1", "field")
	if err := Set(obj, key, "value"); err != nil {
		t.Fatalf("Set() error on nil annotations: %v", err)
	}
	if !Has(obj, key) {
		t.Error("Set() on nil annotations did not create the key")
	}
}
