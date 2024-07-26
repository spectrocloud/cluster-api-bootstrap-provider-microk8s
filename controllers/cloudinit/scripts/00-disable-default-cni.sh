#!/bin/bash -xe

microk8s kubectl delete -f /var/snap/microk8s/current/args/cni-network/cni.yaml
mv /var/snap/microk8s/current/args/cni-network/cni.yaml /var/snap/microk8s/current/args/cni-network/calico.yaml.old