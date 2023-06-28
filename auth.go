package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"
	"oras.land/oras-go/v2/registry"
)

const (
	ecrPublicName = "public.ecr.aws"
)

var ecrPattern = regexp.MustCompile(`(^[a-zA-Z0-9][a-zA-Z0-9-_]*)\.dkr\.ecr(-fips)?\.([a-zA-Z0-9][a-zA-Z0-9-_]*)\.amazonaws\.com(\.cn)?$`)

func getRegion(registry string) (string, error) {
	if registry == ecrPublicName {
		return "", nil
	}
	matches := ecrPattern.FindStringSubmatch(registry)
	if len(matches) == 0 {
		return "", fmt.Errorf("kyverno-notation-aws plugin can only be used with Amazon Elastic Container Registry")
	} else if len(matches) < 3 {
		return "", fmt.Errorf("%q is not a valid repository URI for Amazon Elastic Container Registry", registry)
	}

	ecrRegion := matches[3]
	return ecrRegion, nil
}

func getAuthFromIRSA(ctx context.Context, ref registry.Reference) (authn.AuthConfig, error) {
	awsEcrRegion, err := getRegion(ref.Registry)
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
