#!/bin/bash
#Deprecated. Use setup-flux.sh

# run from ..

kind create cluster --name dev --config setup/dev-cluster.yaml 
kubectl apply -f ../deploy-ingress-nginx.yaml
kubectl create namespace appchat
helm install appchat appchat --values appchat/values.yaml --values appchat/values-dev.yaml -n appchat
kubectl create namespace appvote
helm install appvote appvote --values appvote/values.yaml -n appvote
kubectl create namespace apptique
helm install apptique apptique/helm-chart --values apptique/helm-chart/values.yaml --values apptique/helm-chart/values-dev.yaml -n apptique
