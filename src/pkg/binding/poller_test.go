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
		apiClient = newFakeAPIClient()
		store = newFakeStore()
		metrics = metricsHelpers.NewMetricsRegistry()
	})

	It("contains a function that can merge two bindingMaps into a []Binding", func() {
		nonMtls := binding.BindingsMap{
			"app1": binding.Binding{
				AppID: "app1",
				Drains: []binding.Drain{
					{Url: "syslog://app1.syslog"},
					{Url: "syslog://app1.syslog.b"},
				},
			},
			"app2": binding.Binding{
				AppID: "app2",
				Drains: []binding.Drain{
					{Url: "syslog://app2.syslog"},
				},
			},
		}
		mtls := binding.BindingsMap{
			"app1": binding.Binding{
				AppID: "app1",
				Drains: []binding.Drain{
					{
						Url: "mtls-syslog://app1.syslog",
						TLSCredential: binding.TLSCredential{
							Cert: "aCert",
							Key:  "aKey",
						},
					},
				},
			},
			"app3": binding.Binding{
				AppID: "app3",
				Drains: []binding.Drain{
					{Url: "mtls-syslog://app3.syslog",
						TLSCredential: binding.TLSCredential{
							Cert: "anotherCert",
							Key:  "anotherKey",
						},
					},
				},
			},
		}
		Expect(binding.MergeBindings(nonMtls, mtls)).To(Equal(
			[]binding.Binding{
				{
					AppID: "app1",
					Drains: []binding.Drain{
						{
							Url:           "mtls-syslog://app1.syslog",
							TLSCredential: binding.TLSCredential{Cert: "aCert", Key: "aKey"},
						},
						{
							Url:           "syslog://app1.syslog",
							TLSCredential: binding.TLSCredential{Cert: "", Key: ""},
						},
						{
							Url:           "syslog://app1.syslog.b",
							TLSCredential: binding.TLSCredential{Cert: "", Key: ""},
						},
					},
					Hostname: "",
				},
				{
					AppID: "app2",
					Drains: []binding.Drain{
						{
							Url:           "syslog://app2.syslog",
							TLSCredential: binding.TLSCredential{Cert: "", Key: ""},
						},
					},
					Hostname: "",
				},
				{
					AppID: "app3",
					Drains: []binding.Drain{
						{
							Url:           "mtls-syslog://app3.syslog",
							TLSCredential: binding.TLSCredential{Cert: "anotherCert", Key: "anotherKey"},
						},
					},
					Hostname: "",
				},
			},
		))
	})

	It("polls for bindings on an interval", func() {
		p := binding.NewPoller(apiClient, 10*time.Millisecond, 10*time.Millisecond, 10*time.Millisecond, store, metrics, logger)
		go p.Poll()
		go p.MtlsPoll()
		go p.Process()

		Eventually(apiClient.called).Should(BeNumerically(">=", 2))
	})

	It("calls the api client and stores the result", func() {
		apiClient.bindings <- response{
			Results: map[string]struct {
				Drains   []string
				Hostname string
			}{
				"app-id-1": {
					Drains:   []string{"drain-1", "drain-2"},
					Hostname: "app-hostname",
				},
			},
		}
		apiClient.mtlsBindings <- mtlsResponse{
			Bindings: binding.BindingsMap{
				"app-id-1": {
					AppID: "app-id-1",
					Drains: []binding.Drain{
						{
							Url: "drain-3",
							TLSCredential: binding.TLSCredential{
								Cert: "a cert",
								Key:  "a key",
							},
						},
					},
				},
			},
		}

		p := binding.NewPoller(apiClient, 10*time.Millisecond, 10*time.Millisecond, 10*time.Millisecond, store, metrics, logger)
		go p.Poll()
		go p.MtlsPoll()
		go p.Process()

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
				{
					Url: "drain-3",
					TLSCredential: binding.TLSCredential{
						Cert: "a cert", Key: "a key",
					},
				},
			},
			Hostname: "app-hostname",
		}))
	})

	It("fetches the next page of bindings and stores the result", func() {
		apiClient.bindings <- response{
			NextID: 2,
			Results: map[string]struct {
				Drains   []string
				Hostname string
			}{
				"app-id-1": {
					Drains:   []string{"drain-1", "drain-2"},
					Hostname: "app-hostname",
				},
			},
		}

		apiClient.bindings <- response{
			Results: map[string]struct {
				Drains   []string
				Hostname string
			}{
				"app-id-2": {
					Drains:   []string{"drain-3", "drain-4"},
					Hostname: "app-hostname",
				},
			},
		}

		p := binding.NewPoller(apiClient, 10*time.Millisecond, 10*time.Millisecond, 10*time.Millisecond, store, metrics, logger)
		go p.Poll()
		go p.MtlsPoll()
		go p.Process()

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
		p := binding.NewPoller(apiClient, 10*time.Millisecond, 10*time.Millisecond, 10*time.Millisecond, store, metrics, logger)
		go p.Poll()
		go p.MtlsPoll()
		go p.Process()

		apiClient.errors <- errors.New("expected")

		Eventually(func() float64 {
			return metrics.GetMetric("binding_refresh_error", nil).Value()
		}).Should(BeNumerically("==", 1))
	})

	It("tracks the number of bindings returned from CAPI", func() {
		apiClient.bindings <- response{
			Results: map[string]struct {
				Drains   []string
				Hostname string
			}{
				"app-id-1": {},
				"app-id-2": {},
			},
		}
		binding.NewPoller(apiClient, time.Hour, time.Hour, time.Hour, store, metrics, logger)

		Expect(metrics.GetMetric("last_binding_refresh_count", nil).Value()).
			To(BeNumerically("==", 2))
	})
})

