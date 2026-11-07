package catalog

import (
	"bufio"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// repoRoot is the monorepo root, from this package's directory.
const repoRoot = "../../../../.."

// instanceAliases collects every fixed network alias an instance's network
// carries: the aliases in the app and infra compose files, and the
// `edge-proxy` alias ./launch gives the edge Nginx on each instance network.
// Aliases built from variables, such as ${BIOTRON_INSTANCE}-status-api, are
// left out; the catalog never names them.
func instanceAliases(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(repoRoot, "apps", "*", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, filepath.Join(repoRoot, "infra", "docker-compose.yml"))
	if len(files) < 2 {
		t.Fatalf("found compose files %v; is the test running from its package directory?", files)
	}

	aliases := make(map[string]bool)
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			t.Fatal(err)
		}
		inAliases, indent := false, 0
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimRight(scanner.Text(), "\r")
			trimmed := strings.TrimSpace(line)
			depth := len(line) - len(strings.TrimLeft(line, " "))
			switch {
			case trimmed == "aliases:":
				inAliases, indent = true, depth
			case inAliases && depth > indent && strings.HasPrefix(trimmed, "- "):
				if alias := strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")); !strings.Contains(alias, "$") {
					aliases[alias] = true
				}
			default:
				inAliases = false
			}
		}
		_ = f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
	}

	launch, err := os.ReadFile(filepath.Join(repoRoot, "launch"))
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range regexp.MustCompile(`aliases: \[([a-z0-9-]+)\]`).FindAllSubmatch(launch, -1) {
		aliases[string(match[1])] = true
	}
	return aliases
}

// Every health_url in the default catalog must name a host the instance
// network resolves, or that component reads down for ever. This catches a
// service rename that leaves the catalog behind, and the edge moving to its
// own project without the `edge-proxy` alias.
func TestDefaultCatalogHostsResolveOnTheInstanceNetwork(t *testing.T) {
	aliases := instanceAliases(t)
	if !aliases["edge-proxy"] {
		t.Fatal("./launch no longer gives the edge Nginx the edge-proxy alias on instance networks")
	}

	serviceCatalog, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	for _, app := range serviceCatalog.Applications() {
		for _, component := range app.Components {
			if component.Check != CheckHTTP {
				continue
			}
			parsed, err := url.Parse(component.HealthURL)
			if err != nil {
				t.Fatalf("%s: %v", component.ID, err)
			}
			host := parsed.Hostname()
			// Status probes its own API over loopback.
			if host == "127.0.0.1" {
				continue
			}
			if !aliases[host] {
				t.Errorf("component %s probes %s, but no compose service or the edge answers to %q", component.ID, component.HealthURL, host)
			}
		}
	}
}
