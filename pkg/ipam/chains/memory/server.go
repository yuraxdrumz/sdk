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

// ErrOutOfRange means that ip pool of IPAM is empty
var ErrOutOfRange = errors.New("prefix is out of range or already in use")

type memoryIPAMServer struct {
	pool             *ippool.IPPool
	excludedPrefixes []string
	poolMutex        sync.Mutex
	initialSize       uint8
	clientsPrefixes []string
}

// NewIPAMServer creates a new ipam.IPAMServer handler for grpc.Server
func NewIPAMServer(prefix string, initialNSEPrefixSize uint8) ipam.IPAMV2Server {
	return &memoryIPAMServer{
		pool:       ippool.NewWithNetString(prefix),
		initialSize: initialNSEPrefixSize,
	}
}

var _ ipam.IPAMV2Server = (*memoryIPAMServer)(nil)


func (s *memoryIPAMServer) RegisterClient(ctx context.Context, client *ipam.Client) (*ipam.Client, error) {
	return nil, nil
}
func (s *memoryIPAMServer) UnregisterEndpoint(ctx context.Context, endpoint *ipam.Endpoint) (*emptypb.Empty, error) {
	return nil, nil
}
func (s *memoryIPAMServer) UnregisterClient(ctx context.Context, client *ipam.Client) (*emptypb.Empty, error) {
	return nil, nil
}

func (s *memoryIPAMServer) RegisterEndpoint(ctx context.Context, endpoint *ipam.Endpoint) (*ipam.Endpoint, error) {
	var pool = s.pool
	var mutex = &s.poolMutex
	var err error

	switch endpoint.Type {
		case ipam.Type_UNDEFINED:
			return nil, ErrUndefined
		case ipam.Type_ALLOCATE:
			log.FromContext(ctx).Debugf("starting to allocate new prefix for endpoint name = %s", endpoint.Name)
			var resp ipam.Endpoint
			mutex.Lock()
			for _, excludePrefix := range endpoint.ExcludePrefixes {
				log.FromContext(ctx).Debugf("excluding prefix from pool, prefix = %s", excludePrefix)
				pool.ExcludeString(excludePrefix)
			}
			resp.Prefix = endpoint.Prefix
			if resp.Prefix == "" || !pool.ContainsNetString(resp.Prefix) {
				var ip net.IP
				ip, err = pool.Pull()
				if err != nil {
					mutex.Unlock()
					break
				}
				ipNet := &net.IPNet{
					IP: ip,
					Mask: net.CIDRMask(
						int(s.initialSize),
						len(ip)*8,
					),
				}
				resp.Prefix = ipNet.String()
				log.FromContext(ctx).Debugf("did'nt find prefix, allocating new prefix = %s", resp.Prefix)
			}

			log.FromContext(ctx).Debugf("adding request.prefix = %s to excludedPrefixes", resp.Prefix)
			s.excludedPrefixes = append(s.excludedPrefixes, endpoint.Prefix)
			log.FromContext(ctx).Debugf("adding response.prefix = %s to clientsPrefixes", resp.Prefix)
			s.clientsPrefixes = append(s.clientsPrefixes, resp.Prefix)
			log.FromContext(ctx).Debugf("excluding prefix from pool, prefix = %s", resp.Prefix)
			pool.ExcludeString(resp.Prefix)
			resp.ExcludePrefixes = endpoint.ExcludePrefixes
			resp.ExcludePrefixes = append(resp.ExcludePrefixes, s.excludedPrefixes...)
			mutex.Unlock()
			return &resp, nil
		case ipam.Type_DELETE:
			log.FromContext(ctx).Debugf("starting to delete all client prefixes for endpoint name = %s", endpoint.Name)
			for i, p := range s.clientsPrefixes {
				if p != endpoint.Prefix {
					log.FromContext(ctx).Debugf("client prefix = %s does not match endpoint prefix %s, continue", p, endpoint.Prefix)
					continue
				}
				mutex.Lock()
				pool.AddNetString(p)
				s.clientsPrefixes = append(s.clientsPrefixes[:i], s.clientsPrefixes[i+1:]...)
				mutex.Unlock()
				break
			}
	}

	mutex.Lock()
	for _, prefix := range s.clientsPrefixes {
		pool.AddNetString(prefix)
	}
	mutex.Unlock()
	return endpoint, err
}
