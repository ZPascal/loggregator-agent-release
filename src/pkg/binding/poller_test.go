package binding_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
)

var _ = Describe("Poller", func() {
	var (
		apiClient   *fakeAPIClient
		store       *fakeStore
		legacyStore *fakeLegacyStore
		metrics     *metricsHelpers.SpyMetricsRegistry
		logger      = log.New(GinkgoWriter, "", 0)
	)

	BeforeEach(func() {
		apiClient = newFakeAPIClient()
		store = newFakeStore()
		legacyStore = newFakeLegacyStore()
		metrics = metricsHelpers.NewMetricsRegistry()
	})

	It("polls for bindings on an interval", func() {
		p := binding.NewPoller(apiClient, 10*time.Millisecond, store, legacyStore, metrics, logger)
		go p.Poll()

		Eventually(apiClient.called).Should(BeNumerically(">=", 2))
	})

	It("calls the api client and stores the result", func() {
		apiClient.bindings <- response{
			Results: []binding.Binding{
				{
					Url:  "drain-1",
					Cert: "cert",
					Key:  "key",
					Apps: []binding.App{
						{Hostname: "app-hostname", AppID: "app-id-1"},
					},
				},
				{
					Url:  "drain-2",
					Cert: "cert",
					Key:  "key",
					Apps: []binding.App{
						{Hostname: "app-hostname", AppID: "app-id-1"},
					},
				},
			},
		}

		p := binding.NewPoller(apiClient, 10*time.Millisecond, store, legacyStore, metrics, logger)
		go p.Poll()

		var expectedBindings []binding.Binding
		Eventually(store.bindings).Should(Receive(&expectedBindings))
		Expect(expectedBindings).To(ConsistOf([]binding.Binding{
			{
				Url:  "drain-1",
				Cert: "cert",
				Key:  "key",
				Apps: []binding.App{
					{Hostname: "app-hostname", AppID: "app-id-1"},
				},
			},
			{
				Url:  "drain-2",
				Cert: "cert",
				Key:  "key",
				Apps: []binding.App{
					{Hostname: "app-hostname", AppID: "app-id-1"},
				},
			},
		}))

		var expectedLegacyBindings []binding.LegacyBinding
		Eventually(legacyStore.bindings).Should(Receive(&expectedLegacyBindings))
		Expect(expectedLegacyBindings).To(ConsistOf([]binding.LegacyBinding{
			{
				AppID:    "app-id-1",
				Drains:   []string{"drain-1", "drain-2"},
				Hostname: "app-hostname",
			},
		}))
	})

	It("fetches the next page of bindings and stores the result", func() {
		apiClient.bindings <- response{
			NextID: 2,
			Results: []binding.Binding{
				{
					Url:  "drain-1",
					Cert: "cert",
					Key:  "key",
					Apps: []binding.App{
						{Hostname: "app-hostname", AppID: "app-id-1"},
					},
				},
				{
					Url:  "drain-2",
					Cert: "cert",
					Key:  "key",
					Apps: []binding.App{
						{Hostname: "app-hostname", AppID: "app-id-1"},
					},
				},
			},
		}

		apiClient.bindings <- response{
			Results: []binding.Binding{
				{
					Url:  "drain-3",
					Cert: "cert",
					Key:  "key",
					Apps: []binding.App{
						{Hostname: "app-hostname", AppID: "app-id-2"},
					},
				},
				{
					Url:  "drain-4",
					Cert: "cert",
					Key:  "key",
					Apps: []binding.App{
						{Hostname: "app-hostname", AppID: "app-id-2"},
					},
				},
			},
		}

		p := binding.NewPoller(apiClient, 10*time.Millisecond, store, legacyStore, metrics, logger)
		go p.Poll()

		var expectedBindings []binding.Binding
		Eventually(store.bindings).Should(Receive(&expectedBindings))
		Expect(expectedBindings).To(ConsistOf(
			[]binding.Binding{
				{
					Url:  "drain-1",
					Cert: "cert",
					Key:  "key",
					Apps: []binding.App{
						{Hostname: "app-hostname", AppID: "app-id-1"},
					},
				},
				{
					Url:  "drain-2",
					Cert: "cert",
					Key:  "key",
					Apps: []binding.App{
						{Hostname: "app-hostname", AppID: "app-id-1"},
					},
				},
				{
					Url:  "drain-3",
					Cert: "cert",
					Key:  "key",
					Apps: []binding.App{
						{Hostname: "app-hostname", AppID: "app-id-2"},
					},
				},
				{
					Url:  "drain-4",
					Cert: "cert",
					Key:  "key",
					Apps: []binding.App{
						{Hostname: "app-hostname", AppID: "app-id-2"},
					},
				},
			},
		))

		var expectedLegacyBindings []binding.LegacyBinding
		Eventually(legacyStore.bindings).Should(Receive(&expectedLegacyBindings))
		Expect(expectedLegacyBindings).To(ConsistOf([]binding.LegacyBinding{
			{
				AppID:    "app-id-1",
				Drains:   []string{"drain-1", "drain-2"},
				Hostname: "app-hostname",
			},
			{
				AppID:    "app-id-2",
				Drains:   []string{"drain-3", "drain-4"},
				Hostname: "app-hostname",
			},
		}))

		Expect(apiClient.requestedIDs).To(ConsistOf(0, 2))
	})

	It("tracks the number of API errors", func() {
		p := binding.NewPoller(apiClient, 10*time.Millisecond, store, legacyStore, metrics, logger)
		go p.Poll()

		apiClient.errors <- errors.New("expected")

		Eventually(func() float64 {
			return metrics.GetMetric("binding_refresh_error", nil).Value()
		}).Should(BeNumerically("==", 1))
	})

	It("tracks the number of bindings returned from CAPI", func() {
		apiClient.bindings <- response{
			Results: []binding.Binding{
				{
					Url:  "drain-1",
					Cert: "cert",
					Key:  "key",
					Apps: []binding.App{
						{Hostname: "app-hostname", AppID: "app-id-1"},
					},
				},
				{
					Url:  "drain-2",
					Cert: "cert",
					Key:  "key",
					Apps: []binding.App{
						{Hostname: "app-hostname", AppID: "app-id-2"},
					},
				},
			},
		}
		binding.NewPoller(apiClient, time.Hour, store, legacyStore, metrics, logger)

		Expect(metrics.GetMetric("last_binding_refresh_count", nil).Value()).
			To(BeNumerically("==", 2))
	})

	It("tracks the isolated CalculateBindingsCount call", func() {
		noBinding := []binding.Binding{}
		singleBinding := []binding.Binding{
			{
				Url:  "drain-1",
				Cert: "cert",
				Key:  "key",
				Apps: []binding.App{
					{Hostname: "app-hostname", AppID: "app-id-1"},
				},
			},
			{
				Url:  "drain-2",
				Cert: "cert",
				Key:  "key",
				Apps: []binding.App{
					{Hostname: "app-hostname", AppID: "app-id-1"},
				},
			},
		}
		multipleBindings := []binding.Binding{
			{
				Url:  "drain-1",
				Cert: "cert",
				Key:  "key",
				Apps: []binding.App{
					{Hostname: "app-hostname", AppID: "app-id-1"},
				},
			},
			{
				Url:  "drain-2",
				Cert: "cert",
				Key:  "key",
				Apps: []binding.App{
					{Hostname: "app-hostname", AppID: "app-id-2"},
				},
			},
		}
		Expect(binding.CalculateBindingCount(noBinding)).
			To(BeNumerically("==", 0))
		Expect(binding.CalculateBindingCount(singleBinding)).
			To(BeNumerically("==", 1))
		Expect(binding.CalculateBindingCount(multipleBindings)).
			To(BeNumerically("==", 2))
	})

	It("tracks the correct transformation to LegacyBindings", func() {
		noBinding := []binding.Binding{}
		singleBinding := []binding.Binding{
			{
				Url:  "drain-1",
				Cert: "cert",
				Key:  "key",
				Apps: []binding.App{
					{Hostname: "app-hostname", AppID: "app-id-1"},
				},
			},
			{
				Url:  "drain-2",
				Cert: "cert",
				Key:  "key",
				Apps: []binding.App{
					{Hostname: "app-hostname", AppID: "app-id-1"},
				},
			},
		}
		multipleBindings := []binding.Binding{
			{
				Url:  "drain-1",
				Cert: "cert",
				Key:  "key",
				Apps: []binding.App{
					{Hostname: "app-hostname", AppID: "app-id-1"},
				},
			},
			{
				Url:  "drain-2",
				Cert: "cert",
				Key:  "key",
				Apps: []binding.App{
					{Hostname: "app-hostname", AppID: "app-id-2"},
				},
			},
		}
		expectedSingleLegacyBindings := []binding.LegacyBinding{
			{
				AppID:    "app-id-1",
				Drains:   []string{"drain-1", "drain-2"},
				Hostname: "app-hostname",
			},
		}
		expectedMultiLegacyBindings := []binding.LegacyBinding{
			{
				AppID:    "app-id-1",
				Drains:   []string{"drain-1"},
				Hostname: "app-hostname",
			},
			{
				AppID:    "app-id-2",
				Drains:   []string{"drain-2"},
				Hostname: "app-hostname",
			},
		}

		Expect(binding.ToLegacyBindings(noBinding)).To(ConsistOf([]binding.LegacyBinding{}))
		Expect(binding.ToLegacyBindings(singleBinding)).To(ConsistOf(expectedSingleLegacyBindings))
		Expect(binding.ToLegacyBindings(multipleBindings)).To(ConsistOf(expectedMultiLegacyBindings))

	})

})

