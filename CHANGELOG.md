# Change Log
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/) 
and this project adheres to [Semantic Versioning](http://semver.org/).

## [0.2.3] - 2019-11-11
### Removed
- Dropped support for ClusterIngress CRD, which means dropping support for
Knative Serving < 0.9.

## [0.2.2] - 2019-11-06
### Fixed
- Previous "missing Route" fix was not covering all the cases. Now it's fixed with the implementation of cache for clusters, details can be found in the source code.

## [0.2.1] - 2019-11-04
### Changed
- Updated envoy go control plane dependency to v0.9.0
- Get the cluster local domain automatically from the "/etc/resolv.conf" file
- Replaced deprecated instructions from envoy bootstrap config.
### Fixed
- Fix for a "missing Route" issue when revisions where replaced/modified too quickly.

## [0.2.0] - 2019-10-25
### Added
- New kourier-gateway docker image
### Changed
- Splitted the kourier POD into two new PODS, kourier-control and kourier-gateway.
- Kourier filters knative serving ingress.class, looks for: 'kourier.ingress.networking.knative.dev'.

## [0.1.0] - 2019-10-23

First release.

[0.2.3]: https://github.com/3scale/kourier/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/3scale/kourier/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/3scale/kourier/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/3scale/kourier/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/3scale/kourier/releases/tag/v0.1.0
