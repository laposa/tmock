package catalog

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/laposa/tmock/app-go/internal/liquid"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/logx/logxtest"
	"github.com/laposa/tmock/app-go/internal/store"
)

func ptr[T any](v T) *T { return &v }

func sampleData() store.CatalogData {
	return store.CatalogData{
		Services: []store.CatalogService{
			{Service: store.Service{ID: 1, Path: "users", Name: "Users", UpstreamURL: "http://users.internal/base"},
				// The database's order (by name) is kept, not re-sorted in Go.
				Scenarios: []store.Scenario{
					{ID: 7, Name: "B", RequestMethod: ptr("GET"), RequestCondition: ptr("$request.query.a == '1'")},
					{ID: 3, Name: "a", ResponseCode: ptr(500), ResponseBody: ptr(liquid.Marker + "{{ 1 }}")},
				}},
			{Service: store.Service{ID: 2, Path: "orders", Name: "Orders", UpstreamURL: "http://orders.internal"}},
		},
		Clients: []store.CatalogClient{
			{ID: 4, Name: "qa", Condition: []byte(`{"ip":"10.0.0.1"}`), Scenarios: []int64{3, 7}},
			{ID: 9, Name: "dev", Condition: []byte(`{"cidr":"10.0.0.0/8"}`)},
		},
	}
}

func TestBuild(t *testing.T) {
	snap := Build(sampleData(), map[string]string{"orders": "http://localhost:9000/o"}, slog.New(slog.DiscardHandler))

	users, ok := snap.Service("users")
	if !ok {
		t.Fatal("users not found")
	}
	if users.Upstream.Host != "users.internal" || users.UpstreamURL != "http://users.internal/base" {
		t.Errorf("users upstream = %q", users.UpstreamURL)
	}
	if len(users.Scenarios) != 2 || users.Scenarios[0].Name != "B" || users.Scenarios[1].Name != "a" {
		t.Errorf("scenario order = %+v", users.Scenarios)
	}
	if c := users.Scenarios[0].Condition; c == nil || users.Scenarios[1].Condition != nil {
		t.Errorf("conditions not parsed: %+v", users.Scenarios)
	}
	if users.Scenarios[0].Template != nil || users.Scenarios[1].Template == nil || users.Scenarios[1].TemplateErr != nil {
		t.Errorf("templates not parsed: %+v", users.Scenarios)
	}
	orders, _ := snap.Service("orders")
	if orders.UpstreamURL != "http://localhost:9000/o" || orders.Upstream.Path != "/o" {
		t.Errorf("override not applied: %q", orders.UpstreamURL)
	}
	if _, ok := snap.Service("Users"); ok {
		t.Error("service lookup must be case-sensitive (spec §7.1)")
	}

	clients := snap.Clients()
	if len(clients) != 2 || clients[0].ID != 4 || clients[1].ID != 9 {
		t.Fatalf("clients = %+v", clients)
	}
	if !clients[0].Scenarios[3] || !clients[0].Scenarios[7] || clients[0].Scenarios[1] {
		t.Errorf("scenario set = %v", clients[0].Scenarios)
	}
	if clients[0].Condition == nil || clients[0].Condition.IP == nil || *clients[0].Condition.IP != "10.0.0.1" {
		t.Errorf("condition = %+v", clients[0].Condition)
	}
	if s, sc, c := snap.Counts(); s != 2 || sc != 2 || c != 2 {
		t.Errorf("counts = %d %d %d", s, sc, c)
	}
}

// Rows that internal/app could not have saved are left out, not fatal.
func TestBuildLeavesOutBrokenRows(t *testing.T) {
	data := sampleData()
	data.Services[1].UpstreamURL = "not a url"
	data.Services[0].Scenarios[0].RequestCondition = ptr("$request.a = 1")
	data.Services[0].Scenarios[1].ResponseBody = ptr(liquid.Marker + "{% if %}")
	data.Clients[1].Condition = []byte(`{"bogus":1}`)
	data.Clients = append(data.Clients,
		store.CatalogClient{ID: 10, Name: "empty", Condition: []byte(`{"and":[]}`)},
		store.CatalogClient{ID: 11, Name: "bad regex", Condition: []byte(`{"headerRegex":{"header":"X","value":"("}}`)},
		store.CatalogClient{ID: 12, Name: "none"})
	logs, logger := logxtest.New()
	snap := Build(data, nil, logger)
	if _, ok := snap.Service("orders"); ok {
		t.Error("service with a bad URL kept")
	}
	users, _ := snap.Service("users")
	if len(users.Scenarios) != 1 || users.Scenarios[0].ID != 3 {
		t.Fatalf("scenario with a bad condition kept: %+v", users.Scenarios)
	}
	// A broken template keeps its scenario; the proxy sends the body unrendered.
	if sc := users.Scenarios[0]; sc.Template != nil || sc.TemplateErr == nil {
		t.Errorf("broken template: %+v", sc)
	}
	if len(snap.Clients()) != 1 {
		t.Errorf("clients = %+v", snap.Clients())
	}
	if n := len(logs.All()); n != 7 {
		t.Errorf("%d warnings, want 7", n)
	}
}

