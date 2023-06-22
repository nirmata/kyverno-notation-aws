package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"
	"oras.land/oras-go/v2/registry"
)

func getRegion(ref registry.Reference) (string, error) {
	toks := strings.Split(ref.Registry, ".")
	if len(toks) >= 6 {
		return toks[3], nil
	}

	return "", fmt.Errorf("failed to extract region from %s", ref.Registry)
}

func getAuthFromIRSA(ctx context.Context, ref registry.Reference) (authn.AuthConfig, error) {
	awsEcrRegion, err := getRegion(ref)
	if err != nil {
		awsEcrRegion = os.Getenv("AWS_REGION")
	}

	var authConfig authn.AuthConfig
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(awsEcrRegion))
	if err != nil {
		return authConfig, errors.Wrapf(err, "failed to load default configuration")
	}

	ecrService := ecr.NewFromConfig(cfg)
	ecrToken, err := ecrService.GetAuthorizationToken(ctx, nil)
	if err != nil {
		return authConfig, err
	}

	if len(ecrToken.AuthorizationData) == 0 {
		return authConfig, errors.New("no authorization data")
	}

	if ecrToken.AuthorizationData[0].AuthorizationToken == nil {
		return authConfig, fmt.Errorf("no authorization token")
	}

	token, err := base64.StdEncoding.DecodeString(*ecrToken.AuthorizationData[0].AuthorizationToken)
	if err != nil {
		return authConfig, err
	}

	tokenSplit := strings.Split(string(token), ":")
	if len(tokenSplit) != 2 {
		return authConfig, fmt.Errorf("invalid authorization token, expected the token to have two parts separated by ':', got %d parts", len(tokenSplit))
	}

	authConfig = authn.AuthConfig{
		Username: tokenSplit[0],
		Password: tokenSplit[1],
	}

	return authConfig, nil
}
