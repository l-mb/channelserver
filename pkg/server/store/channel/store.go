package channel

import (
	"net"
	"net/http"
	"strings"

	"github.com/rancher/apiserver/pkg/store/empty"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/channelserver/pkg/config"
	"github.com/rancher/channelserver/pkg/scarf"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
)

type Channel struct {
	empty.Store
	config        *config.Config
	scarf         *scarf.Service
	scarfEndpoint string
}

func New(config *config.Config, scarfEndpoint string) *Channel {
	return &Channel{
		config:        config,
		scarf:         scarf.New(),
		scarfEndpoint: scarfEndpoint,
	}
}

func (c *Channel) List(req *types.APIRequest, _ *types.APISchema) (types.APIObjectList, error) {
	req.Type = "channels"
	resp := types.APIObjectList{}
	for _, channel := range c.config.ChannelsConfig().Channels {
		resp.Objects = append(resp.Objects, types.APIObject{
			Type:   "channel",
			ID:     channel.Name,
			Object: channel,
		})
	}
	return resp, nil
}

func (c *Channel) ByID(apiOp *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	redirect, version, err := c.config.Redirect(id)
	if err != nil {
		return types.APIObject{}, nil
	}
	if redirect != "" {
		c.scarf.Send(c.scarfEndpoint, scarf.Event{
			Channel:         id,
			ResolvedVersion: version,
			LatestVersion:   apiOp.Request.Header.Get("X-SUC-Latest-Version"),
			ClusterID:       apiOp.Request.Header.Get("X-SUC-Cluster-ID"),
			ClientIP:        clientIP(apiOp.Request),
		})
		http.Redirect(apiOp.Response, apiOp.Request, redirect, http.StatusFound)
		return types.APIObject{}, validation.ErrComplete
	}
	return c.Store.ByID(apiOp, schema, id)
}

// clientIP returns the client IP: first X-Forwarded-For hop (channelserver
// runs behind a load balancer), else RemoteAddr.
func clientIP(req *http.Request) string {
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return host
}
