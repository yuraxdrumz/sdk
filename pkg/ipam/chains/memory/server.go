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

// ErrOutOfRange means that ip pool of IPAM is empty
var ErrOutOfRange = errors.New("prefix is out of range or already in use")

var IPPoolNotInitializedWithPrefix = errors.New("ip pool was not initialized with proper prefix")

var ErrClientNotExists = errors.New("client does not have ip information")

type prefixAndIPPool struct {
	prefix *net.IPNet
	pool *ippool.IPPool
	clientInfo map[string]*connectionInfo
}

type connectionInfo struct {
	srcAddr string
	dstAddr string
}

type memoryIPAMServer struct {
	generalIPPool             *ippool.IPPool
	excludedPrefixes []string
	poolMutex        sync.Mutex
	initialSize       uint8
	clientsPrefixes []string
	nameToPrefixAndIPPoolCache map[string]*prefixAndIPPool
}

// NewIPAMServer creates a new ipam.IPAMServer handler for grpc.Server
func NewIPAMServer(prefix string, initialNSEPrefixSize uint8) ipam.IPAMV2Server {
	return &memoryIPAMServer{
		generalIPPool:       ippool.NewWithNetString(prefix),
		initialSize: initialNSEPrefixSize,
		nameToPrefixAndIPPoolCache: make(map[string]*prefixAndIPPool),
	}
}

var _ ipam.IPAMV2Server = (*memoryIPAMServer)(nil)


func (i *connectionInfo) shouldUpdate(exclude *ippool.IPPool) bool {
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
	conn := s.nameToPrefixAndIPPoolCache[nsName]

	// endpoint should have created an ip pool already
	if conn == nil {
		log.FromContext(ctx).WithField("client", client).Errorf("endpoint should have populated info for network service = %s", nsName)
		return nil, ErrEndpointInfoUndefined
	}

	if conn.clientInfo == nil {
		log.FromContext(ctx).Debugf("creating new connection info map for network service = %s, client name = %s", nsName, client.Name)
		conn.clientInfo = make(map[string]*connectionInfo)
	}

	// check client name in inner map
	clientInfo := conn.clientInfo[client.Name]

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

		return client, nil
	}

	// otherwise, either client info does not exist or we need to update the exclude prefixes

	// if not, pull new p2p ip from pool
	dstAddr, srcAddr, err := conn.pool.PullP2PAddrs(ipv4exclude, ipv6exclude)
	if err != nil {
		return nil, err
	}

	log.FromContext(ctx).Infof("pulled new src = %s and dst = %s for client = %s, network service = %s", srcAddr, dstAddr, client.Name, nsName)
	// save new client in map
	newClientConnInfo := &connectionInfo{srcAddr: srcAddr.String(), dstAddr: dstAddr.String()}
	conn.clientInfo[client.Name] = newClientConnInfo


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
	return client, nil
}

func filter(ss []string, test func(string) bool) (ret []string) {
    for _, s := range ss {
        if test(s) {
            ret = append(ret, s)
        }
    }
    return
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
	conn := s.nameToPrefixAndIPPoolCache[nsName]

	// endpoint should have created an ip pool already
	if conn == nil {
		log.FromContext(ctx).WithField("client", client).Errorf("endpoint should have populated info for network service = %s", nsName)
		s.poolMutex.Unlock()
		return nil, ErrEndpointInfoUndefined
	}


	// check client name in inner map
	clientInfo := conn.clientInfo[client.Name]

	// if client exists return same addr
	if clientInfo == nil {
		log.FromContext(ctx).Debugf("did not find information, for network service = %s, client name = %s", nsName, client.Name)
		return &emptypb.Empty{}, ErrClientNotExists
	}

	log.FromContext(ctx).Debugf("returning client name = %s src = %s and dst = %s addresses back to ip pool, for network service = %s", client.Name, clientInfo.srcAddr, clientInfo.dstAddr, nsName)

	conn.pool.AddNetString(clientInfo.srcAddr)
	conn.pool.AddNetString(clientInfo.dstAddr)

	// delete client information
	conn.clientInfo[client.Name] = nil

	return &emptypb.Empty{}, nil
}

