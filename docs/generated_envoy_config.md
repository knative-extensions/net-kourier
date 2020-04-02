# Generating Envoy configurations

This documents explains how Kourier generates Envoy configurations from Knative
and Kubernetes objects. This is a very simplified description, to understand all
the details, please check the code.

There are several Envoy objects that Kourier generates: listeners, HTTP
connection managers, routes, endpoints, clusters, etc. These are the
relationships between them:

- A listener has an HTTP connection manager.
- An HTTP connection manager has virtual hosts.
- A virtual host has routes.
- A route has weighted clusters.
- A weighted cluster is just a reference to a cluster plus a traffic %.

Here's a more detailed description of what each of those objects contains and
how Kourier builds them from Knative and Kubernetes objects:

- Listener: has port, IP, and the TLS config. Kourier creates an external
  listener for services exposed outside the cluster and an internal one that's
  only accessible from inside the cluster.
- HTTP connection manager: it's basically a collection of virtual hosts. There's
  one HTTP connection manager for each listener.
- Virtual Host: it has domains that Kourier gets from the Knative Ingress spec.
  For each rule of each ingress object, there's a private virtual host and a
  public one.
- Route: it has a path, a timeout, a retry policy, and a list of headers to add
  to the request. All this information is fetched from the Knative ingress
  object. A route also has weighted clusters.
- Weighted cluster: there's one for each traffic split defined in the ingress.
  Each weighted cluster contains a traffic % that can be fetched from the
  Knative ingress object, and also a reference to a cluster. Clusters have
  several attributes, but the most important one is the set of endpoints.
  Requests matching the domains and paths defined in the objects above will be
  forwarded to the URLs defined in those endpoints. The endpoints are not
  available in the Knative ingress object. We need to use the Kubernetes API to
  fetch all the endpoints that belong to a serving revision, and then, extract
  the port from the Kubernetes service associated to that serving revision.
