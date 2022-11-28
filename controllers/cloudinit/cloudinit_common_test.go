package cloudinit_test

import (
	"strings"
	"testing"

	"github.com/canonical/cluster-api-bootstrap-provider-microk8s/controllers/cloudinit"
	. "github.com/onsi/gomega"
)

func TestCloudConfigInput(t *testing.T) {
	t.Run("ChannelAndRiskLevel", func(t *testing.T) {
		for _, tc := range []struct {
			name            string
			makeCloudConfig func() (*cloudinit.CloudConfig, error)
		}{
			{
				name: "ControlPlaneInit",
				makeCloudConfig: func() (*cloudinit.CloudConfig, error) {
					return cloudinit.NewInitControlPlane(&cloudinit.ControlPlaneInitInput{
						KubernetesVersion: "v1.25.0",
						Token:             strings.Repeat("a", 32),
						TokenTTL:          100,
						Confinement:       "strict",
						RiskLevel:         "edge",
					})
				},
			},
			{
				name: "ControlPlaneJoin",
				makeCloudConfig: func() (*cloudinit.CloudConfig, error) {
					return cloudinit.NewJoinControlPlane(&cloudinit.ControlPlaneJoinInput{
						KubernetesVersion: "v1.25.0",
						Token:             strings.Repeat("a", 32),
						TokenTTL:          100,
						Confinement:       "strict",
						RiskLevel:         "edge",
					})
				},
			},
			{
				name: "Worker",
				makeCloudConfig: func() (*cloudinit.CloudConfig, error) {
					return cloudinit.NewJoinWorker(&cloudinit.WorkerInput{
						KubernetesVersion: "v1.25.0",
						Token:             strings.Repeat("a", 32),
						Confinement:       "strict",
						RiskLevel:         "edge",
					})
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				g := NewWithT(t)
				c, err := tc.makeCloudConfig()
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(c.RunCommands).To(ContainElement(`/capi-scripts/00-install-microk8s.sh "--channel 1.25-strict/edge"`))

				_, err = cloudinit.GenerateCloudConfig(c)
				g.Expect(err).NotTo(HaveOccurred())
			})
		}
	})
}
