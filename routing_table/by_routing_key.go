package routing_table

import (
	"errors"

	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/route-emitter/cfroutes"
)

type RoutesByRoutingKey map[RoutingKey]Routes
type EndpointsByRoutingKey map[RoutingKey][]Endpoint

func RoutesByRoutingKeyFromDesireds(desireds []*models.DesiredLRP) RoutesByRoutingKey {
	routesByRoutingKey := RoutesByRoutingKey{}
	for _, desired := range desireds {
		routes, err := cfroutes.CFRoutesFromRoutingInfo(desired.Routes)
		if err == nil && len(routes) > 0 {
			for _, cfRoute := range routes {
				key := RoutingKey{ProcessGuid: desired.ProcessGuid, ContainerPort: cfRoute.Port}
				routesByRoutingKey[key] = Routes{
					Hostnames: cfRoute.Hostnames,
					LogGuid:   desired.LogGuid,
				}
			}
		}
	}

	return routesByRoutingKey
}

func EndpointsByRoutingKeyFromActuals(actuals []*ActualLRPRoutingInfo) EndpointsByRoutingKey {
	endpointsByRoutingKey := EndpointsByRoutingKey{}
	for _, actual := range actuals {
		endpoints, err := EndpointsFromActual(actual)
		if err != nil {
			continue
		}

		for containerPort, endpoint := range endpoints {
			key := RoutingKey{ProcessGuid: actual.ActualLRP.ProcessGuid, ContainerPort: containerPort}
			endpointsByRoutingKey[key] = append(endpointsByRoutingKey[key], endpoint)
		}
	}

	return endpointsByRoutingKey
}

func EndpointsFromActual(actualLRPInfo *ActualLRPRoutingInfo) (map[uint32]Endpoint, error) {
	endpoints := map[uint32]Endpoint{}
	actual := actualLRPInfo.ActualLRP

	if len(actual.Ports) == 0 {
		return endpoints, errors.New("missing ports")
	}

	for _, portMapping := range actual.Ports {
		if portMapping != nil {
			endpoint := Endpoint{
				InstanceGuid:  actual.InstanceGuid,
				Host:          actual.Address,
				Port:          portMapping.HostPort,
				ContainerPort: portMapping.ContainerPort,
				Evacuating:    actualLRPInfo.Evacuating,
			}
			endpoints[portMapping.ContainerPort] = endpoint
		}
	}

	return endpoints, nil
}

func RoutingKeysFromActual(actual *models.ActualLRP) []RoutingKey {
	keys := []RoutingKey{}
	for _, portMapping := range actual.Ports {
		if portMapping != nil {
			keys = append(keys, RoutingKey{ProcessGuid: actual.ProcessGuid, ContainerPort: uint32(portMapping.ContainerPort)})
		}
	}

	return keys
}

func RoutingKeysFromDesired(desired *models.DesiredLRP) []RoutingKey {
	keys := []RoutingKey{}
	for _, containerPort := range desired.Ports {
		keys = append(keys, RoutingKey{ProcessGuid: desired.ProcessGuid, ContainerPort: uint32(containerPort)})
	}

	return keys
}
