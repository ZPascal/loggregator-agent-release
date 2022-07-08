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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
)

var _ = Describe("Poller", func() {
	var (
		apiClient *fakeAPIClient
		store     *fakeStore
		metrics   *metricsHelpers.SpyMetricsRegistry
		logger    = log.New(GinkgoWriter, "", 0)
	)

	BeforeEach(func() {
		apiClient = newFakeAPIClient(false)
		store = newFakeStore()
		metrics = metricsHelpers.NewMetricsRegistry()
	})

	It("polls for bindings on an interval", func() {
		p := binding.NewPoller(apiClient, 10*time.Millisecond, store, metrics, logger, false)
		go p.Poll()

		Eventually(apiClient.called).Should(BeNumerically(">=", 2))
	})

	It("calls the api client and stores the result", func() {
		apiClient.bindings <- response{
			Results: map[string]struct {
				Drains []struct {
					Url         string
					Credentials struct {
						Cert string
						Key  string
					}
				}
				Hostname string
			}{
				"app-id-1": {
					Drains: []struct {
						Url         string
						Credentials struct {
							Cert string
							Key  string
						}
					}{
						{
							Url: "drain-1",
							Credentials: struct {
								Cert string
								Key  string
							}{},
						},
						{
							Url: "drain-2",
							Credentials: struct {
								Cert string
								Key  string
							}{},
						},
					},
					Hostname: "app-hostname",
				},
			},
		}

		p := binding.NewPoller(apiClient, 10*time.Millisecond, store, metrics, logger, false)
		go p.Poll()

		var expected []binding.Binding
		Eventually(store.bindings).Should(Receive(&expected))
		Expect(expected).To(ConsistOf(binding.Binding{
			AppID: "app-id-1",
			Drains: []binding.Drain{
				{
					Url: "drain-1",
				},
				{
					Url: "drain-2",
				},
			},
			Hostname: "app-hostname",
		}))
	})

	It("fetches the next page of bindings and stores the result", func() {
		apiClient.bindings <- response{
			NextID: 2,
			Results: map[string]struct {
				Drains []struct {
					Url         string
					Credentials struct {
						Cert string
						Key  string
					}
				}
				Hostname string
			}{
				"app-id-1": {
					Drains: []struct {
						Url         string
						Credentials struct {
							Cert string
							Key  string
						}
					}{
						{
							Url: "drain-1",
						},
						{
							Url: "drain-2",
						},
					},
					Hostname: "app-hostname",
				},
			},
		}

		apiClient.bindings <- response{
			Results: map[string]struct {
				Drains []struct {
					Url         string
					Credentials struct {
						Cert string
						Key  string
					}
				}
				Hostname string
			}{
				"app-id-2": {
					Drains: []struct {
						Url         string
						Credentials struct {
							Cert string
							Key  string
						}
					}{
						{
							Url: "drain-3",
						},
						{
							Url: "drain-4",
						},
					},
					Hostname: "app-hostname",
				},
			},
		}

		p := binding.NewPoller(apiClient, 10*time.Millisecond, store, metrics, logger, false)
		go p.Poll()

		var expected []binding.Binding
		Eventually(store.bindings).Should(Receive(&expected))
		Expect(expected).To(ConsistOf(
			binding.Binding{
				AppID: "app-id-1",
				Drains: []binding.Drain{
					{
						Url: "drain-1",
					},
					{
						Url: "drain-2",
					},
				},
				Hostname: "app-hostname",
			},
			binding.Binding{
				AppID: "app-id-2",
				Drains: []binding.Drain{
					{
						Url: "drain-3",
					},
					{
						Url: "drain-4",
					},
				},
				Hostname: "app-hostname",
			},
		))

		Expect(apiClient.requestedIDs).To(ConsistOf(0, 2))
	})

	It("tracks the number of API errors", func() {
		p := binding.NewPoller(apiClient, 10*time.Millisecond, store, metrics, logger, false)
		go p.Poll()

		apiClient.errors <- errors.New("expected")

		Eventually(func() float64 {
			return metrics.GetMetric("binding_refresh_error", nil).Value()
		}).Should(BeNumerically("==", 1))
	})

	It("tracks the number of bindings returned from CAPI", func() {
		apiClient.bindings <- response{
			Results: map[string]struct {
				Drains []struct {
					Url         string
					Credentials struct {
						Cert string
						Key  string
					}
				}
				Hostname string
			}{
				"app-id-1": {},
				"app-id-2": {},
			},
		}
		binding.NewPoller(apiClient, time.Hour, store, metrics, logger, false)

		Expect(metrics.GetMetric("last_binding_refresh_count", nil).Value()).
			To(BeNumerically("==", 2))
	})
})

