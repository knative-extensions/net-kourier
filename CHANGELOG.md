# Change Log
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/) 
and this project adheres to [Semantic Versioning](http://semver.org/).

## [0.2.1] - 2019-11-04
### CHANGED
- Updated envoy go control plane dependency to v0.9.0
- Get the cluster local domain automatically from the "/etc/resolv.conf" file
- Replaced deprecated instructions from envoy bootstrap config.
### FIXED
- Fix for a "missing Route" issue when revisions where replaced/modified too quickly.

## [0.2.0] - 2019-10-25
### Added
- New kourier-gateway docker image
### Changed
- Splitted the kourier POD into two new PODS, kourier-control and kourier-gateway.
- Kourier filters knative serving ingress.class, looks for: 'kourier.ingress.networking.knative.dev'.

## [0.1.0] - 2019-10-23

First release.
