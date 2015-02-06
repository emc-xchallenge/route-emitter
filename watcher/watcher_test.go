package watcher_test

import (
	"errors"
	"os"

	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/receptor/fake_receptor"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
	"github.com/tedsuo/ifrit"

	"github.com/cloudfoundry-incubator/route-emitter/cfroutes"
	"github.com/cloudfoundry-incubator/route-emitter/nats_emitter/fake_nats_emitter"
	"github.com/cloudfoundry-incubator/route-emitter/routing_table"
	"github.com/cloudfoundry-incubator/route-emitter/routing_table/fake_routing_table"
	. "github.com/cloudfoundry-incubator/route-emitter/watcher"
)

const logGuid = "some-log-guid"

var _ = Describe("Watcher", func() {
	const (
		expectedProcessGuid             = "process-guid"
		expectedInstanceGuid            = "instance-guid"
		expectedHost                    = "1.1.1.1"
		expectedExternalPort            = 11000
		expectedContainerPort           = uint16(11)
		expectedAdditionalContainerPort = uint16(12)
	)
	var expectedRoutes = []string{"route-1", "route-2"}
	var expectedRoutingKey = routing_table.RoutingKey{
		ProcessGuid:   expectedProcessGuid,
		ContainerPort: expectedContainerPort,
	}
	var expectedAdditionalRoutingKey = routing_table.RoutingKey{
		ProcessGuid:   expectedProcessGuid,
		ContainerPort: expectedAdditionalContainerPort,
	}

	var (
		receptorClient *fake_receptor.FakeClient
		table          *fake_routing_table.FakeRoutingTable
		emitter        *fake_nats_emitter.FakeNATSEmitter

		dummyMessagesToEmit routing_table.MessagesToEmit

		watcher *Watcher

		process ifrit.Process
	)

	BeforeEach(func() {
		receptorClient = new(fake_receptor.FakeClient)
		table = &fake_routing_table.FakeRoutingTable{}
		emitter = &fake_nats_emitter.FakeNATSEmitter{}
		logger := lagertest.NewTestLogger("test")

		dummyEndpoint := routing_table.Endpoint{InstanceGuid: expectedInstanceGuid, Host: expectedHost, Port: expectedContainerPort}
		dummyMessage := routing_table.RegistryMessageFor(dummyEndpoint, routing_table.Routes{URIs: []string{"foo.com", "bar.com"}, LogGuid: logGuid})
		dummyMessagesToEmit = routing_table.MessagesToEmit{
			RegistrationMessages: []routing_table.RegistryMessage{dummyMessage},
		}

		watcher = NewWatcher(receptorClient, table, emitter, logger)
	})

	JustBeforeEach(func() {
		process = ifrit.Invoke(watcher)
	})

	AfterEach(func() {
		process.Signal(os.Interrupt)
		Eventually(process.Wait()).Should(Receive())
	})

	Describe("Desired LRP changes", func() {
		Context("when a create event occurs", func() {
			BeforeEach(func() {
				table.SetRoutesReturns(dummyMessagesToEmit)

				eventSource := new(fake_receptor.FakeEventSource)
				receptorClient.SubscribeToEventsReturns(eventSource, nil)

				desiredLRP := receptor.DesiredLRPResponse{
					Action: &models.RunAction{
						Path: "ls",
					},
					Domain:      "tests",
					ProcessGuid: expectedProcessGuid,
					Ports:       []uint16{expectedContainerPort},
					Routes:      cfroutes.CFRoutes{{Hostnames: expectedRoutes, Port: expectedContainerPort}}.RoutingInfo(),
					LogGuid:     logGuid,
				}

				eventSource.NextStub = func() (receptor.Event, error) {
					if eventSource.NextCallCount() == 1 {
						return receptor.NewDesiredLRPCreatedEvent(desiredLRP), nil
					} else {
						return nil, nil
					}
				}
			})

			It("should set the routes on the table", func() {
				Eventually(table.SetRoutesCallCount).Should(Equal(1))
				key, routes := table.SetRoutesArgsForCall(0)
				Ω(key).Should(Equal(expectedRoutingKey))
				Ω(routes).Should(Equal(routing_table.Routes{URIs: expectedRoutes, LogGuid: logGuid}))
			})

			It("passes a 'routes registered' counter to Emit", func() {
				Eventually(emitter.EmitCallCount).Should(Equal(1))
				_, registerCounter, _ := emitter.EmitArgsForCall(0)
				Expect(string(*registerCounter)).To(Equal("RoutesRegistered"))
			})

			It("passes a 'routes unregistered' counter to Emit", func() {
				Eventually(emitter.EmitCallCount).Should(Equal(1))
				_, _, unregisterCounter := emitter.EmitArgsForCall(0)
				Expect(string(*unregisterCounter)).To(Equal("RoutesUnregistered"))
			})

			It("should emit whatever the table tells it to emit", func() {
				Eventually(emitter.EmitCallCount).Should(Equal(1))
				messagesToEmit, _, _ := emitter.EmitArgsForCall(0)
				Ω(messagesToEmit).Should(Equal(dummyMessagesToEmit))
			})
		})

		Context("when a change event occurs", func() {
			BeforeEach(func() {
				table.SetRoutesReturns(dummyMessagesToEmit)

				eventSource := new(fake_receptor.FakeEventSource)
				receptorClient.SubscribeToEventsReturns(eventSource, nil)

				originalDesiredLRP := receptor.DesiredLRPResponse{
					Action: &models.RunAction{
						Path: "ls",
					},
					Domain:      "tests",
					ProcessGuid: expectedProcessGuid,
					LogGuid:     logGuid,
					Ports:       []uint16{expectedContainerPort},
				}
				changedDesiredLRP := receptor.DesiredLRPResponse{
					Action: &models.RunAction{
						Path: "ls",
					},
					Domain:      "tests",
					ProcessGuid: expectedProcessGuid,
					LogGuid:     logGuid,
					Ports:       []uint16{expectedContainerPort},
					Routes:      cfroutes.CFRoutes{{Hostnames: expectedRoutes, Port: expectedContainerPort}}.RoutingInfo(),
				}

				eventSource.NextStub = func() (receptor.Event, error) {
					if eventSource.NextCallCount() == 1 {
						return receptor.NewDesiredLRPChangedEvent(
							originalDesiredLRP,
							changedDesiredLRP,
						), nil
					} else {
						return nil, nil
					}
				}
			})

			It("should set the routes on the table", func() {
				Eventually(table.SetRoutesCallCount).Should(Equal(1))
				key, routes := table.SetRoutesArgsForCall(0)
				Ω(key).Should(Equal(expectedRoutingKey))
				Ω(routes).Should(Equal(routing_table.Routes{URIs: expectedRoutes, LogGuid: logGuid}))
			})

			It("passes a 'routes registered' counter to Emit", func() {
				Eventually(emitter.EmitCallCount).Should(Equal(1))
				_, registerCounter, _ := emitter.EmitArgsForCall(0)
				Expect(string(*registerCounter)).To(Equal("RoutesRegistered"))
			})

			It("passes a 'routes unregistered' counter to Emit", func() {
				Eventually(emitter.EmitCallCount).Should(Equal(1))
				_, _, unregisterCounter := emitter.EmitArgsForCall(0)
				Expect(string(*unregisterCounter)).To(Equal("RoutesUnregistered"))
			})

			It("should emit whatever the table tells it to emit", func() {
				Eventually(emitter.EmitCallCount).Should(Equal(1))
				messagesToEmit, _, _ := emitter.EmitArgsForCall(0)
				Ω(messagesToEmit).Should(Equal(dummyMessagesToEmit))
			})
		})

		Context("adding a host to an existing port", func() {
			Context("when there is no port for the route", func() {
				BeforeEach(func() {
					table.SetRoutesReturns(dummyMessagesToEmit)

					eventSource := new(fake_receptor.FakeEventSource)
					receptorClient.SubscribeToEventsReturns(eventSource, nil)

					originalDesiredLRP := receptor.DesiredLRPResponse{
						Action: &models.RunAction{
							Path: "ls",
						},
						Domain:      "tests",
						ProcessGuid: expectedProcessGuid,
						LogGuid:     logGuid,
						Ports:       []uint16{expectedContainerPort},
						Routes: cfroutes.CFRoutes{
							{Hostnames: []string{expectedRoutes[0]}, Port: expectedContainerPort},
						}.RoutingInfo(),
					}

					changedDesiredLRP := receptor.DesiredLRPResponse{
						Action: &models.RunAction{
							Path: "ls",
						},
						Domain:      "tests",
						ProcessGuid: expectedProcessGuid,
						LogGuid:     logGuid,
						Ports:       []uint16{expectedContainerPort},
						Routes: cfroutes.CFRoutes{
							{Hostnames: []string{expectedRoutes[0]}, Port: expectedContainerPort},
							{Hostnames: []string{expectedRoutes[1]}, Port: expectedAdditionalContainerPort},
						}.RoutingInfo(),
					}

					eventSource.NextStub = func() (receptor.Event, error) {
						if eventSource.NextCallCount() == 1 {
							return receptor.NewDesiredLRPChangedEvent(
								originalDesiredLRP,
								changedDesiredLRP,
							), nil
						} else {
							return nil, nil
						}
					}
				})

				It("should set the routes on the table", func() {
					Eventually(table.SetRoutesCallCount).Should(Equal(1))

					key, routes := table.SetRoutesArgsForCall(0)

					Ω(key).Should(Equal(expectedRoutingKey))
					Ω(routes).Should(Equal(routing_table.Routes{
						URIs:    []string{expectedRoutes[0]},
						LogGuid: logGuid,
					}))
				})

				It("passes a 'routes registered' counter to Emit", func() {
					Eventually(emitter.EmitCallCount).Should(Equal(1))
					_, registerCounter, _ := emitter.EmitArgsForCall(0)
					Expect(string(*registerCounter)).To(Equal("RoutesRegistered"))
				})

				It("passes a 'routes unregistered' counter to Emit", func() {
					Eventually(emitter.EmitCallCount).Should(Equal(1))
					_, _, unregisterCounter := emitter.EmitArgsForCall(0)
					Expect(string(*unregisterCounter)).To(Equal("RoutesUnregistered"))
				})

				It("should emit whatever the table tells it to emit", func() {
					Eventually(emitter.EmitCallCount).Should(Equal(1))
					messagesToEmit, _, _ := emitter.EmitArgsForCall(0)
					Ω(messagesToEmit).Should(Equal(dummyMessagesToEmit))
				})
			})

			Context("when there is a port for the route", func() {
				BeforeEach(func() {
					table.SetRoutesReturns(dummyMessagesToEmit)

					eventSource := new(fake_receptor.FakeEventSource)
					receptorClient.SubscribeToEventsReturns(eventSource, nil)

					originalDesiredLRP := receptor.DesiredLRPResponse{
						Action: &models.RunAction{
							Path: "ls",
						},
						Domain:      "tests",
						ProcessGuid: expectedProcessGuid,
						LogGuid:     logGuid,
						Ports:       []uint16{expectedContainerPort},
						Routes: cfroutes.CFRoutes{
							{Hostnames: []string{expectedRoutes[0]}, Port: expectedContainerPort},
						}.RoutingInfo(),
					}

					changedDesiredLRP := receptor.DesiredLRPResponse{
						Action: &models.RunAction{
							Path: "ls",
						},
						Domain:      "tests",
						ProcessGuid: expectedProcessGuid,
						LogGuid:     logGuid,
						Ports:       []uint16{expectedContainerPort, expectedAdditionalContainerPort},
						Routes: cfroutes.CFRoutes{
							{Hostnames: []string{expectedRoutes[0]}, Port: expectedContainerPort},
							{Hostnames: []string{expectedRoutes[1]}, Port: expectedAdditionalContainerPort},
						}.RoutingInfo(),
					}

					eventSource.NextStub = func() (receptor.Event, error) {
						if eventSource.NextCallCount() == 1 {
							return receptor.NewDesiredLRPChangedEvent(
								originalDesiredLRP,
								changedDesiredLRP,
							), nil
						} else {
							return nil, nil
						}
					}
				})

				It("should set the routes on the table", func() {
					Eventually(table.SetRoutesCallCount).Should(Equal(2))

					routesByRoutingKey := map[routing_table.RoutingKey]routing_table.Routes{}
					for i := 0; i < 2; i++ {
						key, routes := table.SetRoutesArgsForCall(i)
						routesByRoutingKey[key] = routes
					}

					Ω(routesByRoutingKey).Should(Equal(map[routing_table.RoutingKey]routing_table.Routes{
						expectedRoutingKey: {
							URIs:    []string{expectedRoutes[0]},
							LogGuid: logGuid,
						},
						expectedAdditionalRoutingKey: {
							URIs:    []string{expectedRoutes[1]},
							LogGuid: logGuid,
						},
					}))
				})

				It("passes a 'routes registered' counter to Emit", func() {
					Eventually(emitter.EmitCallCount).Should(Equal(2))
					_, registerCounter, _ := emitter.EmitArgsForCall(0)
					Expect(string(*registerCounter)).To(Equal("RoutesRegistered"))
				})

				It("passes a 'routes unregistered' counter to Emit", func() {
					Eventually(emitter.EmitCallCount).Should(Equal(2))
					_, _, unregisterCounter := emitter.EmitArgsForCall(0)
					Expect(string(*unregisterCounter)).To(Equal("RoutesUnregistered"))
				})

				It("should emit whatever the table tells it to emit", func() {
					Eventually(emitter.EmitCallCount).Should(Equal(2))
					messagesToEmit, _, _ := emitter.EmitArgsForCall(0)
					Ω(messagesToEmit).Should(Equal(dummyMessagesToEmit))
				})
			})

			Context("when there is a port for the route", func() {
				BeforeEach(func() {
					table.SetRoutesReturns(dummyMessagesToEmit)

					eventSource := new(fake_receptor.FakeEventSource)
					receptorClient.SubscribeToEventsReturns(eventSource, nil)

					originalDesiredLRP := receptor.DesiredLRPResponse{
						Action: &models.RunAction{
							Path: "ls",
						},
						Domain:      "tests",
						ProcessGuid: expectedProcessGuid,
						LogGuid:     logGuid,
						Ports:       []uint16{expectedContainerPort},
						Routes: cfroutes.CFRoutes{
							{Hostnames: []string{expectedRoutes[0]}, Port: expectedContainerPort},
						}.RoutingInfo(),
					}

					changedDesiredLRP := receptor.DesiredLRPResponse{
						Action: &models.RunAction{
							Path: "ls",
						},
						Domain:      "tests",
						ProcessGuid: expectedProcessGuid,
						LogGuid:     logGuid,
						Ports:       []uint16{expectedContainerPort, expectedAdditionalContainerPort},
						Routes: cfroutes.CFRoutes{
							{Hostnames: []string{expectedRoutes[0]}, Port: expectedContainerPort},
							{Hostnames: []string{expectedRoutes[1]}, Port: expectedAdditionalContainerPort},
						}.RoutingInfo(),
					}

					eventSource.NextStub = func() (receptor.Event, error) {
						if eventSource.NextCallCount() == 1 {
							return receptor.NewDesiredLRPChangedEvent(
								originalDesiredLRP,
								changedDesiredLRP,
							), nil
						} else {
							return nil, nil
						}
					}
				})

				It("should set the routes on the table", func() {
					Eventually(table.SetRoutesCallCount).Should(Equal(2))

					routesByRoutingKey := map[routing_table.RoutingKey]routing_table.Routes{}
					for i := 0; i < 2; i++ {
						key, routes := table.SetRoutesArgsForCall(i)
						routesByRoutingKey[key] = routes
					}

					Ω(routesByRoutingKey).Should(Equal(map[routing_table.RoutingKey]routing_table.Routes{
						expectedRoutingKey: {
							URIs:    []string{expectedRoutes[0]},
							LogGuid: logGuid,
						},
						expectedAdditionalRoutingKey: {
							URIs:    []string{expectedRoutes[1]},
							LogGuid: logGuid,
						},
					}))
				})

				It("passes a 'routes registered' counter to Emit", func() {
					Eventually(emitter.EmitCallCount).Should(Equal(2))
					_, registerCounter, _ := emitter.EmitArgsForCall(0)
					Expect(string(*registerCounter)).To(Equal("RoutesRegistered"))
				})

				It("passes a 'routes unregistered' counter to Emit", func() {
					Eventually(emitter.EmitCallCount).Should(Equal(2))
					_, _, unregisterCounter := emitter.EmitArgsForCall(0)
					Expect(string(*unregisterCounter)).To(Equal("RoutesUnregistered"))
				})

				It("should emit whatever the table tells it to emit", func() {
					Eventually(emitter.EmitCallCount).Should(Equal(2))
					messagesToEmit, _, _ := emitter.EmitArgsForCall(0)
					Ω(messagesToEmit).Should(Equal(dummyMessagesToEmit))
				})
			})
		})

		Context("removing a port from an existing host", func() {
		})

		Context("adding a host to an existing port", func() {
		})

		Context("removing a host from an existing port", func() {
		})

		Context("when a delete event occurs", func() {
			BeforeEach(func() {
				table.RemoveRoutesReturns(dummyMessagesToEmit)

				eventSource := new(fake_receptor.FakeEventSource)
				receptorClient.SubscribeToEventsReturns(eventSource, nil)

				desiredLRP := receptor.DesiredLRPResponse{
					Action: &models.RunAction{
						Path: "ls",
					},
					Domain:      "tests",
					ProcessGuid: expectedProcessGuid,
					Ports:       []uint16{expectedContainerPort},
					Routes:      cfroutes.CFRoutes{{Hostnames: expectedRoutes, Port: expectedContainerPort}}.RoutingInfo(),
					LogGuid:     logGuid,
				}

				eventSource.NextStub = func() (receptor.Event, error) {
					if eventSource.NextCallCount() == 1 {
						return receptor.NewDesiredLRPRemovedEvent(desiredLRP), nil
					} else {
						return nil, nil
					}
				}
			})

			It("should remove the routes from the table", func() {
				Eventually(table.RemoveRoutesCallCount).Should(Equal(1))
				key := table.RemoveRoutesArgsForCall(0)
				Ω(key).Should(Equal(expectedRoutingKey))
			})

			It("should emit whatever the table tells it to emit", func() {
				Eventually(emitter.EmitCallCount).Should(Equal(1))
				messagesToEmit, _, _ := emitter.EmitArgsForCall(0)
				Ω(messagesToEmit).Should(Equal(dummyMessagesToEmit))
			})
		})
	})

	Describe("Actual LRP changes", func() {
		Context("when a create event occurs", func() {
			Context("when the resulting LRP is in the RUNNING state", func() {
				BeforeEach(func() {
					table.AddOrUpdateEndpointReturns(dummyMessagesToEmit)

					eventSource := new(fake_receptor.FakeEventSource)
					receptorClient.SubscribeToEventsReturns(eventSource, nil)

					actualLRP := receptor.ActualLRPResponse{
						ProcessGuid:  expectedProcessGuid,
						Index:        1,
						Domain:       "domain",
						InstanceGuid: expectedInstanceGuid,
						CellID:       "cell-id",
						Address:      expectedHost,
						Ports: []receptor.PortMapping{
							{ContainerPort: expectedContainerPort, HostPort: expectedExternalPort},
							{ContainerPort: expectedAdditionalContainerPort, HostPort: expectedExternalPort},
						},
						State: receptor.ActualLRPStateRunning,
					}

					eventSource.NextStub = func() (receptor.Event, error) {
						if eventSource.NextCallCount() == 1 {
							return receptor.NewActualLRPCreatedEvent(actualLRP), nil
						} else {
							return nil, nil
						}
					}
				})

				It("should add/update the endpoint on the table", func() {
					Eventually(table.AddOrUpdateEndpointCallCount).Should(Equal(2))

					endpointsByRoutingKey := map[routing_table.RoutingKey]routing_table.Endpoint{}
					for i := 0; i < 2; i++ {
						key, endpoint := table.AddOrUpdateEndpointArgsForCall(i)
						endpointsByRoutingKey[key] = endpoint
					}

					Ω(endpointsByRoutingKey).Should(Equal(map[routing_table.RoutingKey]routing_table.Endpoint{
						expectedRoutingKey: {
							InstanceGuid:  expectedInstanceGuid,
							Host:          expectedHost,
							Port:          expectedExternalPort,
							ContainerPort: expectedContainerPort,
						},
						expectedAdditionalRoutingKey: {
							InstanceGuid:  expectedInstanceGuid,
							Host:          expectedHost,
							Port:          expectedExternalPort,
							ContainerPort: expectedAdditionalContainerPort,
						},
					}))
				})

				It("should emit whatever the table tells it to emit", func() {
					Eventually(emitter.EmitCallCount).Should(Equal(2))
					messagesToEmit, _, _ := emitter.EmitArgsForCall(0)
					Ω(messagesToEmit).Should(Equal(dummyMessagesToEmit))
				})

				It("passes a 'routes registered' counter to Emit", func() {
					Eventually(emitter.EmitCallCount).Should(Equal(2))
					_, registerCounter, _ := emitter.EmitArgsForCall(0)
					Expect(string(*registerCounter)).To(Equal("RoutesRegistered"))
				})

				It("passes a 'routes unregistered' counter to Emit", func() {
					Eventually(emitter.EmitCallCount).Should(Equal(2))
					_, _, unregisterCounter := emitter.EmitArgsForCall(0)
					Expect(string(*unregisterCounter)).To(Equal("RoutesUnregistered"))
				})
			})

			Context("when the resulting LRP is not in the RUNNING state", func() {
				BeforeEach(func() {
					eventSource := new(fake_receptor.FakeEventSource)
					receptorClient.SubscribeToEventsReturns(eventSource, nil)

					actualLRP := receptor.ActualLRPResponse{
						ProcessGuid:  expectedProcessGuid,
						Index:        1,
						Domain:       "domain",
						InstanceGuid: expectedInstanceGuid,
						CellID:       "cell-id",
						Address:      expectedHost,
						Ports: []receptor.PortMapping{
							{ContainerPort: expectedContainerPort, HostPort: expectedExternalPort},
						},
						State: receptor.ActualLRPStateUnclaimed,
					}

					eventSource.NextStub = func() (receptor.Event, error) {
						if eventSource.NextCallCount() == 1 {
							return receptor.NewActualLRPCreatedEvent(actualLRP), nil
						} else {
							return nil, nil
						}
					}
				})

				It("doesn't add/update the endpoint on the table", func() {
					Consistently(table.AddOrUpdateEndpointCallCount).Should(Equal(0))
				})

				It("doesn't emit", func() {
					Eventually(emitter.EmitCallCount).Should(Equal(0))
				})
			})
		})

		Context("when a change event occurs", func() {
			Context("when the resulting LRP is in the RUNNING state", func() {
				BeforeEach(func() {
					table.AddOrUpdateEndpointReturns(dummyMessagesToEmit)

					eventSource := new(fake_receptor.FakeEventSource)
					receptorClient.SubscribeToEventsReturns(eventSource, nil)

					beforeActualLRP := receptor.ActualLRPResponse{
						ProcessGuid:  expectedProcessGuid,
						Index:        1,
						Domain:       "domain",
						InstanceGuid: expectedInstanceGuid,
						CellID:       "cell-id",
						State:        receptor.ActualLRPStateClaimed,
					}
					afterActualLRP := receptor.ActualLRPResponse{
						ProcessGuid:  expectedProcessGuid,
						Index:        1,
						Domain:       "domain",
						InstanceGuid: expectedInstanceGuid,
						CellID:       "cell-id",
						Address:      expectedHost,
						Ports: []receptor.PortMapping{
							{ContainerPort: expectedContainerPort, HostPort: expectedExternalPort},
						},
						State: receptor.ActualLRPStateRunning,
					}

					eventSource.NextStub = func() (receptor.Event, error) {
						if eventSource.NextCallCount() == 1 {
							return receptor.NewActualLRPChangedEvent(beforeActualLRP, afterActualLRP), nil
						} else {
							return nil, nil
						}
					}
				})

				It("should add/update the endpoint on the table", func() {
					Eventually(table.AddOrUpdateEndpointCallCount).Should(Equal(1))
					key, endpoint := table.AddOrUpdateEndpointArgsForCall(0)
					Ω(key).Should(Equal(expectedRoutingKey))
					Ω(endpoint).Should(Equal(routing_table.Endpoint{
						InstanceGuid:  expectedInstanceGuid,
						Host:          expectedHost,
						Port:          expectedExternalPort,
						ContainerPort: expectedContainerPort,
					}))
				})

				It("should emit whatever the table tells it to emit", func() {
					Eventually(emitter.EmitCallCount).Should(Equal(1))
					messagesToEmit, _, _ := emitter.EmitArgsForCall(0)
					Ω(messagesToEmit).Should(Equal(dummyMessagesToEmit))
				})

				It("passes a 'routes registered' counter to Emit", func() {
					Eventually(emitter.EmitCallCount).Should(Equal(1))
					_, registerCounter, _ := emitter.EmitArgsForCall(0)
					Expect(string(*registerCounter)).To(Equal("RoutesRegistered"))
				})

				It("passes a 'routes unregistered' counter to Emit", func() {
					Eventually(emitter.EmitCallCount).Should(Equal(1))
					_, _, unregisterCounter := emitter.EmitArgsForCall(0)
					Expect(string(*unregisterCounter)).To(Equal("RoutesUnregistered"))
				})
			})

			Context("when the resulting LRP transitions away form the RUNNING state", func() {
				BeforeEach(func() {
					table.RemoveEndpointReturns(dummyMessagesToEmit)

					eventSource := new(fake_receptor.FakeEventSource)
					receptorClient.SubscribeToEventsReturns(eventSource, nil)

					beforeActualLRP := receptor.ActualLRPResponse{
						ProcessGuid:  expectedProcessGuid,
						Index:        1,
						Domain:       "domain",
						InstanceGuid: expectedInstanceGuid,
						CellID:       "cell-id",
						Address:      expectedHost,
						Ports: []receptor.PortMapping{
							{ContainerPort: expectedContainerPort, HostPort: expectedExternalPort},
						},
						State: receptor.ActualLRPStateRunning,
					}
					afterActualLRP := receptor.ActualLRPResponse{
						ProcessGuid: expectedProcessGuid,
						Index:       1,
						Domain:      "domain",
						State:       receptor.ActualLRPStateUnclaimed,
					}

					eventSource.NextStub = func() (receptor.Event, error) {
						if eventSource.NextCallCount() == 1 {
							return receptor.NewActualLRPChangedEvent(beforeActualLRP, afterActualLRP), nil
						} else {
							return nil, nil
						}
					}
				})

				It("should remove the endpoint from the table", func() {
					Eventually(table.RemoveEndpointCallCount).Should(Equal(1))
					key, endpoint := table.RemoveEndpointArgsForCall(0)
					Ω(key).Should(Equal(expectedRoutingKey))
					Ω(endpoint).Should(Equal(routing_table.Endpoint{
						InstanceGuid:  expectedInstanceGuid,
						Host:          expectedHost,
						Port:          expectedExternalPort,
						ContainerPort: expectedContainerPort,
					}))
				})

				It("should emit whatever the table tells it to emit", func() {
					Eventually(emitter.EmitCallCount).Should(Equal(1))
					messagesToEmit, _, _ := emitter.EmitArgsForCall(0)
					Ω(messagesToEmit).Should(Equal(dummyMessagesToEmit))
				})
			})

			Context("when the endpoint neither starts nor ends in the RUNNING state", func() {
				BeforeEach(func() {
					eventSource := new(fake_receptor.FakeEventSource)
					receptorClient.SubscribeToEventsReturns(eventSource, nil)

					beforeActualLRP := receptor.ActualLRPResponse{
						ProcessGuid: expectedProcessGuid,
						Index:       1,
						Domain:      "domain",
						State:       receptor.ActualLRPStateUnclaimed,
					}
					afterActualLRP := receptor.ActualLRPResponse{
						ProcessGuid:  expectedProcessGuid,
						Index:        1,
						Domain:       "domain",
						InstanceGuid: expectedInstanceGuid,
						CellID:       "cell-id",
						State:        receptor.ActualLRPStateClaimed,
					}

					eventSource.NextStub = func() (receptor.Event, error) {
						if eventSource.NextCallCount() == 1 {
							return receptor.NewActualLRPChangedEvent(beforeActualLRP, afterActualLRP), nil
						} else {
							return nil, nil
						}
					}
				})

				It("should not remove the endpoint", func() {
					Consistently(table.RemoveEndpointCallCount).Should(BeZero())
				})

				It("should not add or update the endpoint", func() {
					Consistently(table.AddOrUpdateEndpointCallCount).Should(BeZero())
				})

				It("should not emit anything", func() {
					Consistently(emitter.EmitCallCount).Should(BeZero())
				})
			})
		})

		Context("when a delete event occurs", func() {
			Context("when the actual is in the RUNNING state", func() {
				BeforeEach(func() {
					table.RemoveEndpointReturns(dummyMessagesToEmit)

					eventSource := new(fake_receptor.FakeEventSource)
					receptorClient.SubscribeToEventsReturns(eventSource, nil)

					actualLRP := receptor.ActualLRPResponse{
						ProcessGuid:  expectedProcessGuid,
						Index:        1,
						Domain:       "domain",
						InstanceGuid: expectedInstanceGuid,
						CellID:       "cell-id",
						Address:      expectedHost,
						Ports: []receptor.PortMapping{
							{ContainerPort: expectedContainerPort, HostPort: expectedExternalPort},
						},
						State: receptor.ActualLRPStateRunning,
					}

					eventSource.NextStub = func() (receptor.Event, error) {
						if eventSource.NextCallCount() == 1 {
							return receptor.NewActualLRPRemovedEvent(actualLRP), nil
						} else {
							return nil, nil
						}
					}
				})

				It("should remove the endpoint from the table", func() {
					Eventually(table.RemoveEndpointCallCount).Should(Equal(1))
					key, endpoint := table.RemoveEndpointArgsForCall(0)
					Ω(key).Should(Equal(expectedRoutingKey))
					Ω(endpoint).Should(Equal(routing_table.Endpoint{
						InstanceGuid:  expectedInstanceGuid,
						Host:          expectedHost,
						Port:          expectedExternalPort,
						ContainerPort: expectedContainerPort,
					}))
				})

				It("should emit whatever the table tells it to emit", func() {
					Eventually(emitter.EmitCallCount).Should(Equal(1))
					messagesToEmit, _, _ := emitter.EmitArgsForCall(0)
					Ω(messagesToEmit).Should(Equal(dummyMessagesToEmit))
				})
			})

			Context("when the actual is not in the RUNNING state", func() {
				BeforeEach(func() {
					eventSource := new(fake_receptor.FakeEventSource)
					receptorClient.SubscribeToEventsReturns(eventSource, nil)

					actualLRP := receptor.ActualLRPResponse{
						ProcessGuid: expectedProcessGuid,
						Index:       1,
						Domain:      "domain",
						State:       receptor.ActualLRPStateCrashed,
					}

					eventSource.NextStub = func() (receptor.Event, error) {
						if eventSource.NextCallCount() == 1 {
							return receptor.NewActualLRPRemovedEvent(actualLRP), nil
						} else {
							return nil, nil
						}
					}
				})

				It("doesn't remove the endpoint from the table", func() {
					Consistently(table.RemoveEndpointCallCount).Should(Equal(0))
				})

				It("doesn't emit", func() {
					Consistently(emitter.EmitCallCount).Should(Equal(0))
				})
			})
		})
	})

	Describe("Unrecognized events", func() {
		BeforeEach(func() {
			eventSource := new(fake_receptor.FakeEventSource)
			receptorClient.SubscribeToEventsReturns(eventSource, nil)

			eventSource.NextStub = func() (receptor.Event, error) {
				if eventSource.NextCallCount() == 1 {
					return unrecognizedEvent{}, nil
				} else {
					return nil, nil
				}
			}
		})

		It("does not emit any messages", func() {
			Consistently(emitter.EmitCallCount).Should(BeZero())
		})
	})

	Context("when the event source returns an error", func() {
		var subscribeErr, nextErr error

		BeforeEach(func() {
			subscribeErr = errors.New("subscribe-error")
			nextErr = errors.New("next-error")

			eventSource := new(fake_receptor.FakeEventSource)
			receptorClient.SubscribeToEventsStub = func() (receptor.EventSource, error) {
				if receptorClient.SubscribeToEventsCallCount() == 1 {
					return eventSource, nil
				}
				return nil, subscribeErr
			}

			eventSource.NextStub = func() (receptor.Event, error) {
				return nil, nextErr
			}
		})

		It("re-subscribes", func() {
			Eventually(receptorClient.SubscribeToEventsCallCount).Should(Equal(2))
		})

		Context("when re-subscribing fails", func() {
			It("returns an error", func() {
				Eventually(process.Wait()).Should(Receive(Equal(subscribeErr)))
			})
		})
	})

	Describe("interrupting the process", func() {
		BeforeEach(func() {
			eventSource := new(fake_receptor.FakeEventSource)
			receptorClient.SubscribeToEventsReturns(eventSource, nil)
		})

		It("should be possible to SIGINT the route emitter", func() {
			process.Signal(os.Interrupt)
			Eventually(process.Wait()).Should(Receive())
		})
	})
})

type unrecognizedEvent struct{}

func (u unrecognizedEvent) EventType() receptor.EventType {
	return "unrecognized-event"
}