type fakeAPIClient struct {
	numRequests  int64
	bindings     chan response
	errors       chan error
	requestedIDs []int
}

func newFakeAPIClient() *fakeAPIClient {
	return &fakeAPIClient{
		bindings: make(chan response, 100),
		errors:   make(chan error, 100),
	}
}

func (c *fakeAPIClient) Get(nextID int) (*http.Response, error) {
	atomic.AddInt64(&c.numRequests, 1)

	var binding response
	select {
	case err := <-c.errors:
		return nil, err
	case binding = <-c.bindings:
		c.requestedIDs = append(c.requestedIDs, nextID)
	default:
	}

	var body []byte
	b, err := json.Marshal(&binding)
	Expect(err).ToNot(HaveOccurred())
	body = b
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	return resp, nil
}

func (c *fakeAPIClient) called() int64 {
	return atomic.LoadInt64(&c.numRequests)
}

type fakeStore struct {
	bindings chan []binding.Binding
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		bindings: make(chan []binding.Binding, 100),
	}
}

func (c *fakeStore) Set(b []binding.Binding, bindingCount int) {
	c.bindings <- b
}

type fakeLegacyStore struct {
	bindings chan []binding.LegacyBinding
}

func newFakeLegacyStore() *fakeLegacyStore {
	return &fakeLegacyStore{
		bindings: make(chan []binding.LegacyBinding, 100),
	}
}

func (c *fakeLegacyStore) Set(b []binding.LegacyBinding) {
	c.bindings <- b
}

type response struct {
	Results []binding.Binding
	NextID  int `json:"next_id"`
}
