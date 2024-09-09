## Integration tests

### Prerequisites

The integration/e2e tests have the following prerequisites:

  * make sure to have ssh key in aws `capi`in `us-east-1 region` if you do not have key refer
    to CAPI on [AWS prerequisites documentation](https://cluster-api-aws.sigs.k8s.io/topics/using-clusterawsadm-to-fulfill-prerequisites#ssh-key-pair)

  * local testing requires the following to be available in the PATH: `clusterctl`, `kubectl`, `helm`

  * a management cluster initialised via `clusterctl` with the infrastructure targeted as well as the version of the MicroK8s providers we want to be tested

  * the `kubeconfig` of the management cluster in the default location `$HOME/.kube/config`


### Running the tests

For local testing, make sure your have the above prerequisites.

#### Checkout to the branch of code you want to test on:

```bash
git clone https://github.com/canonical/cluster-api-bootstrap-provider-microk8s bootstrap -b "<branch-name>"
git clone https://github.com/canonical/cluster-api-control-plane-provider-microk8s control-plane -b "<branch-name>"
```

#### Install microk8s and enable the addons

```bash
snap install microk8s --channel latest/beta --classic
microk8s status --wait-ready
microk8s enable rbac dns
mkdir ~/.kube -p
microk8s config > ~/.kube/config
```

#### Initialize infrastructure provider

Visit [here](https://cluster-api.sigs.k8s.io/user/quick-start.html#initialization-for-common-providers) for a list of common infrastructure providers.

```bash
  clusterctl init --infrastructure <infra> --bootstrap - --control-plane -
```

#### Build Docker images and release manifests from the checked out source code

Build and push a docker image for the bootstrap provider.
```bash
cd bootstrap
docker build -t <username>/capi-bootstrap-provider-microk8s:<tag> .
docker push <username>/capi-bootstrap-provider-microk8s:<tag>
sed "s,docker.io/cdkbot/capi-bootstrap-provider-microk8s:latest,docker.io/<username>/capi-bootstrap-provider-microk8s:<tag>," -i bootstrap-components.yaml
```

Similarly, for control-plane provider
```bash
cd control-plane
docker build -t <username>/capi-control-plane-provider-microk8s:<tag> .
docker push <username>/capi-control-plane-provider-microk8s:<tag>
sed "s,docker.io/cdkbot/capi-control-plane-provider-microk8s:latest,docker.io/<username>/capi-control-plane-provider-microk8s:<tag>," -i control-plane-components.yaml
```

#### Deploy microk8s providers

```bash
kubectl apply -f bootstrap/bootstrap-components.yaml -f control-plane/control-plane-components.yaml
```
### Cluster definitions for e2e

Cluster definitions are stored in the [`manifests`](./cluster-manifests) directory.

#### Trigger the e2e tests

```bash
make e2e
```

#### Remove the test runs

```bash
microk8s kubectl delete cluster --all --timeout=10s || true
```
