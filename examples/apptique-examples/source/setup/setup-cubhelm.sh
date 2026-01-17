#!/bin/bash

# git clone the following repos
#
# Helm charts
# https://github.com/confighub-kubecon-2025/appchat
# https://github.com/confighub-kubecon-2025/appvote
# https://github.com/confighub-kubecon-2025/apptique

# run from ..

# From Helm charts

cub space create --allow-exists appchat-helm-dev
cub space create --allow-exists appchat-helm-prod
cub space create --allow-exists appvote-helm-dev
cub space create --allow-exists appvote-helm--prod
cub space create --allow-exists apptique-helm-dev
cub space create --allow-exists apptique-helm-prod

cub helm install --space appchat-helm-dev appchat appchat --values appchat/values.yaml --values appchat/values-dev.yaml 
cub helm install --space appchat-helm-prod appchat appchat --values appchat/values.yaml --values appchat/values-prod.yaml 
cub helm install --space appvote-helm-dev appvote appvote --values appvote/values.yaml --values appvote/values-dev.yaml
cub helm install --space appvote-helm-prod appvote appvote --values appvote/values.yaml --values appvote/values-prod.yaml
cub helm install --space apptique-helm-dev apptique apptique/helm-chart --values apptique/helm-chart/values.yaml --values apptique/helm-chart/values-dev.yaml
cub helm install --space apptique-helm-prod apptique apptique/helm-chart --values apptique/helm-chart/values.yaml --values apptique/helm-chart/values-prod.yaml
