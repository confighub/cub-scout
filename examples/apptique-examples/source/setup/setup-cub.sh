#!/bin/bash

# git clone the following repos
#
# Helm charts
# https://github.com/confighub-kubecon-2025/appchat
# https://github.com/confighub-kubecon-2025/appvote
# https://github.com/confighub-kubecon-2025/apptique

# run from ..

# From components

homeSpaceID="$(cub space get home --jq '.Space.SpaceID')"

##########################
# appchat
##########################

# Create dev/base units and links
cub space create --allow-exists appchat-dev --label Environment=dev --where-trigger "SpaceID = '$homeSpaceID'"

cub unit create --space appchat-dev --label Application=appchat database appchat/base/postgres.yaml
cub unit create --space appchat-dev --label Application=appchat backend appchat/base/backend.yaml
cub unit create --space appchat-dev --label Application=appchat frontend appchat/base/frontend.yaml
cub function do --space appchat-dev ensure-namespaces
setup/kube-gen.sh namespace appchat | cub unit create --space appchat-dev --label Application=appchat appchat-ns -

cub link create --space appchat-dev - frontend backend
cub link create --space appchat-dev - backend database
cub link create --space "*" --where-space "Slug = 'appchat-dev'" --where-from "Slug != 'appchat-ns'" --where-to "Slug = 'appchat-ns'"

# Clone units and links to prod
cub space create --allow-exists appchat-prod --label Environment=prod --where-trigger "SpaceID = '$homeSpaceID'"
cub unit create --space appchat-dev --where-space "Slug = 'appchat-prod'"

# TODO: create a base or set a base tag for merging

# Customize dev and prod

cub function do --space appchat-dev --unit frontend --unit backend set-hostname dev.appchat.cubby.bz
cub function do --space appchat-dev --unit backend set-env-var backend CHAT_TITLE "AI Chat Dev"

cub function do --space appchat-prod --unit frontend --unit backend set-hostname www.appchat.cubby.bz
cub function do --space appchat-prod --unit backend set-env-var backend REGION NA
cub function do --space appchat-prod --unit backend set-env-var backend ROLE prod

##########################
# appvote
##########################

# Create dev/base units and links
cub space create --allow-exists appvote-dev --label Environment=dev --where-trigger "SpaceID = '$homeSpaceID'"
for unit in db redis vote result worker ; do
cub unit create --space appvote-dev --label Application=appvote $unit appvote/base/${unit}.yaml
done
cub function do --space appvote-dev ensure-namespaces
setup/kube-gen.sh namespace appvote | cub unit create --space appvote-dev --label Application=appvote appvote-ns -

cub link create --space appvote-dev - vote redis
cub link create --space appvote-dev - worker redis
cub link create --space appvote-dev - result db
cub link create --space appvote-dev - worker db
cub link create --space "*" --where-space "Slug = 'appvote-dev'" --where-from "Slug != 'appvote-ns'" --where-to "Slug = 'appvote-ns'"

# Clone units and links to prod
cub space create --allow-exists appvote-prod --label Environment=prod --where-trigger "SpaceID = '$homeSpaceID'"
cub unit create --space appvote-dev --where-space "Slug = 'appvote-prod'"

# Customize dev and prod

cub function do --space appvote-dev --unit vote set-hostname dev-vote.appvote.cubby.bz
cub function do --space appvote-dev --unit result set-hostname dev-results.appvote.cubby.bz

cub function do --space appvote-prod --unit vote set-hostname www.appvote.cubby.bz
cub function do --space appvote-prod --unit result set-hostname results.appvote.cubby.bz

##########################
# apptique
##########################

# Create dev/base units and links
cub space create --allow-exists apptique-dev --label Environment=dev --where-trigger "SpaceID = '$homeSpaceID'"
for file in apptique/kubernetes-manifests/*.yaml ; do
unit="$(basename -s .yaml $file)"
if [[ "$unit" != kustomization ]] && [[ "$unit" != loadgenerator ]] ; then
cub unit create --space apptique-dev --label Application=apptique $unit $file
# Set to pre-built image
cub function do --space apptique-dev --unit $unit set-image server "us-central1-docker.pkg.dev/google-samples/microservices-demo/${unit}:v0.10.3"
fi
done
cub function do --space apptique-dev ensure-namespaces
setup/kube-gen.sh namespace apptique | cub unit create --space apptique-dev --label Application=apptique apptique-ns -

#cub link create --space apptique-dev - loadgenerator frontend
cub link create --space apptique-dev - frontend adservice
cub link create --space apptique-dev - frontend recommendationservice
cub link create --space apptique-dev - frontend productcatalogservice
cub link create --space apptique-dev - frontend cartservice
cub link create --space apptique-dev - frontend shippingservice
cub link create --space apptique-dev - frontend currencyservice
cub link create --space apptique-dev - recommendationservice productcatalogservice
cub link create --space apptique-dev - frontend checkoutservice
cub link create --space apptique-dev - checkoutservice productcatalogservice
cub link create --space apptique-dev - checkoutservice cartservice
cub link create --space apptique-dev - checkoutservice shippingservice
cub link create --space apptique-dev - checkoutservice currencyservice
cub link create --space apptique-dev - checkoutservice paymentservice
cub link create --space apptique-dev - checkoutservice emailservice
cub link create --space "*" --where-space "Slug = 'apptique-dev'" --where-from "Slug != 'apptique-ns'" --where-to "Slug = 'apptique-ns'"

# Clone units and links to prod
cub space create --allow-exists apptique-prod --label Environment=prod --where-trigger "SpaceID = '$homeSpaceID'"
cub unit create --space apptique-dev --where-space "Slug = 'apptique-prod'"

# Customize dev and prod

cub function do --space apptique-dev --unit frontend set-hostname dev.apptique.cubby.bz

cub function do --space apptique-prod --unit frontend set-hostname www.apptique.cubby.bz

##########################
# Attach targets
##########################

cub unit set-target --space "*" --where "Space.Labels.Environment = 'dev'" platform-dev/dev-cluster
cub unit set-target --space "*" --where "Space.Labels.Environment = 'prod'" platform-prod/prod-cluster

##########################
# Apply all the units
##########################

cub unit approve --space "*" --where "Labels.Application LIKE 'app%'"

#cub unit apply --wait --space "*" --where "Labels.Application LIKE 'app%'"
cub unit apply --wait --space appchat-dev
cub unit apply --wait --space appvote-dev
cub unit apply --wait --space apptique-dev
cub unit apply --wait --space appchat-prod
cub unit apply --wait --space appvote-prod
cub unit apply --wait --space apptique-prod
cub tag create --space home post-initial-apply
cub unit tag --space "*" --where "Labels.Application LIKE 'app%'" --revision HeadRevisionNum home/post-initial-apply
cub unit refresh --space "*" --where "Labels.Application LIKE 'app%'"
cub tag create --space home post-refresh
cub unit tag --space "*" --where "Labels.Application LIKE 'app%'" --revision HeadRevisionNum home/post-refresh
