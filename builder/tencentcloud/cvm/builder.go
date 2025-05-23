// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc mapstructure-to-hcl2 -type Config

package cvm

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

const BuilderId = "tencent.cloud"

type Config struct {
	common.PackerConfig      `mapstructure:",squash"`
	TencentCloudAccessConfig `mapstructure:",squash"`
	TencentCloudImageConfig  `mapstructure:",squash"`
	TencentCloudRunConfig    `mapstructure:",squash"`

	// Do not check region and zone when validate.
	SkipRegionValidation bool `mapstructure:"skip_region_validation" required:"false"`

	ctx interpolate.Context
}

type Builder struct {
	config Config
	runner multistep.Runner
}

func (b *Builder) ConfigSpec() hcldec.ObjectSpec { return b.config.FlatMapstructure().HCL2Spec() }

func (b *Builder) Prepare(raws ...interface{}) ([]string, []string, error) {
	err := config.Decode(&b.config, &config.DecodeOpts{
		PluginType:         BuilderId,
		Interpolate:        true,
		InterpolateContext: &b.config.ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{
				"run_command",
			},
		},
	}, raws...)
	b.config.ctx.EnableEnv = true
	if err != nil {
		return nil, nil, err
	}

	// Propagate SkipRegionValidation to Access/Image configs
	b.config.TencentCloudAccessConfig.skipValidation = b.config.SkipRegionValidation
	b.config.TencentCloudImageConfig.skipValidation = b.config.SkipRegionValidation

	// Accumulate any errors
	var errs *packersdk.MultiError
	errs = packersdk.MultiErrorAppend(errs, b.config.TencentCloudAccessConfig.Prepare(&b.config.ctx)...)
	errs = packersdk.MultiErrorAppend(errs, b.config.TencentCloudImageConfig.Prepare(&b.config.ctx)...)
	errs = packersdk.MultiErrorAppend(errs, b.config.TencentCloudRunConfig.Prepare(&b.config.ctx)...)
	if errs != nil && len(errs.Errors) > 0 {
		return nil, nil, errs
	}

	packersdk.LogSecretFilter.Set(b.config.SecretId, b.config.SecretKey)

	return nil, nil, nil
}

func (b *Builder) Run(ctx context.Context, ui packersdk.Ui, hook packersdk.Hook) (packersdk.Artifact, error) {
	cvmClient, vpcClient, err := b.config.Client()
	if err != nil {
		return nil, err
	}

	state := new(multistep.BasicStateBag)
	state.Put("config", &b.config)
	state.Put("cvm_client", cvmClient)
	state.Put("vpc_client", vpcClient)
	state.Put("hook", hook)
	state.Put("ui", ui)

	// Build the steps
	var steps []multistep.Step
	steps = []multistep.Step{
		&stepPreValidate{
			b.config.SkipCreateImage,
		},
		&stepCheckSourceImage{
			b.config.SourceImageId,
		},
		&stepConfigKeyPair{
			Debug:        b.config.PackerDebug,
			Comm:         &b.config.Comm,
			DebugKeyPath: fmt.Sprintf("cvm_%s.pem", b.config.PackerBuildName),
		},
		&stepConfigVPC{
			VpcId:     b.config.VpcId,
			CidrBlock: b.config.CidrBlock,
			VpcName:   b.config.VpcName,
		},
		&stepConfigSubnet{
			SubnetId:        b.config.SubnetId,
			SubnetCidrBlock: b.config.SubnectCidrBlock,
			SubnetName:      b.config.SubnetName,
			Zone:            b.config.Zone,
			CdcId:           b.config.CdcId,
		},
		&stepConfigSecurityGroup{
			SecurityGroupId:   b.config.SecurityGroupId,
			SecurityGroupName: b.config.SecurityGroupName,
			Description:       "securitygroup for packer",
		},
		&stepRunInstance{
			InstanceType:             b.config.InstanceType,
			InstanceChargeType:       b.config.InstanceChargeType,
			UserData:                 b.config.UserData,
			UserDataFile:             b.config.UserDataFile,
			ZoneId:                   b.config.Zone,
			InstanceName:             b.config.InstanceName,
			DiskType:                 b.config.DiskType,
			DiskSize:                 b.config.DiskSize,
			DataDisks:                b.config.DataDisks,
			HostName:                 b.config.HostName,
			InternetChargeType:       b.config.InternetChargeType,
			InternetMaxBandwidthOut:  b.config.InternetMaxBandwidthOut,
			BandwidthPackageId:       b.config.BandwidthPackageId,
			AssociatePublicIpAddress: b.config.AssociatePublicIpAddress,
			CamRoleName:              b.config.CamRoleName,
			Tags:                     b.config.RunTags,
			CdcId:                    b.config.CdcId,
		},
		&communicator.StepConnect{
			Config:    &b.config.TencentCloudRunConfig.Comm,
			SSHConfig: b.config.TencentCloudRunConfig.Comm.SSHConfigFunc(),
			Host:      SSHHost(b.config.AssociatePublicIpAddress),
		},
		&commonsteps.StepProvision{},
		&commonsteps.StepCleanupTempKeys{
			Comm: &b.config.TencentCloudRunConfig.Comm,
		},
		// We need this step to detach keypair from instance, otherwise
		// it always fails to delete the key.
		&stepDetachTempKeyPair{},
		&stepCreateImage{
			SkipCreateImage: b.config.SkipCreateImage,
		},
		&stepShareImage{
			b.config.ImageShareAccounts,
		},
		&stepCopyImage{
			DestinationRegions: b.config.ImageCopyRegions,
			SourceRegion:       b.config.Region,
			SkipCreateImage:    b.config.SkipCreateImage,
		},
	}

	b.runner = commonsteps.NewRunner(steps, b.config.PackerConfig, ui)
	b.runner.Run(ctx, state)

	if rawErr, ok := state.GetOk("error"); ok {
		return nil, rawErr.(error)
	}

	if _, ok := state.GetOk("image"); !ok {
		return nil, nil
	}

	artifact := &Artifact{
		TencentCloudImages: state.Get("tencentcloudimages").(map[string]string),
		BuilderIdValue:     BuilderId,
		Client:             cvmClient,
		StateData:          map[string]interface{}{"generated_data": state.Get("generated_data")},
	}

	return artifact, nil
}
