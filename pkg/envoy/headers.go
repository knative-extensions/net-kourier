package envoy

import (
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/golang/protobuf/ptypes/wrappers"
)

func headersToAdd(headers map[string]string) []*core.HeaderValueOption {
	var res []*core.HeaderValueOption

	for headerName, headerVal := range headers {
		header := core.HeaderValueOption{
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

		res = append(res, &header)

	}

	return res
}
