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

	logger                     *log.Logger
	bindingRefreshErrorCounter metrics.Counter
	lastBindingCount           metrics.Gauge

	legacy bool
}

type client interface {
	Get(int) (*http.Response, error)
}

type Binding struct {
	AppID    string  `json:"app_id"`
	Drains   []Drain `json:"drains"`
	Hostname string  `json:"hostname"`
}

type Drain struct {
	Url         string      `json:"url"`
	Credentials Credentials `json:"credentials"`
}

type Credentials struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

type Setter interface {
	Set([]Binding)
}

func NewPoller(ac client, pi time.Duration, s Setter, m Metrics, logger *log.Logger, legacy bool) *Poller {
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
		legacy: legacy,
	}
	p.poll()
	return p
}

func (p *Poller) Poll() {
	t := time.NewTicker(p.pollingInterval)

	for range t.C {
		p.poll()
	}
}

func (p *Poller) poll() {
	nextID := 0
	var bindings []Binding
	for {
		resp, err := p.apiClient.Get(nextID)
		if err != nil {
			p.bindingRefreshErrorCounter.Add(1)
			p.logger.Printf("failed to get id %d from CUPS Provider: %s", nextID, err)
			return
		}
		var (
			aResp       apiResponse
			legacyAResp legacyApiResponse
		)
		if p.legacy {
			err = json.NewDecoder(resp.Body).Decode(&legacyAResp)
			if err != nil {
				p.logger.Printf("failed to decode JSON: %s", err)
				return
			}

			bindings = append(bindings, p.legacyToBindings(legacyAResp)...)
			nextID = legacyAResp.NextID
		} else {
			err = json.NewDecoder(resp.Body).Decode(&aResp)
			if err != nil {
				p.logger.Printf("failed to decode JSON: %s", err)
				return
			}

			bindings = append(bindings, p.toBindings(aResp)...)
			nextID = aResp.NextID
		}

		if nextID == 0 {
			break
		}
	}

	p.lastBindingCount.Set(float64(len(bindings)))
	p.store.Set(bindings)
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

func (p *Poller) legacyToBindings(aResp legacyApiResponse) []Binding {
	var bindings []Binding
	for k, v := range aResp.Results {
		var drains []Drain
		for _, d := range v.Drains {
			drains = append(drains, Drain{Url: d})
		}
		bindings = append(bindings, Binding{
			AppID:    k,
			Drains:   drains,
			Hostname: v.Hostname,
		})
	}
	return bindings
}

type apiResponse struct {
	Results map[string]struct {
		Drains   []Drain
		Hostname string
	}
	NextID int `json:"next_id"`
}

type legacyApiResponse struct {
	Results map[string]struct {
		Drains   []string
		Hostname string
	}
	NextID int `json:"next_id"`
}
