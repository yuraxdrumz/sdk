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

package point2pointipam_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/networkservicemesh/sdk/pkg/ipam/chains/memory"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"

	"github.com/networkservicemesh/api/pkg/api/ipam"
	"github.com/networkservicemesh/api/pkg/api/networkservice"

	"github.com/networkservicemesh/sdk/pkg/networkservice/common/updatepath"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/chain"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/next"
	"github.com/networkservicemesh/sdk/pkg/networkservice/ipam/point2pointipam"
	"github.com/networkservicemesh/sdk/pkg/networkservice/utils/inject/injecterror"
	"github.com/networkservicemesh/sdk/pkg/networkservice/utils/metadata"
)


func newMemoryIpamServer(ctx context.Context, t *testing.T, prefix string, initialSize uint8) url.URL {
	var s = grpc.NewServer()
	ipam.RegisterIPAMV2Server(s, memory.NewIPAMServer(prefix, initialSize))

	var serverAddr url.URL

	require.Len(t, grpcutils.ListenAndServe(ctx, &serverAddr, s), 0)

	return serverAddr
}

func newIPAMClient(ctx context.Context, t *testing.T, connectTO *url.URL) ipam.IPAMV2Client {
	var cc, err = grpc.DialContext(
		ctx, grpcutils.URLToTarget(connectTO),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	go func() {
		<-ctx.Done()
		_ = cc.Close()
	}()

	return ipam.NewIPAMV2Client(cc)
}

func newIpamServer(ctx context.Context, t *testing.T, prefix string, initialSize uint8) networkservice.NetworkServiceServer {
	serverAddress := newMemoryIpamServer(ctx, t, prefix, initialSize)
	ipamClient := newIPAMClient(ctx, t, &serverAddress)

	endpoint, err := ipamClient.RegisterEndpoint(ctx, &ipam.Endpoint{
		Type: ipam.Type_ALLOCATE,
		Name: "endpoint-1",
		NetworkServiceNames: []string{"ns-1"},
	})

	require.NoError(t, err, "newIpamServer")
	require.NotEmpty(t, endpoint.Prefix, "newIpamServer")

	return next.NewNetworkServiceServer(
		updatepath.NewServer("ipam"),
		metadata.NewServer(),
		point2pointipam.NewServer(ipamClient),
	)
}

func newRequest(clientName string) *networkservice.NetworkServiceRequest {
	return &networkservice.NetworkServiceRequest{
		Connection: &networkservice.Connection{
			Context: &networkservice.ConnectionContext{
				IpContext: new(networkservice.IPContext),
			},
			NetworkService: "ns-1",
			Path: &networkservice.Path{
				Index: 0,
				PathSegments: []*networkservice.PathSegment{
					{Name: clientName},
				},
			},
		},
	}
}

func validateConn(t *testing.T, conn *networkservice.Connection, dst, src string) {
	require.Equal(t, conn.Context.IpContext.DstIpAddrs[0], dst)
	require.Equal(t, conn.Context.IpContext.DstRoutes, []*networkservice.Route{
		{
			Prefix: src,
		},
	})

	require.Equal(t, conn.Context.IpContext.SrcIpAddrs[0], src)
	require.Equal(t, conn.Context.IpContext.SrcRoutes, []*networkservice.Route{
		{
			Prefix: dst,
		},
	})
}

func validateConns(t *testing.T, conn *networkservice.Connection, dsts, srcs []string) {
	for i, dst := range dsts {
		require.Equal(t, conn.Context.IpContext.DstIpAddrs[i], dst)
		require.Equal(t, conn.Context.IpContext.SrcRoutes[i].Prefix, dst)
	}
	for i, src := range srcs {
		require.Equal(t, conn.Context.IpContext.SrcIpAddrs[i], src)
		require.Equal(t, conn.Context.IpContext.DstRoutes[i].Prefix, src)
	}
}

//nolint:dupl
func TestServer(t *testing.T) {
	srv := newIpamServer(context.Background(), t, "192.168.3.4/16", 24)

	conn1, err := srv.Request(context.Background(), newRequest("cl-1"))
	require.NoError(t, err)
	validateConn(t, conn1, "192.168.0.0/32", "192.168.0.1/32")

	conn2, err := srv.Request(context.Background(), newRequest("cl-2"))
	require.NoError(t, err)
	validateConn(t, conn2, "192.168.0.2/32", "192.168.0.3/32")

	_, err = srv.Close(context.Background(), conn1)
	require.NoError(t, err)

	conn3, err := srv.Request(context.Background(), newRequest("cl-3"))
	require.NoError(t, err)
	validateConn(t, conn3, "192.168.0.0/32", "192.168.0.1/32")

	conn4, err := srv.Request(context.Background(), newRequest("cl-4"))
	require.NoError(t, err)
	validateConn(t, conn4, "192.168.0.4/32", "192.168.0.5/32")
}

//nolint:dupl
func TestServerIPv6(t *testing.T) {
	srv := newIpamServer(context.Background(), t, "fe80::/64", 24)

	conn1, err := srv.Request(context.Background(), newRequest("cl-1"))
	require.NoError(t, err)
	validateConn(t, conn1, "fe80::/128", "fe80::1/128")

	conn2, err := srv.Request(context.Background(), newRequest("cl-2"))
	require.NoError(t, err)
	validateConn(t, conn2, "fe80::2/128", "fe80::3/128")

	_, err = srv.Close(context.Background(), conn1)
	require.NoError(t, err)

	conn3, err := srv.Request(context.Background(), newRequest("cl-3"))
	require.NoError(t, err)
	validateConn(t, conn3, "fe80::/128", "fe80::1/128")

	conn4, err := srv.Request(context.Background(), newRequest("cl-4"))
	require.NoError(t, err)
	validateConn(t, conn4, "fe80::4/128", "fe80::5/128")
}

//nolint:dupl
func TestExclude32Prefix(t *testing.T) {
	srv := newIpamServer(context.Background(), t, "192.168.1.0/24", 24)
	// Test center of assigned
	req1 := newRequest("cl-1")
	req1.Connection.Context.IpContext.ExcludedPrefixes = []string{"192.168.1.1/32", "192.168.1.3/32", "192.168.1.6/32"}
	conn1, err := srv.Request(context.Background(), req1)
	require.NoError(t, err)
	validateConn(t, conn1, "192.168.1.0/32", "192.168.1.2/32")

	// Test exclude before assigned
	req2 := newRequest("cl-2")
	req2.Connection.Context.IpContext.ExcludedPrefixes = []string{"192.168.1.1/32", "192.168.1.3/32", "192.168.1.6/32"}
	conn2, err := srv.Request(context.Background(), req2)
	require.NoError(t, err)
	validateConn(t, conn2, "192.168.1.4/32", "192.168.1.5/32")

	// Test after assigned
	req3 := newRequest("cl-3")
	req3.Connection.Context.IpContext.ExcludedPrefixes = []string{"192.168.1.1/32", "192.168.1.3/32", "192.168.1.6/32"}
	conn3, err := srv.Request(context.Background(), req3)
	require.NoError(t, err)
	validateConn(t, conn3, "192.168.1.7/32", "192.168.1.8/32")
}

//nolint:dupl
func TestExclude128PrefixIPv6(t *testing.T) {
	srv := newIpamServer(context.Background(), t, "fe80::1:0/112", 124)

	// Test center of assigned
	req1 := newRequest("cl-1")
	req1.Connection.Context.IpContext.ExcludedPrefixes = []string{"fe80::1:1/128", "fe80::1:3/128", "fe80::1:6/128"}
	conn1, err := srv.Request(context.Background(), req1)
	require.NoError(t, err)
	validateConn(t, conn1, "fe80::1:0/128", "fe80::1:2/128")

	// Test exclude before assigned
	req2 := newRequest("cl-2")
	req2.Connection.Context.IpContext.ExcludedPrefixes = []string{"fe80::1:1/128", "fe80::1:3/128", "fe80::1:6/128"}
	conn2, err := srv.Request(context.Background(), req2)
	require.NoError(t, err)
	validateConn(t, conn2, "fe80::1:4/128", "fe80::1:5/128")

	// Test after assigned
	req3 := newRequest("cl-3")
	req3.Connection.Context.IpContext.ExcludedPrefixes = []string{"fe80::1:1/128", "fe80::1:3/128", "fe80::1:6/128"}
	conn3, err := srv.Request(context.Background(), req3)
	require.NoError(t, err)
	validateConn(t, conn3, "fe80::1:7/128", "fe80::1:8/128")
}

func TestOutOfIPs(t *testing.T) {
	srv := newIpamServer(context.Background(), t, "192.168.1.2/31", 31)

	req1 := newRequest("cl-1")
	conn1, err := srv.Request(context.Background(), req1)
	require.NoError(t, err)
	validateConn(t, conn1, "192.168.1.2/32", "192.168.1.3/32")

	req2 := newRequest("cl-2")
	_, err = srv.Request(context.Background(), req2)
	require.Error(t, err)
}

func TestOutOfIPsIPv6(t *testing.T) {
	srv := newIpamServer(context.Background(), t, "fe80::1:2/127", 127)

	req1 := newRequest("cl-1")
	conn1, err := srv.Request(context.Background(), req1)
	require.NoError(t, err)
	validateConn(t, conn1, "fe80::1:2/128", "fe80::1:3/128")

	req2 := newRequest("cl-2")
	_, err = srv.Request(context.Background(), req2)
	require.Error(t, err)
}

func TestAllIPsExcluded(t *testing.T) {
	srv := newIpamServer(context.Background(), t, "192.168.1.2/31", 31)

	req1 := newRequest("cl-1")
	req1.Connection.Context.IpContext.ExcludedPrefixes = []string{"192.168.1.2/31"}
	conn1, err := srv.Request(context.Background(), req1)
	require.Nil(t, conn1)
	require.Error(t, err)
}

func TestAllIPsExcludedIPv6(t *testing.T) {
	srv := newIpamServer(context.Background(), t, "fe80::1:2/127", 127)

	req1 := newRequest("cl-1")
	req1.Connection.Context.IpContext.ExcludedPrefixes = []string{"fe80::1:2/127"}
	conn1, err := srv.Request(context.Background(), req1)
	require.Nil(t, conn1)
	require.Error(t, err)
}

//nolint:dupl
func TestRefreshRequest(t *testing.T) {
	srv := newIpamServer(context.Background(), t, "192.168.3.4/16", 24)

	req := newRequest("cl-1")
	req.Connection.Context.IpContext.ExcludedPrefixes = []string{"192.168.0.1/32"}
	conn, err := srv.Request(context.Background(), req)
	require.NoError(t, err)
	validateConn(t, conn, "192.168.0.0/32", "192.168.0.2/32")

	req = newRequest("cl-1")
	req.Connection.Id = conn.Id
	conn, err = srv.Request(context.Background(), req)
	require.NoError(t, err)
	validateConn(t, conn, "192.168.0.0/32", "192.168.0.2/32")

	req.Connection = conn.Clone()
	req.Connection.Context.IpContext.ExcludedPrefixes = []string{"192.168.0.1/30"}
	conn, err = srv.Request(context.Background(), req)
	require.NoError(t, err)
	validateConn(t, conn, "192.168.0.4/32", "192.168.0.5/32")
}

//nolint:dupl
func TestRefreshRequestIPv6(t *testing.T) {
	srv := newIpamServer(context.Background(), t, "fe80::/64", 124)

	req := newRequest("cl-1")
	req.Connection.Context.IpContext.ExcludedPrefixes = []string{"fe80::1/128"}
	conn, err := srv.Request(context.Background(), req)
	require.NoError(t, err)
	validateConn(t, conn, "fe80::/128", "fe80::2/128")

	req = newRequest("cl-1")
	req.Connection.Id = conn.Id
	conn, err = srv.Request(context.Background(), req)
	require.NoError(t, err)
	validateConn(t, conn, "fe80::/128", "fe80::2/128")

	req.Connection = conn.Clone()
	req.Connection.Context.IpContext.ExcludedPrefixes = []string{"fe80::/126"}
	conn, err = srv.Request(context.Background(), req)
	require.NoError(t, err)
	validateConn(t, conn, "fe80::4/128", "fe80::5/128")
}

func TestNextError(t *testing.T) {
	srv := newIpamServer(context.Background(), t, "192.168.3.4/16", 24)

	_, err := next.NewNetworkServiceServer(srv, injecterror.NewServer()).Request(context.Background(), newRequest("cl-1"))
	require.Error(t, err)

	conn, err := srv.Request(context.Background(), newRequest("cl-1"))
	require.NoError(t, err)
	validateConn(t, conn, "192.168.0.0/32", "192.168.0.1/32")
}

func TestRefreshNextError(t *testing.T) {
	srv := newIpamServer(context.Background(), t, "192.168.3.4/16", 24)

	req := newRequest("cl-1")
	conn, err := srv.Request(context.Background(), req)
	require.NoError(t, err)
	validateConn(t, conn, "192.168.0.0/32", "192.168.0.1/32")

	req.Connection = conn.Clone()
	_, err = next.NewNetworkServiceServer(srv, injecterror.NewServer()).Request(context.Background(), newRequest("cl-1"))
	require.Error(t, err)

	conn, err = srv.Request(context.Background(), newRequest("cl-1"))
	require.NoError(t, err)
	validateConn(t, conn, "192.168.0.0/32", "192.168.0.1/32")
}

//nolint:dupl
func TestServers(t *testing.T) {

	srv := chain.NewNetworkServiceServer(
		newIpamServer(context.Background(), t, "192.168.3.4/16", 24),
		newIpamServer(context.Background(), t, "fd00::/8", 24),
	)

	conn1, err := srv.Request(context.Background(), newRequest("cl-1"))
	require.NoError(t, err)
	t.Log(conn1.GetContext().IpContext)
	validateConns(t, conn1, []string{"192.168.0.0/32", "fd00::/128"}, []string{"192.168.0.1/32", "fd00::1/128"})

	conn2, err := srv.Request(context.Background(), newRequest("cl-2"))
	require.NoError(t, err)
	validateConns(t, conn2, []string{"192.168.0.2/32", "fd00::2/128"}, []string{"192.168.0.3/32", "fd00::3/128"})

	_, err = srv.Close(context.Background(), conn1)
	require.NoError(t, err)

	conn3, err := srv.Request(context.Background(), newRequest("cl-3"))
	require.NoError(t, err)
	validateConns(t, conn3, []string{"192.168.0.0/32", "fd00::/128"}, []string{"192.168.0.1/32", "fd00::1/128"})

	conn4, err := srv.Request(context.Background(), newRequest("cl-4"))
	require.NoError(t, err)
	validateConns(t, conn4, []string{"192.168.0.4/32", "fd00::4/128"}, []string{"192.168.0.5/32", "fd00::5/128"})
}

//nolint:dupl
func TestRefreshRequestMultiServer(t *testing.T) {
	srv := chain.NewNetworkServiceServer(
		newIpamServer(context.Background(), t, "192.168.3.4/16", 24),
		newIpamServer(context.Background(), t, "fe80::/64", 24),
	)

	req := newRequest("cl-1")
	req.Connection.Context.IpContext.ExcludedPrefixes = []string{"192.168.0.1/32", "fe80::1/128"}
	conn, err := srv.Request(context.Background(), req)
	require.NoError(t, err)
	validateConns(t, conn, []string{"192.168.0.0/32", "fe80::/128"}, []string{"192.168.0.2/32", "fe80::2/128"})

	req = newRequest("cl-1")
	req.Connection = conn
	conn, err = srv.Request(context.Background(), req)
	require.NoError(t, err)
	validateConns(t, conn, []string{"192.168.0.0/32", "fe80::/128"}, []string{"192.168.0.2/32", "fe80::2/128"})

	req.Connection = conn.Clone()
	req.Connection.Context.IpContext.ExcludedPrefixes = []string{"192.168.0.1/30", "fe80::1/126"}
	conn, err = srv.Request(context.Background(), req)
	require.NoError(t, err)
	t.Log(conn.Context.IpContext)
	validateConns(t, conn, []string{"192.168.0.4/32", "fe80::4/128"}, []string{"192.168.0.5/32", "fe80::5/128"})
}

// //nolint:dupl
// func TestRecoveryServer(t *testing.T) {
// 	srv := newIpamServer(context.Background(), t, "192.168.3.4/16", 24)

// 	// Create requests with predefined addresses
// 	request1 := newRequest("cl-1")
// 	request1.Connection.Context.IpContext.DstIpAddrs = []string{"192.168.10.0/32"}
// 	request1.Connection.Context.IpContext.SrcRoutes = []*networkservice.Route{{Prefix: "192.168.10.0/32"}}
// 	request1.Connection.Context.IpContext.SrcIpAddrs = []string{"192.168.10.1/32"}
// 	request1.Connection.Context.IpContext.DstRoutes = []*networkservice.Route{{Prefix: "192.168.10.1/32"}}
// 	request2 := request1.Clone()
// 	request3 := request1.Clone()

// 	// Recovery case
// 	conn1, err := srv.Request(context.Background(), request1)
// 	require.NoError(t, err)
// 	validateConn(t, conn1, "192.168.10.0/32", "192.168.10.1/32")

// 	// Recovery failed. Added new ones
// 	conn2, err := srv.Request(context.Background(), request2)
// 	require.NoError(t, err)
// 	validateConns(t, conn2, []string{"192.168.10.0/32", "192.168.0.0/32"}, []string{"192.168.10.1/32", "192.168.0.1/32"})

// 	// Close - addresses release
// 	_, err = srv.Close(context.Background(), conn1)
// 	require.NoError(t, err)

// 	// Recovery for another request
// 	conn3, err := srv.Request(context.Background(), request3)
// 	require.NoError(t, err)
// 	validateConn(t, conn3, "192.168.10.0/32", "192.168.10.1/32")
// }

// //nolint:dupl
// func TestRecoveryServerIPv6(t *testing.T) {
// 	srv := newIpamServer(context.Background(), t, "fe80::/64", 24)

// 	request1 := newRequest()
// 	request1.Connection.Context.IpContext.DstIpAddrs = []string{"fe80::fa00/128"}
// 	request1.Connection.Context.IpContext.SrcRoutes = []*networkservice.Route{{Prefix: "fe80::fa00/128"}}
// 	request1.Connection.Context.IpContext.SrcIpAddrs = []string{"fe80::fa01/128"}
// 	request1.Connection.Context.IpContext.DstRoutes = []*networkservice.Route{{Prefix: "fe80::fa01/128"}}
// 	request2 := request1.Clone()
// 	request3 := request1.Clone()

// 	// Recovery case
// 	conn1, err := srv.Request(context.Background(), request1)
// 	require.NoError(t, err)
// 	validateConn(t, conn1, "fe80::fa00/128", "fe80::fa01/128")

// 	// Recovery failed. Added new ones
// 	conn2, err := srv.Request(context.Background(), request2)
// 	require.NoError(t, err)
// 	validateConns(t, conn2, []string{"fe80::fa00/128", "fe80::/128"}, []string{"fe80::fa01/128", "fe80::1/128"})

// 	// Close - addresses release
// 	_, err = srv.Close(context.Background(), conn1)
// 	require.NoError(t, err)

// 	// Recovery for another request
// 	conn3, err := srv.Request(context.Background(), request3)
// 	require.NoError(t, err)
// 	validateConn(t, conn3, "fe80::fa00/128", "fe80::fa01/128")
// }

// //nolint:dupl
// func TestRecoveryServers(t *testing.T) {
// 	srv := chain.NewNetworkServiceServer(
// 		newIpamServer(context.Background(), t, "192.168.3.4/16", 24),
// 		newIpamServer(context.Background(), t, "fe80::/64", 24),
// 	)

// 	request1 := newRequest()
// 	request1.Connection.Context.IpContext.DstIpAddrs = []string{"192.168.10.0/32", "fe80::fa00/128"}
// 	request1.Connection.Context.IpContext.SrcRoutes = []*networkservice.Route{{Prefix: "192.168.10.0/32"}, {Prefix: "fe80::fa00/128"}}
// 	request1.Connection.Context.IpContext.SrcIpAddrs = []string{"192.168.10.1/32", "fe80::fa01/128"}
// 	request1.Connection.Context.IpContext.DstRoutes = []*networkservice.Route{{Prefix: "192.168.10.1/32"}, {Prefix: "fe80::fa01/128"}}
// 	request2 := request1.Clone()
// 	request3 := request1.Clone()

// 	// Recovery case
// 	conn1, err := srv.Request(context.Background(), request1)
// 	require.NoError(t, err)
// 	validateConns(t, conn1, []string{"192.168.10.0/32", "fe80::fa00/128"}, []string{"192.168.10.1/32", "fe80::fa01/128"})

// 	// Recovery failed. Added new ones
// 	conn2, err := srv.Request(context.Background(), request2)
// 	require.NoError(t, err)
// 	validateConns(t, conn2, []string{"192.168.10.0/32", "fe80::fa00/128", "192.168.0.0/32", "fe80::/128"}, []string{"192.168.10.1/32", "fe80::fa01/128", "192.168.0.1/32", "fe80::1/128"})

// 	// Close - addresses release
// 	_, err = srv.Close(context.Background(), conn1)
// 	require.NoError(t, err)

// 	// Recovery for another request
// 	conn3, err := srv.Request(context.Background(), request3)
// 	require.NoError(t, err)
// 	validateConns(t, conn3, []string{"192.168.10.0/32", "fe80::fa00/128"}, []string{"192.168.10.1/32", "fe80::fa01/128"})
// }