// fakeLoader counts loads and can block or fail them.
type fakeLoader struct {
	mu    sync.Mutex
	data  store.CatalogData
	err   error
	loads atomic.Int32
	gate  chan struct{} // if not nil, each Load waits for a value
}

func (f *fakeLoader) Load(ctx context.Context) (store.CatalogData, error) {
	f.loads.Add(1)
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.data, f.err
}

func (f *fakeLoader) set(data store.CatalogData, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data, f.err = data, err
}

func services(t *testing.T, c *Cache) int {
	t.Helper()
	snap, err := c.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	n, _, _ := snap.Counts()
	return n
}

func TestCacheTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := &fakeLoader{data: sampleData()}
		c := NewCache(l, Options{TTL: 10 * time.Minute, Logger: slog.New(slog.DiscardHandler)})
		if services(t, c) != 2 || l.loads.Load() != 1 {
			t.Fatalf("first Get: loads = %d", l.loads.Load())
		}
		l.set(store.CatalogData{}, nil)
		time.Sleep(10*time.Minute - time.Second)
		if services(t, c) != 2 || l.loads.Load() != 1 {
			t.Fatalf("within the TTL: loads = %d", l.loads.Load())
		}
		time.Sleep(time.Second)
		if services(t, c) != 0 || l.loads.Load() != 2 {
			t.Fatalf("after the TTL: loads = %d", l.loads.Load())
		}
	})
}

// A mutation (Invalidate) is visible on the next request, long before the TTL.
func TestCacheInvalidate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := &fakeLoader{data: sampleData()}
		c := NewCache(l, Options{TTL: time.Hour, Logger: slog.New(slog.DiscardHandler)})
		services(t, c)
		l.set(store.CatalogData{}, nil)
		c.Invalidate()
		if services(t, c) != 0 {
			t.Fatal("invalidated snapshot served")
		}
		if services(t, c); l.loads.Load() != 2 {
			t.Fatalf("loads = %d, want 2", l.loads.Load())
		}
	})
}

// Only one rebuild runs at a time. While a snapshot that is only past its TTL is
// rebuilt, other requests use it instead of waiting.
func TestCacheSingleFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := &fakeLoader{data: sampleData()}
		c := NewCache(l, Options{TTL: time.Minute, Logger: slog.New(slog.DiscardHandler)})
		services(t, c)

		l.gate = make(chan struct{})
		l.set(store.CatalogData{}, nil)
		time.Sleep(time.Minute)
		done := make(chan int)
		go func() { done <- services(t, c) }() // starts the rebuild and waits in Load
		synctest.Wait()

		for range 5 {
			if services(t, c) != 2 {
				t.Fatal("a concurrent request waited for the rebuild or got a partial snapshot")
			}
		}
		if n := l.loads.Load(); n != 2 {
			t.Fatalf("loads = %d, want 2 (one rebuild)", n)
		}
		l.gate <- struct{}{}
		if n := <-done; n != 0 {
			t.Fatalf("rebuilding request got %d services", n)
		}
	})
}

// After Invalidate, a request never gets the invalidated snapshot, even while a rebuild
// that started before the change is running: it waits, and since that rebuild may have
// read the data before the change, a second rebuild follows.
func TestCacheInvalidateDuringRebuild(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := &fakeLoader{data: sampleData()}
		c := NewCache(l, Options{TTL: time.Minute, Logger: slog.New(slog.DiscardHandler)})
		services(t, c)

		l.gate = make(chan struct{})
		time.Sleep(time.Minute)
		first := make(chan int)
		go func() { first <- services(t, c) }() // TTL rebuild, reads the old data
		synctest.Wait()

		l.set(store.CatalogData{}, nil) // the change, committed during that rebuild
		c.Invalidate()
		second := make(chan int)
		go func() { second <- services(t, c) }()
		synctest.Wait()
		select {
		case n := <-second:
			t.Fatalf("request after the change got the invalidated snapshot (%d services)", n)
		default:
		}

		l.gate <- struct{}{} // the first rebuild finishes; its snapshot is stale already
		<-first
		l.gate <- struct{}{} // the second rebuild reads the new data
		if n := <-second; n != 0 {
			t.Fatalf("request after the change got %d services, want 0", n)
		}
		if n := l.loads.Load(); n != 3 {
			t.Fatalf("loads = %d, want 3", n)
		}
	})
}

