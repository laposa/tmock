package proxy

import (
	"net/http"
	"net/netip"
	"strings"

	"github.com/laposa/tmock/app-go/internal/catalog"
	"github.com/laposa/tmock/app-go/internal/clientip"
	"github.com/laposa/tmock/app-go/internal/condition"
)

// selection is the outcome of client matching and scenario selection for one request.
type selection struct {
	client   *catalog.Client   // the reported client; nil when no client matched
	scenario *catalog.Scenario // the winning scenario; nil when none applies
}

// matchingClients returns the enabled clients whose condition is true for r, in id order
// (spec §7.3).
func matchingClients(clients []catalog.Client, r *http.Request) []*catalog.Client {
	if len(clients) == 0 {
		return nil
	}
	cr := condition.Request{Header: r.Header, Host: r.Host}
	if ip, err := netip.ParseAddr(clientip.From(r)); err == nil {
		cr.IP = ip.Unmap()
	}
	var out []*catalog.Client
	for i := range clients {
		if clients[i].Match.Match(cr) {
			out = append(out, &clients[i])
		}
	}
	return out
}

// requestPath is the part of the path after /api/proxy/{service}/, decoded, without
// leading or trailing "/" (spec §7.5 step 2). The query is not part of it.
func requestPath(r *http.Request) string {
	return strings.Trim(r.PathValue("rest"), "/")
}

// selectScenario applies spec §7.5 to the scenarios of svc for the matching clients:
// the first scenario, in name order, that one of the clients has assigned and whose
// method and path filters fit the request wins. The reported client is the first
// matching client that has the winner assigned, or the first matching client when no
// scenario wins.
//
// A scenario with a request condition is skipped: request conditions are evaluated from
// I11 (internal/expr), and until then such a scenario must not apply to requests its
// condition may exclude.
func selectScenario(svc *catalog.Service, clients []*catalog.Client, method, reqPath string) selection {
	if len(clients) == 0 {
		return selection{}
	}
	for i := range svc.Scenarios {
		s := &svc.Scenarios[i]
		var owner *catalog.Client
		for _, c := range clients {
			if c.Scenarios[s.ID] {
				owner = c
				break
			}
		}
		switch {
		case owner == nil:
		case s.RequestPath != nil && !strings.EqualFold(*s.RequestPath, reqPath):
		case s.RequestMethod != nil && *s.RequestMethod != method:
		case s.RequestCondition != nil:
		default:
			return selection{client: owner, scenario: s}
		}
	}
	return selection{client: clients[0]}
}
