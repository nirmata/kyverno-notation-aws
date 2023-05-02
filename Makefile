build:
	go build -o kyverno-notation-aws

docker:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o kyverno-notation-aws .
	docker buildx build --platform linux/arm64/v8 -t ghcr.io/nirmata/kyverno-notation-aws:v1-alpha1 .
	docker push ghcr.io/nirmata/kyverno-notation-aws:v1-alpha1