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

package memory_test

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/networkservicemesh/api/pkg/api/ipam"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/grpc"

	"google.golang.org/grpc/credentials/insecure"

	"github.com/networkservicemesh/sdk/pkg/ipam/chains/memory"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"
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

func Test_IPAM_Endpoint_Allocate_Different_CIDRS_Different_NS(t *testing.T) {
	t.Cleanup(func() {
		goleak.VerifyNone(t)
	})

	var ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	connectTO := newMemoryIpamServer(ctx, t, "172.16.0.0/16", 24)

	for i := 0; i < 10; i++ {
		c := newIPAMClient(ctx, t, &connectTO)

		endpoint, err := c.RegisterEndpoint(ctx, &ipam.Endpoint{
			Name: fmt.Sprintf("endpoint-%d", i),
			NetworkServiceNames: []string{fmt.Sprintf("ns-%d", i)},
		})

		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("172.16.%v.0/24", i), endpoint.Prefix, i)
		require.NotEmpty(t, endpoint.ExcludePrefixes)
	}
}

func Test_IPAM_Endpoint_Allocate_Same_CIDRS_Same_NS(t *testing.T) {
	t.Cleanup(func() {
		goleak.VerifyNone(t)
	})

	var ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	connectTO := newMemoryIpamServer(ctx, t, "172.16.0.0/16", 24)

	for i := 0; i < 10; i++ {
		c := newIPAMClient(ctx, t, &connectTO)

		endpoint, err := c.RegisterEndpoint(ctx, &ipam.Endpoint{
			Name: fmt.Sprintf("endpoint-%d", i),
			NetworkServiceNames: []string{"same-ns"},
		})

		require.NoError(t, err)
		require.Equal(t, "172.16.0.0/24", endpoint.Prefix, i)
		require.NotEmpty(t, endpoint.ExcludePrefixes)
	}
}

func Test_IPAM_Endpoint_Allocate_Unique_CIDRS_Same_NS(t *testing.T) {
	t.Cleanup(func() {
		goleak.VerifyNone(t)
	})

	var ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	connectTO := newMemoryIpamServer(ctx, t, "172.16.0.0/16", 24)

	for i := 0; i < 10; i++ {
		c := newIPAMClient(ctx, t, &connectTO)

		endpoint, err := c.RegisterEndpoint(ctx, &ipam.Endpoint{
			Name: fmt.Sprintf("endpoint-%d", i),
			NetworkServiceNames: []string{"same-ns"},
			UniqueCidr: true,
		})

		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("172.16.%d.0/24", i), endpoint.Prefix, i)
		require.NotEmpty(t, endpoint.ExcludePrefixes)
	}
}

func Test_IPAM_Same_Client_Allocate_Twice(t *testing.T) {
	t.Cleanup(func() {
		goleak.VerifyNone(t)
	})

	var ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	connectTO := newMemoryIpamServer(ctx, t, "172.31.0.0/16", 24)

	c := newIPAMClient(ctx, t, &connectTO)

	endpoint, err := c.RegisterEndpoint(ctx, &ipam.Endpoint{
		Name: "endpoint-1",
		NetworkServiceNames: []string{"ns-1"},
	})

	require.NoError(t, err)
	require.Equal(t, "172.31.0.0/24", endpoint.Prefix, "endpoint-1")
	require.NotEmpty(t, endpoint.ExcludePrefixes)


	client, err := c.RegisterClient(ctx, &ipam.Client{
		Id: "1",
		Name: "client-1",
		NetworkServiceNames: []string{"ns-1"},
		ExcludePrefixes: endpoint.ExcludePrefixes,
		EndpointName: "endpoint-1",
	})

	require.NoError(t, err)
	require.Equal(t, "172.31.0.1/32", client.SrcAddress[0], "client-1")
	require.Equal(t, "172.31.0.0/32", client.DstAddress[0], "client-1")
	require.Equal(t, "172.31.0.0/24", client.Prefix, "client-1")
	require.NotEmpty(t, client.ExcludePrefixes)


	sameClient, err := c.RegisterClient(ctx, &ipam.Client{
		Id: "1",
		Name: "client-1",
		NetworkServiceNames: []string{"ns-1"},
		ExcludePrefixes: endpoint.ExcludePrefixes,
		EndpointName: "endpoint-1",
	})

	require.NoError(t, err)
	require.Equal(t, "172.31.0.1/32", sameClient.SrcAddress[0], "client-1")
	require.Equal(t, "172.31.0.0/32", sameClient.DstAddress[0], "client-1")
	require.NotEmpty(t, sameClient.ExcludePrefixes)
}


