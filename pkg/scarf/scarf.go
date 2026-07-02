// Package scarf reports channel-resolution events to a Scarf gateway.
package scarf

import (
	"context"
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
	client   *http.Client
	endpoint string
	limiter  *limiter
}

// New returns a Service reporting to endpoint, rate-limiting per cluster
// within window (0 disables), tracking up to size distinct clusters.
func New(ctx context.Context, endpoint string, window time.Duration, size int) *Service {
	s := &Service{
		client:   &http.Client{Timeout: requestTimeout},
		endpoint: endpoint,
	}
	if endpoint != "" {
		s.limiter = newLimiter(window, size)
		go s.limiter.reportLoop(ctx)
	}
	return s
}

// Send fires the event in the background; no-op when endpoint is empty.
// Failures are logged, never returned or blocking.
func (s *Service) Send(event Event) {
	if s.endpoint == "" {
		return
	}
	if !s.limiter.allow(event.ClusterID) {
		logrus.Debugf("rate-limited Scarf event for cluster %q channel %q", event.ClusterID, event.Channel)
		return
	}
	go func() {
		if err := s.send(event); err != nil {
			logrus.Errorf("failed to send Scarf event for channel %q: %v", event.Channel, err)
		} else {
			logrus.Debugf("sent Scarf event for channel %q", event.Channel)
		}
	}()
}

func (s *Service) send(event Event) error {
	endpoint := strings.ReplaceAll(s.endpoint, channelPlaceholder, url.PathEscape(event.Channel))
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
