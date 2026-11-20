package httpapi

import (
	"bytes"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The served contract is a copy of contracts/openapi.yaml (go generate
// ./internal/httpapi copies it): they must be the same.
func TestEmbeddedContractIsTheRepositorys(t *testing.T) {
	repo, err := os.ReadFile("../../../../contracts/openapi.yaml")
	if err != nil {
		t.Skip("contracts/openapi.yaml isn't beside this module")
	}
	if !bytes.Equal(repo, openapiYAML) {
		t.Fatal("internal/httpapi/openapi.yaml differs from contracts/openapi.yaml: run go generate ./internal/httpapi")
	}
}

// Every route the server registers is in the contract, and the other way
// around.
func TestRoutesAreTheContracts(t *testing.T) {
	var served []string
	for _, line := range strings.Split(routesSource(t), "\n") {
		m := regexp.MustCompile(`HandleFunc\("([A-Z]+) ([^"]+)"`).FindStringSubmatch(line)
		if m != nil {
			served = append(served, m[1]+" "+m[2])
		}
	}
	var documented []string
	path := ""
	for _, line := range strings.Split(string(openapiYAML), "\n") {
		if m := regexp.MustCompile(`^  (/[^:]*):$`).FindStringSubmatch(line); m != nil {
			path = m[1]
			continue
		}
		if m := regexp.MustCompile(`^    (get|post|put|patch|delete):$`).FindStringSubmatch(line); m != nil && path != "" {
			documented = append(documented, strings.ToUpper(m[1])+" "+path)
		}
		if strings.HasPrefix(line, "components:") {
			break
		}
	}
	slices.Sort(served)
	slices.Sort(documented)
	if !slices.Equal(served, documented) {
		t.Fatalf("served and documented routes differ:\nserved:     %v\ndocumented: %v", served, documented)
	}
}

func routesSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
