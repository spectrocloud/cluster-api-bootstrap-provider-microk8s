#!/bin/bash -xe

# Usage:
#   $0 $microk8s_snap_args
#
# Assumptions:
#   - snapd is installed

if snap list microk8s; do
  echo "MicroK8s is already installed, will not install"
  exit 0
fi

while ! snap install microk8s ${1}; do
  echo "Failed to install MicroK8s snap, will retry"
  sleep 5
done
