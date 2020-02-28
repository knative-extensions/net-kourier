/*
 Copyright 2020 The Knative Authors
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.

*/

package envoy

import (
	"testing"

	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"gotest.tools/assert"

	envoy_api_v2_endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
)

func TestNewLBEndpoint(t *testing.T) {
	ip := "127.0.0.1"
	port := uint32(8080)

	endpoint := NewLBEndpoint(ip, port)

	lbEndpoint := endpoint.GetHostIdentifier().(*envoy_api_v2_endpoint.LbEndpoint_Endpoint).Endpoint
	socketAddress := lbEndpoint.GetAddress().GetSocketAddress()
	assert.Equal(t, ip, socketAddress.Address)
	assert.Equal(t, port, socketAddress.PortSpecifier.(*core.SocketAddress_PortValue).PortValue)
}