func (s *memoryIPAMServer) RegisterEndpoint(ctx context.Context, endpoint *ipam.Endpoint) (*ipam.Endpoint, error) {
	var generalIPPool = s.generalIPPool
	var mutex = &s.poolMutex
	var err error
	nsName := endpoint.NetworkServiceNames[0]

	mutex.Lock()
	defer mutex.Unlock()

	if generalIPPool == nil {
		return nil, IPPoolNotInitializedWithPrefix
	}

	switch endpoint.Type {
		case ipam.Type_UNDEFINED:
			return nil, ErrUndefined
		case ipam.Type_ALLOCATE:
			log.FromContext(ctx).Debugf("trying to find if endpoint name = %s in network service = %s has an existing ip pool", endpoint.Name, nsName)

			pool := s.nameToPrefixAndIPPoolCache[nsName]

			var resp ipam.Endpoint
			
			for _, excludePrefix := range endpoint.ExcludePrefixes {
				log.FromContext(ctx).Debugf("excluding prefix from pool, prefix = %s", excludePrefix)
				generalIPPool.ExcludeString(excludePrefix)
			}

			if pool == nil  {
				log.FromContext(ctx).Debugf("starting to allocate new prefix for endpoint name = %s in network service = %s", endpoint.Name, nsName)
				var ip net.IP
				ip, err = generalIPPool.Pull()
				if err != nil {
					break
				}

				networkServiceCIDR := &net.IPNet{
					IP: ip,
					Mask: net.CIDRMask(
						int(s.initialSize),
						len(ip)*8,
					),
				}

				resp.Prefix = networkServiceCIDR.String()
				endpointIPPool := ippool.NewWithNet(networkServiceCIDR)

				s.nameToPrefixAndIPPoolCache[nsName] = &prefixAndIPPool{
					prefix: networkServiceCIDR,
					pool: endpointIPPool,
				}
				log.FromContext(ctx).Debugf("didn't find prefix, allocating new prefix = %s and ip pool for network service = %s, endpoint name = %s", resp.Prefix, nsName, endpoint.Name)
			} else {
				log.FromContext(ctx).Debugf("found prefix = %s for network service = %s, endpoint name = %s", pool.prefix, nsName, endpoint.Name)
				resp.Prefix = pool.prefix.String()
			}

			log.FromContext(ctx).Debugf("adding request.prefix = %s to excludedPrefixes", resp.Prefix)
			s.excludedPrefixes = append(s.excludedPrefixes, endpoint.Prefix)
			log.FromContext(ctx).Debugf("adding response.prefix = %s to clientsPrefixes", resp.Prefix)
			s.clientsPrefixes = append(s.clientsPrefixes, resp.Prefix)
			log.FromContext(ctx).Debugf("excluding prefix from pool, prefix = %s", resp.Prefix)
			generalIPPool.ExcludeString(resp.Prefix)
			resp.ExcludePrefixes = endpoint.ExcludePrefixes
			resp.ExcludePrefixes = append(resp.ExcludePrefixes, s.excludedPrefixes...)
			return &resp, nil
		case ipam.Type_DELETE:
			log.FromContext(ctx).Debugf("starting to delete all client prefixes for endpoint name = %s", endpoint.Name)
			for i, p := range s.clientsPrefixes {
				if p != endpoint.Prefix {
					log.FromContext(ctx).Debugf("client prefix = %s does not match endpoint prefix %s, continue", p, endpoint.Prefix)
					continue
				}
				generalIPPool.AddNetString(p)
				s.nameToPrefixAndIPPoolCache[nsName] = nil
				s.clientsPrefixes = append(s.clientsPrefixes[:i], s.clientsPrefixes[i+1:]...)
				break
			}
	}

	for _, prefix := range s.clientsPrefixes {
		generalIPPool.AddNetString(prefix)
	}

	return endpoint, err
}
