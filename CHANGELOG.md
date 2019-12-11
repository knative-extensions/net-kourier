# Change Log
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/) 
and this project adheres to [Semantic Versioning](http://semver.org/).

## [0.3.4] - 2019-12-11
### Security
- Updated Kourier Gateway to use envoy version 1.12.2

## [0.3.3] - 2019-12-05
### Fixed
- Bug that caused some Envoy clusters to be deleted when they were still
referenced by a route.

## [0.3.2] - 2019-12-04
### Changed
- Checking whether an ingress should be marked as ready is no longer done
online. It's done in separate go routines.

### Fixed
- "concurrent map writes" errors caused by an incorrect usage of locks.

## [0.3.1] - 2019-12-03
### Changed
- Instead of refreshing the whole Envoy config, now Kourier updates only the
parts that affect the modified ingress or endpoint.

## [0.3.0] - 2019-11-29
### Changed
- Adapted the codebase to use Knative's controllers and reconcilers.

## [0.2.6] - 2019-11-14
### Fixed
- Kourier now only routes the public endpoints object of a revision.

## [0.2.5] - 2019-11-13
### Changed
- Added readiness probe to the kourier gateway pod.

## [0.2.4] - 2019-11-13
### Fixed
- Now a knative Ingress is not marked as ready until all the kourier gateways are in sync with the latest configuration version.

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

[0.3.4]: https://github.com/3scale/kourier/compare/v0.3.3...v0.3.4
[0.3.3]: https://github.com/3scale/kourier/compare/v0.3.2...v0.3.3
[0.3.2]: https://github.com/3scale/kourier/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/3scale/kourier/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/3scale/kourier/compare/v0.2.6...v0.3.0
[0.2.6]: https://github.com/3scale/kourier/compare/v0.2.5...v0.2.6
[0.2.5]: https://github.com/3scale/kourier/compare/v0.2.4...v0.2.5
[0.2.4]: https://github.com/3scale/kourier/compare/v0.2.3...v0.2.4
[0.2.3]: https://github.com/3scale/kourier/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/3scale/kourier/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/3scale/kourier/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/3scale/kourier/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/3scale/kourier/releases/tag/v0.1.0
