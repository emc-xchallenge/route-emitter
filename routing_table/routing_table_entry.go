package routing_table

import "github.com/cloudfoundry-incubator/bbs/models"

type EndpointKey struct {
	InstanceGuid string
	Evacuating   bool
}

type Endpoint struct {
	InstanceGuid    string
	Host            string
	Port            uint32
	ContainerPort   uint32
	Evacuating      bool
	ModificationTag *models.ModificationTag
}

func (e Endpoint) key() EndpointKey {
	return EndpointKey{InstanceGuid: e.InstanceGuid, Evacuating: e.Evacuating}
}

type Routes struct {
	Hostnames       []string
	LogGuid         string
	ModificationTag *models.ModificationTag
}

type RoutableEndpoints struct {
	Hostnames       map[string]struct{}
	Endpoints       map[EndpointKey]Endpoint
	LogGuid         string
	ModificationTag *models.ModificationTag
}

type RoutingKey struct {
	ProcessGuid   string
	ContainerPort uint32
}

func NewRoutableEndpoints() RoutableEndpoints {
	return RoutableEndpoints{
		Hostnames: map[string]struct{}{},
		Endpoints: map[EndpointKey]Endpoint{},
	}
}

func (entry RoutableEndpoints) hasEndpoint(endpoint Endpoint) bool {
	key := endpoint.key()
	_, found := entry.Endpoints[key]
	if !found {
		key.Evacuating = !key.Evacuating
		_, found = entry.Endpoints[key]
	}
	return found
}

func (entry RoutableEndpoints) hasHostname(hostname string) bool {
	_, ok := entry.Hostnames[hostname]
	return ok
}

func (entry RoutableEndpoints) copy() RoutableEndpoints {
	clone := RoutableEndpoints{
		Hostnames:       map[string]struct{}{},
		Endpoints:       map[EndpointKey]Endpoint{},
		LogGuid:         entry.LogGuid,
		ModificationTag: entry.ModificationTag,
	}

	for k, v := range entry.Hostnames {
		clone.Hostnames[k] = v
	}

	for k, v := range entry.Endpoints {
		clone.Endpoints[k] = v
	}

	return clone
}

func (entry RoutableEndpoints) routes() Routes {
	hostnames := make([]string, len(entry.Hostnames))

	i := 0
	for hostname := range entry.Hostnames {
		hostnames[i] = hostname
		i++
	}

	return Routes{
		Hostnames: hostnames,
		LogGuid:   entry.LogGuid,
	}
}

func routesAsMap(routes []string) map[string]struct{} {
	routesMap := map[string]struct{}{}
	for _, route := range routes {
		routesMap[route] = struct{}{}
	}
	return routesMap
}

func EndpointsAsMap(endpoints []Endpoint) map[EndpointKey]Endpoint {
	endpointsMap := map[EndpointKey]Endpoint{}
	for _, endpoint := range endpoints {
		endpointsMap[endpoint.key()] = endpoint
	}
	return endpointsMap
}