var _ = Describe("LegacyPoller", func() {
	var (
		apiClient *fakeAPIClient
		store     *fakeStore
		metrics   *metricsHelpers.SpyMetricsRegistry
		logger    = log.New(GinkgoWriter, "", 0)
	)

	BeforeEach(func() {
		apiClient = newFakeAPIClient(true)
		store = newFakeStore()
		metrics = metricsHelpers.NewMetricsRegistry()
	})

	It("polls for bindings on an interval", func() {
		p := binding.NewPoller(apiClient, 10*time.Millisecond, store, metrics, logger, false)
		go p.Poll()

		Eventually(apiClient.called).Should(BeNumerically(">=", 2))
	})

	It("calls the api client and stores the result", func() {
		apiClient.legacyBindings <- legacyResponse{
			Results: map[string]struct {
				Drains   []string
				Hostname string
			}{
				"app-id-1": {
					Drains: []string{
						"drain-1",
						"drain-2",
					},
					Hostname: "app-hostname",
				},
			},
		}

		p := binding.NewPoller(apiClient, 10*time.Millisecond, store, metrics, logger, true)
		go p.Poll()

		var expected []binding.Binding
		Eventually(store.bindings).Should(Receive(&expected))
		Expect(expected).To(ConsistOf(binding.Binding{
			AppID: "app-id-1",
			Drains: []binding.Drain{
				{
					Url: "drain-1",
				},
				{
					Url: "drain-2",
				},
			},
			Hostname: "app-hostname",
		}))
	})

	It("fetches the next page of bindings and stores the result", func() {
		apiClient.legacyBindings <- legacyResponse{
			NextID: 2,
			Results: map[string]struct {
				Drains   []string
				Hostname string
			}{
				"app-id-1": {
					Drains: []string{
						"drain-1",
						"drain-2",
					},
					Hostname: "app-hostname",
				},
			},
		}

		apiClient.legacyBindings <- legacyResponse{
			Results: map[string]struct {
				Drains   []string
				Hostname string
			}{
				"app-id-2": {
					Drains: []string{
						"drain-3",
						"drain-4",
					},
					Hostname: "app-hostname",
				},
			},
		}

		p := binding.NewPoller(apiClient, 10*time.Millisecond, store, metrics, logger, true)
		go p.Poll()

		var expected []binding.Binding
		Eventually(store.bindings).Should(Receive(&expected))
		Expect(expected).To(ConsistOf(
			binding.Binding{
				AppID: "app-id-1",
				Drains: []binding.Drain{
					{
						Url: "drain-1",
					},
					{
						Url: "drain-2",
					},
				},
				Hostname: "app-hostname",
			},
			binding.Binding{
				AppID: "app-id-2",
				Drains: []binding.Drain{
					{
						Url: "drain-3",
					},
					{
						Url: "drain-4",
					},
				},
				Hostname: "app-hostname",
			},
		))

		Expect(apiClient.requestedIDs).To(ConsistOf(0, 2))
	})

	It("tracks the number of API errors", func() {
		p := binding.NewPoller(apiClient, 10*time.Millisecond, store, metrics, logger, true)
		go p.Poll()

		apiClient.errors <- errors.New("expected")

		Eventually(func() float64 {
			return metrics.GetMetric("binding_refresh_error", nil).Value()
		}).Should(BeNumerically("==", 1))
	})

	It("tracks the number of bindings returned from CAPI", func() {
		apiClient.legacyBindings <- legacyResponse{
			Results: map[string]struct {
				Drains   []string
				Hostname string
			}{
				"app-id-1": {},
				"app-id-2": {},
			},
		}
		binding.NewPoller(apiClient, time.Hour, store, metrics, logger, true)

		Expect(metrics.GetMetric("last_binding_refresh_count", nil).Value()).
			To(BeNumerically("==", 2))
	})
})

type fakeAPIClient struct {
	numRequests    int64
	bindings       chan response
	legacyBindings chan legacyResponse
	legacy         bool
	errors         chan error
	requestedIDs   []int
}

func newFakeAPIClient(legacy bool) *fakeAPIClient {
	return &fakeAPIClient{
		bindings:       make(chan response, 100),
		legacyBindings: make(chan legacyResponse, 100),
		errors:         make(chan error, 100),
		legacy:         legacy,
	}
}

func (c *fakeAPIClient) Get(nextID int) (*http.Response, error) {
	atomic.AddInt64(&c.numRequests, 1)

	var binding response
	var legacyBinding legacyResponse
	select {
	case err := <-c.errors:
		return nil, err
	case binding = <-c.bindings:
		c.requestedIDs = append(c.requestedIDs, nextID)
	case legacyBinding = <-c.legacyBindings:
		c.requestedIDs = append(c.requestedIDs, nextID)
	default:
	}

	var body []byte
	if c.legacy {
		b, err := json.Marshal(&legacyBinding)
		Expect(err).ToNot(HaveOccurred())
		body = b
	} else {
		b, err := json.Marshal(&binding)
		Expect(err).ToNot(HaveOccurred())
		body = b
	}
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

func (c *fakeStore) Set(b []binding.Binding) {
	c.bindings <- b
}

type response struct {
	Results map[string]struct {
		Drains []struct {
			Url         string
			Credentials struct {
				Cert string
				Key  string
			}
		}
		Hostname string
	}
	NextID int `json:"next_id"`
}

type legacyResponse struct {
	Results map[string]struct {
		Drains   []string
		Hostname string
	}
	NextID int `json:"next_id"`
}
