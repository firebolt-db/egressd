#!/usr/bin/env bash

VERSION=$1

if [ -z "$VERSION" ]; then
  echo "Please provide a version"
fi

GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/egressd-x86_64 cmd/collector/main.go

docker buildx build --build-arg TARGETARCH=x86_64 -f Dockerfile -t "231290928314.dkr.ecr.us-east-1.amazonaws.com/castai/egressd:0.12.8-custom-fork-v$VERSION" .

docker push "231290928314.dkr.ecr.us-east-1.amazonaws.com/castai/egressd:0.12.8-custom-fork-v$VERSION"

IMAGE_WITH_SHA=$(docker inspect --format='{{index .RepoDigests 0}}' "231290928314.dkr.ecr.us-east-1.amazonaws.com/castai/egressd:0.12.8-custom-fork-v$VERSION")

export AWS_DEFAULT_REGION=us-east-1
export AWS_PROFILE=publishing

cosign sign -y --key awskms:///alias/cosign-key "$IMAGE_WITH_SHA"
