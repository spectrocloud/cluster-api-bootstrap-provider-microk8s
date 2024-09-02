//go:build integration
// +build integration

/*
Copyright 2024.

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

package e2e_test

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	KUBECONFIG                   string = "/var/tmp/kubeconfig.e2e.conf"
	DisableDefaultCNIClusterPath string = "cluster-manifests/cluster-disable-default-cni.yaml"
	BasicClusterPath             string = "cluster-manifests/cluster.yaml"
	InPlaceUpgradeClusterPath    string = "cluster-manifests/cluster-inplace.yaml"
)

func init() {
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-s
		teardownCluster()
		os.Exit(1)
	}()
}

// TestBasic waits for the target cluster to deploy and start a 30 pod deployment.
// kubectl and clusterctl have to be available in the caller's path.
// kubectl should be setup so it uses the kubeconfig of the management cluster by default.
func TestBasic(t *testing.T) {
	t.Logf("Cluster to setup is in %s", BasicClusterPath)

	setupCheck(t)
	t.Cleanup(teardownCluster)

	t.Run("DeployCluster", func(t *testing.T) { deployCluster(t, BasicClusterPath) })
	t.Run("VerifyCluster", func(t *testing.T) { verifyCluster(t) })
	t.Run("DeployMicrobot", func(t *testing.T) { deployMicrobot(t) })
	t.Run("UpgradeClusterRollout", func(t *testing.T) { upgradeCluster(t, "RollingUpgrade") })

	// Important: the cluster is deleted in the Cleanup function
	// which is called after all subtests are finished.
	t.Logf("Deleting the cluster")
}

// TestInPlaceUpgrade waits for the target cluster to deploy and start a 30 pod deployment.
// This cluster will be upgraded via an in-place upgrade.
func TestInPlaceUpgrade(t *testing.T) {
	t.Logf("Cluster for in-place upgrade test setup is in %s", InPlaceUpgradeClusterPath)

	setupCheck(t)
	t.Cleanup(teardownCluster)

	t.Run("DeployCluster", func(t *testing.T) { deployCluster(t, InPlaceUpgradeClusterPath) })
	t.Run("VerifyCluster", func(t *testing.T) { verifyCluster(t) })
	t.Run("DeployMicrobot", func(t *testing.T) { deployMicrobot(t) })
	t.Run("UpgradeClusterInplace", func(t *testing.T) { upgradeCluster(t, "InPlaceUpgrade") })

	// Important: the cluster is deleted in the Cleanup function
	// which is called after all subtests are finished.
	t.Logf("Deleting the cluster")
}

// TestDisableDefaultCNI deploys cluster disabled default CNI.
// Next Cilium is installed
// helm have to be available in the caller's path.
func TestDisableDefaultCNI(t *testing.T) {
	t.Logf("Cluster to setup is in %s", DisableDefaultCNIClusterPath)

	setupCheck(t)
	t.Cleanup(teardownCluster)

	t.Run("DeployCluster", func(t *testing.T) { deployCluster(t, DisableDefaultCNIClusterPath) })
	t.Run("ValidateNoCalico", func(t *testing.T) { validateNoCalico(t) })
	t.Run("installCilium", func(t *testing.T) { installCilium(t) })
	t.Run("validateCilium", func(t *testing.T) { validateCilium(t) })
	t.Run("VerifyCluster", func(t *testing.T) { verifyCluster(t) })

	// Important: the cluster is deleted in the Cleanup function
	// which is called after all subtests are finished.
	t.Logf("Deleting the cluster")
}

// checkBinary validates if binary is available on PATH
func checkBinary(t testing.TB, binaryName string) {
	_, err := exec.LookPath(binaryName)
	if err != nil {
		t.Fatalf("Please make sure %s is in your PATH. %s", binaryName, err)
	}
}

// setupCheck checks that the environment is ready to run the tests.
func setupCheck(t testing.TB) {

	for _, binaryName := range []string{"kubectl", "clusterctl", "helm"} {
		checkBinary(t, binaryName)
	}
	t.Logf("Waiting for the MicroK8s providers to deploy on the management cluster.")
	waitForPod(t, "capi-microk8s-bootstrap-controller-manager", "capi-microk8s-bootstrap-system")
	waitForPod(t, "capi-microk8s-control-plane-controller-manager", "capi-microk8s-control-plane-system")
}

// waitForPod waits for a pod to be available.
func waitForPod(t testing.TB, pod string, ns string) {
	attempt := 0
	maxAttempts := 10
	command := []string{"kubectl", "wait", "--timeout=15s", "--for=condition=available", "deploy/" + pod, "-n", ns}
	for {
		cmd := exec.Command(command[0], command[1:]...)
		outputBytes, err := cmd.CombinedOutput()
		if err != nil {
			t.Log(string(outputBytes))
			if attempt >= maxAttempts {
				t.Fatal(err)
			} else {
				t.Logf("Retrying...")
				attempt++
				time.Sleep(10 * time.Second)
			}
		} else {
			break
		}
	}
}

// teardownCluster deletes the cluster created by deployCluster.
func teardownCluster() {
	command := []string{"kubectl", "get", "cluster", "--no-headers", "-o", "custom-columns=:metadata.name"}
	cmd := exec.Command(command[0], command[1:]...)
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		// Could not get any produced clusters. Nothing to cleanup... I hope.
		return
	}

	cluster := strings.Trim(string(outputBytes), "\n")
	command = []string{"kubectl", "delete", "cluster", cluster}
	cmd = exec.Command(command[0], command[1:]...)
	cmd.Run()
}

// deployCluster deploys a cluster using the manifest in CLUSTER_MANIFEST_FILE.
func deployCluster(t testing.TB, clusterManifestFile string) {
	t.Logf("Setting up the cluster using %s", clusterManifestFile)
	command := []string{"kubectl", "apply", "-f", clusterManifestFile}
	cmd := exec.Command(command[0], command[1:]...)
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Error(string(outputBytes))
		t.Fatalf("Failed to create the requested cluster. %s", err)
	}

	time.Sleep(30 * time.Second)

	command = []string{"kubectl", "get", "cluster", "--no-headers", "-o", "custom-columns=:metadata.name"}
	cmd = exec.Command(command[0], command[1:]...)
	outputBytes, err = cmd.CombinedOutput()
	if err != nil {
		t.Error(string(outputBytes))
		t.Fatalf("Failed to get the name of the cluster. %s", err)
	}

	cluster := strings.Trim(string(outputBytes), "\n")
	t.Logf("Cluster name is %s", cluster)

	attempt := 0
	maxAttempts := 120
	command = []string{"clusterctl", "get", "kubeconfig", cluster}
	for {
		cmd = exec.Command(command[0], command[1:]...)
		outputBytes, err = cmd.Output()
		if err != nil {
			if attempt >= maxAttempts {
				t.Fatal(err)
			} else {
				attempt++
				t.Logf("Failed to get the target's kubeconfig for %s, retrying.", cluster)
				time.Sleep(20 * time.Second)
			}
		} else {
			cfg := strings.Trim(string(outputBytes), "\n")
			err = os.WriteFile(KUBECONFIG, []byte(cfg), 0644)
			if err != nil {
				t.Fatalf("Could not persist the targets kubeconfig file. %s", err)
			}
			t.Logf("Target's kubeconfig file is at %s", KUBECONFIG)
			t.Log(cfg)
			break
		}
	}

	// Wait until the cluster is provisioned
	attempt = 0
	maxAttempts = 120
	command = []string{"kubectl", "get", "cluster", cluster}
	for {
		cmd = exec.Command(command[0], command[1:]...)
		outputBytes, err = cmd.CombinedOutput()
		if err != nil {
			t.Log(string(outputBytes))
			if attempt >= maxAttempts {
				t.Fatal(err)
			} else {
				attempt++
				t.Log("Retrying")
				time.Sleep(10 * time.Second)
			}
		} else {
			if strings.Contains(string(outputBytes), "Provisioned") {
				break
			} else {
				attempt++
				time.Sleep(20 * time.Second)
				t.Log("Waiting for the cluster to be provisioned")
			}
		}
	}
}

// verifyCluster check if cluster is functional
func verifyCluster(t testing.TB) {
	// Wait until all machines are running
	attempt := 0
	maxAttempts := 120
	machines := 0
	command := []string{"kubectl", "get", "machine", "--no-headers"}
	for {
		cmd := exec.Command(command[0], command[1:]...)
		outputBytes, err := cmd.CombinedOutput()
		output := string(outputBytes)
		if err != nil {
			t.Log(output)
			if attempt >= maxAttempts {
				t.Fatal(err)
			} else {
				attempt++
				t.Log("Retrying")
				time.Sleep(10 * time.Second)
			}
		} else {
			machines = strings.Count(output, "\n")
			running := strings.Count(output, "Running")
			t.Logf("Machines %d out of which %d are Running", machines, running)
			if machines == running {
				break
			} else {
				attempt++
				time.Sleep(10 * time.Second)
				t.Log("Waiting for machines to start running")
			}
		}
	}

	// Make sure we have as many nodes as machines
	attempt = 0
	maxAttempts = 120
	command = []string{"kubectl", "--kubeconfig=" + KUBECONFIG, "get", "no", "--no-headers"}
	for {
		cmd := exec.Command(command[0], command[1:]...)
		outputBytes, err := cmd.CombinedOutput()
		output := string(outputBytes)
		if err != nil {
			t.Log(output)
			if attempt >= maxAttempts {
				t.Fatal(err)
			} else {
				attempt++
				time.Sleep(10 * time.Second)
				t.Log("Retrying")
			}
		} else {
			nodes := strings.Count(output, "\n")
			ready := strings.Count(output, " Ready")
			t.Logf("Machines are %d, Nodes are %d out of which %d are Ready", machines, nodes, ready)
			if machines == nodes && ready == nodes {
				break
			} else {
				attempt++
				time.Sleep(20 * time.Second)
				t.Log("Waiting for nodes to become ready")
			}
		}
	}
}

// deployMicrobot deploys a deployment of microbot.
func deployMicrobot(t testing.TB) {
	t.Log("Deploying microbot")

	command := []string{"kubectl", "--kubeconfig=" + KUBECONFIG, "create", "deploy", "--image=cdkbot/microbot:1", "--replicas=30", "bot"}
	cmd := exec.Command(command[0], command[1:]...)
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Error(string(outputBytes))
		t.Fatalf("Failed to create the requested microbot deployment. %s", err)
	}

	// Make sure we have as many nodes as machines
	attempt := 0
	maxAttempts := 120
	t.Log("Waiting for the deployment to complete")
	command = []string{"kubectl", "--kubeconfig=" + KUBECONFIG, "wait", "deploy/bot", "--for=jsonpath={.status.readyReplicas}=30"}
	for {
		cmd = exec.Command(command[0], command[1:]...)
		outputBytes, err = cmd.CombinedOutput()
		if err != nil {
			t.Log(string(outputBytes))
			if attempt >= maxAttempts {
				t.Fatal(err)
			} else {
				attempt++
				t.Log("Retrying")
				time.Sleep(10 * time.Second)
			}
		} else {
			break
		}
	}
}

// validateNoCalico checks a there is no calico daemon set.
func validateNoCalico(t testing.TB) {
	t.Log("Validate no Calico daemon set")

	t.Log("Checking for calico daemon set")
	command := []string{"kubectl", "--kubeconfig=" + KUBECONFIG, "-n", "kube-system", "get", "ds"}
	attempt := 0
	maxAttempts := 120
	for {
		t.Logf("running command: %s", strings.Join(command, " "))
		cmd := exec.Command(command[0], command[1:]...)
		outputBytes, err := cmd.CombinedOutput()
		if err != nil {
			t.Log(string(outputBytes))
			if attempt >= maxAttempts {
				t.Fatal(err)
			} else {
				attempt++
				t.Log("Retrying")
				time.Sleep(10 * time.Second)
			}
		} else {
			if !strings.Contains(string(outputBytes), "calico") {
				break
			}
		}
	}
	t.Log("No calico daemon set")
}

// installCilium installs cilium from helm chart
func installCilium(t testing.TB) {
	t.Log("Deploy Cilium")
	command := []string{"helm", "install", "cilium", "--repo", "https://helm.cilium.io/",
		"cilium", "--namespace", "kube-system", "--set", "cni.confPath=/var/snap/microk8s/current/args/cni-network",
		"--set", "cni.binPath=/var/snap/microk8s/current/opt/cni/bin",
		"--set", "daemon.runPath=/var/snap/microk8s/current/var/run/cilium",
		"--set", "operator.replicas=1",
		"--set", "ipam.operator.clusterPoolIPv4PodCIDRList=10.1.0.0/16",
		"--set", "nodePort.enabled=true",
	}

	t.Logf("running command: %s", strings.Join(command, " "))
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = append(cmd.Env, "KUBECONFIG="+KUBECONFIG)
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Error(string(outputBytes))
		t.Fatalf("Failed to deploy cilium from heml chart. %s", err)
	}
}

// validateCilium checks a deployment of cilium daemon set.
func validateCilium(t testing.TB) {
	t.Log("Validate Cilium")

	//check control plane machine exists
	attempt := 0
	maxAttempts := 120
	machines := 0
	command := []string{"kubectl", "get", "machine", "--no-headers"}
	for {
		t.Logf("running command: %s", strings.Join(command, " "))
		cmd := exec.Command(command[0], command[1:]...)
		outputBytes, err := cmd.CombinedOutput()
		output := string(outputBytes)
		if err != nil {
			t.Log(output)
			if attempt >= maxAttempts {
				t.Fatal(err)
			} else {
				attempt++
				t.Log("Retrying")
				time.Sleep(10 * time.Second)
			}
		} else {
			machines = strings.Count(output, "\n")
			if machines > 0 {
				break
			}
		}
	}

	t.Log("Checking Cilium daemon set")
	command = []string{"kubectl", "--kubeconfig=" + KUBECONFIG, "-n", "kube-system", "wait", "ds/cilium", fmt.Sprintf("--for=jsonpath={.status.numberAvailable}=%d", machines)}
	for {
		cmd := exec.Command(command[0], command[1:]...)
		outputBytes, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("running command: %s", strings.Join(command, " "))
			t.Log(string(outputBytes))
			if attempt >= maxAttempts {
				t.Fatal(err)
			} else {
				attempt++
				t.Log("Retrying")
				time.Sleep(20 * time.Second)
			}
		} else {
			break
		}
	}
}

// upgradeCluster upgrades the cluster to a new version based on the upgrade strategy.
func upgradeCluster(t testing.TB, upgradeStrategy string) {

	version := "v1.28.0"
	controlPlaneName := "test-ci-cluster-control-plane"
	controlPlaneType := "microk8scontrolplanes.controlplane.cluster.x-k8s.io"
	workerDeploymentName := "test-ci-cluster-md-0"
	workerDeploymentType := "machinedeployments.cluster.x-k8s.io"

	t.Logf("Upgrading cluster to %s via %s", version, upgradeStrategy)
	// Patch control plane machine upgrades based on type of upgrade strategy.
	outputBytes, err := controlPlanePatch(controlPlaneName, controlPlaneType, version, upgradeStrategy)
	if err != nil {
		t.Error(string(outputBytes))
		t.Fatalf("Failed to merge the patch to control plane. %s", err)
	}

	// Patch worker machine upgrades.
	outputBytes, err = workerPatch(workerDeploymentName, workerDeploymentType, version)
	if err != nil {
		t.Error(string(outputBytes))
		t.Fatalf("Failed to merge the patch to the machine deployments. %s", err)
	}

	time.Sleep(30 * time.Second)

	// Now all the machines should be upgraded to the new version.
	attempt := 0
	maxAttempts := 120
	command := []string{"kubectl", "get", "machine", "--no-headers"}
	for {
		cmd := exec.Command(command[0], command[1:]...)
		outputBytes, err = cmd.CombinedOutput()
		output := string(outputBytes)
		if err != nil {
			t.Log(output)
			if attempt >= maxAttempts {
				t.Fatal(err)
			}

			attempt++
			t.Log("Retrying")
			time.Sleep(20 * time.Second)
		} else {
			totalMachines := strings.Count(output, "Running")

			// We count all the "Running" machines with the new version.
			re := regexp.MustCompile("Running .* " + version)
			upgradedMachines := len(re.FindAllString(output, -1))
			t.Logf("Total machines %d out of which %d are upgraded", totalMachines, upgradedMachines)
			if totalMachines == upgradedMachines {
				break
			} else {
				attempt++
				time.Sleep(20 * time.Second)
				t.Log("Waiting for machines to upgrade and start running")
			}
		}
	}
}

// controlPlanePatch patches the control plane machines based on the upgrade strategy and version.
func controlPlanePatch(controlPlaneName, controlPlaneType, version, upgradeStrategy string) ([]byte, error) {
	command := []string{"kubectl", "patch", "--type=merge", controlPlaneType, controlPlaneName, "--patch",
		fmt.Sprintf(`{"spec":{"version":"%s","upgradeStrategy":"%s"}}`, version, upgradeStrategy)}
	cmd := exec.Command(command[0], command[1:]...)

	return cmd.CombinedOutput()
}

// workerPatch patches a given worker machines with the given version.
func workerPatch(workerDeploymentName, workerDeploymentType, version string) ([]byte, error) {
	command := []string{"kubectl", "patch", "--type=merge", workerDeploymentType, workerDeploymentName, "--patch",
		fmt.Sprintf(`{"spec":{"template":{"spec":{"version":"%s"}}}}`, version)}
	cmd := exec.Command(command[0], command[1:]...)

	return cmd.CombinedOutput()
}