func Test_IPAM_Different_Client_Allocate_Twice(t *testing.T) {
	t.Cleanup(func() {
		goleak.VerifyNone(t)
	})

	var ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	connectTO := newMemoryIpamServer(ctx, t, "172.31.0.0/16", 24)

	c := newIPAMClient(ctx, t, &connectTO)

	endpoint, err := c.RegisterEndpoint(ctx, &ipam.Endpoint{
		Name: "endpoint-1",
		NetworkServiceNames: []string{"ns-1"},
	})

	require.NoError(t, err)
	require.Equal(t, "172.31.0.0/24", endpoint.Prefix, "endpoint-1")
	require.NotEmpty(t, endpoint.ExcludePrefixes)


	client, err := c.RegisterClient(ctx, &ipam.Client{
		Id: "1",
		Name: "client-1",
		NetworkServiceNames: []string{"ns-1"},
		ExcludePrefixes: endpoint.ExcludePrefixes,
		EndpointName: "endpoint-1",
	})

	require.NoError(t, err)
	require.Equal(t, "172.31.0.1/32", client.SrcAddress[0], "client-1")
	require.Equal(t, "172.31.0.0/32", client.DstAddress[0], "client-1")
	require.NotEmpty(t, client.ExcludePrefixes)


	client2, err := c.RegisterClient(ctx, &ipam.Client{
		Id: "1",
		Name: "client-2",
		NetworkServiceNames: []string{"ns-1"},
		ExcludePrefixes: endpoint.ExcludePrefixes,
		EndpointName: "endpoint-1",
	})

	require.NoError(t, err)
	require.Equal(t, "172.31.0.3/32", client2.SrcAddress[0], "client-2")
	require.Equal(t, "172.31.0.2/32", client2.DstAddress[0], "client-2")
	require.NotEmpty(t, client2.ExcludePrefixes)
}


func Test_IPAM_Different_Client_Twice_Different_Endpoints_Unique_CIDR(t *testing.T) {
	t.Cleanup(func() {
		goleak.VerifyNone(t)
	})

	var ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	connectTO := newMemoryIpamServer(ctx, t, "172.31.0.0/16", 24)

	c := newIPAMClient(ctx, t, &connectTO)

	endpoint, err := c.RegisterEndpoint(ctx, &ipam.Endpoint{
		Name: "endpoint-1",
		NetworkServiceNames: []string{"ns-1"},
	})

	require.NoError(t, err)
	require.Equal(t, "172.31.0.0/24", endpoint.Prefix, "endpoint-1")
	require.NotEmpty(t, endpoint.ExcludePrefixes)


	_, err = c.RegisterEndpoint(ctx, &ipam.Endpoint{
		Name: "endpoint-2",
		NetworkServiceNames: []string{"ns-1"},
	})

	require.NoError(t, err)
	require.Equal(t, "172.31.0.0/24", endpoint.Prefix, "endpoint-2")
	require.NotEmpty(t, endpoint.ExcludePrefixes)

	client, err := c.RegisterClient(ctx, &ipam.Client{
		Id: "1",
		Name: "client-1",
		NetworkServiceNames: []string{"ns-1"},
		ExcludePrefixes: endpoint.ExcludePrefixes,
		EndpointName: "endpoint-1",
	})

	require.NoError(t, err)
	require.Equal(t, "172.31.0.1/32", client.SrcAddress[0], "client-1")
	require.Equal(t, "172.31.0.0/32", client.DstAddress[0], "client-1")
	require.NotEmpty(t, client.ExcludePrefixes)


	client2, err := c.RegisterClient(ctx, &ipam.Client{
		Id: "1",
		Name: "client-1",
		NetworkServiceNames: []string{"ns-1"},
		ExcludePrefixes: endpoint.ExcludePrefixes,
		EndpointName: "endpoint-2",
	})

	require.NoError(t, err)
	require.Equal(t, "172.31.0.1/32", client2.SrcAddress[0], "client-2")
	require.Equal(t, "172.31.0.0/32", client2.DstAddress[0], "client-2")
	require.NotEmpty(t, client2.ExcludePrefixes)
}



