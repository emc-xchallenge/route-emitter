package cfroutes

import (
	"encoding/json"

	"github.com/cloudfoundry-incubator/bbs/models"
)

const CF_ROUTER = "cf-router"

type CFRoutes []CFRoute

type CFRoute struct {
	Hostnames []string `json:"hostnames"`
	Port      uint32   `json:"port"`
}

func (c CFRoutes) RoutingInfo() *models.Routes {
	data, _ := json.Marshal(c)
	routingInfo := json.RawMessage(data)
	return &models.Routes{
		CF_ROUTER: &routingInfo,
	}
}

func CFRoutesFromRoutingInfo(routingInfo *models.Routes) (CFRoutes, error) {
	if routingInfo == nil {
		return nil, nil
	}

	routes := *routingInfo
	data, found := routes[CF_ROUTER]
	if !found {
		return nil, nil
	}

	if data == nil {
		return nil, nil
	}

	cfRoutes := CFRoutes{}
	err := json.Unmarshal(*data, &cfRoutes)

	return cfRoutes, err
}
