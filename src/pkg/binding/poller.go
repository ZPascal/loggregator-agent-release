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

	responses                  map[string]Binding
	logger                     *log.Logger
	bindingRefreshErrorCounter metrics.Counter
	lastBindingCount           metrics.Gauge
}

type client interface {
	GetBindings(int) (*http.Response, error)
	GetCredentials(string) (*http.Response, error)
}

type AppBindingsCredentials struct {
	Cert       string `json:"cert"`
	PrivateKey string `json:"private-key"`
}

type Binding struct {
	AppID       string                 `json:"app_id"`
	Drains      []string               `json:"drains"`
	Hostname    string                 `json:"hostname"`
	Credentials AppBindingsCredentials `json:"credentials"`
}

type Certificate struct {
	AppIds      []string               `json:"app_ids"`
	Credentials AppBindingsCredentials `json:"credentials"`
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
		err := p.pollBindings()
		if err != nil {
			continue
		}
		beforeCredentialPoll := time.Now().UTC()
		p.pollCredentials(lastTime)
		lastTime = beforeCredentialPoll

		var bindings []Binding
		for _, v := range p.responses {
			bindings = append(bindings, v)
		}
		p.lastBindingCount.Set(float64(len(bindings)))
		p.store.Set(bindings)
	}
}

func (p *Poller) pollBindings() error {
	nextID := 0
	summaryOfResponces := make(map[string]Binding)
	for {
		resp, err := p.apiClient.GetBindings(nextID)
		if err != nil {
			p.bindingRefreshErrorCounter.Add(1)
			p.logger.Printf("failed to get id %d from CUPS Provider: %s", nextID, err)
			return err
		}
		var aResp apiResponse
		err = json.NewDecoder(resp.Body).Decode(&aResp)
		if err != nil {
			p.logger.Printf("failed to decode JSON: %s", err)
			return err
		}

		for k, v := range aResp.Results {
			summaryOfResponces[k] = Binding{
				AppID:       k,
				Drains:      v.Drains,
				Hostname:    v.Hostname,
				Credentials: AppBindingsCredentials{},
			}
		}

		nextID = aResp.NextID

		if nextID == 0 {
			break
		}
	}
	p.responses = summaryOfResponces
	return nil
}

func (p *Poller) pollCredentials(since time.Time) {
	resp, err := p.apiClient.GetCredentials(since.Format(time.RFC3339))
	if err != nil {
		p.bindingRefreshErrorCounter.Add(1)
		p.logger.Printf("failed to get syslog credentials from CUPS Provider: %s", err)
		return
	}
	var aResp credentialsApiResponse
	err = json.NewDecoder(resp.Body).Decode(&aResp)
	if err != nil {
		p.logger.Printf("failed to decode JSON: %s", err)
		return
	}

	for _, v := range aResp.Certificates {
		for _, a := range v.AppIds {
			if binding, ok := p.responses[a]; ok {
				binding.Credentials = v.Credentials
				p.responses[a] = binding
			}
		}
	}
}

type apiResponse struct {
	Results map[string]struct {
		Drains   []string
		Hostname string
	}
	NextID int `json:"next_id"`
}

type credentialsApiResponse struct {
	UpdatedAt    string        `json:"updated_at"`
	Certificates []Certificate `json:"certificates"`
}
