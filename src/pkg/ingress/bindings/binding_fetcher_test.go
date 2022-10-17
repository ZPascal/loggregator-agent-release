package bindings_test

import (
	"errors"
	"time"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/bindings"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BindingFetcher", func() {
	var (
		getter    *SpyGetter
		fetcher   *bindings.BindingFetcher
		metrics   *metricsHelpers.SpyMetricsRegistry
		maxDrains = 3
	)

	BeforeEach(func() {
		getter = &SpyGetter{}
		metrics = metricsHelpers.NewMetricsRegistry()
		fetcher = bindings.NewBindingFetcher(maxDrains, getter, metrics, false)
	})

	BeforeEach(func() {
		getter.bindings = []binding.Binding{
			{
				Url:         "syslog://v3.zzz-not-included.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "9be15160-4845-4f05-b089-40e827ba61f1"}}}},
			},
			{
				Url:         "syslog://v3.other.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "9be15160-4845-4f05-b089-40e827ba61f1"}}}},
			},
			{
				Url:         "syslog://v3.zzz-not-included-again.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "9be15160-4845-4f05-b089-40e827ba61f1"}}}},
			},
			{
				Url:         "https://v3.other.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "9be15160-4845-4f05-b089-40e827ba61f1"}}}},
			},
			{
				Url:         "syslog://v3.other-included.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "9be15160-4845-4f05-b089-40e827ba61f1"}}}},
			},
			{
				Url:         "syslog://v3.zzz-not-included.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "testAppID"}}}},
			},
			{
				Url:         "syslog://v3.other.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "testAppID"}}}},
			}, {
				Url:         "syslog://v3.zzz-not-included-again.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "testAppID"}}}},
			}, {
				Url:         "https://v3.other.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "testAppID"}}}},
			}, {
				Url:         "syslog://v3.other-included.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "testAppID"}}}},
			},
		}

		getter.legacyBindings = []binding.LegacyBinding{
			{
				AppID: "9be15160-4845-4f05-b089-40e827ba61f1",
				Drains: []string{
					"syslog://v3.zzz-not-included.url",
					"syslog://v3.other.url",
					"syslog://v3.zzz-not-included-again.url",
					"https://v3.other.url",
					"syslog://v3.other-included.url"},
				Hostname: "org.space.logspinner",
			},
			{
				AppID: "testAppID",
				Drains: []string{
					"syslog://v3.zzz-not-included.url",
					"syslog://v3.other.url",
					"syslog://v3.zzz-not-included-again.url",
					"https://v3.other.url",
					"syslog://v3.other-included.url",
				},
				Hostname: "org.space.logspinner",
			},
		}

	})

	It("remodels the bindings into molds without filtering them", func() {
		bindings, err := getter.Get()
		molds := fetcher.RemodelBindings(bindings)
		Expect(err).ToNot(HaveOccurred())
		Expect(molds).To(HaveLen(2))
		Expect(molds["testAppID"].Drains).To(HaveLen(5))
		Expect(molds["9be15160-4845-4f05-b089-40e827ba61f1"].Drains).To(HaveLen(5))
	})

	It("returns the max number of v3 bindings by app id", func() {
		fetchedBindings, err := fetcher.FetchBindings()
		Expect(err).ToNot(HaveOccurred())
		Expect(fetchedBindings).To(HaveLen(6))

		appID := "9be15160-4845-4f05-b089-40e827ba61f1"
		otherAppID := "testAppID"
		expectedSyslogBindings := []syslog.Binding{
			{
				AppId:    appID,
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "https://v3.other.url"},
			},
			{
				AppId:    appID,
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "syslog://v3.other-included.url"},
			},
			{
				AppId:    appID,
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "syslog://v3.other.url"},
			},
			{
				AppId:    otherAppID,
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "https://v3.other.url"},
			},
			{
				AppId:    otherAppID,
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "syslog://v3.other-included.url"},
			},
			{
				AppId:    otherAppID,
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "syslog://v3.other.url"},
			},
		}
		Expect(fetchedBindings).To(ConsistOf(expectedSyslogBindings))
	})

	It("returns the max number of v3 bindings by app id in legacy mode", func() {
		fetcher = bindings.NewBindingFetcher(maxDrains, getter, metrics, true)
		fetchedBindings, err := fetcher.FetchBindings()
		Expect(err).ToNot(HaveOccurred())
		Expect(fetchedBindings).To(HaveLen(6))

		appID := "9be15160-4845-4f05-b089-40e827ba61f1"
		otherAppID := "testAppID"
		expectedSyslogBindings := []syslog.Binding{
			{
				AppId:    appID,
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "https://v3.other.url"},
			},
			{
				AppId:    appID,
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "syslog://v3.other-included.url"},
			},
			{
				AppId:    appID,
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "syslog://v3.other.url"},
			},
			{
				AppId:    otherAppID,
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "https://v3.other.url"},
			},
			{
				AppId:    otherAppID,
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "syslog://v3.other-included.url"},
			},
			{
				AppId:    otherAppID,
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "syslog://v3.other.url"},
			},
		}
		Expect(fetchedBindings).To(ConsistOf(expectedSyslogBindings))

	})

	Describe("Binding Type", func() {
		DescribeTable("determines the binding type from the drain url", func(url string, expectedType syslog.BindingType) {
			getter.bindings = []binding.Binding{
				{
					Url:         url,
					Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "9be15160-4845-4f05-b089-40e827ba61f1"}}}},
				},
			}

			fetcher = bindings.NewBindingFetcher(2, getter, metrics, false)
			bindings, err := fetcher.FetchBindings()
			Expect(err).ToNot(HaveOccurred())

			Expect(bindings).To(HaveLen(1))
			Expect(bindings[0].Type).To(Equal(expectedType))
		},
			Entry("default", "syslog://v3.something.url", syslog.BINDING_TYPE_LOG),
			Entry("logs", "syslog://v3.something.url?drain-type=logs", syslog.BINDING_TYPE_LOG),
			Entry("metrics", "syslog://v3.something.url?drain-type=metrics", syslog.BINDING_TYPE_METRIC),
			Entry("all", "syslog://v3.something.url?drain-type=all", syslog.BINDING_TYPE_ALL),
		)
	})

	It("tracks the number of binding refreshes", func() {
		_, err := fetcher.FetchBindings()
		Expect(err).ToNot(HaveOccurred())

		Expect(
			metrics.GetMetric("binding_refresh_count", nil).Value(),
		).To(BeNumerically("==", 1))
	})

	It("tracks the max latency of the requests", func() {
		_, err := fetcher.FetchBindings()
		Expect(err).ToNot(HaveOccurred())

		Expect(
			metrics.GetMetric("latency_for_last_binding_refresh", map[string]string{"unit": "ms"}).Value(),
		).To(BeNumerically(">", 0))
	})

	It("returns all the bindings when there are fewer bindings than the limit", func() {
		getter.bindings = []binding.Binding{
			{
				Url:         "syslog://v3.other.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "9be15160-4845-4f05-b089-40e827ba61f1"}}}},
			},
		}
		fetcher = bindings.NewBindingFetcher(2, getter, metrics, false)
		bindings, err := fetcher.FetchBindings()
		Expect(err).ToNot(HaveOccurred())
		Expect(bindings).To(HaveLen(1))

		Expect(bindings).To(Equal([]syslog.Binding{
			{
				AppId:    "9be15160-4845-4f05-b089-40e827ba61f1",
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "syslog://v3.other.url"},
			},
		}))
	})

	It("returns an error if the Getter returns an error", func() {
		getter.err = errors.New("boom")

		_, err := fetcher.FetchBindings()

		Expect(err).To(MatchError("boom"))
	})
})

type SpyGetter struct {
	bindings       []binding.Binding
	legacyBindings []binding.LegacyBinding
	err            error
}

func (s *SpyGetter) Get() ([]binding.Binding, error) {
	time.Sleep(10 * time.Millisecond)
	return s.bindings, s.err
}

func (s *SpyGetter) LegacyGet() ([]binding.LegacyBinding, error) {
	time.Sleep(10 * time.Millisecond)
	return s.legacyBindings, s.err
}
