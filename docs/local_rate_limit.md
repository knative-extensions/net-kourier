# local rate limit implementation

This document explains how a local rate limit is implemented within envoy.

The local rate limit filter applies a token bucket rate limit to incoming connections
that are processed by the filter’s filter chain. Each connection processed by the
filter utilizes a single token, and if no tokens are available, the connection will
be immediately closed without further filter iteration.

Basically, what we need to add comparing to the original envoy configuration:
- the local rate limit filter to envoy filter chain
```
name: envoy.filters.http.local_ratelimit
typed_config:
  '@type': type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit
```
- the virtual host-specific configuration for local rate limit filter for each route
```
typed_per_filter_config:
    envoy.filters.http.local_ratelimit:
      '@type': type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit
      stat_prefix: http_local_rate_limiter
      token_bucket:
        max_tokens: <max_token>
        tokens_per_fill: <tokens_per_fill>
        fill_interval: <fill_interval>
```

The token bucket configuration to use for rate limiting connections that are processed by the filter’s
filter chain. Each incoming connection processed by the filter consumes a single token. If the token is
available, the connection will be allowed. If no tokens are available, the connection will be immediately
closed:
- `max_token` : The maximum tokens that the bucket can hold. This is also the number of tokens that the bucket initially contains.
- `tokens_per_fill`: The number of tokens added to the bucket during each fill interval. If not specified, defaults to a single token.
- `fill_interval`: The fill interval that tokens are added to the bucket.
                   During each fill interval tokens_per_fill are added to the bucket.
                   The bucket will never contain more than `max_tokens` tokens.

Optionally, we can add some other configuration to enforce

```
filter_enabled:
  default_value:
    numerator: 100
    denominator: HUNDRED
filter_enforced:
  default_value:
    numerator: 100
    denominator: HUNDRED
```
`filter_enabled`: % of requests that will check the local rate limit decision, but not enforce, for a given route_key specified in the
                  local rate limit configuration. Defaults to 0.
`filter_enforced`: % of requests that will enforce the local rate limit decision for a given route_key specified in the local rate limit
                   configuration. Defaults to 0.

###Comparing envoy generated config before and after local rate limit implementation
- [before](./withoutLocalRateLimit.yaml)
- [after](./withLocalRateLimit.yaml)

###Test

Using [hey](https://github.com/rakyll/hey), we send 20 requests simultaneously 

before:
```
hey -c 20 -n 20 https://scalewayratelimitd16wgy1x-first.functions.fnc.dev.fr-par.scw.cloud                                                                                                                                                           ✔

Summary:
  Total:	3.6871 secs
  Slowest:	3.6860 secs
  Fastest:	0.4105 secs
  Average:	1.8633 secs
  Requests/sec:	5.4244


Response time histogram:
  0.410 [1]	|■■■■■■■
  0.738 [6]	|■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■
  1.066 [3]	|■■■■■■■■■■■■■■■■■■■■
  1.393 [0]	|
  1.721 [0]	|
  2.048 [0]	|
  2.376 [0]	|
  2.703 [2]	|■■■■■■■■■■■■■
  3.031 [3]	|■■■■■■■■■■■■■■■■■■■■
  3.358 [2]	|■■■■■■■■■■■■■
  3.686 [3]	|■■■■■■■■■■■■■■■■■■■■


Latency distribution:
  10% in 0.5029 secs
  25% in 0.7127 secs
  50% in 2.6045 secs
  75% in 3.0868 secs
  90% in 3.6025 secs
  95% in 3.6860 secs
  0% in 0.0000 secs

Details (average, fastest, slowest):
  DNS+dialup:	0.0943 secs, 0.4105 secs, 3.6860 secs
  DNS-lookup:	0.0428 secs, 0.0426 secs, 0.0430 secs
  req write:	0.0001 secs, 0.0000 secs, 0.0008 secs
  resp wait:	1.7623 secs, 0.3065 secs, 3.5816 secs
  resp read:	0.0059 secs, 0.0001 secs, 0.0624 secs

Status code distribution:
  [200]	20 responses
```
after
```
hey -c 20 -n 20 https://scalewayratelimitd16wgy1x-first.functions.fnc.dev.fr-par.scw.cloud                                                                                                                                                           ✔

Summary:
  Total:	0.7458 secs
  Slowest:	0.7456 secs
  Fastest:	0.0621 secs
  Average:	0.2630 secs
  Requests/sec:	26.8159

  Total data:	180 bytes
  Size/request:	9 bytes

Response time histogram:
  0.062 [1]	|■■■■
  0.130 [9]	|■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■
  0.199 [1]	|■■■■
  0.267 [2]	|■■■■■■■■■
  0.335 [0]	|
  0.404 [2]	|■■■■■■■■■
  0.472 [1]	|■■■■
  0.541 [0]	|
  0.609 [1]	|■■■■
  0.677 [1]	|■■■■
  0.746 [2]	|■■■■■■■■■


Latency distribution:
  10% in 0.0682 secs
  25% in 0.0706 secs
  50% in 0.1728 secs
  75% in 0.4548 secs
  90% in 0.7388 secs
  95% in 0.7456 secs
  0% in 0.0000 secs

Details (average, fastest, slowest):
  DNS+dialup:	0.0396 secs, 0.0621 secs, 0.7456 secs
  DNS-lookup:	0.0014 secs, 0.0010 secs, 0.0016 secs
  req write:	0.0001 secs, 0.0000 secs, 0.0003 secs
  resp wait:	0.2101 secs, 0.0081 secs, 0.6933 secs
  resp read:	0.0035 secs, 0.0000 secs, 0.0136 secs

Status code distribution:
  [200]	10 responses
  [429]	10 responses
```
So after implementation of local rate limit, we can see that only half of requests have been accepted
and the other half have been refused with `429 http code`
