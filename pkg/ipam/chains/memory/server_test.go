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

	"github.com/networkservicemesh/sdk/pkg/registry/chains/memory"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"
)

func newMemoryIpamServer(ctx context.Context, t *testing.T, prefix string, initialSize uint8) url.URL {
	var s = grpc.NewServer()
	ipam.RegisterIPAMServer(s, memory.NewIPAMServer(prefix, initialSize))

	var serverAddr url.URL

	require.Len(t, grpcutils.ListenAndServe(ctx, &serverAddr, s), 0)

	return serverAddr
}

func newVL3IPAMClient(ctx context.Context, t *testing.T, connectTO *url.URL) ipam.IPAMClient {
	var cc, err = grpc.DialContext(
		ctx, grpcutils.URLToTarget(connectTO),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	go func() {
		<-ctx.Done()
		_ = cc.Close()
	}()

	return ipam.NewIPAMClient(cc)
}

func Test_vl3_IPAM_Allocate(t *testing.T) {
	t.Cleanup(func() {
		goleak.VerifyNone(t)
	})

	var ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	connectTO := newMemoryIpamServer(ctx, t, "172.16.0.0/16", 24)

	for i := 0; i < 10; i++ {
		c := newVL3IPAMClient(ctx, t, &connectTO)

		var stream, err = c.ManagePrefixes(ctx)

		require.NoError(t, err, i)

		err = stream.Send(&ipam.PrefixRequest{
			Type: ipam.Type_ALLOCATE,
		})

		require.NoError(t, err)

		resp, err := stream.Recv()
		require.NoError(t, err)

		require.Equal(t, fmt.Sprintf("172.16.%v.0/24", i), resp.Prefix, i)
		require.NotEmpty(t, resp.ExcludePrefixes)
	}
}

func Test_vl3_IPAM_Allocate2(t *testing.T) {
	t.Cleanup(func() {
		goleak.VerifyNone(t)
	})

	var ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	connectTO := newMemoryIpamServer(ctx, t, "173.16.0.0/16", 24)

	for i := 0; i < 10; i++ {
		clientCTX, cancel := context.WithCancel(ctx)
		c := newVL3IPAMClient(clientCTX, t, &connectTO)

		var stream, err = c.ManagePrefixes(clientCTX)
		require.NoError(t, err, i)

		err = stream.Send(&ipam.PrefixRequest{
			Type: ipam.Type_ALLOCATE,
		})

		require.NoError(t, err)

		resp, err := stream.Recv()
		require.NoError(t, err)

		require.Equal(t, "173.16.0.0/24", resp.Prefix, i)
		require.NotEmpty(t, resp.ExcludePrefixes, i)
		cancel()
		time.Sleep(time.Millisecond * 50)
	}
}

func Test_vl3_IPAM_Allocate3(t *testing.T) {
	t.Cleanup(func() {
		goleak.VerifyNone(t)
	})

	var ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	connectTO := newMemoryIpamServer(ctx, t, "172.16.0.0/16", 24)

	for i := 0; i < 10; i++ {
		clientCTX, cancel := context.WithCancel(ctx)
		c := newVL3IPAMClient(clientCTX, t, &connectTO)

		var stream, err = c.ManagePrefixes(clientCTX)
		require.NoError(t, err, i)

		err = stream.Send(&ipam.PrefixRequest{
			Type:   ipam.Type_ALLOCATE,
			Prefix: "172.16.0.0/30",
		})

		require.NoError(t, err)

		resp, err := stream.Recv()
		require.NoError(t, err)

		require.Equal(t, "172.16.0.0/30", resp.Prefix, i)
		require.NotEmpty(t, resp.ExcludePrefixes, i)
		cancel()
		time.Sleep(time.Millisecond * 50)
	}
}