// A request waiting for a rebuild gives up when its context ends.
func TestCacheWaitHonoursContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := &fakeLoader{data: sampleData(), gate: make(chan struct{})}
		c := NewCache(l, Options{TTL: time.Minute, Logger: slog.New(slog.DiscardHandler)})
		go func() { _, _ = c.Get(context.Background()) }()
		synctest.Wait()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, err := c.Get(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want the context's", err)
		}
		l.gate <- struct{}{}
	})
}

// With no snapshot to fall back on, requests queued behind a failing rebuild get its error
// instead of each running a rebuild of their own.
func TestCacheFailureWithoutSnapshot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := &fakeLoader{err: errors.New("db down"), gate: make(chan struct{})}
		c := NewCache(l, Options{TTL: time.Minute, Logger: slog.New(slog.DiscardHandler)})
		errs := make(chan error, 3)
		for range 3 {
			go func() { _, err := c.Get(context.Background()); errs <- err }()
		}
		synctest.Wait()
		l.gate <- struct{}{}
		for range 3 {
			if err := <-errs; err == nil {
				t.Fatal("no error")
			}
		}
		if n := l.loads.Load(); n != 1 {
			t.Fatalf("loads = %d, want 1", n)
		}
		time.Sleep(retryAfterFailure)
		go func() { l.gate <- struct{}{} }()
		if _, err := c.Get(context.Background()); err == nil || l.loads.Load() != 2 {
			t.Fatalf("not retried after the pause: %v, loads %d", err, l.loads.Load())
		}
	})
}

// The very first request has nothing to fall back on, so concurrent first requests wait
// for the one rebuild.
func TestCacheFirstRequestsWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := &fakeLoader{data: sampleData(), gate: make(chan struct{})}
		c := NewCache(l, Options{TTL: time.Hour, Logger: slog.New(slog.DiscardHandler)})
		results := make(chan int, 3)
		for range 3 {
			go func() { results <- services(t, c) }()
		}
		synctest.Wait()
		l.gate <- struct{}{}
		for range 3 {
			if n := <-results; n != 2 {
				t.Fatalf("got %d services", n)
			}
		}
		if n := l.loads.Load(); n != 1 {
			t.Fatalf("loads = %d, want 1", n)
		}
	})
}

func TestCacheRebuildFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logs, logger := logxtest.New()
		l := &fakeLoader{err: errors.New("db down")}
		c := NewCache(l, Options{TTL: time.Minute, Logger: logger})

		if _, err := c.Get(context.Background()); err == nil {
			t.Fatal("no snapshot and a failed rebuild must be an error")
		}
		failed := logs.Events(logx.EventCatalogFailed)
		if len(failed) != 1 || failed[0].Level != slog.LevelError ||
			failed[0].Attrs[logx.AttrExceptionMessage] != "db down" || failed[0].Attrs[logx.AttrErrorType] == "" {
			t.Fatalf("rebuild_failed = %+v", failed)
		}

		l.set(sampleData(), nil)
		time.Sleep(retryAfterFailure) // not retried before that
		if services(t, c) != 2 {
			t.Fatal("no recovery")
		}
		l.set(store.CatalogData{}, errors.New("db down again"))
		time.Sleep(time.Minute)
		if services(t, c) != 2 {
			t.Fatal("the old snapshot must stay when a rebuild fails")
		}
		// Not retried on every request while the database is down.
		services(t, c)
		if n := l.loads.Load(); n != 3 {
			t.Fatalf("loads = %d, want 3", n)
		}
		time.Sleep(retryAfterFailure)
		l.set(store.CatalogData{}, nil)
		if services(t, c) != 0 {
			t.Fatal("not retried after the pause")
		}
	})
}

func TestCacheDisabled(t *testing.T) {
	l := &fakeLoader{data: sampleData()}
	c := NewCache(l, Options{TTL: time.Hour, Disabled: true, Logger: slog.New(slog.DiscardHandler)})
	for range 3 {
		services(t, c)
	}
	if n := l.loads.Load(); n != 3 {
		t.Fatalf("loads = %d, want one per request with DISABLE_CACHE", n)
	}
}

func TestCacheRebuiltEvent(t *testing.T) {
	logs, logger := logxtest.New()
	c := NewCache(&fakeLoader{data: sampleData()}, Options{TTL: time.Hour, Logger: logger})
	services(t, c)
	ev := logs.Events(logx.EventCatalogRebuilt)
	if len(ev) != 1 {
		t.Fatalf("events: %+v", logs.All())
	}
	r := ev[0]
	if r.Level != slog.LevelDebug || r.Attrs[logx.AttrCatalogServices] != "2" ||
		r.Attrs[logx.AttrCatalogScenarios] != "2" || r.Attrs[logx.AttrCatalogClients] != "2" ||
		r.Attrs[logx.AttrDurationMS] == "" {
		t.Errorf("rebuilt = %+v", r)
	}
}
