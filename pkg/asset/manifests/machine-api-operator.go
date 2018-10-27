package manifests

import (
	"context"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/installer/pkg/asset"
	"github.com/openshift/installer/pkg/asset/installconfig"
	"github.com/openshift/installer/pkg/asset/tls"
	"github.com/openshift/installer/pkg/rhcos"
)

const (
	maoTargetNamespace = "openshift-cluster-api"
	// DefaultChannel is the default RHCOS channel for the cluster.
	DefaultChannel = "tested"
	maoCfgFilename = "machine-api-operator-config.yml"
)

// machineAPIOperator generates the network-operator-*.yml files
type machineAPIOperator struct {
	Config *maoOperatorConfig
	File   *asset.File
}

var _ asset.WritableAsset = (*machineAPIOperator)(nil)

// maoOperatorConfig contains configuration for mao managed stack
// TODO(enxebre): move up to github.com/coreos/tectonic-config (to install-config? /rchopra)
type maoOperatorConfig struct {
	metav1.TypeMeta `json:",inline"`
	TargetNamespace string           `json:"targetNamespace"`
	APIServiceCA    string           `json:"apiServiceCA"`
	Provider        string           `json:"provider"`
	AWS             *awsConfig       `json:"aws"`
	Libvirt         *libvirtConfig   `json:"libvirt"`
	OpenStack       *openstackConfig `json:"openstack"`
}

type libvirtConfig struct {
	ClusterName string `json:"clusterName"`
	URI         string `json:"uri"`
	NetworkName string `json:"networkName"`
	IPRange     string `json:"iprange"`
	Replicas    int    `json:"replicas"`
}

type awsConfig struct {
	ClusterName      string `json:"clusterName"`
	ClusterID        string `json:"clusterID"`
	Region           string `json:"region"`
	AvailabilityZone string `json:"availabilityZone"`
	Image            string `json:"image"`
	Replicas         int    `json:"replicas"`
}

type openstackConfig struct {
	ClusterName string `json:"clusterName"`
	ClusterID   string `json:"clusterID"`
	Region      string `json:"region"`
	Replicas    int    `json:"replicas"`
}

// Name returns a human friendly name for the operator
func (mao *machineAPIOperator) Name() string {
	return "Machine API Operator"
}

// Dependencies returns all of the dependencies directly needed by an
// machineAPIOperator asset.
func (mao *machineAPIOperator) Dependencies() []asset.Asset {
	return []asset.Asset{
		&installconfig.InstallConfig{},
		&tls.AggregatorCA{},
	}
}

// Generate generates the network-operator-config.yml and network-operator-manifest.yml files
func (mao *machineAPIOperator) Generate(dependencies asset.Parents) error {
	installConfig := &installconfig.InstallConfig{}
	aggregatorCA := &tls.AggregatorCA{}
	dependencies.Get(installConfig, aggregatorCA)

	mao.Config = &maoOperatorConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "machineAPIOperatorConfig",
		},
		TargetNamespace: maoTargetNamespace,
		APIServiceCA:    string(aggregatorCA.Cert()),
		Provider:        tectonicCloudProvider(installConfig.Config.Platform),
	}

	switch {
	case installConfig.Config.Platform.AWS != nil:
		var ami string

		ami, err := rhcos.AMI(context.TODO(), DefaultChannel, installConfig.Config.Platform.AWS.Region)
		if err != nil {
			return errors.Wrapf(err, "failed to get AMI for %s config", mao.Name())
		}

		mao.Config.AWS = &awsConfig{
			ClusterName:      installConfig.Config.ObjectMeta.Name,
			ClusterID:        installConfig.Config.ClusterID,
			Region:           installConfig.Config.Platform.AWS.Region,
			AvailabilityZone: "",
			Image:            ami,
			Replicas:         0, // setting replicas to 0 so that MAO doesn't create competing MachineSets
		}
	case installConfig.Config.Platform.Libvirt != nil:
		mao.Config.Libvirt = &libvirtConfig{
			ClusterName: installConfig.Config.ObjectMeta.Name,
			URI:         installConfig.Config.Platform.Libvirt.URI,
			NetworkName: installConfig.Config.Platform.Libvirt.Network.Name,
			IPRange:     installConfig.Config.Platform.Libvirt.Network.IPRange,
			Replicas:    0, // setting replicas to 0 so that MAO doesn't create competing MachineSets
		}
	case installConfig.Config.Platform.OpenStack != nil:
		mao.Config.OpenStack = &openstackConfig{
			ClusterName: installConfig.Config.ObjectMeta.Name,
			ClusterID:   installConfig.Config.ClusterID,
			Region:      installConfig.Config.Platform.OpenStack.Region,
			Replicas:    0, // setting replicas to 0 so that MAO doesn't create competing MachineSets
		}
	default:
		return errors.Errorf("unknown provider for machine-api-operator")
	}

	data, err := yaml.Marshal(mao.Config)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal %s config", mao.Name())
	}
	mao.File = &asset.File{
		Filename: maoCfgFilename,
		Data:     data,
	}

	return nil
}

// Files returns the files generated by the asset.
func (mao *machineAPIOperator) Files() []*asset.File {
	return []*asset.File{mao.File}
}

// Load is a no-op because machine-api-operator manifest is not written to disk.
func (mao *machineAPIOperator) Load(asset.FileFetcher) (bool, error) {
	return false, nil
}