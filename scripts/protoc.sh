#!/usr/bin/env bash

PROMETHEUS_PROTO_DIR="$(go list -f '{{ .Dir }}' -m github.com/prometheus/client_model)"

protoc --proto_path=stream --proto_path="$PROMETHEUS_PROTO_DIR" stream/stream.proto --go_out=plugins=grpc,paths=source_relative:./stream
protoc --proto_path=test test/test.proto --go_out=plugins=grpc,paths=source_relative:./test
