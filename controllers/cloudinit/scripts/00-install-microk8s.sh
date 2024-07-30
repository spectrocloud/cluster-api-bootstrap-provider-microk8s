#!/bin/bash -xe

# Usage:
#   $0 $microk8s_snap_args $disable_default_cni
#
# Arguments:
#   $microk8s_snap_args   Arguments to pass to snap install.
#   $disable_default_cni  Boolean flag (true or false) to disable the default CNI.
#
# Assumptions:
#   - snapd is installed

if snap list microk8s; then
  echo "MicroK8s is already installed, will not install"
  exit 0
fi

while ! snap install microk8s ${1}; do
  echo "Failed to install MicroK8s snap, will retry"
  sleep 5
done

if [ "${2}" == "true" ]; then
  mv /var/snap/microk8s/current/args/cni-network/cni.yaml /var/snap/microk8s/current/args/cni-network/cni.yaml.old
fi
