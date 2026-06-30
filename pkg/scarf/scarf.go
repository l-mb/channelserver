// Package scarf reports channel-resolution events to a Scarf gateway.
package scarf

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	channelPlaceholder = "{channel}"
	requestTimeout     = 5 * time.Second
	userAgent          = "channelserver"
)

// Event holds the data reported for a single channel resolution.
type Event struct {
	Channel         string
	ResolvedVersion string
	LatestVersion   string // X-SUC-Latest-Version, set only by system-upgrade-controller
	ClusterID       string // X-SUC-Cluster-ID, set only by system-upgrade-controller
	ClientIP        string
}

// Service sends events to a Scarf gateway endpoint.
type Service struct {
	client *http.Client
}

// New returns a Service with a bounded HTTP client.
func New() *Service {
	return &Service{client: &http.Client{Timeout: requestTimeout}}
}

// Send fires the event to endpointTemplate in the background; no-op when
// empty. Failures are logged, never returned or blocking.
func (s *Service) Send(endpointTemplate string, event Event) {
	if endpointTemplate == "" {
		return
	}
	go func() {
		if err := s.send(endpointTemplate, event); err != nil {
			logrus.Errorf("failed to send Scarf event for channel %q: %v", event.Channel, err)
		} else {
			logrus.Debugf("sent Scarf event for channel %q", event.Channel)
		}
	}()
}

func (s *Service) send(endpointTemplate string, event Event) error {
	endpoint := strings.ReplaceAll(endpointTemplate, channelPlaceholder, url.PathEscape(event.Channel))
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}

	q := u.Query()
	for key, value := range map[string]string{
		"version_resolved": event.ResolvedVersion,
		"version_latest":   event.LatestVersion,
		"cluster_id":       event.ClusterID,
	} {
		if value != "" {
			q.Set(key, value)
		}
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	if event.ClientIP != "" {
		req.Header.Set("X-Scarf-IP", event.ClientIP)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
