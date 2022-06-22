package binding

import (
	"sync"

	metrics "code.cloudfoundry.org/go-metric-registry"
)

type Store struct {
	mu              sync.RWMutex
	bindings        []Binding
	nonMtlsBindings BindingsMap
	mtlsBindings    BindingsMap
	bindingCount    metrics.Gauge
}

func NewStore(m Metrics) *Store {
	return &Store{
		bindings:        make([]Binding, 0),
		nonMtlsBindings: make(BindingsMap),
		mtlsBindings:    make(BindingsMap),
		bindingCount: m.NewGauge(
			"cached_bindings",
			"Current number of bindings stored in the binding cache.",
		),
	}
}

func (s *Store) Get() []Binding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bindings
}

func (s *Store) Merge(f func(nonMtlsBindings, mtlsBindings BindingsMap) []Binding) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.bindings = f(s.nonMtlsBindings, s.mtlsBindings)
	s.bindingCount.Set(float64(len(s.bindings)))
}

func (s *Store) SetNonMtls(bindings BindingsMap) {
	if bindings == nil {
		bindings = make(BindingsMap, 0)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nonMtlsBindings = bindings
}

func (s *Store) SetMtls(bindings BindingsMap) {
	if bindings == nil {
		bindings = make(BindingsMap, 0)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.mtlsBindings = bindings
}

type AggregateStore struct {
	AggregateDrains []string
}

func (store *AggregateStore) Get() []Binding {
	var aggregateDrains []Drain
	for _, d := range store.AggregateDrains {
		aggregateDrains = append(aggregateDrains, Drain{
			Url: d,
		})
	}
	return []Binding{
		{
			AppID:  "",
			Drains: aggregateDrains,
		},
	}
}
