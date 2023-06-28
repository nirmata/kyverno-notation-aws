package main

import (
	"testing"

	"gotest.tools/assert"
)

func TestECRRegionExtract(t *testing.T) {
	tests := []struct {
		registry   string
		wantRegion string
		wantOK     bool
		wantErr    string
	}{
		{
			registry:   "012345678901.dkr.ecr.us-east-1.amazonaws.com",
			wantRegion: "us-east-1",
			wantOK:     true,
		},
		{
			registry:   "210987654321.dkr.ecr.cn-north-1.amazonaws.com.cn",
			wantRegion: "cn-north-1",
			wantOK:     true,
		},
		{
			registry:   "123456789012.dkr.ecr-fips.us-gov-west-1.amazonaws.com",
			wantRegion: "us-gov-west-1",
			wantOK:     true,
		},
		{
			registry:   "public.ecr.aws",
			wantRegion: "",
			wantOK:     true,
		},
		{
			registry: "gcr.io",
			wantOK:   false,
			wantErr:  "kyverno-notation-aws plugin can only be used with Amazon Elastic Container Registry",
		},
		{
			registry: "not.ecr.io",
			wantOK:   false,
			wantErr:  "kyverno-notation-aws plugin can only be used with Amazon Elastic Container Registry",
		},
		{
			registry: "public.ecr.aws.fake.example.com",
			wantOK:   false,
			wantErr:  "kyverno-notation-aws plugin can only be used with Amazon Elastic Container Registry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.registry, func(t *testing.T) {
			region, err := getRegion(tt.registry)
			if tt.wantOK {
				assert.NilError(t, err, "unexpected error")
			} else {
				assert.Error(t, err, tt.wantErr, "unexpected OK")
			}

			assert.Equal(t, region, tt.wantRegion, "unexpected region")
		})
	}
}
