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
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/golang/protobuf/ptypes/wrappers"
)

func headersToAdd(headers map[string]string) []*core.HeaderValueOption {
	res := make([]*core.HeaderValueOption, 0, len(headers))
	for headerName, headerVal := range headers {
		header := &core.HeaderValueOption{
			Header: &core.HeaderValue{
				Key:   headerName,
				Value: headerVal,
			},
			Append: &wrappers.BoolValue{
				// In Knative Serving, headers are set instead of appended.
				// Ref: https://github.com/knative/serving/pull/6366
				Value: false,
			},
		}

		res = append(res, header)

	}

	return res
}
