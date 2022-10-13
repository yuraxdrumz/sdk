// Copyright (c) 2020-2022 Doc.ai and/or its affiliates.
//
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

// Package point2pointipam provides a p2p IPAM server chain element.
package point2pointipam

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"

	"github.com/networkservicemesh/api/pkg/api/networkservice"

	ipamapi "github.com/networkservicemesh/api/pkg/api/ipam"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/next"
)

type ipamServer struct {
	ipamClient ipamapi.IPAMV2Client
}

// NewServer - creates a new NetworkServiceServer chain element that implements IPAM service.
func NewServer(ipamClient ipamapi.IPAMV2Client) networkservice.NetworkServiceServer {
	return &ipamServer{
		ipamClient: ipamClient,
	}
}

func (s *ipamServer) Request(ctx context.Context, request *networkservice.NetworkServiceRequest) (*networkservice.Connection, error) {
	conn := request.GetConnection()
	if conn.GetContext() == nil {
		conn.Context = &networkservice.ConnectionContext{}
	}
	if conn.GetContext().GetIpContext() == nil {
		conn.GetContext().IpContext = &networkservice.IPContext{}
	}
	ipContext := conn.GetContext().GetIpContext()

	// clients always start the flow and are first in path segements
	extractedClientName := request.Connection.Path.PathSegments[0].Name

	clientResponse, err := s.ipamClient.RegisterClient(ctx, &ipamapi.Client{
		Id: conn.GetId(),
		Name: extractedClientName,
		NetworkServiceNames: []string{request.Connection.NetworkService},
		NetworkServiceLabels: nil,
		ExcludePrefixes: ipContext.GetExcludedPrefixes(),
	})


	if err != nil {
		return nil, err
	}

	ipContext.SrcIpAddrs = []string{clientResponse.SrcAddress}
	ipContext.DstIpAddrs = []string{clientResponse.DstAddress}
	ipContext.SrcIpAddrs = []string{clientResponse.SrcAddress}
	ipContext.DstIpAddrs = []string{clientResponse.DstAddress}
	

	conn, err = next.Server(ctx).Request(ctx, request)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (s *ipamServer) Close(ctx context.Context, conn *networkservice.Connection) (_ *empty.Empty, err error) {
	return next.Server(ctx).Close(ctx, conn)
}