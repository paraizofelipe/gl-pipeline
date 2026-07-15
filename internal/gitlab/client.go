package gitlab

import (
	"sync"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

var (
	restClient   *gitlab.Client
	restClientMu sync.Mutex
)

// SetClient overrides the cached REST client. Used by tests to inject a client
// pointed at a mock server.
func SetClient(rest *gitlab.Client) {
	restClientMu.Lock()
	restClient = rest
	restClientMu.Unlock()
}

// RESTClient returns a cached GitLab REST client, building one from the
// resolved AuthConfig on first use.
func RESTClient() (*gitlab.Client, error) {
	restClientMu.Lock()
	defer restClientMu.Unlock()
	if restClient != nil {
		return restClient, nil
	}

	auth, err := LoadAuthConfig()
	if err != nil {
		return nil, err
	}

	newClient := gitlab.NewClient
	if auth.IsJobToken {
		newClient = gitlab.NewJobClient
	}
	c, err := newClient(auth.Token, gitlab.WithBaseURL(baseURL(auth)+"/api/v4"))
	if err != nil {
		return nil, err
	}
	restClient = c
	return restClient, nil
}

func baseURL(auth AuthConfig) string {
	return auth.APIProtocol + "://" + auth.Host
}
