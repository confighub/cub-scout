#!/bin/bash

# Add the following to /etc/hosts
# 127.0.0.1	dev.appchat.cubby.bz
# 127.0.0.1	www.appchat.cubby.bz
# 127.0.0.1	dev-vote.appvote.cubby.bz
# 127.0.0.1	dev-results.appvote.cubby.bz
# 127.0.0.1	www.appvote.cubby.bz
# 127.0.0.1	results.appvote.cubby.bz
# 127.0.0.1	dev.apptique.cubby.bz
# 127.0.0.1	www.apptique.cubby.bz

# run from ..

kind create cluster --name dev --config setup/dev-cluster.yaml 
flux install
#https://kind.sigs.k8s.io/docs/user/ingress
kubectl apply -f setup/deploy-ingress-nginx.yaml
#kubectl apply -f https://kind.sigs.k8s.io/examples/ingress/deploy-ingress-nginx.yaml
kubectl create namespace appchat
kubectl create namespace appvote
kubectl create namespace apptique

# This sleep appears to be necessary so that Ingress resources aren't blocked
sleep 30

kubectl apply -f appchat/flux/gitrepository.yaml
kubectl apply -f appvote/flux/gitrepository.yaml
kubectl apply -f apptique/flux/gitrepository.yaml

# This sleep appears to be necessary so that helm installation doesn't fail
sleep 30

kubectl apply -f appchat/flux/helmrelease-dev.yaml
kubectl apply -f appvote/flux/helmrelease-dev.yaml
kubectl apply -f apptique/flux/helmrelease-dev.yaml

if ! [[ -z "$SETUP_PROD" ]] ; then

kind create cluster --name prod --config setup/prod-cluster.yaml 
flux install
kubectl apply -f setup/deploy-ingress-nginx.yaml
#kubectl apply -f https://kind.sigs.k8s.io/examples/ingress/deploy-ingress-nginx.yaml
kubectl create namespace appchat
kubectl create namespace appvote
kubectl create namespace apptique

sleep 30

kubectl apply -f appchat/flux/gitrepository.yaml
kubectl apply -f appvote/flux/gitrepository.yaml
kubectl apply -f apptique/flux/gitrepository.yaml

sleep 30

kubectl apply -f appchat/flux/helmrelease-prod.yaml
kubectl apply -f appvote/flux/helmrelease-prod.yaml
kubectl apply -f apptique/flux/helmrelease-prod.yaml

fi
