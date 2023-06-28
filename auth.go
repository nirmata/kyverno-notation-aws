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
	kauth "github.com/google/go-containerregistry/pkg/authn/kubernetes"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"oras.land/oras-go/v2/registry"
)

const (
	ecrPublicName = "public.ecr.aws"
)

var ecrPattern = regexp.MustCompile(`(^[a-zA-Z0-9][a-zA-Z0-9-_]*)\.dkr\.ecr(-fips)?\.([a-zA-Z0-9][a-zA-Z0-9-_]*)\.amazonaws\.com(\.cn)?$`)

func (v *verifier) getAuthConfig(ctx context.Context, ref registry.Reference) (authn.AuthConfig, error) {
	if v.imagePullSecrets != "" {
		return v.getAuthFromSecret(ctx, ref)
	}

	ecrRegion, err := getRegion(ref.Registry)
	if err == nil {
		v.logger.Infof("using region: %s", ecrRegion)
	} else {
		ecrRegion = os.Getenv("AWS_REGION")
		v.logger.Infof("using default region '%s': %v", ecrRegion, err)
	}

	return v.getAuthFromIRSA(ctx, ecrRegion)
}

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

func (v *verifier) getAuthFromIRSA(ctx context.Context, awsEcrRegion string) (authn.AuthConfig, error) {
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

func (v *verifier) getAuthFromSecret(ctx context.Context, ref registry.Reference) (authn.AuthConfig, error) {
	if v.imagePullSecrets == "" {
		return authn.AuthConfig{}, errors.Errorf("secret not configured")
	}

	v.logger.Infof("fetching credentials from secret %s...", v.imagePullSecrets)
	var secrets []corev1.Secret
	for _, imagePullSecret := range strings.Split(v.imagePullSecrets, ",") {
		secret, err := v.secretLister.Get(imagePullSecret)
		if err != nil {
			return authn.AuthConfig{}, err
		}

		secrets = append(secrets, *secret)
	}

	keychain, err := kauth.NewFromPullSecrets(ctx, secrets)
	if err != nil {
		return authn.AuthConfig{}, err
	}

	authenticator, err := keychain.Resolve(&imageResource{ref})
	if err != nil {
		return authn.AuthConfig{}, err
	}

	authConfig, err := authenticator.Authorization()
	if err != nil {
		return authn.AuthConfig{}, errors.Wrapf(err, "failed to get auth config for %s", ref.String())
	}

	return *authConfig, nil
}

type imageResource struct {
	ref registry.Reference
}

func (ir *imageResource) String() string {
	return ir.ref.String()
}

func (ir *imageResource) RegistryStr() string {
	return ir.ref.Registry
}
