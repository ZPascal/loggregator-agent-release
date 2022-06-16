package app_test

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/tlsconfig"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/syslog-binding-cache/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
)

var _ = Describe("SyslogBindingCache", func() {
	var (
		logger       = log.New(GinkgoWriter, "", log.LstdFlags)
		metricClient *metricsHelpers.SpyMetricsRegistry
		config       app.Config

		capi *fakeCC
		sbc  *app.SyslogBindingCache

		cachePort = 40000

		capiTestCerts         = testhelper.GenerateCerts("capiCA")
		bindingCacheTestCerts = testhelper.GenerateCerts("bindingCacheCA")
	)

	BeforeEach(func() {
		r := results{
			"app-id-1": appBindings{
				Drains:   []string{"syslog://drain-a", "syslog://drain-b"},
				Hostname: "org.space.app-name",
			},
			"app-id-2": appBindings{
				Drains:   []string{"syslog://drain-c", "syslog://drain-d"},
				Hostname: "org.space.app-name-2",
			},
			// Trying to check if there are any issues consuming an other schema
			// from CAPI
			// "app-id-3": appBindings{
			// 	Drains:   []string{"syslog://drain-a"},
			// 	Hostname: "org.space.app-name",
			// 	Credentials: appBindingsCredentials{
			// 		Cert:       "",
			// 		PrivateKey: "",
			// 	},
			// },
		}

		c := credentialResults{
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
			Certificates: []binding.Certificate{
				{
					AppIds: []string{"app-id-1"},
					Credentials: binding.AppBindingsCredentials{
						Cert:       "a Cert",
						PrivateKey: "a PrivateKey",
					},
				},
			},
		}

		capi = &fakeCC{
			results:           r,
			credentialResults: c,
		}
		capi.startTLS(capiTestCerts)

		config = app.Config{
			APIURL:             capi.URL,
			APIPollingInterval: 10 * time.Millisecond,
			APIBatchSize:       1000,
			APICAFile:          capiTestCerts.CA(),
			APICertFile:        capiTestCerts.Cert("capi"),
			APIKeyFile:         capiTestCerts.Key("capi"),
			APICommonName:      "capi",
			CacheCAFile:        bindingCacheTestCerts.CA(),
			CacheCertFile:      bindingCacheTestCerts.Cert("binding-cache"),
			CacheKeyFile:       bindingCacheTestCerts.Key("binding-cache"),
			CacheCommonName:    "binding-cache",
			CachePort:          cachePort,
			AggregateDrains:    []string{"syslog://drain-e", "syslog://drain-f"},
		}
		metricClient = metricsHelpers.NewMetricsRegistry()
	})

	AfterEach(func() {
		capi.CloseClientConnections()
		capi.Close()

		cachePort++
	})

	It("debug metrics arn't enabled by default", func() {
		config.MetricsServer.PprofPort = 1234
		sbc = app.NewSyslogBindingCache(config, metricClient, logger)
		go sbc.Run()
		defer sbc.Stop()
		Consistently(metricClient.GetDebugMetricsEnabled()).Should(BeFalse())
		Consistently(func() error {
			_, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", config.MetricsServer.PprofPort))
			return err
		}).ShouldNot(BeNil())
	})

	It("debug metrics can be enabled", func() {
		config.MetricsServer.DebugMetrics = true
		config.MetricsServer.PprofPort = 1235
		sbc = app.NewSyslogBindingCache(config, metricClient, logger)
		go sbc.Run()
		defer sbc.Stop()
		Eventually(metricClient.GetDebugMetricsEnabled).Should(BeTrue())
		var resp *http.Response
		Eventually(func() error {
			var err error
			resp, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", config.MetricsServer.PprofPort))
			return err
		}).Should(BeNil())
		Expect(resp.StatusCode).To(Equal(200))
	})

	It("polls CAPI on an interval for results", func() {
		sbc = app.NewSyslogBindingCache(config, metricClient, logger)
		go sbc.Run()
		defer sbc.Stop()
		Eventually(capi.numRequests).Should(BeNumerically(">=", 2))
	})

	It("has an HTTP endpoint that returns bindings", func() {
		sbc = app.NewSyslogBindingCache(config, metricClient, logger)
		go sbc.Run()
		defer sbc.Stop()
		client := plumbing.NewTLSHTTPClient(
			bindingCacheTestCerts.Cert("binding-cache"),
			bindingCacheTestCerts.Key("binding-cache"),
			bindingCacheTestCerts.CA(),
			"binding-cache",
		)

		addr := fmt.Sprintf("https://localhost:%d/bindings", cachePort)

		var resp *http.Response
		Eventually(func() error {
			var err error
			resp, err = client.Get(addr)
			return err
		}).Should(Succeed())

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())

		var results []binding.Binding
		err = json.Unmarshal(body, &results)
		Expect(err).ToNot(HaveOccurred())

		Expect(results).To(HaveLen(2))
		b := findBinding(results, "app-id-1")
		Expect(b.Drains).To(ConsistOf("syslog://drain-a", "syslog://drain-b"))
		Expect(b.Hostname).To(Equal("org.space.app-name"))
		Expect(b.Credentials).To(Equal(binding.AppBindingsCredentials{
			Cert:       "a Cert",
			PrivateKey: "a PrivateKey",
		}))

		b = findBinding(results, "app-id-2")
		Expect(b.Drains).To(ConsistOf("syslog://drain-c", "syslog://drain-d"))
		Expect(b.Hostname).To(Equal("org.space.app-name-2"))
	})

	It("has an HTTP endpoint that returns aggregate drains", func() {
		sbc = app.NewSyslogBindingCache(config, metricClient, logger)
		go sbc.Run()
		defer sbc.Stop()
		client := plumbing.NewTLSHTTPClient(
			bindingCacheTestCerts.Cert("binding-cache"),
			bindingCacheTestCerts.Key("binding-cache"),
			bindingCacheTestCerts.CA(),
			"binding-cache",
		)

		addr := fmt.Sprintf("https://localhost:%d/aggregate", cachePort)

		var resp *http.Response
		Eventually(func() error {
			var err error
			resp, err = client.Get(addr)
			return err
		}).Should(Succeed())

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())

		var result []binding.Binding
		err = json.Unmarshal(body, &result)
		Expect(err).ToNot(HaveOccurred())

		Expect(result).To(HaveLen(1))
		Expect(result[0].Drains).To(ConsistOf("syslog://drain-e", "syslog://drain-f"))
	})
})

