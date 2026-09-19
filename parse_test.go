package main

import "testing"

// Nodes imported from cobbler carry class parameters, which encapi emits as a
// nested block under the role name. host2020/2021/2022 carry the role
// name as its own class parameter, so a line-based parser that treats every
// indented line as a class name emits the role twice.
func TestParseClassesNestedBlockIsOneClass(t *testing.T) {
	body := []byte("classes:\n" +
		"    roles::infra::proxy::jumphost:\n" +
		"        roles::infra::proxy::jumphost: '~'\n" +
		"environment: develop\n" +
		"parameters: {}\n")

	doc, err := parseCobbler(body)
	if err != nil {
		t.Fatalf("parseCobbler returned error: %v", err)
	}
	want := []string{"roles::infra::proxy::jumphost"}
	if len(doc.classes) != len(want) || doc.classes[0] != want[0] {
		t.Errorf("classes = %q, want %q", doc.classes, want)
	}
	if doc.environment != "develop" {
		t.Errorf("environment = %q, want develop", doc.environment)
	}
}

// The same shape with ordinary class parameters: the parameter keys are values
// of the class, never classes themselves.
func TestParseClassesNestedParamsAreNotClasses(t *testing.T) {
	body := []byte("classes:\n" +
		"    roles::base:\n" +
		"        listen_port: 8140\n" +
		"        tls: true\n" +
		"    roles::infra::dns::master: {}\n" +
		"environment: production\n" +
		"parameters: {}\n")

	doc, err := parseCobbler(body)
	if err != nil {
		t.Fatalf("parseCobbler returned error: %v", err)
	}
	want := []string{"roles::base", "roles::infra::dns::master"}
	if len(doc.classes) != len(want) {
		t.Fatalf("classes = %q, want %q", doc.classes, want)
	}
	for i := range want {
		if doc.classes[i] != want[i] {
			t.Errorf("classes[%d] = %q, want %q", i, doc.classes[i], want[i])
		}
	}
}
