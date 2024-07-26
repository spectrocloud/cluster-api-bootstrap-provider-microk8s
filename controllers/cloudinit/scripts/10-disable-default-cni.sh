#!/bin/bash -xe

CNI_YAML="/var/snap/microk8s/current/args/cni-network/cni.yaml"
CNI_DIR="/var/snap/microk8s/current/args/cni-network"

if [ ! -f "${CNI_YAML}" ]; then
  echo "will not disable default CNI, missing cni.yaml"
  exit 0
fi

/capi-scripts/50-wait-apiserver.sh

microk8s kubectl delete -f "${CNI_YAML}"

for file in "${CNI_DIR}"/*; do
  mv "$file" "$file.old"
done
