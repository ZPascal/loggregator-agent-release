package cache

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
)

type httpGetter interface {
	Get(string) (*http.Response, error)
}

type CacheClient struct {
	cacheAddr string
	h         httpGetter
}

func NewClient(cacheAddr string, h httpGetter) *CacheClient {
	return &CacheClient{
		cacheAddr: cacheAddr,
		h:         h,
	}
}

func (c *CacheClient) Get() ([]binding.Binding, error) {
	return c.get("bindings")
}

func (c *CacheClient) GetAggregate() ([]string, error) {
	return c.getAggregate("aggregate")
}

func (c *CacheClient) get(path string) ([]binding.Binding, error) {
	var bindings []binding.Binding
	resp, err := c.h.Get(fmt.Sprintf("%s/"+path, c.cacheAddr))
	if err != nil {
		return nil, err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected http response from binding cache: %d", resp.StatusCode)
	}

	err = json.NewDecoder(resp.Body).Decode(&bindings)
	if err != nil {
		return nil, err
	}

	return bindings, nil
}

func (c *CacheClient) getAggregate(path string) ([]string, error) {
	var bindings []string
	resp, err := c.h.Get(fmt.Sprintf("%s/"+path, c.cacheAddr))
	if err != nil {
		return nil, err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected http response from binding cache: %d", resp.StatusCode)
	}

	err = json.NewDecoder(resp.Body).Decode(&bindings)
	if err != nil {
		return nil, err
	}

	return bindings, nil
}
