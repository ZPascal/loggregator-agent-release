package binding

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	metrics "code.cloudfoundry.org/go-metric-registry"
)

type Poller struct {
	apiClient       client
	pollingInterval time.Duration
	store           Setter
	dataSince       time.Time

	logger                     *log.Logger
	bindingRefreshErrorCounter metrics.Counter
	lastBindingCount           metrics.Gauge
}

type client interface {
	GetBindings(int) (*http.Response, error)
	GetCredentials(string) (*http.Response, error)
}

type Binding struct {
	AppID    string   `json:"app_id"`
	Drains   []string `json:"drains"`
	Hostname string   `json:"hostname"`
}

type Setter interface {
	Set([]Binding)
}

func NewPoller(ac client, pi time.Duration, s Setter, m Metrics, logger *log.Logger, ds time.Time) *Poller {
	p := &Poller{
		apiClient:       ac,
		pollingInterval: pi,
		store:           s,
		logger:          logger,
		bindingRefreshErrorCounter: m.NewCounter(
			"binding_refresh_error",
			"Total number of failed requests to the binding provider.",
		),
		lastBindingCount: m.NewGauge(
			"last_binding_refresh_count",
			"Current number of bindings received from binding provider during last refresh.",
		),
		dataSince: ds,
	}
	p.pollBindings()
	return p
}

func (p *Poller) Poll() {
	t := time.NewTicker(p.pollingInterval)
	// NOTE(panagiotis.xynos): Fetch a year old credentials at first (should be configurable)
	lastTime := p.dataSince.UTC()

	for range t.C {
		p.pollBindings()
		beforeCredentialPoll := time.Now().UTC()
		p.pollCredentials(lastTime)
		lastTime = beforeCredentialPoll
	}
}

func (p *Poller) pollBindings() {
	nextID := 0
	var bindings []Binding
	for {
		resp, err := p.apiClient.GetBindings(nextID)
		if err != nil {
			p.bindingRefreshErrorCounter.Add(1)
			p.logger.Printf("failed to get id %d from CUPS Provider: %s", nextID, err)
			return
		}
		var aResp apiResponse
		err = json.NewDecoder(resp.Body).Decode(&aResp)
		if err != nil {
			p.logger.Printf("failed to decode JSON: %s", err)
			return
		}

		bindings = append(bindings, p.toBindings(aResp)...)
		nextID = aResp.NextID

		if nextID == 0 {
			break
		}
	}

	p.lastBindingCount.Set(float64(len(bindings)))
	p.store.Set(bindings)
}

func (p *Poller) pollCredentials(since time.Time) {
}

func (p *Poller) toBindings(aResp apiResponse) []Binding {
	var bindings []Binding
	for k, v := range aResp.Results {
		bindings = append(bindings, Binding{
			AppID:    k,
			Drains:   v.Drains,
			Hostname: v.Hostname,
		})
	}
	return bindings
}

type apiResponse struct {
	Results map[string]struct {
		Drains   []string
		Hostname string
	}
	NextID int `json:"next_id"`
}
