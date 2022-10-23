// Copyright (c) 2022 Cisco and/or its affiliates.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package memory provides implementation of api/pkg/api/ipam.IPAMServer
package memory

import (
	"context"
	"net"
	"sync"

	"github.com/networkservicemesh/api/pkg/api/ipam"
	"github.com/networkservicemesh/sdk/pkg/tools/ippool"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ErrUndefined means that operation is not supported
var ErrUndefined = errors.New("request type is undefined")

var ErrEndpointInfoUndefined = errors.New("client tried to create new ipam entry but endpoint does not exist")

var ErrEndpointNameUndefined = errors.New("endpoint name for client does not exist")

// ErrOutOfRange means that ip pool of IPAM is empty
var ErrOutOfRange = errors.New("prefix is out of range or already in use")

var IPPoolNotInitializedWithPrefix = errors.New("ip pool was not initialized with proper prefix")


var ErrClientNotExists = errors.New("client does not have ip information")

type Client struct {
	srcAddr string
	dstAddr string
}
type Endpoint struct {
	pool *ippool.IPPool
	prefix *net.IPNet
	clientInfo map[string]*Client
}

type NetworkServiceSettings struct {
	uniqueCIDR bool
}

type NetworkService struct {
	// if cidr is unique pool will be empty as it will be set uniquely inside each endpoint
	pool *ippool.IPPool
	// if cidr is unique prefix will be empty as it will be set uniquely inside each endpoint
	prefix *net.IPNet
	settings *NetworkServiceSettings
	endpoints map[string]*Endpoint
}

type memoryIPAMServer struct {
	generalIPPool             *ippool.IPPool
	excludedPrefixes []string
	poolMutex        sync.Mutex
	initialSize       uint8
	nameToNetworkService map[string]*NetworkService
}

// NewIPAMServer creates a new ipam.IPAMServer handler for grpc.Server
func NewIPAMServer(prefix string, initialNSEPrefixSize uint8) ipam.IPAMV2Server {
	return &memoryIPAMServer{
		generalIPPool:       ippool.NewWithNetString(prefix),
		initialSize: initialNSEPrefixSize,
		nameToNetworkService: make(map[string]*NetworkService),
	}
}

var _ ipam.IPAMV2Server = (*memoryIPAMServer)(nil)


func (i *Client) shouldUpdate(exclude *ippool.IPPool) bool {
	srcIP, _, srcErr := net.ParseCIDR(i.srcAddr)
	dstIP, _, dstErr := net.ParseCIDR(i.dstAddr)

	return srcErr == nil && exclude.ContainsString(srcIP.String()) || dstErr == nil && exclude.ContainsString(dstIP.String())
}

func (s *memoryIPAMServer) RegisterClient(ctx context.Context, client *ipam.Client) (*ipam.Client, error) {
	s.poolMutex.Lock()
	defer s.poolMutex.Unlock()
	ipv4exclude := ippool.New(net.IPv4len)
	ipv6exclude := ippool.New(net.IPv6len)
	for _, prefix := range client.ExcludePrefixes {
		ipv4exclude.AddNetString(prefix)
		ipv6exclude.AddNetString(prefix)
	}

	// get ns name
	nsName := client.NetworkServiceNames[0]
	// get ns map from cache, should have endpoint already populated it
	ns := s.nameToNetworkService[nsName]

	if client.EndpointName == "" {
		log.FromContext(ctx).WithField("client", client).Errorf("endpoint name is empty for network service = %s", nsName)
		return nil, ErrEndpointNameUndefined	
	}

	endpoint := ns.endpoints[client.EndpointName]

	// endpoint should have created an ip pool already
	if endpoint == nil {
		log.FromContext(ctx).WithField("client", client).Errorf("endpoint should have populated info for network service = %s", nsName)
		return nil, ErrEndpointInfoUndefined
	}

	// check client name in inner map
	clientInfo := endpoint.clientInfo[client.Name]

	// if client info exists and src and dst do not fall in exclude range
	if clientInfo != nil && !clientInfo.shouldUpdate(ipv4exclude) && !clientInfo.shouldUpdate(ipv6exclude) {
		log.FromContext(ctx).Debugf("found existing info for client, for network service = %s, client name = %s, src = %s, dst = %s", nsName, client.Name, clientInfo.srcAddr, clientInfo.dstAddr)
		// TODO, check if addresses from Request and clientInfo differ?
		// client.SrcAddress = []string{clientInfo.srcAddr}
		// client.DstAddress = []string{clientInfo.dstAddr}
		if client.SrcAddress == nil {
			client.SrcAddress = []string{clientInfo.srcAddr}
		}
		if client.DstAddress == nil {
			client.DstAddress = []string{clientInfo.dstAddr}
		}
		if client.Prefix == "" {
			client.Prefix = endpoint.prefix.String()
		}
		return client, nil
	}

	// otherwise, either client info does not exist or we need to update the exclude prefixes

	// if not, pull new p2p ip from pool
	dstAddr, srcAddr, err := endpoint.pool.PullP2PAddrs(ipv4exclude, ipv6exclude)
	if err != nil {
		return nil, err
	}

	log.FromContext(ctx).Infof("pulled new src = %s and dst = %s for client = %s, network service = %s", srcAddr, dstAddr, client.Name, nsName)
	// save new client in map
	newClientConnInfo := &Client{
		srcAddr: srcAddr.String(),
		dstAddr: dstAddr.String(),
	}

	endpoint.clientInfo[client.Name] = newClientConnInfo

	clientSrcAddresses := []string{}
	clientDstAddresses := []string{}
	// clientInfo changed due to ip exclusion, update client response list
	if clientInfo != nil && clientInfo.srcAddr != newClientConnInfo.srcAddr {
		for _, ip := range client.SrcAddress {
			if ip != clientInfo.srcAddr {
				clientSrcAddresses = append(clientSrcAddresses, ip)
			}
		}
		client.SrcAddress = clientSrcAddresses
	}

	if clientInfo != nil && clientInfo.dstAddr != newClientConnInfo.dstAddr {
		for _, ip := range client.DstAddress {
			if ip != clientInfo.dstAddr {
				clientDstAddresses = append(clientDstAddresses, ip)
			}
		}
		client.DstAddress = clientDstAddresses
	}

	// return new adresses
	client.SrcAddress = append(client.SrcAddress, newClientConnInfo.srcAddr)
	client.DstAddress = append(client.DstAddress, newClientConnInfo.dstAddr)
	client.Prefix = endpoint.prefix.String()
	return client, nil
}

func (s *memoryIPAMServer) UnregisterEndpoint(ctx context.Context, endpoint *ipam.Endpoint) (*emptypb.Empty, error) {
	return nil, nil
}
func (s *memoryIPAMServer) UnregisterClient(ctx context.Context, client *ipam.Client) (*emptypb.Empty, error) {
	s.poolMutex.Lock()
	defer s.poolMutex.Unlock()
	// get ns name
	nsName := client.NetworkServiceNames[0]
	// get ns map from cache, should have endpoint already populated it
	ns := s.nameToNetworkService[nsName]

	endpoint := ns.endpoints[client.EndpointName]

	// endpoint should have created an ip pool already
	if endpoint == nil {
		log.FromContext(ctx).WithField("client", client).Errorf("endpoint with name = %s should have populated info for network service = %s", client.EndpointName, nsName)
		return nil, ErrEndpointInfoUndefined
	}


	// check client name in inner map
	clientInfo := endpoint.clientInfo[client.Name]

	// if client exists return same addr
	if clientInfo == nil {
		log.FromContext(ctx).Debugf("did not find information, for network service = %s, client name = %s", nsName, client.Name)
		return &emptypb.Empty{}, ErrClientNotExists
	}

	log.FromContext(ctx).Debugf("returning client name = %s src = %s and dst = %s addresses back to ip pool, for network service = %s", client.Name, clientInfo.srcAddr, clientInfo.dstAddr, nsName)

	endpoint.pool.AddNetString(clientInfo.srcAddr)
	endpoint.pool.AddNetString(clientInfo.dstAddr)

	// delete client information
	endpoint.clientInfo[client.Name] = nil

	return &emptypb.Empty{}, nil
}

func (s *memoryIPAMServer) RegisterEndpoint(ctx context.Context, endpoint *ipam.Endpoint) (*ipam.Endpoint, error) {
	var generalIPPool = s.generalIPPool
	var mutex = &s.poolMutex

	mutex.Lock()
	defer mutex.Unlock()

	if generalIPPool == nil {
		return nil, IPPoolNotInitializedWithPrefix
	}

	var resp ipam.Endpoint

	for _, excludePrefix := range endpoint.ExcludePrefixes {
		log.FromContext(ctx).Debugf("excluding prefix from pool, prefix = %s", excludePrefix)
		generalIPPool.ExcludeString(excludePrefix)
	}

	log.FromContext(ctx).Debugf("trying to find if endpoint name = %s in network service = %s has an existing ip pool", endpoint.Name, endpoint.NetworkServiceNames[0])

	// find ns
	ns := s.nameToNetworkService[endpoint.NetworkServiceNames[0]]

	// existing service
	if ns != nil {
		ep := ns.endpoints[endpoint.Name]
		// endpoint exists (for example on endpoint restart / reschedule)
		if ep != nil {
			log.FromContext(ctx).Debugf("found prefix = %s for network service = %s, endpoint name = %s", ep.prefix, endpoint.NetworkServiceNames[0], endpoint.Name)
			// return existing prefix
			resp.Prefix = ep.prefix.String()
		} else {
			// create new endpoint
			log.FromContext(ctx).Debugf("starting to create new endpoint name = %s in network service = %s", endpoint.Name, endpoint.NetworkServiceNames[0])
			newEndpoint, err := s.createEndpoint(ctx, endpoint.Name, endpoint.NetworkServiceNames[0], ns)
			if err != nil {
				return nil, err
			}

			ns.endpoints[endpoint.Name] = newEndpoint
			resp.Prefix = newEndpoint.prefix.String()
		}
	} else {
		// create new ns
		settings := &NetworkServiceSettings{
			uniqueCIDR: endpoint.UniqueCidr,
		}

		log.FromContext(ctx).Debugf("creating new network service = %s, unique cidr = %t", endpoint.NetworkServiceNames[0], settings.uniqueCIDR)

		newNS, err := s.createNetworkService(ctx, settings)
		if err != nil {
			return nil, err
		}

		// create new endpoint
		log.FromContext(ctx).Debugf("starting to create new endpoint name = %s in network service = %s", endpoint.Name, endpoint.NetworkServiceNames[0])
		newEndpoint, err := s.createEndpoint(ctx, endpoint.Name, endpoint.NetworkServiceNames[0], nil)
		if err != nil {
			return nil, err
		}

		// first ns and endpoint created, check if unique cidr exists
		if !endpoint.UniqueCidr {
			newNS.pool = newEndpoint.pool
			newNS.prefix = newEndpoint.prefix
		}

		s.nameToNetworkService[endpoint.NetworkServiceNames[0]] = newNS
		ns = newNS
		ns.endpoints[endpoint.Name] = newEndpoint
		resp.Prefix = newEndpoint.prefix.String()
	}

	log.FromContext(ctx).Debugf("adding request.prefix = %s to excludedPrefixes", resp.Prefix)
	s.excludedPrefixes = append(s.excludedPrefixes, endpoint.Prefix)
	log.FromContext(ctx).Debugf("excluding prefix from pool, prefix = %s", resp.Prefix)
	generalIPPool.ExcludeString(resp.Prefix)
	resp.ExcludePrefixes = endpoint.ExcludePrefixes
	resp.ExcludePrefixes = append(resp.ExcludePrefixes, s.excludedPrefixes...)
	return &resp, nil

}


func (s *memoryIPAMServer) createNetworkService(ctx context.Context, nsSettings *NetworkServiceSettings) (*NetworkService, error) {
	return &NetworkService{
		settings: nsSettings,
		endpoints: make(map[string]*Endpoint),
	}, nil
}

func (s *memoryIPAMServer) createEndpoint(ctx context.Context, epName string, nsName string, ns *NetworkService) (*Endpoint, error) {

	// if cidr must be unique continue
	if ns != nil && ns.settings != nil && !ns.settings.uniqueCIDR {
		return &Endpoint{
			pool: ns.pool,
			prefix: ns.prefix,
			clientInfo: make(map[string]*Client),
		}, nil
	}

	var ip net.IP
	ip, err := s.generalIPPool.Pull()
	if err != nil {
		return nil, err
	}

	networkServiceCIDR := &net.IPNet{
		IP: ip,
		Mask: net.CIDRMask(
			int(s.initialSize),
			len(ip)*8,
		),
	}

	endpointIPPool := ippool.NewWithNet(networkServiceCIDR)

	return &Endpoint{
		pool: endpointIPPool,
		prefix: networkServiceCIDR,
		clientInfo: make(map[string]*Client),
	}, nil
}
