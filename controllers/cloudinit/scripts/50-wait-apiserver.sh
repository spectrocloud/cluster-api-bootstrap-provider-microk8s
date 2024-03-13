#!/bin/bash -xe

# Usage:
#   $0
#
# Assumptions:
#   - microk8s is installed
#   - microk8s kubelite service is running

while ! microk8s kubectl get --raw /readyz; do
  echo Waiting for kube-apiserver
  sleep 3
done
