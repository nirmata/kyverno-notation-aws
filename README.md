# kyverno-notary-aws
Kyverno extension service for Notation and the AWS signer


# Setup AWS Signer

1. Create a signing profile:

```sh
aws signer put-signing-profile --profile-name notation-test --platform-id Notary-v2-OCI-SHA384-ECDSA --signature-validity-period 'value=12, type=MONTHS'
```

2. Get the signing profile ARN

```sh
 aws signer get-signing-profile --profile-name notationtest
{
    "profileName": "notationtest",
    "profileVersion": "FpVhlaR6yz",
    "profileVersionArn": "arn:aws:signer:${aws_region}:${aws_account_id}:/signing-profiles/notationtest/FpVhlaR6yz",
    "platformId": "Notary-v2-OCI-SHA384-ECDSA",
    "platformDisplayName": "Notary v2 for Container Registries",
    "signatureValidityPeriod": {
        "value": 12,
        "type": "MONTHS"
    },
    "status": "Active",
    "arn": "arn:aws:signer:${aws_region}:${aws_account_id}:/signing-profiles/notationtest",
    "tags": {}
}
```

3. Sign the image using `notation` and the AWS signer:

```sh
notation sign 844333597536.dkr.ecr.us-east-1.amazonaws.com/net-monitor:v1 --key notationtest --signature-manifest image
```

# Install

1. Install cert-manager

```sh
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.11.0/cert-manager.yaml
```

2. Install Kyverno

```sh
kubectl create -f https://raw.githubusercontent.com/kyverno/kyverno/main/config/install-latest-testing.yaml
```

3. Install kyverno-notation-aws

```sh
kubectl apply -f configs/install.yaml
```

4. Create CRs for Notation TrustPolicy and TrustStore

```sh
kubectl apply -f configs/crds/
```

```sh
kubectl apply -f configs/samples/truststore.yaml
```

Update the the `${aws_region}` and `${aws_account_id}` in the [trustpolicy.yaml](configs/samples/trustpolicy.yaml) and then install in your cluster:

```yaml
    trustedIdentities:
    - "arn:aws:signer:${aws_region}:${aws_account_id}:/signing-profiles/notationtest"
```

```sh
kubectl apply -f configs/samples/trustpolicy.yaml
```

5. Get the TLS cert chain from your `kube-notation-aws` service

```sh
kubectl -n kyverno-notation-aws get secret kyverno-notation-aws-tls -o json | jq -r '.data."tls.crt"' | base64 -d && kubectl -n kyverno-notation-aws get secret kyverno-notation-aws-tls -o json | jq -r '.data."ca.crt"' | base64 -d
```

6. Update the [Kyverno policy](configs/samples/kyverno-policy.yaml) with the TLS cert chain and then apply in your cluster:

```sh
kubectl apply -f configs/samples/kyverno-policy.yaml
```

7. Configure ECR Registry credentials

If you are using IRSA (recommended):

a. Setup a custom policy `notation-signer-policy` with the following permissions:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "VisualEditor0",
            "Effect": "Allow",
            "Action": [
                "signer:GetRevocationStatus"
            ],
            "Resource": "*"
        }
    ]
}
```

b. Setup a IRSA role `kyverno-notation-aws` and attach two policies to it:
* the `notation-signer-policy` 
* the `AmazonEC2ContainerRegistryReadOnly` policy

c. Configure [IRSA](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html) for your cluster

d. Configure the `kyverno-notation-aws` ServiceAccount in the [install.yaml](configs/install.yaml) with the correct role:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kyverno-notation-aws
  namespace: kyverno-notation-aws
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::${aws_account_id}:role/kyverno-notation-aws
```

If you are NOT using IRSA you can configure registry credentials and IAM tokens as follows:

```sh
aws sso login
```

```sh
set AWS_TOKEN (aws ecr get-login-password --region us-east-1)
```

```sh
kubectl create secret docker-registry regcred --docker-username=AWS --docker-password=$AWS_TOKEN --docker-server=844333597536.dkr.ecr.us-east-1.amazonaws.com -n kyverno-notation-aws 
```

Update the `kyverno-notation-aws` Deployment in the [install.yaml](configs/install.yaml) to add the `--imagePullSecrets=regcred` argument:

8. Test signed and unsigned images:

Create the test namespace which the [policy](configs/samples/kyverno-policy.yaml) applies to:

```sh
kubectl create ns test-notation
```

Run a signed image:

```sh
kubectl -n test-notation run test --image=844333597536.dkr.ecr.us-east-1.amazonaws.com/net-monitor:v1 --dry-run=server
pod/test created (server dry run)
```

Attempt to run an unsigned image:

```sh
kubectl -n test-notation run test --image=844333597536.dkr.ecr.us-east-1.amazonaws.com/net-monitor:v1-unsigned
Error from server: admission webhook "validate.kyverno.svc-fail" denied the request:

policy Pod/test-notation/test for resource error:

check-images:
  call-aws-signer-extension: |
    failed to load context: failed to execute APICall: HTTP 500 Internal Server Error: failed to verify image 844333597536.dkr.ecr.us-east-1.amazonaws.com/net-monitor:v1-unsigned: no signature is associated with "844333597536.dkr.ecr.us-east-1.amazonaws.com/net-monitor@sha256:f04288efc7e65a84be74d4fc63e235ac3c6c603cf832e442e0bd3f240b10a91b", make sure the image was signed successfully
```

# Troubleshooting

1. Check service

```sh
kubectl -n kyverno-notation-aws logs deploy/kyverno-notation-aws -f
```

The logs should show:

```sh
Defaulted container "kyverno-notation-aws" out of: kyverno-notation-aws, kube-notation
2023-04-30T18:22:28.438Z	INFO	kyverno-notation-aws/main.go:46	configuring notation	{"dir.UserConfigDir": "/notation", "dir.UserLibexecDir": "/notation"}
2023-04-30T18:22:28.459Z	INFO	kyverno-notation-aws/verify.go:65	initialized	{"namespace": "kyverno-notation-aws", "secrets": "regcred"}
2023-04-30T18:22:28.460Z	INFO	kyverno-notation-aws/main.go:68	Listening...
```


2. Run netshoot

```sh
kubectl run netshoot --rm -i --tty --image nicolaka/netshoot
```

3. Use curl to make a call to the service as follows:

```sh
curl -k https://svc.kyverno-notation-aws/checkimages -X POST -d '{"images": ["844333597536.dkr.ecr.us-east-1.amazonaws.com/net-monitor:v1"]}'
```

The output should look like this:

```sh
{
    "digests": {
      "844333597536.dkr.ecr.us-east-1.amazonaws.com/net-monitor:v1": "sha256:4ee9dc6abbf5e8181101fc1f8cd6d91ec0c5657f8c71274a8209637630eec48d"
    },
    "verified": true
}
````
