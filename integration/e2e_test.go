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
	"context"
	"errors"
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
	retryMaxAttempts                    = 120
	secondsBetweenAttempts              = 20
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
	ctx := context.Background()

	setupCheck(ctx, t)
	t.Cleanup(teardownCluster)

	t.Run("DeployCluster", func(t *testing.T) { deployCluster(ctx, t, BasicClusterPath) })
	t.Run("VerifyCluster", func(t *testing.T) { verifyCluster(ctx, t) })
	t.Run("DeployMicrobot", func(t *testing.T) { deployMicrobot(ctx, t) })
	t.Run("UpgradeClusterRollout", func(t *testing.T) { upgradeCluster(ctx, t, "RollingUpgrade") })

	// Important: the cluster is deleted in the Cleanup function
	// which is called after all subtests are finished.
	t.Logf("Deleting the cluster")
}

// TestInPlaceUpgrade waits for the target cluster to deploy and start a 30 pod deployment.
// This cluster will be upgraded via an in-place upgrade.
func TestInPlaceUpgrade(t *testing.T) {
	t.Logf("Cluster for in-place upgrade test setup is in %s", InPlaceUpgradeClusterPath)
	ctx := context.Background()

	setupCheck(ctx, t)
	t.Cleanup(teardownCluster)

	t.Run("DeployCluster", func(t *testing.T) { deployCluster(ctx, t, InPlaceUpgradeClusterPath) })
	t.Run("VerifyCluster", func(t *testing.T) { verifyCluster(ctx, t) })
	t.Run("DeployMicrobot", func(t *testing.T) { deployMicrobot(ctx, t) })
	t.Run("UpgradeClusterInplace", func(t *testing.T) { upgradeCluster(ctx, t, "InPlaceUpgrade") })

	// Important: the cluster is deleted in the Cleanup function
	// which is called after all subtests are finished.
	t.Logf("Deleting the cluster")
}

