#!/bin/bash

go build -o m3u-stream-merger-proxy

./m3u-stream-merger-proxy &

sleep 10

kill %1

echo "Build and deployment process completed."