type fakeAPIClient struct {
	numRequests  int64
	bindings     chan response
	mtlsBindings chan mtlsResponse
	errors       chan error
	requestedIDs []int
}

func newFakeAPIClient() *fakeAPIClient {
	return &fakeAPIClient{
		bindings:     make(chan response, 100),
		mtlsBindings: make(chan mtlsResponse, 100),
		errors:       make(chan error, 100),
	}
}

func (c *fakeAPIClient) GetUrls(nextID int) (*http.Response, error) {
	atomic.AddInt64(&c.numRequests, 1)

	var binding response
	select {
	case err := <-c.errors:
		return nil, err
	case binding = <-c.bindings:
		c.requestedIDs = append(c.requestedIDs, nextID)
	default:
	}

	b, err := json.Marshal(&binding)
	Expect(err).ToNot(HaveOccurred())
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(b)),
	}

	return resp, nil
}

func (c *fakeAPIClient) GetCerts() (*http.Response, error) {

	var binding mtlsResponse
	select {
	case err := <-c.errors:
		return nil, err
	case binding = <-c.mtlsBindings:
	default:
	}

	b, err := json.Marshal(&binding)
	Expect(err).ToNot(HaveOccurred())
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(b)),
	}

	return resp, nil
}

func (c *fakeAPIClient) called() int64 {
	return atomic.LoadInt64(&c.numRequests)
}

type fakeStore struct {
	bindings        chan []binding.Binding
	nonMtlsBindings chan binding.BindingsMap
	mtlsBindings    chan binding.BindingsMap
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		bindings:        make(chan []binding.Binding, 100),
		nonMtlsBindings: make(chan binding.BindingsMap, 100),
		mtlsBindings:    make(chan binding.BindingsMap, 100),
	}
}

func (c *fakeStore) Merge(f func(nonMtlsBindings, mtlsBindings binding.BindingsMap) []binding.Binding) {
	mrg := f(<-c.nonMtlsBindings, <-c.mtlsBindings)
	c.bindings <- mrg
}

func (c *fakeStore) SetNonMtls(b binding.BindingsMap) {
	c.nonMtlsBindings <- b
}

func (c *fakeStore) SetMtls(b binding.BindingsMap) {
	c.mtlsBindings <- b
}

type response struct {
	Results map[string]struct {
		Drains   []string
		Hostname string
	}
	NextID int `json:"next_id"`
}

type mtlsResponse struct {
	Bindings binding.BindingsMap `json:"bindings"`
}