// TestDisableDefaultCNI deploys cluster disabled default CNI.
// Next Cilium is installed
// helm have to be available in the caller's path.
func TestDisableDefaultCNI(t *testing.T) {
	t.Logf("Cluster to setup is in %s", DisableDefaultCNIClusterPath)
	ctx := context.Background()

	setupCheck(ctx, t)
	t.Cleanup(teardownCluster)

	t.Run("DeployCluster", func(t *testing.T) { deployCluster(ctx, t, DisableDefaultCNIClusterPath) })
	t.Run("ValidateNoCalico", func(t *testing.T) { validateNoCalico(ctx, t) })
	t.Run("installCilium", func(t *testing.T) { installCilium(t) })
	t.Run("validateCilium", func(t *testing.T) { validateCilium(ctx, t) })
	t.Run("VerifyCluster", func(t *testing.T) { verifyCluster(ctx, t) })

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
func setupCheck(ctx context.Context, t testing.TB) {
	for _, binaryName := range []string{"kubectl", "clusterctl", "helm"} {
		checkBinary(t, binaryName)
	}
	t.Logf("Waiting for the MicroK8s providers to deploy on the management cluster.")
	waitForPod(ctx, t, "capi-microk8s-bootstrap-controller-manager", "capi-microk8s-bootstrap-system")
	waitForPod(ctx, t, "capi-microk8s-control-plane-controller-manager", "capi-microk8s-control-plane-system")
}

// waitForPod waits for a pod to be available.
func waitForPod(ctx context.Context, t testing.TB, pod string, ns string) {
	if err := retryFor(ctx, retryMaxAttempts, secondsBetweenAttempts*time.Second, func() error {
		_, err := execCommand(t, "kubectl", "wait", "--timeout=15s", "--for=condition=available", "deploy/"+pod, "-n", ns)
		return err
	}); err != nil {
		t.Fatal(err)
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
func deployCluster(ctx context.Context, t testing.TB, clusterManifestFile string) {
	t.Logf("Setting up the cluster using %s", clusterManifestFile)
	if _, err := execCommand(t, "kubectl", "apply", "-f", clusterManifestFile); err != nil {
		t.Fatalf("Failed to get the name of the cluster. %s", err)
	}

	time.Sleep(30 * time.Second)

	output, err := execCommand(t, "kubectl", "get", "cluster", "--no-headers", "-o", "custom-columns=:metadata.name")
	if err != nil {
		t.Fatalf("Failed to get the name of the cluster. %s", err)
	}

	cluster := strings.Trim(output, "\n")
	t.Logf("Cluster name is %s", cluster)

	if err = retryFor(ctx, retryMaxAttempts, secondsBetweenAttempts*time.Second, func() error {
		output, err = execCommand(t, "clusterctl", "get", "kubeconfig", cluster)
		if err != nil {
			return err
		}

		cfg := strings.Trim(output, "\n")
		err = os.WriteFile(KUBECONFIG, []byte(cfg), 0644)
		if err != nil {
			t.Fatalf("Could not persist the targets kubeconfig file. %s", err)
		}
		t.Logf("Target's kubeconfig file is at %s", KUBECONFIG)
		return nil

	}); err != nil {
		t.Fatal(err)
	}

	// Wait until the cluster is provisioned
	if err = retryFor(ctx, retryMaxAttempts, secondsBetweenAttempts*time.Second, func() error {
		output, err = execCommand(t, "kubectl", "get", "cluster", cluster)
		if err != nil {
			return err
		}
		if strings.Contains(output, "Provisioned") {
			return nil
		}
		return errors.New("cluster not provisioned")
	}); err != nil {
		t.Fatal(err)
	}
}

// verifyCluster check if cluster is functional
func verifyCluster(ctx context.Context, t testing.TB) {
	// Wait until all machines are running
	t.Log("Verify cluster deployment")

	machines := 0
	if err := retryFor(ctx, retryMaxAttempts, secondsBetweenAttempts*time.Second, func() error {
		output, err := execCommand(t, "kubectl", "get", "machine", "--no-headers")
		if err != nil {
			return err
		}
		machines = strings.Count(output, "\n")
		running := strings.Count(output, "Running")
		msg := fmt.Sprintf("Machines %d out of which %d are Running", machines, running)
		t.Logf(msg)
		if machines == running {
			return nil
		}
		return errors.New(msg)
	}); err != nil {
		t.Fatal(err)
	}

	// Make sure we have as many nodes as machines
	if err := retryFor(ctx, retryMaxAttempts, secondsBetweenAttempts*time.Second, func() error {
		output, err := execCommand(t, "kubectl", "--kubeconfig="+KUBECONFIG, "get", "no", "--no-headers")
		if err != nil {
			return err
		}
		nodes := strings.Count(output, "\n")
		ready := strings.Count(output, " Ready")
		msg := fmt.Sprintf("Machines are %d, Nodes are %d out of which %d are Ready", machines, nodes, ready)
		t.Log(msg)
		if machines == nodes && ready == nodes {
			return nil
		}
		return errors.New(msg)
	}); err != nil {
		t.Fatal(err)
	}
}

// deployMicrobot deploys a deployment of microbot.
func deployMicrobot(ctx context.Context, t testing.TB) {
	t.Log("Deploying microbot")

	if output, err := execCommand(t, "kubectl", "--kubeconfig="+KUBECONFIG, "create", "deploy", "--image=cdkbot/microbot:1", "--replicas=30", "bot"); err != nil {
		t.Error(output)
		t.Fatalf("Failed to create the requested microbot deployment. %s", err)
	}

	// Make sure we have as many nodes as machines
	t.Log("Waiting for the deployment to complete")
	if err := retryFor(ctx, retryMaxAttempts, secondsBetweenAttempts*time.Second, func() error {
		_, err := execCommand(t, "kubectl", "--kubeconfig="+KUBECONFIG, "wait", "deploy/bot", "--for=jsonpath={.status.readyReplicas}=30")
		return err
	}); err != nil {
		t.Fatal(err)
	}

}

// validateNoCalico Checks if calico daemon set is not deployed on the cluster.
func validateNoCalico(ctx context.Context, t testing.TB) {
	t.Log("Validate no Calico daemon set")

	if err := retryFor(ctx, retryMaxAttempts, secondsBetweenAttempts*time.Second, func() error {
		output, err := execCommand(t, "kubectl", "--kubeconfig="+KUBECONFIG, "-n", "kube-system", "get", "ds")
		if err != nil {
			return err
		}
		if strings.Contains(output, "calico") {
			return errors.New("there is calico daemon set")

		}
		return nil
	}); err != nil {
		t.Fatal(err)
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
func validateCilium(ctx context.Context, t testing.TB) {
	t.Log("Validate Cilium")

	machines := 0
	if err := retryFor(ctx, retryMaxAttempts, secondsBetweenAttempts*time.Second, func() error {
		output, err := execCommand(t, "kubectl", "get", "machine", "--no-headers")
		if err != nil {
			return err
		}
		machines = strings.Count(output, "\n")
		if machines == 0 {
			return errors.New("machines to haven't start yet")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}

	t.Log("Checking Cilium daemon set")
	if err := retryFor(ctx, retryMaxAttempts, secondsBetweenAttempts*time.Second, func() error {
		_, err := execCommand(t, "kubectl", "--kubeconfig="+KUBECONFIG, "-n", "kube-system", "wait", "ds/cilium", fmt.Sprintf("--for=jsonpath={.status.numberAvailable}=%d", machines))
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// execCommand executes command transforms output bytes to string and reruns error from exec
func execCommand(t testing.TB, command ...string) (string, error) {
	t.Logf("running command: %s", strings.Join(command, " "))
	cmd := exec.Command(command[0], command[1:]...)
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	t.Logf(output)
	return output, err
}

// upgradeCluster upgrades the cluster to a new version based on the upgrade strategy.
func upgradeCluster(ctx context.Context, t testing.TB, upgradeStrategy string) {
	version := "v1.28.0"
	t.Logf("Upgrading cluster to %s via %s", version, upgradeStrategy)

	// Patch control plane machine upgrades based on type of upgrade strategy.
	if _, err := execCommand(t, "kubectl", "patch", "--type=merge",
		"microk8scontrolplanes.controlplane.cluster.x-k8s.io", "test-ci-cluster-control-plane", "--patch",
		fmt.Sprintf(`{"spec":{"version":"%s","upgradeStrategy":"%s"}}`, version, upgradeStrategy)); err != nil {
		t.Fatalf("Failed to merge the patch to control plane. %s", err)
	}

	// Patch worker machine upgrades.
	if _, err := execCommand(t, "kubectl", "patch", "--type=merge",
		"machinedeployments.cluster.x-k8s.io", "test-ci-cluster-md-0", "--patch",
		fmt.Sprintf(`{"spec":{"template":{"spec":{"version":"%s"}}}}`, version)); err != nil {
		t.Fatalf("Failed to merge the patch to the machine deployments. %s", err)
	}

	time.Sleep(30 * time.Second)

	// Now all the machines should be upgraded to the new version.
	if err := retryFor(ctx, retryMaxAttempts, secondsBetweenAttempts*time.Second, func() error {
		output, err := execCommand(t, "kubectl", "get", "machine", "--no-headers")
		if err != nil {
			return err
		}
		totalMachines := strings.Count(output, "Running")
		re := regexp.MustCompile("Running .* " + version)
		upgradedMachines := len(re.FindAllString(output, -1))
		msg := fmt.Sprintf("Total machines %d out of which %d are upgraded", totalMachines, upgradedMachines)
		t.Logf(msg)
		if totalMachines == upgradedMachines {
			return nil
		}
		return errors.New(msg)
	}); err != nil {
		t.Fatal(err)
	}
}

// retryFor will retry a given function for the given amount of times.
// retryFor will wait for backoff between retries.
func retryFor(ctx context.Context, retryCount int, delayBetweenRetry time.Duration, retryFunc func() error) error {
	var err error = nil
	for i := 0; i < retryCount; i++ {
		if err = retryFunc(); err != nil {
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-time.After(delayBetweenRetry):
				continue
			}
		}
		break
	}
	return err
}