type results map[string]appBindings

type credentialResults struct {
	UpdatedAt    string                `json:"updated_at"`
	Certificates []binding.Certificate `json:"certificates"`
}

type appBindings struct {
	Drains      []string                       `json:"drains"`
	Hostname    string                         `json:"hostname"`
	Credentials binding.AppBindingsCredentials `json:"credentials"`
}

type fakeCC struct {
	*httptest.Server
	countSyslogDrainUrls  int
	calledSyslogDrainUrls int64
	countGetClientCerts   int
	calledGetClientCerts  int64
	withEmptyResult       bool
	results               results
	credentialResults     credentialResults
}

func (f *fakeCC) startTLS(testCerts *testhelper.TestCerts) {
	tlsConfig, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentityFromFile(
			testCerts.Cert("capi"),
			testCerts.Key("capi"),
		),
	).Server(
		tlsconfig.WithClientAuthenticationFromFile(testCerts.CA()),
	)

	Expect(err).ToNot(HaveOccurred())

	mux := http.NewServeMux()
	mux.HandleFunc("/internal/v4/syslog_drain_urls", f.serveSyslogDrainUrls)
	mux.HandleFunc("/internal/v4/get_client_certs", f.serveGetClientCerts)

	f.Server = httptest.NewUnstartedServer(mux)
	f.Server.TLS = tlsConfig
	f.Server.StartTLS()
}

func (f *fakeCC) serveSyslogDrainUrls(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&f.calledSyslogDrainUrls, 1)

	if r.URL.Query().Get("batch_size") != "1000" {
		w.WriteHeader(500)
		return
	}

	f.serveWithResults(w, r)
}

func (f *fakeCC) serveGetClientCerts(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&f.calledGetClientCerts, 1)

	f.serveWithCredentials(w, r)
}

func (f *fakeCC) serveWithResults(w http.ResponseWriter, r *http.Request) {
	resultData, err := json.Marshal(struct {
		Results results `json:"results"`
	}{
		Results: f.results,
	})
	if err != nil {
		w.WriteHeader(500)
		return
	}

	if f.countSyslogDrainUrls > 0 {
		resultData = []byte(`{"results": {}}`)
	}

	w.Write(resultData)
	if f.withEmptyResult {
		f.countSyslogDrainUrls++
	}
}

func (f *fakeCC) serveWithCredentials(w http.ResponseWriter, r *http.Request) {
	td, err := json.Marshal(f.credentialResults)
	if err != nil {
		w.WriteHeader(500)
		return
	}
	w.Write(td)
	f.countGetClientCerts++
}

func (f *fakeCC) numRequests() int64 {
	return atomic.LoadInt64(&f.calledSyslogDrainUrls)
}

func findBinding(bindings []binding.Binding, appID string) binding.Binding {
	for _, b := range bindings {
		if b.AppID == appID {
			return b
		}
	}
	panic(fmt.Sprintf("unable to find binding with appID %s", appID))
}
