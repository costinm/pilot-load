# Pilot Load Testing

This repo contains tooling to perform low cost load testing of Pilot.

## Architecture

A standalone etcd and api-server will be deployed to namespace `pilot-load`. These will be used just as an object store
essentially - there is no kubelet, controller-manager, kube-proxy, etc. No pods are scheduled onto any physical machine, and
exist only as objects in the data base.

Another deployment, `pilot-load` is also deployed, and will connect to our fake api-server. Based on the config provided,
Pods, Services, Endpoints, and Istio configuration will be applied to the api-server. Because there is no controller-manager, we need
to manage some objects ourselves, such as keeping Endpoints in sync with pods.

Pilot will be modified to use a KUBECONFIG pointing to our fake api-server. As a result, it will get events from the api-server
just like it would in a real cluster, but without the cost. This exercises the exact same code paths as a real cluster; it is not using
any "fake" client.

In a real cluster, each pod adds additional load as well. These are also handled. When a pod is created, we will directly send
an injection request and CSR, simulating what a real pod would do. We will then start an XDS connection to Pilot, simulating the
same pattern that a real Envoy would.

Overall, we get something that is very close to the load on a real cluster, with much smaller overhead. etcd and api-server generally use
less than 1 CPU/1 GB combined, and pilot-load will use roughly 1 CPU/1k pods. Compared to 1 CPU/4 pods, we get a ~250x efficiency gain. Additionally,
because we run our own api-server without rate limits, we can perform operations much faster.

The expense of this is dropping coverage:
* No coverage of any data plane aspects
* Less coverage of overloading Kubernetes
* The simulated behavior may not exactly match reality

## Getting Started

1. Install Istio in cluster. No special configuration is needed. You may want to ensure no Envoy's are connected, as they will be sent invalid configuration

1. Install `pilot-load` by running `go install`.

1. Run [`./install/deploy.sh`](./install/deploy.sh). This will configure the api-server and kubeconfig to access it. It will also bootstrap the cluster with CRDs and telemetry filters.

1. Restart istiod to pick up the new kubeconfig: `kubectl rollout restart deployment -n istio-system istiod`.

1. Deploy the load test

    1. In cluster:

      ```shell script
      # Select a configuration to run
      kubectl apply -f install/configs/canonical.yaml
      # Apply the actual deployment
      kubectl apply -f install/load-deployment.yaml
      ```

    1. Locally:

      ```shell script
      # Connect to the remote kubeconfig
      kubectl port-forward -n pilot-load svc/apiserver 18090
      export KUBECONFIG=install/local-kubeconfig.yaml
      # Connect to Istiod, if its not running locally as well
      kubectl port-forward -n istio-system svc/istiod 15010
      # Apply the actual deployment
      pilot-load cluster --config example-config.yaml
      ```
1. Optional: Import the [load testing dashboard](./install/dashboard.json) in Grafana.

## Discovery Address

All commands take a few flags related to connecting to Istiod. Some common examples:

```shell
pilot-load adsc --pilot-address foo.com:80 --auth plaintext # no auth at all
pilot-load adsc --pilot-address meshconfig.googleapis.com # infers google auth. Cluster information is inferred but can be set explicitly
```

## XDS Only

To just simulate XDS connections, without any api-server interaction, the adsc mode can be used:

```shell script
pilot-load adsc --count=2
```

This will start up two XDS connections.

NOTE: these connections will not be associated with any Services, and as such will get a different config than real pods, including sidecar scoping.

## Ingress Prober

Note: this is independent of the above fake api server and can be run on a real cluster.

This test continuously applies virtual services and sends traffic to see how long it takes for a virtual service to become ready.

Usage: `pilot-load prober --replicas=1000 --delay=1s`.

## Reproduce

The `reproduce` command allows replaying a cluster's configuration.

First, capture their current cluster config: `kubectl get vs,gw,dr,sidecar,svc,endpoints,pod,namespace -oyaml -A | kubectl grep`

Then:

```shell script
pilot-load reproduce -f my-config.yaml --delay=50ms
```

This will deploy all of the configs to the cluster, except Pods. For each pod, an XDS connection simulating that pod will be made.
Some resources are slightly modified to allow running in a cluster they were not originally in, such as Service selectors.

## Pod startup speed

The `startup` command tests pod startup times

Example usage:

```shell script
pilot-load startup --namespace default --concurrency 2
```

This will spin up 2 workers which will continually spawn pods, measure the latency, and then terminate them.
That is, there will be roughly 2 pods at all times with the command above.

Latency is report as each pod completes, and summary when the process is terminated.

Pods spin up a simple alpine image and serve a readiness probe doing a TCP health check.

Note: if testing Istio/etc, ensure the namespace specified has sidecar injection enabled.

Example:
```
2022-05-06T16:51:17.486681Z     info    Report: scheduled:0s    init:12.647s    ready:13.647s   full ready:1.417s       complete:14.065s        name:startup-test-kytobohu
2022-05-06T16:51:18.507336Z     info    Report: scheduled:0s    init:14.419s    ready:15.419s   full ready:1.436s       complete:15.856s        name:startup-test-ukbwqdfl
2022-05-06T16:51:18.555901Z     info    Avg:    scheduled:0s    init:7.673032973s       ready:1s        full ready:1.730793412s complete:9.403826385s
2022-05-06T16:51:18.556263Z     info    Max:    scheduled:0s    init:14.419771526s      ready:1s        full ready:3.515997645s complete:15.856143083s
```

Metric meanings:

|Metric| Meaning                                                                                                                                                            |
|------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|scheduled| Time from start until init container starts (TODO: this is always 0 without sideacr)                                                                               |
|init| Time from `scheduled` until the application container starts                                                                                                       |
|ready| Time from application container starting until kubelet reports readiness                                                                                           |
|full ready| Time from application container starting until the Pod spec is fully declared as "Ready". This may be high than `ready` due to latency in kubelet updating the Pod |
|complete| End to end time to completion|
