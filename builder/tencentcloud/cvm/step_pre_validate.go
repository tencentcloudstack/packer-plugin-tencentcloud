// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cvm

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
)

type stepPreValidate struct {
	SkipCreateImage bool
}

func (s *stepPreValidate) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	// No need to validate the image name if we're not creating an image
	if s.SkipCreateImage {
		return multistep.ActionContinue
	}

	config := state.Get("config").(*Config)
	client := state.Get("cvm_client").(*cvm.Client)

	Say(state, config.ImageName, "Trying to check image name")

	image, err := GetImageByName(ctx, client, config.ImageName)
	if err != nil {
		return Halt(state, err, "Failed to get images info")
	}

	if image != nil {
		return Halt(state, fmt.Errorf("Image name %s has exists", config.ImageName), "")
	}

	Message(state, "useable", "Image name")

	return multistep.ActionContinue
}

func (s *stepPreValidate) Cleanup(multistep.StateBag) {}
