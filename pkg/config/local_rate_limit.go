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

package config

import (
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	rateLimitService "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/duration"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"os"
	"strconv"
)

const (
	localRateLimitFilterName = "envoy.filters.http.local_ratelimit"
	statePrefixName          = "http_local_rate_limiter"
	bucketFillInterval       = 1
	numerator                = 100
)

// LocalRateLimit is the configuration of rate limit.
var LocalRateLimit = &RateLimitConfig{
	Enabled: false,
}

// RateLimitConfig specifies parameters for rate limit configuration.
type RateLimitConfig struct {
	Enabled      bool
	HTTPFilter   *hcm.HttpFilter
	FilterConfig map[string]*any.Any
}

func init() {

	bucketMaxToken := os.Getenv("LOCAL_RATE_LIMIT_BUCKET_MAX_TOKEN")
	bucketTokenPerFill := os.Getenv("LOCAL_RATE_LIMIT_BUCKET_TOKEN_PER_FILL")

	if !(bucketMaxToken != "" && bucketTokenPerFill != "") {
		// no local rate limit setup
		return

	}

	bucketToken, err := strconv.Atoi(bucketMaxToken)
	if err != nil {
		panic(err)
	}

	bucketFill, err := strconv.Atoi(bucketTokenPerFill)
	if err != nil {
		panic(err)
	}

	LocalRateLimit = &RateLimitConfig{
		Enabled:      true,
		HTTPFilter:   rateLimitFilter(),
		FilterConfig: rateLimitFilterConfig(uint32(bucketToken), uint32(bucketFill)),
	}
}

func rateLimitFilterConfig(bucketMaxToken, bucketTokenPerFill uint32) map[string]*any.Any {

	c := &rateLimitService.LocalRateLimit{
		StatPrefix: statePrefixName,
		TokenBucket: &envoy_type_v3.TokenBucket{
			MaxTokens:     bucketMaxToken,
			TokensPerFill: wrapperspb.UInt32(bucketTokenPerFill),
			FillInterval:  &duration.Duration{Seconds: bucketFillInterval},
		},
		FilterEnabled: &envoy_core_v3.RuntimeFractionalPercent{
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   numerator,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
		},
		FilterEnforced: &envoy_core_v3.RuntimeFractionalPercent{
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   numerator,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
		},
	}

	filterConfig, err := anypb.New(c)
	if err != nil {
		panic(err)
	}

	return map[string]*any.Any{
		localRateLimitFilterName: filterConfig,
	}
}

func rateLimitFilter() *hcm.HttpFilter {

	rateLimitConfigConfig := &rateLimitService.LocalRateLimit{
		StatPrefix: statePrefixName,
	}

	envoyConf, err := anypb.New(rateLimitConfigConfig)
	if err != nil {
		panic(err)
	}

	return &hcm.HttpFilter{
		Name: localRateLimitFilterName,
		ConfigType: &hcm.HttpFilter_TypedConfig{
			TypedConfig: envoyConf,
		},
	}
}
