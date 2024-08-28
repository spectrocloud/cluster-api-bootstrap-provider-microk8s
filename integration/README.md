## Integration tests

### Prerequisites

The integration/e2e tests have the following prerequisites:

  * an environment variable `CLUSTER_MANIFEST_FILE` pointing to the cluster manifest. Cluster manifests can be produced with the help of the templates found under [`templates`](../templates). For example:
  ```bash 
export AWS_REGION=us-east-1
export AWS_SSH_KEY_NAME=capi
export CONTROL_PLANE_MACHINE_COUNT=3
export WORKER_MACHINE_COUNT=3
export AWS_CREATE_BASTION=false
export AWS_PUBLIC_IP=false
export AWS_CONTROL_PLANE_MACHINE_FLAVOR=t3.large
export AWS_NODE_MACHINE_FLAVOR=t3.large
export CLUSTER_NAME=test-ci-cluster
clusterctl generate cluster ${CLUSTER_NAME} --from "templates/cluster-template-aws.yaml" --kubernetes-version 1.27.0 > cluster.yaml
export CLUSTER_MANIFEST_FILE=$PWD/cluster.yaml
  ```
> NOTE: AWS_SSH_KEY_NAME is name of ssh key in AWS that you are plan to use, if you don't have one yet refer
> to capi on [aws prerequisites documentation](https://cluster-api-aws.sigs.k8s.io/topics/using-clusterawsadm-to-fulfill-prerequisites#ssh-key-pair)
  *  Additional environment variables when testing cluster upgrades:
  ```bash
export CAPI_UPGRADE_VERSION=v1.28.0
export CAPI_UPGRADE_MD_NAME=${CLUSTER_NAME}-md-0
export CAPI_UPGRADE_MD_TYPE=machinedeployments.cluster.x-k8s.io
export CAPI_UPGRADE_CP_NAME=${CLUSTER_NAME}-control-plane
export CAPI_UPGRADE_CP_TYPE=microk8scontrolplanes.controlplane.cluster.x-k8s.io
# Change the control plane and worker machine count to desired values for in-place upgrades tests and create a new cluster manifest.
CONTROL_PLANE_MACHINE_COUNT=1
WORKER_MACHINE_COUNT=1
clusterctl generate cluster ${CLUSTER_NAME} --from "templates/cluster-template-aws.yaml" --kubernetes-version 1.27.0 > cluster-inplace.yaml
export CLUSTER_INPLACE_MANIFEST_FILE=$PWD/cluster-inplace.yaml
```

  *  Additional environment variables when testing disable default CNI flag:
  ```bash
export DISABLE_DEFAULT_CNI=true
export POST_RUN_COMMANDS='["helm install cilium cilium/cilium  --namespace kube-system --set cni.confPath=/var/snap/microk8s/current/args/cni-network --set cni.binPath=/var/snap/microk8s/current/opt/cni/bin --set daemon.runPath=/var/snap/microk8s/current/var/run/cilium --set operator.replicas=1 --set ipam.operator.clusterPoolIPv4PodCIDRList=\"10.1.0.0/16\" --set nodePort.enabled=true"]' # install Calico in place default CNI
clusterctl generate cluster ${CLUSTER_NAME} --from "templates/cluster-template-aws.yaml" --kubernetes-version 1.27.0 > cluster_disable_default_cni.yaml
export CLUSTER_DISABLE_DEFAULT_CNI_MANIFEST_FILE=$PWD/cluster_disable_default_cni.yaml
```

  * `clusterctl` available in the PATH

  * `kubectl` available in the PATH

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

#### Trigger the e2e tests

```bash
make e2e
```

#### Remove the test runs

```bash
microk8s kubectl delete cluster --all --timeout=10s || true
```
