/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cloudinit

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/util/version"
)

// ControlPlaneJoinInput defines the context needed to generate a controlplane instance to join a cluster.
type ControlPlaneJoinInput struct {
	// ControlPlaneEndpoint is the control plane endpoint of the cluster.
	ControlPlaneEndpoint string
	// Token is the token that will be used for joining other nodes to the cluster.
	Token string
	// TokenTTL configures how many seconds the join token will be valid.
	TokenTTL int64
	// KubernetesVersion is the Kubernetes version we want to install.
	KubernetesVersion string
	// ClusterAgentPort is the port that cluster-agent binds to.
	ClusterAgentPort string
	// DqlitePort is the port that dqlite binds to.
	DqlitePort string
	// ContainerdHTTPProxy is http_proxy configuration for containerd.
	ContainerdHTTPProxy string
	// ContainerdHTTPSProxy is https_proxy configuration for containerd.
	ContainerdHTTPSProxy string
	// ContainerdNoProxy is no_proxy configuration for containerd.
	ContainerdNoProxy string
	// IPinIP defines whether Calico will use IPinIP mode for cluster networking.
	IPinIP bool
	// JoinNodeIPs is the IP addresses of the nodes to join.
	JoinNodeIPs [2]string
	// Confinement specifies a classic or strict deployment of microk8s snap.
	Confinement string
	// RiskLevel specifies the risk level (strict, candidate, beta, edge) for the snap channels.
	RiskLevel string
	// SnapstoreProxyDomain specifies the domain of the snapstore proxy if one is to be used.
	SnapstoreProxyDomain string
	// SnapstoreProxyId specifies the snapstore proxy ID if one is to be used.
	SnapstoreProxyId string
	// ExtraWriteFiles is a list of extra files to inject with cloud-init.
	ExtraWriteFiles []File
	// ExtraKubeletArgs is a list of arguments to add to kubelet.
	ExtraKubeletArgs []string
	// SnapstoreHTTPProxy is http_proxy configuration for snap store.
	SnapstoreHTTPProxy string
	// SnapstoreHTTPSProxy is https_proxy configuration for snap store.
	SnapstoreHTTPSProxy string
}

func NewJoinControlPlane(input *ControlPlaneJoinInput) (*CloudConfig, error) {
	// ensure token is valid
	if len(input.Token) != 32 {
		return nil, fmt.Errorf("join token %q is invalid; length must be 32 characters", input.Token)
	}
	if input.TokenTTL <= 0 {
		return nil, fmt.Errorf("join token TTL %q is not a positive number", input.TokenTTL)
	}

	// figure out endpoint type
	endpointType := "DNS"
	if net.ParseIP(input.ControlPlaneEndpoint) != nil {
		endpointType = "IP"
	}

	// figure out snap channel from KubernetesVersion
	kubernetesVersion, err := version.ParseSemantic(input.KubernetesVersion)
	if err != nil {
		return nil, fmt.Errorf("kubernetes version %q is not a semantic version: %w", input.KubernetesVersion, err)
	}

	// strict confinement is only available for microk8s v1.25+
	if input.Confinement == "strict" && kubernetesVersion.Minor() < 25 {
		return nil, fmt.Errorf("strict confinement is only available for microk8s v1.25+")
	}
	installArgs := createInstallArgs(input.Confinement, input.RiskLevel, kubernetesVersion)

	cloudConfig := NewBaseCloudConfig()
	cloudConfig.WriteFiles = append(cloudConfig.WriteFiles, input.ExtraWriteFiles...)
	if args := input.ExtraKubeletArgs; len(args) > 0 {
		cloudConfig.WriteFiles = append(cloudConfig.WriteFiles, File{
			Content:     strings.Join(args, "\n"),
			Path:        filepath.Join("/var", "tmp", "extra-kubelet-args"),
			Permissions: "0400",
			Owner:       "root:root",
		})
	}

	joinStr := fmt.Sprintf("%s:%s/%s", input.JoinNodeIPs[0], input.ClusterAgentPort, input.Token)
	joinStrAlt := fmt.Sprintf("%s:%s/%s", input.JoinNodeIPs[1], input.ClusterAgentPort, input.Token)

	cloudConfig.RunCommands = append(cloudConfig.RunCommands,
		"set -x",
		fmt.Sprintf("%s %q %q", scriptPath(snapstoreHTTPProxyScript), input.SnapstoreHTTPProxy, input.SnapstoreHTTPSProxy),
		fmt.Sprintf("%s %q %q", scriptPath(snapstoreProxyScript), input.SnapstoreProxyDomain, input.SnapstoreProxyId),
		scriptPath(disableHostServicesScript),
		fmt.Sprintf("%s %q", scriptPath(installMicroK8sScript), installArgs),
		fmt.Sprintf("%s %q %q %q", scriptPath(configureContainerdProxyScript), input.ContainerdHTTPProxy, input.ContainerdHTTPSProxy, input.ContainerdNoProxy),
		scriptPath(configureKubeletScript),
		"microk8s status --wait-ready",
		fmt.Sprintf("%s %v", scriptPath(configureCalicoIPIPScript), input.IPinIP),
		fmt.Sprintf("%s %q", scriptPath(configureClusterAgentPortScript), input.ClusterAgentPort),
		fmt.Sprintf("%s %q", scriptPath(configureDqlitePortScript), input.DqlitePort),
		"microk8s status --wait-ready",
		fmt.Sprintf("%s %q %q", scriptPath(configureCertLB), endpointType, input.ControlPlaneEndpoint),
		fmt.Sprintf("%s no %q %q", scriptPath(microk8sJoinScript), joinStr, joinStrAlt),
		scriptPath(configureAPIServerScript),
		fmt.Sprintf("microk8s add-node --token-ttl %v --token %q", input.TokenTTL, input.Token),
	)

	return cloudConfig, nil
}
