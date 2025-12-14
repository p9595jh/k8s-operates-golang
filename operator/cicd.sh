#!/bin/bash

set -xe

make undeploy

make docker-build

docker save operator:latest | sudo k3s ctr images import -

make deploy

kubectl apply -f config/samples/app_v1_operatable.yaml -n app
