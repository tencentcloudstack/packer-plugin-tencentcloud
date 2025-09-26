// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cvm

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
)

type stepCheckSourceImageFamily struct {
	sourceImageId     string
	sourceImageName   string
	sourceImageFamily string
}

func (s *stepCheckSourceImageFamily) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	if s.sourceImageId != "" || s.sourceImageName != "" {
		return multistep.ActionContinue
	}
	config := state.Get("config").(*Config)
	client := state.Get("cvm_client").(*cvm.Client)

	Say(state, config.SourceImageFamily, "Try to check the source image and get the latest valid image of the image family")

	req := cvm.NewDescribeImageFromFamilyRequest()
	req.ImageFamily = &config.SourceImageFamily

	var resp *cvm.DescribeImageFromFamilyResponse
	err := Retry(ctx, func(ctx context.Context) error {
		var err error
		resp, err = client.DescribeImageFromFamily(req)
		return err
	})
	if err != nil {
		return Halt(state, err, "Failed to get source image info from the image family")
	}

	if resp != nil && resp.Response != nil && resp.Response.Image != nil {
		image := resp.Response.Image
		if image.ImageId != nil && !*image.ImageDeprecated {
			state.Put("source_image", image)
			Message(state, fmt.Sprintf("Get the latest image from the image family, id: %v", *image.ImageId), "Image found")
			return multistep.ActionContinue
		}
	} else {
		return Halt(state, fmt.Errorf("failed to get source image: %v", resp.ToJsonString()), "No image family found")
	}

	return Halt(state, fmt.Errorf("No image found under current instance_type(%s) restriction", config.InstanceType), "")
}

func (s *stepCheckSourceImageFamily) Cleanup(bag multistep.StateBag) {}