func Test_IPAM_Client_Allocate_Two_NS(t *testing.T) {
	t.Cleanup(func() {
		goleak.VerifyNone(t)
	})

	var ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	connectTO := newMemoryIpamServer(ctx, t, "172.31.0.0/16", 24)

	c := newIPAMClient(ctx, t, &connectTO)

	endpoint, err := c.RegisterEndpoint(ctx, &ipam.Endpoint{
		Name: "endpoint-1",
		NetworkServiceNames: []string{"ns-1"},
	})

	require.NoError(t, err)
	require.Equal(t, "172.31.0.0/24", endpoint.Prefix, "endpoint-1")
	require.NotEmpty(t, endpoint.ExcludePrefixes)


	endpoint2, err := c.RegisterEndpoint(ctx, &ipam.Endpoint{
		Name: "endpoint-2",
		NetworkServiceNames: []string{"ns-2"},
	})

	require.NoError(t, err)
	require.Equal(t, "172.31.1.0/24", endpoint2.Prefix, "endpoint-2")
	require.NotEmpty(t, endpoint2.ExcludePrefixes)


	client, err := c.RegisterClient(ctx, &ipam.Client{
		Id: "1",
		Name: "client-1",
		NetworkServiceNames: []string{"ns-1"},
		ExcludePrefixes: nil,
		EndpointName: "endpoint-1",
	})

	require.NoError(t, err)
	require.Equal(t, "172.31.0.1/32", client.SrcAddress[0], "client-1")
	require.Equal(t, "172.31.0.0/32", client.DstAddress[0], "client-1")

	client2, err := c.RegisterClient(ctx, &ipam.Client{
		Id: "2",
		Name: "client-2",
		NetworkServiceNames: []string{"ns-2"},
		ExcludePrefixes: nil,
		EndpointName: "endpoint-2",
	})

	require.NoError(t, err)
	require.Equal(t, "172.31.1.1/32", client2.SrcAddress[0], "client-2")
	require.Equal(t, "172.31.1.0/32", client2.DstAddress[0], "client-2")

}


func Test_IPAM_Client_Allocate_SameNS_UniqueCIDR(t *testing.T) {
	t.Cleanup(func() {
		goleak.VerifyNone(t)
	})

	var ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	connectTO := newMemoryIpamServer(ctx, t, "172.31.0.0/16", 24)

	c := newIPAMClient(ctx, t, &connectTO)

	endpoint, err := c.RegisterEndpoint(ctx, &ipam.Endpoint{
		Name: "endpoint-1",
		NetworkServiceNames: []string{"ns-1"},
		UniqueCidr: true,
	})

	require.NoError(t, err)
	require.Equal(t, "172.31.0.0/24", endpoint.Prefix, "endpoint-1")
	require.NotEmpty(t, endpoint.ExcludePrefixes)


	endpoint2, err := c.RegisterEndpoint(ctx, &ipam.Endpoint{
		Name: "endpoint-2",
		NetworkServiceNames: []string{"ns-1"},
		UniqueCidr: true,
	})

	require.NoError(t, err)
	require.Equal(t, "172.31.1.0/24", endpoint2.Prefix, "endpoint-2")
	require.NotEmpty(t, endpoint2.ExcludePrefixes)


	client, err := c.RegisterClient(ctx, &ipam.Client{
		Id: "1",
		Name: "client-1",
		NetworkServiceNames: []string{"ns-1"},
		ExcludePrefixes: nil,
		EndpointName: "endpoint-1",
	})

	require.NoError(t, err)
	require.Equal(t, "172.31.0.1/32", client.SrcAddress[0], "client-1")
	require.Equal(t, "172.31.0.0/32", client.DstAddress[0], "client-1")

	client2, err := c.RegisterClient(ctx, &ipam.Client{
		Id: "2",
		Name: "client-2",
		NetworkServiceNames: []string{"ns-1"},
		ExcludePrefixes: nil,
		EndpointName: "endpoint-2",
	})

	require.NoError(t, err)
	require.Equal(t, "172.31.1.1/32", client2.SrcAddress[0], "client-2")
	require.Equal(t, "172.31.1.0/32", client2.DstAddress[0], "client-2")

}

func Test_IPAM_Client_Allocate_Endpoint_Not_Exists(t *testing.T) {
	t.Cleanup(func() {
		goleak.VerifyNone(t)
	})

	var ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	connectTO := newMemoryIpamServer(ctx, t, "172.31.0.0/16", 24)

	c := newIPAMClient(ctx, t, &connectTO)

	_, err := c.RegisterClient(ctx, &ipam.Client{
		Id: "1",
		Name: "client-1",
		NetworkServiceNames: []string{"ns-1"},
		ExcludePrefixes: nil,
	})

	require.ErrorContains(t, err, memory.ErrEndpointNameUndefined.Error())
}