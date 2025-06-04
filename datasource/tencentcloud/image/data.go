// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc mapstructure-to-hcl2 -type DatasourceOutput,Config,Image

package image

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/hcl2helper"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	buildCvm "github.com/hashicorp/packer-plugin-tencentcloud/builder/tencentcloud/cvm"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
	"github.com/zclconf/go-cty/cty"
)

type ImageFilterOptions struct {
	// Filters used to select an image. Any filter described in the documentation for
	// [DescribeImages](https://www.tencentcloud.com/document/product/213/33272) can be used.
	Filters map[string]string `mapstructure:"filters"`
	// Image family used to select an image. Uses the
	// [DescribeImageFromFamily](https://www.tencentcloud.com/document/product/213/64971) API.
	// Mutually exclusive with `filters`, and `most_recent` will have no effect.
	ImageFamily string `mapstructure:"image_family"`
	// Selects the most recently created image when multiple results are returned. Note that
	// public images don't have a creation date, so this flag is only really useful for private
	// images.
	MostRecent bool `mapstructure:"most_recent"`
}

type Config struct {
	common.PackerConfig               `mapstructure:",squash"`
	buildCvm.TencentCloudAccessConfig `mapstructure:",squash"`
	ImageFilterOptions                `mapstructure:",squash"`
	ctx                               interpolate.Context
}

type Datasource struct {
	config Config
}

func (d *Datasource) ConfigSpec() hcldec.ObjectSpec {
	return d.config.FlatMapstructure().HCL2Spec()
}

func (d *Datasource) Configure(raws ...interface{}) error {
	err := config.Decode(&d.config, nil, raws...)
	if err != nil {
		return err
	}

	var errs *packersdk.MultiError
	errs = packersdk.MultiErrorAppend(errs, d.config.TencentCloudAccessConfig.Prepare(&d.config.ctx)...)

	if len(d.config.Filters) == 0 && d.config.ImageFamily == "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("`filters` or `image_family` must be specified"))
	}

	if len(d.config.Filters) > 0 && d.config.ImageFamily != "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("`filters` and `image_family` are mutually exclusive"))
	}

	if errs != nil && len(errs.Errors) > 0 {
		return errs
	}
	return nil
}

type DatasourceOutput struct {
	// The image ID
	ID string `mapstructure:"id"`
	// The image name
	Name string `mapstructure:"name"`
}

func (d *Datasource) OutputSpec() hcldec.ObjectSpec {
	return (&DatasourceOutput{}).FlatMapstructure().HCL2Spec()
}

func (d *Datasource) Execute() (cty.Value, error) {
	var image *cvm.Image
	var err error

	if len(d.config.Filters) > 0 {
		image, err = d.ResolveImageByFilters()
	} else {
		image, err = d.ResolveImageByImageFamily()
	}

	if err != nil {
		return cty.NullVal(cty.EmptyObject), err
	}

	output := DatasourceOutput{
		ID:   *image.ImageId,
		Name: *image.ImageName,
	}
	return hcl2helper.HCL2ValueFromConfig(output, d.OutputSpec()), nil
}

func (d *Datasource) ResolveImageByFilters() (*cvm.Image, error) {
	client, _, err := d.config.Client()
	if err != nil {
		return nil, err
	}

	req := cvm.NewDescribeImagesRequest()

	var filters []*cvm.Filter
	for k, v := range d.config.Filters {
		k := k
		v := v
		filters = append(filters, &cvm.Filter{
			Name:   &k,
			Values: []*string{&v},
		})
	}
	req.Filters = filters

	ctx := context.TODO()
	var resp *cvm.DescribeImagesResponse
	err = buildCvm.Retry(ctx, func(ctx context.Context) error {
		var e error
		resp, e = client.DescribeImages(req)
		return e
	})
	if err != nil {
		return nil, err
	}

	if *resp.Response.TotalCount == 0 {
		return nil, fmt.Errorf("No image found using the specified filters")
	}

	if *resp.Response.TotalCount > 1 && !d.config.MostRecent {
		return nil, fmt.Errorf("Your image query returned more than result. Please try a more specific search, or set `most_recent` to `true`.")
	}

	if d.config.MostRecent {
		return mostRecentImage(resp.Response.ImageSet), nil
	} else {
		return resp.Response.ImageSet[0], nil
	}
}

func (d *Datasource) ResolveImageByImageFamily() (*cvm.Image, error) {
	client, _, err := d.config.Client()
	if err != nil {
		return nil, err
	}

	var resp *cvm.DescribeImageFromFamilyResponse
	req := cvm.NewDescribeImageFromFamilyRequest()
	req.ImageFamily = &d.config.ImageFamily

	ctx := context.TODO()
	err = buildCvm.Retry(ctx, func(ctx context.Context) error {
		var e error
		resp, e = client.DescribeImageFromFamily(req)
		return e
	})

	if err != nil {
		return nil, err
	}

	if resp.Response.Image == nil {
		return nil, fmt.Errorf("No image found using the specified image family")
	}

	return resp.Response.Image, nil
}
