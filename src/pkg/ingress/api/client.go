package api

import (
	"fmt"
	"net/http"
)

var (
	bindingsPathTemplate    = "%s/internal/v4/syslog_drain_urls?batch_size=%d&next_id=%d"
	credentialsPathTemplate = "%s/internal/v4/get_client_certs?updated_at=%s"
)

type Client struct {
	Client    *http.Client
	Addr      string
	BatchSize int
}

func (w Client) GetBindings(nextID int) (*http.Response, error) {
	return w.Client.Get(fmt.Sprintf(bindingsPathTemplate, w.Addr, w.BatchSize, nextID))
}

func (w Client) GetCredentials(updatedAt string) (*http.Response, error) {
	return w.Client.Get(fmt.Sprintf(credentialsPathTemplate, w.Addr, updatedAt))
}
